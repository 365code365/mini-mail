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

	// 根据收件人邮箱找到对应的用户ID
	var userID int64 = 0
	if len(msg.To) > 0 {
		// 查找邮箱域名记录，获取用户ID
		domain, err := h.storage.GetMailDomainByEmail(msg.To[0])
		if err != nil {
			log.Printf("Warning: Failed to find mail domain for %s: %v", msg.To[0], err)
		} else if domain != nil {
			// 从域名记录中获取email，然后查找用户
			user, err := h.storage.GetUserByEmail(domain.Email)
			if err != nil {
				log.Printf("Warning: Failed to find user for email %s: %v", domain.Email, err)
			} else if user != nil {
				userID = user.ID
			}
		}
	}

	// 如果找不到用户，使用默认userID=0（可以后续扩展为公共邮箱）
	return h.storage.SaveMail(userID, msg.From, msg.To, msg.Subject, msg.Body, msg.RawData)
}

func main() {
	// 配置
	config := Config{
		Domain:           "niuma946.com",                         // 主域名
		SMTPPort:         25,                                     // SMTP端口
		HTTPPort:         9989,                                   // HTTP API端口
		DatabasePath:     "./mails.db",                           // 数据库文件路径
		PublicIP:         "124.156.188.238",                      // 公网IP
		TencentSecretID:  "AKIDWAcqxOjsoX3MRK2XobHpFXezBJOF98xZ", // 腾讯云SecretID
		TencentSecretKey: "VmqgHjy0pSzKRK1VmpePyJSP9g060nMi",     // 腾讯云SecretKey
		// 邮件发送配置
		EmailSMTPHost: "mail.niuma946.com",  // SMTP服务器地址
		EmailSMTPPort: 25,                   // SMTP端口
		EmailSender:   "admin@niuma946.com", // 发件人邮箱
		EmailPassword: "",                   // 邮箱密码（如需要）
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
