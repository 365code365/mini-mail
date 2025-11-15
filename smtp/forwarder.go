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

// 邮箱服务商端口配置
var providerPorts = map[string][]int{
	"qq.com":      {25, 587, 465},
	"foxmail.com": {25, 587, 465},
	"163.com":     {25, 587, 465},
	"126.com":     {25, 587, 465},
	"sina.com":    {25, 587, 465},
	"sohu.com":    {25, 587},
	"gmail.com":   {587, 465},
	"outlook.com": {587, 25},
	"live.com":    {587, 25},
	"hotmail.com": {587, 25},
	"yahoo.com":   {587, 465},
	"icloud.com":  {587, 25},
	"mail.ru":     {25, 587, 465},
	"yandex.com":  {25, 587, 465},
	// 默认端口（如果服务商不在列表中）
	"default": {25, 587, 465, 2525},
}

// NewMailForwarder 创建邮件转发器
func NewMailForwarder(localDomain string) *MailForwarder {
	return &MailForwarder{
		localDomain: localDomain,
	}
}

// getPortsForDomain 根据域名获取推荐的SMTP端口
func (f *MailForwarder) getPortsForDomain(domain string) []int {
	// 检查是否有匹配的邮箱服务商
	for provider, ports := range providerPorts {
		if strings.Contains(strings.ToLower(domain), provider) {
			log.Printf("[Forwarder] 检测到邮箱服务商: %s, 使用端口: %v", provider, ports)
			return ports
		}
	}

	// 如果没有匹配，使用默认端口
	log.Printf("[Forwarder] 未知邮箱服务商: %s, 使用默认端口: %v", domain, providerPorts["default"])
	return providerPorts["default"]
}

// checkPortAvailability 检查端口是否可用
func (f *MailForwarder) checkPortAvailability(host string, port int, timeout time.Duration) bool {
	addr := fmt.Sprintf("%s:%d", host, port)

	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		log.Printf("[Forwarder] 端口 %d 不可用: %v", port, err)
		return false
	}
	conn.Close()

	log.Printf("[Forwarder] ✓ 端口 %d 可用", port)
	return true
}

// findAvailablePorts 找到可用的端口
func (f *MailForwarder) findAvailablePorts(host string, ports []int) []int {
	var availablePorts []int

	for _, port := range ports {
		if f.checkPortAvailability(host, port, 2*time.Second) {
			availablePorts = append(availablePorts, port)
		}
	}

	if len(availablePorts) == 0 {
		log.Printf("[Forwarder] ⚠️  所有端口都不可用")
		return []int{25} // 至少尝试25端口
	}

	log.Printf("[Forwarder] ✓ 找到可用端口: %v", availablePorts)
	return availablePorts
}

// Forward 转发邮件到外部邮箱服务器
func (f *MailForwarder) Forward(from string, to string, rawData string) error {
	// 检查是否是本地域名
	if f.isLocalDomain(to) {
		return fmt.Errorf("cannot forward to local domain: %s", to)
	}

	// 使用直接转发
	return f.sendDirect(from, to, rawData)
}

// sendDirect 直接转发邮件到目标邮箱服务器
func (f *MailForwarder) sendDirect(from string, to string, rawData string) error {
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
		return f.sendToServerDirect(domain, from, to, rawData)
	}

	if len(mxRecords) == 0 {
		return fmt.Errorf("no MX records found for domain: %s", domain)
	}

	// 按优先级排序，尝试每个MX记录
	log.Printf("[Forwarder] 找到 %d 个MX记录", len(mxRecords))
	for _, mx := range mxRecords {
		log.Printf("[Forwarder] 尝试MX服务器: %s (优先级: %d)", mx.Host, mx.Pref)
		err = f.sendToServerDirect(strings.TrimSuffix(mx.Host, "."), from, to, rawData)
		if err == nil {
			log.Printf("[Forwarder] ✓ 邮件成功转发到: %s", mx.Host)
			return nil
		}
		log.Printf("[Forwarder] ✗ 发送失败: %v, 尝试下一个MX服务器", err)
	}

	return fmt.Errorf("failed to forward to all MX servers for domain: %s", domain)
}

