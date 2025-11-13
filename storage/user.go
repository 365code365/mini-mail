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
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Password  string    `json:"-"` // 不返回给前端
	CreatedAt time.Time `json:"created_at"`
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
func (s *SQLiteStorage) CreateUser(email, password string) (*User, error) {
	query := `INSERT INTO users (email, password, created_at) VALUES (?, ?, ?)`
	result, err := s.db.Exec(query, email, password, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %v", err)
	}

	id, _ := result.LastInsertId()
	return &User{
		ID:        id,
		Email:     email,
		CreatedAt: time.Now(),
	}, nil
}

// GetUserByEmail 根据邮箱获取用户
func (s *SQLiteStorage) GetUserByEmail(email string) (*User, error) {
	query := `SELECT id, email, password, created_at FROM users WHERE email = ?`

	var user User
	err := s.db.QueryRow(query, email).Scan(&user.ID, &user.Email, &user.Password, &user.CreatedAt)
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
