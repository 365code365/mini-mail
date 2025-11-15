package services

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"html/template"
	"net/smtp"
	"strings"
)

// EmailSender 邮件发送服务
type EmailSender struct {
	smtpHost    string
	smtpPort    int
	senderEmail string
	senderName  string
	password    string
}

// NewEmailSender 创建邮件发送服务
func NewEmailSender(smtpHost string, smtpPort int, senderEmail, senderName, password string) *EmailSender {
	return &EmailSender{
		smtpHost:    smtpHost,
		smtpPort:    smtpPort,
		senderEmail: senderEmail,
		senderName:  senderName,
		password:    password,
	}
}

// SendVerifyCode 发送验证码邮件
func (e *EmailSender) SendVerifyCode(to, code string) error {
	subject := "您的邮箱服务验证码"
	body := e.generateVerifyCodeHTML(code)

	return e.sendHTML(to, subject, body)
}

// SendEmail 发送通用邮件
func (e *EmailSender) SendEmail(to, subject, htmlBody string) error {
	return e.sendHTML(to, subject, htmlBody)
}

// SendTextEmail 发送纯文本邮件
func (e *EmailSender) SendTextEmail(to, subject, textBody string) error {
	// 构建邮件头
	headers := make(map[string]string)
	headers["From"] = fmt.Sprintf("%s <%s>", e.senderName, e.senderEmail)
	headers["To"] = to
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/plain; charset=UTF-8"

	// 组装邮件内容
	message := ""
	for k, v := range headers {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + textBody

	// 连接SMTP服务器并发送邮件
	addr := fmt.Sprintf("%s:%d", e.smtpHost, e.smtpPort)
	fmt.Printf("[EmailSender] 正在发送文本邮件到 %s，使用SMTP服务器: %s\n", to, addr)

	// 创建客户端连接
	client, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("连接SMTP服务器失败: %v", err)
	}
	defer client.Close()

	// 发送HELO
	if err = client.Hello("localhost"); err != nil {
		return fmt.Errorf("HELO失败: %v", err)
	}

	// 对于587端口，尝试启动TLS（但本地服务器可能不支持）
	if e.smtpPort == 587 {
		// 检查是否是本地服务器，如果是则跳过TLS
		isLocal := e.smtpHost == "localhost" ||
			e.smtpHost == "127.0.0.1" ||
			strings.HasPrefix(e.smtpHost, "mail.") ||
			strings.HasSuffix(e.smtpHost, ".local") ||
			strings.HasSuffix(e.smtpHost, ".lan")

		if !isLocal {
			tlsConfig := &tls.Config{
				ServerName:         e.smtpHost,
				InsecureSkipVerify: true, // 对于自签名证书
			}
			if err = client.StartTLS(tlsConfig); err != nil {
				fmt.Printf("[EmailSender] STARTTLS失败: %v\n", err)
				return fmt.Errorf("STARTTLS失败: %v", err)
			}
			fmt.Printf("[EmailSender] ✓ TLS已启动\n")
		} else {
			fmt.Printf("[EmailSender] 检测到本地服务器，跳过TLS\n")
		}
	}

	// 如果有密码，进行认证
	if e.password != "" {
		auth := smtp.PlainAuth("", e.senderEmail, e.password, e.smtpHost)
		if err = client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP认证失败: %v", err)
		}
	}

	// 设置发件人
	if err = client.Mail(e.senderEmail); err != nil {
		return fmt.Errorf("设置发件人失败: %v", err)
	}

	// 设置收件人
	if err = client.Rcpt(to); err != nil {
		return fmt.Errorf("设置收件人失败: %v", err)
	}

	// 发送邮件内容
	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("获取数据写入器失败: %v", err)
	}
	defer wc.Close()

	_, err = fmt.Fprintf(wc, "%s", message)
	if err != nil {
		return fmt.Errorf("写入邮件内容失败: %v", err)
	}

	fmt.Printf("[EmailSender] ✓ 邮件发送成功！\n")
	return nil
}

