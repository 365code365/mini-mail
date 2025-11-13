package smtp

import (
	"fmt"
	"log"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// MailForwarder 邮件转发器
type MailForwarder struct {
	localDomain string
}

// NewMailForwarder 创建邮件转发器
func NewMailForwarder(localDomain string) *MailForwarder {
	return &MailForwarder{
		localDomain: localDomain,
	}
}

// Forward 转发邮件到外部邮箱服务器
func (f *MailForwarder) Forward(from string, to string, rawData string) error {
	// 检查是否是本地域名
	if f.isLocalDomain(to) {
		return fmt.Errorf("cannot forward to local domain: %s", to)
	}

	// 提取收件人域名
	domain := f.extractDomain(to)
	if domain == "" {
		return fmt.Errorf("invalid email address: %s", to)
	}

	log.Printf("[Forwarder] 准备转发邮件到外部邮箱: %s (域名: %s)", to, domain)

	// 查询MX记录
	mxRecords, err := net.LookupMX(domain)
	if err != nil {
		log.Printf("[Forwarder] 查询MX记录失败: %v, 尝试使用域名直连", err)
		// 如果MX记录查询失败，尝试直接使用域名
		return f.sendToServer(domain, from, to, rawData)
	}

	if len(mxRecords) == 0 {
		return fmt.Errorf("no MX records found for domain: %s", domain)
	}

	// 按优先级排序，尝试每个MX记录
	log.Printf("[Forwarder] 找到 %d 个MX记录", len(mxRecords))
	for _, mx := range mxRecords {
		log.Printf("[Forwarder] 尝试MX服务器: %s (优先级: %d)", mx.Host, mx.Pref)
		err = f.sendToServer(strings.TrimSuffix(mx.Host, "."), from, to, rawData)
		if err == nil {
			log.Printf("[Forwarder] ✓ 邮件成功转发到: %s", mx.Host)
			return nil
		}
		log.Printf("[Forwarder] ✗ 发送失败: %v, 尝试下一个MX服务器", err)
	}

	return fmt.Errorf("failed to forward to all MX servers for domain: %s", domain)
}

// sendToServer 发送邮件到指定SMTP服务器
func (f *MailForwarder) sendToServer(host string, from string, to string, rawData string) error {
	// 尝试标准SMTP端口
	ports := []int{25, 587}

	var lastErr error
	for _, port := range ports {
		addr := fmt.Sprintf("%s:%d", host, port)
		log.Printf("[Forwarder] 连接到 %s", addr)

		// 设置超时
		conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
		if err != nil {
			lastErr = fmt.Errorf("连接失败: %v", err)
			continue
		}

		// 创建SMTP客户端
		client, err := smtp.NewClient(conn, host)
		if err != nil {
			conn.Close()
			lastErr = fmt.Errorf("创建SMTP客户端失败: %v", err)
			continue
		}

		// 发送HELO/EHLO
		if err = client.Hello(f.localDomain); err != nil {
			client.Close()
			lastErr = fmt.Errorf("HELO失败: %v", err)
			continue
		}

		// 设置发件人
		if err = client.Mail(from); err != nil {
			client.Close()
			lastErr = fmt.Errorf("MAIL FROM失败: %v", err)
			continue
		}

		// 设置收件人
		if err = client.Rcpt(to); err != nil {
			client.Close()
			lastErr = fmt.Errorf("RCPT TO失败: %v", err)
			continue
		}

		// 发送邮件数据
		wc, err := client.Data()
		if err != nil {
			client.Close()
			lastErr = fmt.Errorf("DATA失败: %v", err)
			continue
		}

		_, err = wc.Write([]byte(rawData))
		if err != nil {
			wc.Close()
			client.Close()
			lastErr = fmt.Errorf("写入数据失败: %v", err)
			continue
		}

		err = wc.Close()
		if err != nil {
			client.Close()
			lastErr = fmt.Errorf("关闭数据连接失败: %v", err)
			continue
		}

		// 退出
		client.Quit()

		log.Printf("[Forwarder] 成功通过 %s 端口 %d 发送邮件", host, port)
		return nil
	}

	return lastErr
}

// isLocalDomain 检查是否是本地域名
func (f *MailForwarder) isLocalDomain(email string) bool {
	// 提取域名部分
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}
	domain := strings.ToLower(parts[1])
	// 检查是否是本地域名
	return domain == strings.ToLower(f.localDomain)
}

// extractDomain 从邮箱地址提取域名
func (f *MailForwarder) extractDomain(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return ""
	}
	return strings.ToLower(parts[1])
}
