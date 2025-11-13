package services

import (
	"fmt"
	"log"
	"mail-server/storage"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	dnspod "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/dnspod/v20210323"
)

// MailDNSService 邮箱DNS管理服务
type MailDNSService struct {
	dnsService *DNSPodService
	storage    storage.Storage
}

// NewMailDNSService 创建邮箱DNS服务
func NewMailDNSService(domain, publicIP, secretId, secretKey string, storage storage.Storage) (*MailDNSService, error) {
	dnsService, err := NewDNSPodServiceWithCredentials(domain, publicIP, secretId, secretKey)
	if err != nil {
		return nil, err
	}

	return &MailDNSService{
		dnsService: dnsService,
		storage:    storage,
	}, nil
}

// CreateMailDomain 为邮箱创建域名解析
func (m *MailDNSService) CreateMailDomain(userID int64, email string) (*storage.MailDomain, error) {
	// 检查邮箱是否已经存在域名
	existing, err := m.storage.GetMailDomainByEmail(email)
	if err != nil {
		return nil, fmt.Errorf("检查邮箱域名失败: %v", err)
	}
	if existing != nil {
		return existing, nil
	}

	// 生成子域名
	subdomain, err := m.dnsService.GenerateSubdomain()
	if err != nil {
		return nil, fmt.Errorf("生成子域名失败: %v", err)
	}

	// 创建MX记录和A记录
	err = m.createMailRecords(subdomain, email)
	if err != nil {
		return nil, fmt.Errorf("创建DNS记录失败: %v", err)
	}

	fullDomain := fmt.Sprintf("%s.%s", subdomain, m.dnsService.domain)

	// 保存到数据库（使用子域名作为recordID的占位符）
	err = m.storage.CreateMailDomain(userID, subdomain, fullDomain, subdomain, email)
	if err != nil {
		// 如果保存失败，尝试清理DNS记录
		m.deleteMailRecords(subdomain)
		return nil, fmt.Errorf("保存邮箱域名失败: %v", err)
	}

	domain := &storage.MailDomain{
		Subdomain:  subdomain,
		FullDomain: fullDomain,
		RecordID:   subdomain,
		Email:      email,
	}

	log.Printf("邮箱域名创建成功: %s -> %s", email, fullDomain)
	return domain, nil
}

// createMailRecords 创建邮箱相关的DNS记录
func (m *MailDNSService) createMailRecords(subdomain, email string) error {
	// 创建MX记录指向主域名的mail子域名
	err := m.createMXRecord(subdomain)
	if err != nil {
		return fmt.Errorf("创建MX记录失败: %v", err)
	}

	log.Printf("为子域名 %s 创建MX记录成功", subdomain)
	return nil
}

// createMXRecord 创建MX记录
func (m *MailDNSService) createMXRecord(subdomain string) error {
	// 使用DNSPod API创建MX记录
	// MX记录指向 mail.主域名
	request := dnspod.NewCreateRecordRequest()
	request.Domain = common.StringPtr(m.dnsService.domain)
	request.RecordType = common.StringPtr("MX")
	request.RecordLine = common.StringPtr("默认")
	// MX记录的Value只需要域名，不需要优先级和点号
	request.Value = common.StringPtr(fmt.Sprintf("mail.%s", m.dnsService.domain))
	request.SubDomain = common.StringPtr(subdomain)
	request.TTL = common.Uint64Ptr(600)
	request.Status = common.StringPtr("ENABLE")
	// 优先级单独设置在MX字段
	request.MX = common.Uint64Ptr(10)

	_, err := m.dnsService.client.CreateRecord(request)
	return err
}

// deleteMailRecords 删除邮箱相关的DNS记录
func (m *MailDNSService) deleteMailRecords(subdomain string) error {
	// 这里需要实现删除逻辑，暂时简化
	log.Printf("尝试删除子域名 %s 的DNS记录", subdomain)
	return nil
}

// DeleteMailDomain 删除邮箱域名
func (m *MailDNSService) DeleteMailDomain(userID int64, id int64) error {
	// 从数据库删除
	err := m.storage.DeleteMailDomain(userID, id)
	if err != nil {
		return fmt.Errorf("删除邮箱域名失败: %v", err)
	}

	// TODO: 删除对应的DNS记录
	return nil
}

// GetMailDomains 获取所有邮箱域名
func (m *MailDNSService) GetMailDomains(userID int64) ([]*storage.MailDomain, error) {
	return m.storage.GetMailDomains(userID)
}

// GetMailDomainByEmail 根据邮箱获取域名
func (m *MailDNSService) GetMailDomainByEmail(email string) (*storage.MailDomain, error) {
	return m.storage.GetMailDomainByEmail(email)
}
