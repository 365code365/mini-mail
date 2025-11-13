package api

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	jwtSecret        = "mail-server-secret-key-change-in-production"
	tokenExpireHours = 24 * 7 // 7天有效期
)

type SendCodeRequest struct {
	Email string `json:"email"`
}

type VerifyCodeRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

type SetPasswordRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type PasswordLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token       string `json:"token"`
	ExpiresAt   int64  `json:"expires_at"`
	NeedSetPass bool   `json:"need_set_password"` // 是否需要设置密码
}

// hashPassword 对密码进行SHA256哈希
func hashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}

// getClientIP 获取客户端IP地址
func getClientIP(r *http.Request) string {
	// 检查X-Forwarded-For
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return strings.Split(ip, ",")[0]
	}
	// 检查X-Real-IP
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	// 使用RemoteAddr
	return strings.Split(r.RemoteAddr, ":")[0]
}

// generateToken 生成JWT token
func generateToken(email string, userID int64) (string, int64, error) {
	expiresAt := time.Now().Add(time.Hour * tokenExpireHours).Unix()

	claims := jwt.MapClaims{
		"email":  email,
		"userID": userID,
		"exp":    expiresAt,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		return "", 0, err
	}

	return tokenString, expiresAt, nil
}

// validateToken 验证JWT token并返回用户信息
func validateToken(tokenString string) (string, int64, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return []byte(jwtSecret), nil
	})

	if err != nil {
		return "", 0, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		email, emailOk := claims["email"].(string)
		if !emailOk {
			return "", 0, jwt.ErrInvalidKey
		}

		// 安全地转换userID
		var userID int64
		if userIDFloat, ok := claims["userID"].(float64); ok {
			userID = int64(userIDFloat)
		} else {
			return "", 0, jwt.ErrInvalidKey
		}

		return email, userID, nil
	}

	return "", 0, jwt.ErrSignatureInvalid
}

// register 注册新用户
func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := parseJSON(r, &req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的请求"})
		return
	}

	// 验证邮箱和密码
	if req.Email == "" || req.Password == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "邮箱和密码不能为空"})
		return
	}

	if len(req.Password) < 6 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "密码必须大于6位"})
		return
	}

	// 检查邮箱是否已存在
	existing, err := s.storage.GetUserByEmail(req.Email)
	if err != nil {
		log.Printf("Failed to check user: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "注册失败"})
		return
	}

	if existing != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "邮箱已被注册"})
		return
	}

	// 获取客户端IP
	clientIP := getClientIP(r)

	// 检查IP是否超过限制（非管理员）
	if req.Email != "admin@admin.com" {
		ipCount, err := s.storage.GetUserCountByIP(clientIP)
		if err != nil {
			log.Printf("Failed to check IP count: %v", err)
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "注册失败"})
			return
		}

		if ipCount >= 5 {
			respondJSON(w, http.StatusForbidden, map[string]string{"error": "该IP已达到最大注册数量限制（5个账户）"})
			return
		}
	}

	// 创建用户
	hashedPassword := hashPassword(req.Password)
	user, err := s.storage.CreateUser(req.Email, hashedPassword, clientIP)
	if err != nil {
		log.Printf("Failed to create user: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "注册失败"})
		return
	}

	log.Printf("用户注册成功: %s (IP: %s)", user.Email, clientIP)

	// 生成token并登录
	token, expiresAt, err := generateToken(user.Email, user.ID)
	if err != nil {
		log.Printf("Failed to generate token: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "登录失败"})
		return
	}

	respondJSON(w, http.StatusOK, LoginResponse{
		Token:       token,
		ExpiresAt:   expiresAt,
		NeedSetPass: false,
	})
}

// sendCode 发送验证码
func (s *Server) sendCode(w http.ResponseWriter, r *http.Request) {
	var req SendCodeRequest
	if err := parseJSON(r, &req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的请求"})
		return
	}

	if req.Email == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "邮箱地址不能为空"})
		return
	}

	// 生成验证码
	code, err := s.storage.CreateVerifyCode(req.Email)
	if err != nil {
		log.Printf("Failed to create verify code: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "生成验证码失败"})
		return
	}

	// 发送验证码邮件
	if s.emailSender != nil {
		err = s.emailSender.SendVerifyCode(req.Email, code)
		if err != nil {
			log.Printf("Failed to send email to %s: %v", req.Email, err)
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "发送邮件失败"})
			return
		}
		log.Printf("Verification code sent to %s", req.Email)
	} else {
		// 没有配置邮件服务，记录到日志
		log.Printf("Email service not configured. Verify code for %s: %s", req.Email, code)
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"message": "验证码已发送，请查收邮件",
	})
}