// sendToServerDirect 使用智能端口检测发送邮件到指定SMTP服务器
func (f *MailForwarder) sendToServerDirect(host string, from string, to string, rawData string) error {
	// 提取域名用于端口检测
	domain := f.extractDomain(to)

	// 获取该邮箱服务商推荐的端口
	recommendedPorts := f.getPortsForDomain(domain)

	// 检测哪些端口可用
	availablePorts := f.findAvailablePorts(host, recommendedPorts)

	var lastErr error
	for _, port := range availablePorts {
		addr := fmt.Sprintf("%s:%d", host, port)
		log.Printf("[Forwarder] 尝试连接 %s", addr)

		// 使用较短的超时时间快速失败
		conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
		if err != nil {
			lastErr = fmt.Errorf("连接失败: %v", err)
			log.Printf("[Forwarder] ✗ %s", lastErr.Error())
			continue
		}

		// 设置连接超时
		conn.SetDeadline(time.Now().Add(8 * time.Second))

		// 创建SMTP客户端
		client, err := smtp.NewClient(conn, host)
		if err != nil {
			conn.Close()
			lastErr = fmt.Errorf("创建SMTP客户端失败: %v", err)
			log.Printf("[Forwarder] ✗ %s", lastErr.Error())
			continue
		}

		// 发送HELO/EHLO
		if err = client.Hello(f.localDomain); err != nil {
			client.Close()
			lastErr = fmt.Errorf("HELO失败: %v", err)
			log.Printf("[Forwarder] ✗ %s", lastErr.Error())
			continue
		}

		// 根据端口尝试TLS
		if port == 587 {
			tlsOk, _ := client.Extension("STARTTLS")
			if tlsOk {
				if err = client.StartTLS(nil); err != nil {
					log.Printf("[Forwarder] STARTTLS失败: %v，尝试不加密连接", err)
					// 继续尝试不加密连接
				} else {
					log.Printf("[Forwarder] ✓ TLS已启动")
				}
			}
		} else if port == 465 {
			// 465端口通常使用SSL/TLS
			log.Printf("[Forwarder] 465端口需要SSL，当前实现不支持，跳过")
			client.Close()
			continue
		}

		// 设置发件人
		if err = client.Mail(from); err != nil {
			client.Close()
			lastErr = fmt.Errorf("MAIL FROM失败: %v", err)
			log.Printf("[Forwarder] ✗ %s", lastErr.Error())
			continue
		}

		// 设置收件人
		if err = client.Rcpt(to); err != nil {
			client.Close()
			lastErr = fmt.Errorf("RCPT TO失败: %v", err)
			log.Printf("[Forwarder] ✗ %s", lastErr.Error())
			continue
		}

		// 发送邮件数据
		wc, err := client.Data()
		if err != nil {
			client.Close()
			lastErr = fmt.Errorf("DATA失败: %v", err)
			log.Printf("[Forwarder] ✗ %s", lastErr.Error())
			continue
		}

		_, err = wc.Write([]byte(rawData))
		if err != nil {
			wc.Close()
			client.Close()
			lastErr = fmt.Errorf("发送数据失败: %v", err)
			log.Printf("[Forwarder] ✗ %s", lastErr.Error())
			continue
		}

		wc.Close()
		client.Quit()
		log.Printf("[Forwarder] ✓ 邮件发送成功到 %s:%d", host, port)
		return nil
	}

	return lastErr
}

// sendToServer 发送邮件到指定SMTP服务器
func (f *MailForwarder) sendToServer(host string, from string, to string, rawData string) error {
	// 尝试更多端口和连接策略
	ports := []int{25, 587, 465, 2525}

	var lastErr error
	for _, port := range ports {
		addr := fmt.Sprintf("%s:%d", host, port)
		log.Printf("[Forwarder] 连接到 %s", addr)

		// 使用更短的超时时间快速失败
		conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
		if err != nil {
			lastErr = fmt.Errorf("连接失败: %v", err)
			log.Printf("[Forwarder] ✗ %s", lastErr.Error())
			continue
		}

		// 设置连接超时
		conn.SetDeadline(time.Now().Add(8 * time.Second))

		// 创建SMTP客户端
		client, err := smtp.NewClient(conn, host)
		if err != nil {
			conn.Close()
			lastErr = fmt.Errorf("创建SMTP客户端失败: %v", err)
			log.Printf("[Forwarder] ✗ %s", lastErr.Error())
			continue
		}

		// 发送HELO/EHLO
		if err = client.Hello(f.localDomain); err != nil {
			client.Close()
			lastErr = fmt.Errorf("HELO失败: %v", err)
			log.Printf("[Forwarder] ✗ %s", lastErr.Error())
			continue
		}

		// 根据端口尝试TLS
		if port == 587 {
			tlsOk, _ := client.Extension("STARTTLS")
			if tlsOk {
				if err = client.StartTLS(nil); err != nil {
					log.Printf("[Forwarder] STARTTLS失败: %v，尝试不加密连接", err)
					// 继续尝试不加密连接
				} else {
					log.Printf("[Forwarder] ✓ TLS已启动")
				}
			}
		} else if port == 465 {
			// 465端口通常使用SSL/TLS
			log.Printf("[Forwarder] 465端口需要SSL，当前实现不支持，跳过")
			client.Close()
			continue
		}

		// 设置发件人
		if err = client.Mail(from); err != nil {
			client.Close()
			lastErr = fmt.Errorf("MAIL FROM失败: %v", err)
			log.Printf("[Forwarder] ✗ %s", lastErr.Error())
			continue
		}

		// 设置收件人
		if err = client.Rcpt(to); err != nil {
			client.Close()
			lastErr = fmt.Errorf("RCPT TO失败: %v", err)
			log.Printf("[Forwarder] ✗ %s", lastErr.Error())
			continue
		}

		// 发送邮件数据
		wc, err := client.Data()
		if err != nil {
			client.Close()
			lastErr = fmt.Errorf("DATA失败: %v", err)
			log.Printf("[Forwarder] ✗ %s", lastErr.Error())
			continue
		}

		_, err = wc.Write([]byte(rawData))
		if err != nil {
			wc.Close()
			client.Close()
			lastErr = fmt.Errorf("发送数据失败: %v", err)
			log.Printf("[Forwarder] ✗ %s", lastErr.Error())
			continue
		}

		wc.Close()
		client.Quit()
		log.Printf("[Forwarder] ✓ 邮件发送成功到 %s:%d", host, port)
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
