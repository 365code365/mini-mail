package storage

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"math/big"
	"time"
)

// User 用户模型
type User struct {
	ID          int64     `json:"id"`
	Email       string    `json:"email"`
	Password    string    `json:"-"` // 不返回给前端
	RegisterIP  string    `json:"register_ip"`
	IsAdmin     bool      `json:"is_admin"`
	DomainCount int       `json:"domain_count"` // 已创建的邮箱域名数量
	CreatedAt   time.Time `json:"created_at"`
}

// VerifyCode 验证码模型
type VerifyCode struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
	Used      bool      `json:"used"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateUser 创建用户
func (s *SQLiteStorage) CreateUser(email, password, registerIP string) (*User, error) {
	// 检查是否是管理员
	isAdmin := email == "admin@admin.com"

	query := `INSERT INTO users (email, password, register_ip, is_admin, domain_count, created_at) VALUES (?, ?, ?, ?, 0, ?)`
	result, err := s.db.Exec(query, email, password, registerIP, isAdmin, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %v", err)
	}

	id, _ := result.LastInsertId()
	return &User{
		ID:          id,
		Email:       email,
		RegisterIP:  registerIP,
		IsAdmin:     isAdmin,
		DomainCount: 0,
		CreatedAt:   time.Now(),
	}, nil
}

// GetUserByEmail 根据邮箱获取用户
func (s *SQLiteStorage) GetUserByEmail(email string) (*User, error) {
	query := `SELECT id, email, password, register_ip, is_admin, domain_count, created_at FROM users WHERE email = ?`

	var user User
	err := s.db.QueryRow(query, email).Scan(&user.ID, &user.Email, &user.Password, &user.RegisterIP, &user.IsAdmin, &user.DomainCount, &user.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query user: %v", err)
	}

	return &user, nil
}

// UpdateUserPassword 更新用户密码
func (s *SQLiteStorage) UpdateUserPassword(email, password string) error {
	query := `UPDATE users SET password = ? WHERE email = ?`
	_, err := s.db.Exec(query, password, email)
	return err
}

// GetUserCountByIP 获取IP创建的用户数量
func (s *SQLiteStorage) GetUserCountByIP(ip string) (int, error) {
	query := `SELECT COUNT(*) FROM users WHERE register_ip = ?`
	var count int
	err := s.db.QueryRow(query, ip).Scan(&count)
	return count, err
}

// IncrementDomainCount 增加用户域名计数
func (s *SQLiteStorage) IncrementDomainCount(userID int64) error {
	query := `UPDATE users SET domain_count = domain_count + 1 WHERE id = ?`
	_, err := s.db.Exec(query, userID)
	return err
}

// DecrementDomainCount 减少用户域名计数
func (s *SQLiteStorage) DecrementDomainCount(userID int64) error {
	query := `UPDATE users SET domain_count = domain_count - 1 WHERE id = ? AND domain_count > 0`
	_, err := s.db.Exec(query, userID)
	return err
}

// CreateVerifyCode 创建验证码
func (s *SQLiteStorage) CreateVerifyCode(email string) (string, error) {
	// 生成6位随机验证码
	code := generateCode(6)
	expiresAt := time.Now().Add(10 * time.Minute) // 10分钟有效期

	query := `INSERT INTO verify_codes (email, code, expires_at, used, created_at) VALUES (?, ?, ?, ?, ?)`
	_, err := s.db.Exec(query, email, code, expiresAt, false, time.Now())
	if err != nil {
		return "", fmt.Errorf("failed to create verify code: %v", err)
	}

	return code, nil
}

// VerifyCode 验证验证码
func (s *SQLiteStorage) VerifyCode(email, code string) (bool, error) {
	query := `
		SELECT id, expires_at, used 
		FROM verify_codes 
		WHERE email = ? AND code = ? 
		ORDER BY created_at DESC 
		LIMIT 1
	`

	var id int64
	var expiresAt time.Time
	var used bool

	err := s.db.QueryRow(query, email, code).Scan(&id, &expiresAt, &used)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	// 检查是否已使用
	if used {
		return false, nil
	}

	// 检查是否过期
	if time.Now().After(expiresAt) {
		return false, nil
	}

	// 标记为已使用
	updateQuery := `UPDATE verify_codes SET used = ? WHERE id = ?`
	_, err = s.db.Exec(updateQuery, true, id)
	if err != nil {
		return false, err
	}

	return true, nil
}

// generateCode 生成随机验证码
func generateCode(length int) string {
	const digits = "0123456789"
	code := make([]byte, length)
	for i := range code {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		code[i] = digits[n.Int64()]
	}
	return string(code)
}
