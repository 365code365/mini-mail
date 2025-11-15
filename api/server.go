package api

import (
	"encoding/json"
	"fmt"
	"log"
	"mail-server/services"
	"mail-server/storage"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
)

// Server HTTP API服务器
type Server struct {
	storage     storage.Storage
	dnsService  *services.MailDNSService
	emailSender *services.EmailSender
	router      *mux.Router
	port        int
}

// getUserIDFromRequest 从请求中获取用户ID
func getUserIDFromRequest(r *http.Request) int64 {
	// 这里从认证中间件传递过来的header获取用户ID
	userIDStr := r.Header.Get("X-User-ID")
	userID, _ := strconv.ParseInt(userIDStr, 10, 64)
	return userID
}

// NewServer 创建新的API服务器
func NewServer(storage storage.Storage, dnsService *services.MailDNSService, emailSender *services.EmailSender, port int) *Server {
	s := &Server{
		storage:     storage,
		dnsService:  dnsService,
		emailSender: emailSender,
		router:      mux.NewRouter(),
		port:        port,
	}
	s.setupRoutes()
	return s
}

// setupRoutes 设置路由
func (s *Server) setupRoutes() {
	// 启用CORS
	s.router.Use(corsMiddleware)

	// 认证相关路由（不需要认证）
	s.router.HandleFunc("/api/auth/register", s.register).Methods("POST", "OPTIONS")
	s.router.HandleFunc("/api/auth/login", s.passwordLogin).Methods("POST", "OPTIONS")
	s.router.HandleFunc("/api/auth/send-code", s.sendCode).Methods("POST", "OPTIONS")
	s.router.HandleFunc("/api/auth/verify-code", s.verifyCode).Methods("POST", "OPTIONS")
	s.router.HandleFunc("/api/auth/password-login", s.passwordLogin).Methods("POST", "OPTIONS")
	s.router.HandleFunc("/api/auth/set-password", s.setPassword).Methods("POST", "OPTIONS")

	// API路由 - 需要认证
	s.router.HandleFunc("/api/mails", s.authMiddleware(s.getMails)).Methods("GET", "OPTIONS")
	s.router.HandleFunc("/api/mails/{id}", s.authMiddleware(s.getMailByID)).Methods("GET", "OPTIONS")
	s.router.HandleFunc("/api/stats", s.authMiddleware(s.getStats)).Methods("GET", "OPTIONS")

	// DNS管理API - 需要认证
	s.router.HandleFunc("/api/domains", s.authMiddleware(s.getDomains)).Methods("GET", "OPTIONS")
	s.router.HandleFunc("/api/domains", s.authMiddleware(s.createDomain)).Methods("POST", "OPTIONS")
	s.router.HandleFunc("/api/domains/{id}", s.authMiddleware(s.deleteDomain)).Methods("DELETE", "OPTIONS")

	// 邮件发送API - 需要认证
	s.router.HandleFunc("/api/send-email", s.authMiddleware(s.sendEmail)).Methods("POST", "OPTIONS")

	// 静态文件 - 不需要认证
	s.router.PathPrefix("/").Handler(http.FileServer(http.Dir("./web")))
}

// Start 启动HTTP服务器
func (s *Server) Start() error {
	addr := ":" + strconv.Itoa(s.port)
	log.Printf("HTTP API server listening on %s", addr)
	return http.ListenAndServe(addr, s.router)
}

