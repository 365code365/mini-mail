package main

import (
	"log"
	"mail-server/api"
	"mail-server/services"
	"mail-server/smtp"
	"mail-server/storage"
	"os"
	"os/signal"
	"syscall"
)

// Config 配置
type Config struct {
	Domain           string
	SMTPPort         int
	HTTPPort         int
	DatabasePath     string
	PublicIP         string
	TencentSecretID  string
	TencentSecretKey string
	// 邮件发送配置
	EmailSMTPHost string
	EmailSMTPPort int
	EmailSender   string
	EmailPassword string
}

// MailHandler 邮件处理器
type MailHandler struct {
	storage storage.Storage
}

func (h *MailHandler) HandleMail(msg *smtp.MailMessage) error {
	log.Printf("Received mail from %s to %v with subject: %s", msg.From, msg.To, msg.Subject)

	// 根据收件人邮箱地址找到创建者
	var userID int64 = 0
	if len(msg.To) > 0 {
		// 直接查找这个邮箱是谁创建的
		recipientEmail := msg.To[0]
		domain, err := h.storage.GetMailDomainByEmail(recipientEmail)
		if err != nil {
			log.Printf("Warning: Failed to find mail domain for %s: %v", recipientEmail, err)
		} else if domain != nil {
			// 直接从域名记录中获取 user_id
			userID = domain.UserID
			log.Printf("[Mail] 邮件归属用户ID: %d (邮箱: %s)", userID, recipientEmail)
		} else {
			log.Printf("Warning: 邮箱 %s 未在系统中创建", recipientEmail)
		}
	}

	// 如果找不到用户，使用默认userID=0（公共邮件）
	if userID == 0 {
		log.Printf("Warning: 邮件保存为公共邮件 (userID=0)，需要先在系统中创建该邮箱")
	}

	err := h.storage.SaveMail(userID, msg.From, msg.To, msg.Subject, msg.Body, msg.RawData)
	if err != nil {
		log.Printf("Error: 保存邮件失败: %v", err)
		return err
	}
	log.Printf("✓ 邮件已保存 (userID: %d, from: %s, to: %v)", userID, msg.From, msg.To)
	return nil
}

func main() {
	// 配置
	config := Config{
		Domain:           "",           // 主域名
		SMTPPort:         25,           // SMTP端口
		HTTPPort:         9989,         // HTTP API端口
		DatabasePath:     "./mails.db", // 数据库文件路径
		PublicIP:         "",           // 公网IP
		TencentSecretID:  "",           // 腾讯云SecretID
		TencentSecretKey: "",           // 腾讯云SecretKey
		// 邮件发送配置（使用自己的SMTP服务器）
		EmailSMTPHost: "127.0.0.1", // 本地SMTP服务器
		EmailSMTPPort: 25,          // SMTP端口
		EmailSender:   "",          // 发件人邮箱
		EmailPassword: "",          // 无需密码（本地服务器）
	}

	// 初始化存储
	store, err := storage.NewSQLiteStorage(config.DatabasePath)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer store.Close()

	// 初始化DNS服务
	mailDNSService, err := services.NewMailDNSService(
		config.Domain,
		config.PublicIP,
		config.TencentSecretID,
		config.TencentSecretKey,
		store,
	)
	if err != nil {
		log.Printf("Warning: Failed to initialize DNS service: %v", err)
		log.Printf("DNS management features will be disabled")
	}

	// 初始化邮件发送服务
	emailSender := services.NewEmailSender(
		config.EmailSMTPHost,
		config.EmailSMTPPort,
		config.EmailSender,
		"邮箱服务", // 发件人名称
		config.EmailPassword,
	)
	log.Printf("Email sender initialized: %s", config.EmailSender)

	// 创建邮件处理器
	handler := &MailHandler{storage: store}

	// 启动SMTP服务器
	smtpDomain := "mail." + config.Domain
	smtpServer := smtp.NewServer(smtpDomain, config.SMTPPort, handler)
	go func() {
		if err := smtpServer.Start(); err != nil {
			log.Fatalf("SMTP server error: %v", err)
		}
	}()

	// 启动HTTP API服务器
	apiServer := api.NewServer(store, mailDNSService, emailSender, config.HTTPPort)
	go func() {
		if err := apiServer.Start(); err != nil {
			log.Fatalf("HTTP API server error: %v", err)
		}
	}()

	log.Printf("Mail server started successfully!")
	log.Printf("SMTP Server: %s:%d", smtpDomain, config.SMTPPort)
	log.Printf("HTTP API Server: http://localhost:%d", config.HTTPPort)
	log.Printf("Web Management: http://localhost:%d/", config.HTTPPort)
	log.Printf("API Endpoints:")
	log.Printf("  - GET  /api/mails?limit=20&offset=0  - 获取邮件列表")
	log.Printf("  - GET  /api/mails/{id}               - 获取单个邮件")
	log.Printf("  - GET  /api/stats                    - 获取统计信息")
	log.Printf("  - GET  /api/domains                  - 获取邮箱域名列表")
	log.Printf("  - POST /api/domains                  - 创建邮箱域名")
	log.Printf("  - DELETE /api/domains/{id}           - 删除邮箱域名")

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down server...")
}
