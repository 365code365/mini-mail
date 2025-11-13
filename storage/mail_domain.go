package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// MailDomain 邮箱域名记录
type MailDomain struct {
	ID         int64     `json:"id"`
	UserID     int64     `json:"user_id"`
	Subdomain  string    `json:"subdomain"`
	FullDomain string    `json:"full_domain"`
	RecordID   string    `json:"record_id"`
	Email      string    `json:"email"`
	CreatedAt  time.Time `json:"created_at"`
}

// CreateMailDomain 创建邮箱域名记录
func (s *SQLiteStorage) CreateMailDomain(userID int64, subdomain, fullDomain, recordID, email string) error {
	query := `
	INSERT INTO mail_domains (user_id, subdomain, full_domain, record_id, email, created_at)
	VALUES (?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query, userID, subdomain, fullDomain, recordID, email, time.Now())
	if err != nil {
		return fmt.Errorf("failed to create mail domain: %v", err)
	}
	return nil
}

// GetMailDomains 获取所有邮箱域名
func (s *SQLiteStorage) GetMailDomains(userID int64) ([]*MailDomain, error) {
	query := `
	SELECT id, user_id, subdomain, full_domain, record_id, email, created_at
	FROM mail_domains
	WHERE user_id = ?
	ORDER BY created_at DESC
	`
	rows, err := s.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query mail domains: %v", err)
	}
	defer rows.Close()

	var domains []*MailDomain
	for rows.Next() {
		var domain MailDomain
		err := rows.Scan(&domain.ID, &domain.UserID, &domain.Subdomain, &domain.FullDomain, &domain.RecordID, &domain.Email, &domain.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan mail domain: %v", err)
		}
		domains = append(domains, &domain)
	}
	return domains, nil
}

// DeleteMailDomain 删除邮箱域名记录
func (s *SQLiteStorage) DeleteMailDomain(userID int64, id int64) error {
	query := `DELETE FROM mail_domains WHERE id = ? AND user_id = ?`
	_, err := s.db.Exec(query, id, userID)
	if err != nil {
		return fmt.Errorf("failed to delete mail domain: %v", err)
	}
	return nil
}

// GetMailDomainByEmail 根据邮箱地址获取域名
func (s *SQLiteStorage) GetMailDomainByEmail(email string) (*MailDomain, error) {
	query := `
	SELECT id, user_id, subdomain, full_domain, record_id, email, created_at
	FROM mail_domains
	WHERE email = ?
	LIMIT 1
	`
	var domain MailDomain
	err := s.db.QueryRow(query, email).Scan(&domain.ID, &domain.UserID, &domain.Subdomain, &domain.FullDomain, &domain.RecordID, &domain.Email, &domain.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query mail domain: %v", err)
	}
	return &domain, nil
}

// GetMailDomainsByDomain 根据域名查找所有记录（如 niuma946.com）
func (s *SQLiteStorage) GetMailDomainsByDomain(domain string) ([]*MailDomain, error) {
	query := `
	SELECT id, user_id, subdomain, full_domain, record_id, email, created_at
	FROM mail_domains
	WHERE full_domain = ?
	ORDER BY created_at DESC
	`
	rows, err := s.db.Query(query, domain)
	if err != nil {
		return nil, fmt.Errorf("failed to query mail domains by domain: %v", err)
	}
	defer rows.Close()

	var domains []*MailDomain
	for rows.Next() {
		var d MailDomain
		err := rows.Scan(&d.ID, &d.UserID, &d.Subdomain, &d.FullDomain, &d.RecordID, &d.Email, &d.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan mail domain: %v", err)
		}
		domains = append(domains, &d)
	}
	return domains, nil
}