// sendHTML 发送HTML邮件
func (e *EmailSender) sendHTML(to, subject, htmlBody string) error {
	// 构建邮件头
	headers := make(map[string]string)
	headers["From"] = fmt.Sprintf("%s <%s>", e.senderName, e.senderEmail)
	headers["To"] = to
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=UTF-8"

	// 组装邮件内容
	message := ""
	for k, v := range headers {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + htmlBody

	// 连接SMTP服务器并发送邮件
	addr := fmt.Sprintf("%s:%d", e.smtpHost, e.smtpPort)
	fmt.Printf("[EmailSender] 正在发送邮件到 %s，使用SMTP服务器: %s\n", to, addr)

	// 创建客户端连接
	client, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("连接SMTP服务器失败: %v", err)
	}
	defer client.Close()

	// 发送HELO
	if err = client.Hello("localhost"); err != nil {
		return fmt.Errorf("HELO失败: %v", err)
	}

	// 对于587端口，尝试启动TLS（但本地服务器可能不支持）
	if e.smtpPort == 587 {
		// 检查是否是本地服务器，如果是则跳过TLS
		isLocal := e.smtpHost == "localhost" ||
			e.smtpHost == "127.0.0.1" ||
			strings.HasPrefix(e.smtpHost, "mail.") ||
			strings.HasSuffix(e.smtpHost, ".local") ||
			strings.HasSuffix(e.smtpHost, ".lan")

		if !isLocal {
			tlsConfig := &tls.Config{
				ServerName:         e.smtpHost,
				InsecureSkipVerify: true, // 对于自签名证书
			}
			if err = client.StartTLS(tlsConfig); err != nil {
				fmt.Printf("[EmailSender] STARTTLS失败: %v\n", err)
				return fmt.Errorf("STARTTLS失败: %v", err)
			}
			fmt.Printf("[EmailSender] ✓ TLS已启动\n")
		} else {
			fmt.Printf("[EmailSender] 检测到本地服务器，跳过TLS\n")
		}
	}

	// 如果有密码，进行认证
	if e.password != "" {
		auth := smtp.PlainAuth("", e.senderEmail, e.password, e.smtpHost)
		if err = client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP认证失败: %v", err)
		}
	}

	// 设置发件人
	if err = client.Mail(e.senderEmail); err != nil {
		return fmt.Errorf("设置发件人失败: %v", err)
	}

	// 设置收件人
	if err = client.Rcpt(to); err != nil {
		return fmt.Errorf("设置收件人失败: %v", err)
	}

	// 发送邮件内容
	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("获取数据写入器失败: %v", err)
	}
	defer wc.Close()

	_, err = fmt.Fprintf(wc, "%s", message)
	if err != nil {
		return fmt.Errorf("写入邮件内容失败: %v", err)
	}

	fmt.Printf("[EmailSender] ✓ 邮件发送成功！\n")
	return nil
}

// generateVerifyCodeHTML 生成验证码邮件HTML模板
func (e *EmailSender) generateVerifyCodeHTML(code string) string {
	tmpl := `
<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>邮箱验证码</title>
</head>
<body style="margin: 0; padding: 0; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif; background-color: #f5f7fa;">
    <table cellpadding="0" cellspacing="0" border="0" width="100%" style="background-color: #f5f7fa; padding: 40px 0;">
        <tr>
            <td align="center">
                <table cellpadding="0" cellspacing="0" border="0" width="600" style="background-color: #ffffff; border-radius: 12px; box-shadow: 0 4px 12px rgba(0,0,0,0.1); overflow: hidden;">
                    <!-- 邮件头部 -->
                    <tr>
                        <td style="background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); padding: 40px 30px; text-align: center;">
                            <h1 style="margin: 0; color: #ffffff; font-size: 28px; font-weight: 600;">
                                📧 邮箱服务
                            </h1>
                            <p style="margin: 10px 0 0 0; color: #ffffff; font-size: 14px; opacity: 0.9;">
                                Mail Server Verification
                            </p>
                        </td>
                    </tr>
                    
                    <!-- 邮件正文 -->
                    <tr>
                        <td style="padding: 40px 30px;">
                            <h2 style="margin: 0 0 20px 0; color: #333333; font-size: 22px; font-weight: 600;">
                                您的登录验证码
                            </h2>
                            
                            <p style="margin: 0 0 30px 0; color: #666666; font-size: 15px; line-height: 1.6;">
                                您好！您正在登录邮箱服务系统，请使用以下验证码完成登录：
                            </p>
                            
                            <!-- 验证码框 -->
                            <table cellpadding="0" cellspacing="0" border="0" width="100%">
                                <tr>
                                    <td align="center" style="padding: 30px 0;">
                                        <div style="background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); border-radius: 8px; padding: 20px 40px; display: inline-block;">
                                            <span style="color: #ffffff; font-size: 36px; font-weight: 700; letter-spacing: 8px; font-family: 'Courier New', monospace;">
                                                {{.Code}}
                                            </span>
                                        </div>
                                    </td>
                                </tr>
                            </table>
                            
                            <!-- 提示信息 -->
                            <div style="background-color: #fff3cd; border-left: 4px solid #ffc107; padding: 15px 20px; margin: 30px 0; border-radius: 4px;">
                                <p style="margin: 0; color: #856404; font-size: 14px; line-height: 1.6;">
                                    <strong>⏰ 重要提示：</strong><br>
                                    • 验证码有效期为 <strong>10分钟</strong><br>
                                    • 请勿将验证码透露给他人<br>
                                    • 如非本人操作，请忽略此邮件
                                </p>
                            </div>
                            
                            <p style="margin: 30px 0 0 0; color: #999999; font-size: 13px; line-height: 1.6;">
                                如有任何疑问，请联系系统管理员。
                            </p>
                        </td>
                    </tr>
                    
                    <!-- 邮件底部 -->
                    <tr>
                        <td style="background-color: #f8f9fa; padding: 30px; text-align: center; border-top: 1px solid #e9ecef;">
                            <p style="margin: 0 0 10px 0; color: #999999; font-size: 12px;">
                                此邮件由系统自动发送，请勿直接回复
                            </p>
                            <p style="margin: 0; color: #cccccc; font-size: 11px;">
                                © 2025 Mail Server. All rights reserved.
                            </p>
                        </td>
                    </tr>
                </table>
            </td>
        </tr>
    </table>
</body>
</html>
`

	t := template.Must(template.New("verify").Parse(tmpl))
	var buf bytes.Buffer
	t.Execute(&buf, map[string]string{"Code": code})
	return buf.String()
}
