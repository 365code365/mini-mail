package main

import (
	"io/ioutil"
	"log"
	"mail-server/api"
	"mail-server/services"
	"mail-server/smtp"
	"mail-server/storage"
	"os"
	"os/signal"
	"syscall"

	"gopkg.in/yaml.v3"
)

// Config 配置
type Config struct {
	Domain           string `yaml:"domain"`
	SMTPPort         int    `yaml:"smtp_port"`
	HTTPPort         int    `yaml:"http_port"`
	DatabasePath     string `yaml:"database_path"`
	PublicIP         string `yaml:"public_ip"`
	TencentSecretID  string `yaml:"tencent_secret_id"`
	TencentSecretKey string `yaml:"tencent_secret_key"`
	// 邮件发送配置
	EmailSMTPHost   string `yaml:"email_smtp_host"`
	EmailSMTPPort   int    `yaml:"email_smtp_port"`
	EmailSender     string `yaml:"email_sender"`
	EmailPassword   string `yaml:"email_password"`
	EmailSenderName string `yaml:"email_sender_name"`
	// 邮件转发配置
	ForwardEnabled bool `yaml:"forward_enabled"`
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
	// 默认配置
	config := Config{
		Domain:           "xxx.com",         // 主域名
		SMTPPort:         25,                // SMTP端口
		HTTPPort:         9989,              // HTTP API端口
		DatabasePath:     "./mails.db",      // 数据库文件路径
		PublicIP:         "124.xxx.xxx.238", // 公网IP
		TencentSecretID:  "xxx",             // 腾讯云SecretID
		TencentSecretKey: "xxx",             // 腾讯云SecretKey
		// 邮件发送配置（使用自己的SMTP服务器）
		EmailSMTPHost:   "mail.xxx.com",  // 自己的SMTP服务器
		EmailSMTPPort:   587,             // 使用587端口进行邮件提交
		EmailSender:     "admin@xxx.com", // 发件人邮箱
		EmailPassword:   "",              // 本地服务器无需密码
		EmailSenderName: "邮箱服务",          // 发件人名称
		// 邮件转发配置
		ForwardEnabled: false, // 暂时关闭邮件转发避免超时
	}

	// 尝试读取配置文件
	if _, err := os.Stat("config.yaml"); err == nil {
		log.Printf("Loading configuration from config.yaml...")
		data, err := ioutil.ReadFile("config.yaml")
		if err != nil {
			log.Printf("Failed to read config.yaml: %v, using default config", err)
		} else {
			err = yaml.Unmarshal(data, &config)
			if err != nil {
				log.Printf("Failed to parse config.yaml: %v, using default config", err)
			} else {
				log.Printf("Configuration loaded from config.yaml")
			}
		}
	} else {
		log.Printf("config.yaml not found, using default configuration")
		log.Printf("To customize settings, copy config.example.yaml to config.yaml")
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
		log.Printf("Error: Failed to initialize DNS service: %v", err)
		log.Printf("DNS management features will be disabled")
		mailDNSService = nil
	}

	// 初始化邮件发送服务
	emailSender := services.NewEmailSender(
		config.EmailSMTPHost,
		config.EmailSMTPPort,
		config.EmailSender,
		config.EmailSenderName, // 发件人名称
		config.EmailPassword,
	)
	log.Printf("Email sender initialized: %s", config.EmailSender)

	// 创建邮件处理器
	handler := &MailHandler{storage: store}

	// 启动SMTP服务器（25端口接收邮件）
	smtpDomain := "mail." + config.Domain
	smtpServer := smtp.NewServer(smtpDomain, config.SMTPPort, handler, config.ForwardEnabled)
	go func() {
		if err := smtpServer.Start(); err != nil {
			log.Fatalf("SMTP server error: %v", err)
		}
	}()

	// 启动SMTP提交服务器（587端口用于邮件提交）
	smtpSubmitServer := smtp.NewServer(smtpDomain, 587, handler, config.ForwardEnabled)
	go func() {
		if err := smtpSubmitServer.Start(); err != nil {
			log.Printf("SMTP submit server error: %v", err)
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
	log.Printf("SMTP Server (接收邮件): %s:%d", smtpDomain, config.SMTPPort)
	log.Printf("SMTP Submit Server (邮件提交): %s:587", smtpDomain)
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