// verifyCode 验证验证码并登录
func (s *Server) verifyCode(w http.ResponseWriter, r *http.Request) {
	var req VerifyCodeRequest
	if err := parseJSON(r, &req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的请求"})
		return
	}

	if req.Email == "" || req.Code == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "邮箱和验证码不能为空"})
		return
	}

	// 验证验证码
	valid, err := s.storage.VerifyCode(req.Email, req.Code)
	if err != nil {
		log.Printf("Failed to verify code: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "验证失败"})
		return
	}

	if !valid {
		respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "验证码错误或已过期"})
		return
	}

	// 查找或创建用户
	user, err := s.storage.GetUserByEmail(req.Email)
	if err != nil {
		log.Printf("Failed to get user: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "获取用户信息失败"})
		return
	}

	needSetPass := false
	if user == nil {
		// 首次登录，创建用户（使用客户端IP）
		clientIP := getClientIP(r)
		user, err = s.storage.CreateUser(req.Email, "", clientIP)
		if err != nil {
			log.Printf("Failed to create user: %v", err)
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "创建用户失败"})
			return
		}
		needSetPass = true
	} else if user.Password == "" {
		// 用户存在但未设置密码
		needSetPass = true
	}

	// 生成token
	token, expiresAt, err := generateToken(user.Email, user.ID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "生成token失败"})
		return
	}

	respondJSON(w, http.StatusOK, LoginResponse{
		Token:       token,
		ExpiresAt:   expiresAt,
		NeedSetPass: needSetPass,
	})
}

// setPassword 设置初始密码
func (s *Server) setPassword(w http.ResponseWriter, r *http.Request) {
	var req SetPasswordRequest
	if err := parseJSON(r, &req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的请求"})
		return
	}

	if req.Email == "" || req.Password == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "邮箱和密码不能为空"})
		return
	}

	if len(req.Password) < 6 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "密码长度至少6位"})
		return
	}

	// 更新密码
	hashedPassword := hashPassword(req.Password)
	err := s.storage.UpdateUserPassword(req.Email, hashedPassword)
	if err != nil {
		log.Printf("Failed to update password: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "设置密码失败"})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "密码设置成功"})
}

// passwordLogin 密码登录
func (s *Server) passwordLogin(w http.ResponseWriter, r *http.Request) {
	var req PasswordLoginRequest
	if err := parseJSON(r, &req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的请求"})
		return
	}

	if req.Email == "" || req.Password == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "邮箱和密码不能为空"})
		return
	}

	// 查找用户
	user, err := s.storage.GetUserByEmail(req.Email)
	if err != nil {
		log.Printf("Failed to get user: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "登录失败"})
		return
	}

	if user == nil {
		respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "邮箱或密码错误"})
		return
	}

	// 验证密码
	hashedPassword := hashPassword(req.Password)
	if user.Password != hashedPassword {
		respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "邮箱或密码错误"})
		return
	}

	// 生成token
	token, expiresAt, err := generateToken(user.Email, user.ID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "生成token失败"})
		return
	}

	respondJSON(w, http.StatusOK, LoginResponse{
		Token:       token,
		ExpiresAt:   expiresAt,
		NeedSetPass: false,
	})
}

// authMiddleware 认证中间件
func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 从请求头获取token
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "未授权，请先登录"})
			return
		}

		// 验证Bearer token格式
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "无效的token格式"})
			return
		}

		// 验证token
		email, userID, err := validateToken(parts[1])
		if err != nil {
			respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "token无效或已过期"})
			return
		}

		// 将用户信息传递给后续处理函数
		r.Header.Set("X-User-Email", email)
		r.Header.Set("X-User-ID", strconv.FormatInt(userID, 10))

		next(w, r)
	}
}
