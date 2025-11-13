package smtp

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"net/mail"
	"strings"
	"time"
)

// MailMessage 表示接收到的邮件
type MailMessage struct {
	From       string
	To         []string
	Subject    string
	Body       string
	RawData    string
	ReceivedAt time.Time
}

// MailHandler 处理接收到的邮件
type MailHandler interface {
	HandleMail(msg *MailMessage) error
}

// Server SMTP服务器
type Server struct {
	Domain   string
	Port     int
	Handler  MailHandler
	listener net.Listener
}

// NewServer 创建新的SMTP服务器
func NewServer(domain string, port int, handler MailHandler) *Server {
	return &Server{
		Domain:  domain,
		Port:    port,
		Handler: handler,
	}
}

// Start 启动SMTP服务器
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %v", addr, err)
	}
	s.listener = listener
	log.Printf("SMTP server listening on %s", addr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("failed to accept connection: %v", err)
			continue
		}
		go s.handleConnection(conn)
	}
}

// Stop 停止SMTP服务器
func (s *Server) Stop() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

// handleConnection 处理单个连接
func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	session := &smtpSession{
		conn:   conn,
		server: s,
	}
	session.handle()
}

// smtpSession SMTP会话
type smtpSession struct {
	conn       net.Conn
	server     *Server
	mailFrom   string
	rcptTo     []string
	dataBuffer bytes.Buffer
}

// handle 处理SMTP会话
func (s *smtpSession) handle() {
	// 发送欢迎消息
	s.writeLine(fmt.Sprintf("220 %s SMTP Service Ready", s.server.Domain))

	reader := io.Reader(s.conn)
	buffer := make([]byte, 4096)

	for {
		n, err := reader.Read(buffer)
		if err != nil {
			if err != io.EOF {
				log.Printf("read error: %v", err)
			}
			return
		}

		line := strings.TrimSpace(string(buffer[:n]))
		if line == "" {
			continue
		}

		log.Printf("Received: %s", line)

		// 处理SMTP命令
		if !s.processCommand(line) {
			return
		}
	}
}

// processCommand 处理SMTP命令
func (s *smtpSession) processCommand(line string) bool {
	parts := strings.SplitN(line, " ", 2)
	cmd := strings.ToUpper(parts[0])
	var arg string
	if len(parts) > 1 {
		arg = parts[1]
	}

	switch cmd {
	case "HELO", "EHLO":
		s.writeLine(fmt.Sprintf("250 %s Hello", s.server.Domain))
	case "MAIL":
		// MAIL FROM:<sender@example.com>
		if strings.HasPrefix(strings.ToUpper(arg), "FROM:") {
			email := extractEmail(arg[5:])
			s.mailFrom = email
			s.writeLine("250 OK")
		} else {
			s.writeLine("501 Syntax error")
		}
	case "RCPT":
		// RCPT TO:<recipient@example.com>
		if strings.HasPrefix(strings.ToUpper(arg), "TO:") {
			email := extractEmail(arg[3:])
			s.rcptTo = append(s.rcptTo, email)
			s.writeLine("250 OK")
		} else {
			s.writeLine("501 Syntax error")
		}
	case "DATA":
		s.writeLine("354 Start mail input; end with <CRLF>.<CRLF>")
		s.receiveData()
	case "QUIT":
		s.writeLine("221 Bye")
		return false
	case "RSET":
		s.reset()
		s.writeLine("250 OK")
	case "NOOP":
		s.writeLine("250 OK")
	default:
		s.writeLine("500 Command not recognized")
	}

	return true
}

// receiveData 接收邮件数据
func (s *smtpSession) receiveData() {
	s.dataBuffer.Reset()
	reader := io.Reader(s.conn)
	buffer := make([]byte, 4096)

	for {
		n, err := reader.Read(buffer)
		if err != nil {
			log.Printf("error reading data: %v", err)
			return
		}

		s.dataBuffer.Write(buffer[:n])

		// 检查是否以 \r\n.\r\n 结束
		data := s.dataBuffer.String()
		if strings.HasSuffix(data, "\r\n.\r\n") || strings.HasSuffix(data, "\n.\n") {
			// 移除结束标记
			data = strings.TrimSuffix(data, "\r\n.\r\n")
			data = strings.TrimSuffix(data, "\n.\n")
			s.processMailData(data)
			s.reset()
			return
		}
	}
}

// processMailData 处理邮件数据
func (s *smtpSession) processMailData(data string) {
	// 解析邮件
	msg, err := mail.ReadMessage(strings.NewReader(data))
	if err != nil {
		log.Printf("failed to parse mail: %v", err)
		s.writeLine("550 Failed to parse message")
		return
	}

	// 读取邮件正文
	body, err := io.ReadAll(msg.Body)
	if err != nil {
		log.Printf("failed to read body: %v", err)
		body = []byte("")
	}

	mailMsg := &MailMessage{
		From:       s.mailFrom,
		To:         s.rcptTo,
		Subject:    msg.Header.Get("Subject"),
		Body:       string(body),
		RawData:    data,
		ReceivedAt: time.Now(),
	}

	// 调用处理器
	if s.server.Handler != nil {
		err := s.server.Handler.HandleMail(mailMsg)
		if err != nil {
			log.Printf("failed to handle mail: %v", err)
			s.writeLine("550 Failed to process message")
			return
		}
	}

	s.writeLine("250 OK: Message accepted for delivery")
}

// reset 重置会话状态
func (s *smtpSession) reset() {
	s.mailFrom = ""
	s.rcptTo = []string{}
	s.dataBuffer.Reset()
}

// writeLine 写入一行响应
func (s *smtpSession) writeLine(line string) {
	s.conn.Write([]byte(line + "\r\n"))
	log.Printf("Sent: %s", line)
}

// extractEmail 从字符串中提取邮箱地址
func extractEmail(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "<>")
	return s
}
