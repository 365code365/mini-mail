package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Mail 邮件数据模型
type Mail struct {
	ID         int64     `json:"id"`
	From       string    `json:"from"`
	To         string    `json:"to"` // JSON array
	Subject    string    `json:"subject"`
	Body       string    `json:"body"`
	RawData    string    `json:"raw_data"`
	ReceivedAt time.Time `json:"received_at"`
}

// Storage 邮件存储接口
type Storage interface {
	SaveMail(userID int64, from string, to []string, subject, body, rawData string) error
	GetMails(userID int64, limit, offset int) ([]*Mail, error)
	GetMailByID(userID int64, id int64) (*Mail, error)
	GetMailCount(userID int64) (int64, error)
	Close() error

	// 邮箱域名管理
	CreateMailDomain(userID int64, subdomain, fullDomain, recordID, email string) error
	GetMailDomains(userID int64) ([]*MailDomain, error)
	DeleteMailDomain(userID int64, id int64) error
	GetMailDomainByEmail(email string) (*MailDomain, error)
	GetMailDomainsByDomain(domain string) ([]*MailDomain, error)

	// 用户管理
	CreateUser(email, password, registerIP string) (*User, error)
	GetUserByEmail(email string) (*User, error)
	UpdateUserPassword(email, password string) error
	GetUserCountByIP(ip string) (int, error)
	IncrementDomainCount(userID int64) error
	DecrementDomainCount(userID int64) error

	// 验证码管理
	CreateVerifyCode(email string) (string, error)
	VerifyCode(email, code string) (bool, error)
}

// SQLiteStorage SQLite存储实现
type SQLiteStorage struct {
	db *sql.DB
}

// NewSQLiteStorage 创建SQLite存储
func NewSQLiteStorage(dbPath string) (*SQLiteStorage, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}

	storage := &SQLiteStorage{db: db}
	if err := storage.init(); err != nil {
		db.Close()
		return nil, err
	}

	return storage, nil
}

// init 初始化数据库表
func (s *SQLiteStorage) init() error {
	query := `
	-- 用户表
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT NOT NULL UNIQUE,
		password TEXT,
		register_ip TEXT NOT NULL,
		is_admin BOOLEAN DEFAULT 0,
		domain_count INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_user_email ON users(email);
	CREATE INDEX IF NOT EXISTS idx_user_ip ON users(register_ip);
	
	-- 验证码表
	CREATE TABLE IF NOT EXISTS verify_codes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT NOT NULL,
		code TEXT NOT NULL,
		expires_at DATETIME NOT NULL,
		used BOOLEAN DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_verify_email ON verify_codes(email, created_at DESC);
	
	-- 邮件表（添加user_id）
	CREATE TABLE IF NOT EXISTS mails (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		mail_from TEXT NOT NULL,
		mail_to TEXT NOT NULL,
		subject TEXT,
		body TEXT,
		raw_data TEXT,
		received_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id)
	);
	CREATE INDEX IF NOT EXISTS idx_mails_user ON mails(user_id, received_at DESC);
	CREATE INDEX IF NOT EXISTS idx_mail_from ON mails(mail_from);
	
	-- 邮箱域名表（添加user_id）
	CREATE TABLE IF NOT EXISTS mail_domains (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		subdomain TEXT NOT NULL,
		full_domain TEXT NOT NULL UNIQUE,
		record_id TEXT NOT NULL,
		email TEXT NOT NULL UNIQUE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id)
	);
	CREATE INDEX IF NOT EXISTS idx_domains_user ON mail_domains(user_id);
	CREATE INDEX IF NOT EXISTS idx_email ON mail_domains(email);
	`

	_, err := s.db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create table: %v", err)
	}
	return nil
}

// SaveMail 保存邮件
func (s *SQLiteStorage) SaveMail(userID int64, from string, to []string, subject, body, rawData string) error {
	toJSON, err := json.Marshal(to)
	if err != nil {
		return fmt.Errorf("failed to marshal recipients: %v", err)
	}

	query := `
	INSERT INTO mails (user_id, mail_from, mail_to, subject, body, raw_data, received_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	_, err = s.db.Exec(query, userID, from, string(toJSON), subject, body, rawData, time.Now())
	if err != nil {
		return fmt.Errorf("failed to insert mail: %v", err)
	}

	return nil
}

// GetMails 获取邮件列表
func (s *SQLiteStorage) GetMails(userID int64, limit, offset int) ([]*Mail, error) {
	if limit <= 0 {
		limit = 20
	}

	query := `
	SELECT id, mail_from, mail_to, subject, body, raw_data, received_at
	FROM mails
	WHERE user_id = ?
	ORDER BY received_at DESC
	LIMIT ? OFFSET ?
	`

	rows, err := s.db.Query(query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query mails: %v", err)
	}
	defer rows.Close()

	var mails []*Mail
	for rows.Next() {
		var mail Mail
		var toJSON string
		err := rows.Scan(&mail.ID, &mail.From, &toJSON, &mail.Subject, &mail.Body, &mail.RawData, &mail.ReceivedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan mail: %v", err)
		}
		mail.To = toJSON
		mails = append(mails, &mail)
	}

	return mails, nil
}

// GetMailByID 根据ID获取邮件
func (s *SQLiteStorage) GetMailByID(userID int64, id int64) (*Mail, error) {
	query := `
	SELECT id, mail_from, mail_to, subject, body, raw_data, received_at
	FROM mails
	WHERE id = ? AND user_id = ?
	`

	var mail Mail
	var toJSON string
	err := s.db.QueryRow(query, id, userID).Scan(&mail.ID, &mail.From, &toJSON, &mail.Subject, &mail.Body, &mail.RawData, &mail.ReceivedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("mail not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query mail: %v", err)
	}

	mail.To = toJSON
	return &mail, nil
}

// GetMailCount 获取邮件总数
func (s *SQLiteStorage) GetMailCount(userID int64) (int64, error) {
	var count int64
	err := s.db.QueryRow("SELECT COUNT(*) FROM mails WHERE user_id = ?", userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count mails: %v", err)
	}
	return count, nil
}

// Close 关闭数据库连接
func (s *SQLiteStorage) Close() error {
	return s.db.Close()
}