// getMails 获取邮件列表
func (s *Server) getMails(w http.ResponseWriter, r *http.Request) {
	// 获取当前用户ID
	userID := getUserIDFromRequest(r)

	// 解析查询参数
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// 获取用户的邮件
	mails, err := s.storage.GetMails(userID, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 获取用户邮件总数
	total, err := s.storage.GetMailCount(userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 构建响应
	response := map[string]interface{}{
		"total":  total,
		"limit":  limit,
		"offset": offset,
		"mails":  mails,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// getMailByID 根据ID获取邮件
func (s *Server) getMailByID(w http.ResponseWriter, r *http.Request) {
	// 获取当前用户ID
	userID := getUserIDFromRequest(r)

	vars := mux.Vars(r)
	id, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	mail, err := s.storage.GetMailByID(userID, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(mail)
}

// getStats 获取统计信息
func (s *Server) getStats(w http.ResponseWriter, r *http.Request) {
	// 获取当前用户ID
	userID := getUserIDFromRequest(r)

	total, err := s.storage.GetMailCount(userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"total_mails": total,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// corsMiddleware CORS中间件
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// parseJSON 解析JSON请求体
func parseJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// respondJSON 返回JSON响应
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// getDomains 获取所有邮箱域名
func (s *Server) getDomains(w http.ResponseWriter, r *http.Request) {
	// 获取当前用户ID
	userID := getUserIDFromRequest(r)

	domains, err := s.storage.GetMailDomains(userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"domains": domains,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// createDomain 创建邮箱域名
func (s *Server) createDomain(w http.ResponseWriter, r *http.Request) {
	// 获取当前用户ID
	userID := getUserIDFromRequest(r)
	// 获取用户邮箱
	userEmail := r.Header.Get("X-User-Email")

	var req struct {
		Email string `json:"email"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.Email == "" {
		http.Error(w, "Email is required", http.StatusBadRequest)
		return
	}

	// 检查用户是否达到创建限制（非管理员）
	if userEmail != "admin@admin.com" {
		user, err := s.storage.GetUserByEmail(userEmail)
		if err != nil {
			http.Error(w, "获取用户信息失败", http.StatusInternalServerError)
			return
		}

		if user != nil && user.DomainCount >= 20 {
			response := map[string]string{"error": "您已达到最大邮箱创建数量限制（20个）"}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	// 如果DNS服务不可用，创建一个简化的域名记录
	if s.dnsService == nil {
		log.Printf("DNS service not available, creating simplified domain record for: %s", req.Email)

		// 生成虚拟域名
		parts := strings.Split(req.Email, "@")
		var subdomain string
		if len(parts) == 2 {
			subdomain = parts[0]
		} else {
			subdomain = fmt.Sprintf("user%d", userID)
		}
		fullDomain := fmt.Sprintf("%s.mail.example.com", subdomain)

		// 直接保存到数据库
		err := s.storage.CreateMailDomain(userID, subdomain, fullDomain, subdomain, req.Email)
		if err != nil {
			response := map[string]string{"error": err.Error()}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}

		domain := &storage.MailDomain{
			Subdomain:  subdomain,
			FullDomain: fullDomain,
			RecordID:   subdomain,
			Email:      req.Email,
		}

		// 增加用户域名计数
		s.storage.IncrementDomainCount(userID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(domain)
		return
	}

	domain, err := s.dnsService.CreateMailDomain(userID, req.Email)
	if err != nil {
		response := map[string]string{"error": err.Error()}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	// 增加用户域名计数
	s.storage.IncrementDomainCount(userID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(domain)
}

// deleteDomain 删除邮箱域名
func (s *Server) deleteDomain(w http.ResponseWriter, r *http.Request) {
	// 获取当前用户ID
	userID := getUserIDFromRequest(r)

	vars := mux.Vars(r)
	id, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	// 如果DNS服务可用，尝试删除DNS记录
	if s.dnsService != nil {
		err = s.dnsService.DeleteMailDomain(userID, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		// DNS服务不可用时，只删除数据库记录
		log.Printf("DNS service not available, only deleting database record for domain ID: %d", id)
		err = s.storage.DeleteMailDomain(userID, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// 减少用户域名计数
	s.storage.DecrementDomainCount(userID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "success"})
}

// sendEmail 发送邮件
func (s *Server) sendEmail(w http.ResponseWriter, r *http.Request) {
	if s.emailSender == nil {
		response := map[string]string{"error": "邮件发送服务不可用"}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(response)
		return
	}

	// 获取当前用户ID
	userID := getUserIDFromRequest(r)

	var req struct {
		From    string `json:"from"`
		To      string `json:"to"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response := map[string]string{"error": "请求格式错误"}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	// 验证必填字段
	if req.From == "" || req.To == "" || req.Subject == "" || req.Body == "" {
		response := map[string]string{"error": "所有字段都是必填的"}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	// 验证发件人邮箱是否属于当前用户
	if s.dnsService != nil {
		domains, err := s.dnsService.GetMailDomains(userID)
		if err != nil {
			response := map[string]string{"error": "获取用户邮箱失败"}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}

		// 检查发件人邮箱是否在用户的邮箱列表中
		isValidSender := false
		for _, domain := range domains {
			if domain.Email == req.From {
				isValidSender = true
				break
			}
		}

		if !isValidSender {
			response := map[string]string{"error": "发件人邮箱不属于当前用户"}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	// 将纯文本内容转换为HTML格式
	htmlBody := s.convertTextToHTML(req.Body)

	// 发送邮件
	err := s.emailSender.SendEmail(req.To, req.Subject, htmlBody)
	if err != nil {
		log.Printf("发送邮件失败: %v", err)
		response := map[string]string{"error": fmt.Sprintf("邮件发送失败: %v", err)}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	// 记录发送的邮件（可选，用于统计）
	log.Printf("用户 %d 发送邮件: %s -> %s, 主题: %s", userID, req.From, req.To, req.Subject)

	response := map[string]string{"message": "邮件发送成功"}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// convertTextToHTML 将纯文本转换为HTML格式
func (s *Server) convertTextToHTML(text string) string {
	// 简单的文本到HTML转换，保留换行符
	html := strings.ReplaceAll(text, "\n", "<br>")
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>%s</title>
</head>
<body style="font-family: Arial, sans-serif; line-height: 1.6; color: #333;">
    %s
</body>
</html>`, "邮件", html)
}
