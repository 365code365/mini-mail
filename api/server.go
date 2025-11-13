package api

import (
	"encoding/json"
	"log"
	"mail-server/services"
	"mail-server/storage"
	"net/http"
	"strconv"

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
	if s.dnsService == nil {
		http.Error(w, "DNS service not available", http.StatusServiceUnavailable)
		return
	}

	// 获取当前用户ID
	userID := getUserIDFromRequest(r)

	domains, err := s.dnsService.GetMailDomains(userID)
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
	if s.dnsService == nil {
		http.Error(w, "DNS service not available", http.StatusServiceUnavailable)
		return
	}

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
	if s.dnsService == nil {
		http.Error(w, "DNS service not available", http.StatusServiceUnavailable)
		return
	}

	// 获取当前用户ID
	userID := getUserIDFromRequest(r)

	vars := mux.Vars(r)
	id, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	err = s.dnsService.DeleteMailDomain(userID, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 减少用户域名计数
	s.storage.DecrementDomainCount(userID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "success"})
}
