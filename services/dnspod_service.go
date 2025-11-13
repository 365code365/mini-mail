package services

import (
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"
	"sync"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	dnspod "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/dnspod/v20210323"
)

// DNSPodService DNSPod服务
type DNSPodService struct {
	client       *dnspod.Client
	domain       string
	publicIP     string
	subdomainMap map[string]int    // 子域名到端口的映射
	portMap      map[int]string    // 端口到子域名的映射
	recordMap    map[string]string // 子域名到记录ID的映射
	mu           sync.RWMutex
}

// DNSRecord DNS记录信息
type DNSRecord struct {
	SubDomain  string `json:"subdomain"`
	Domain     string `json:"domain"`
	RecordID   string `json:"record_id"`
	Port       int    `json:"port"`
	FullDomain string `json:"full_domain"`
}

// NewDNSPodService 创建DNSPod服务实例
// NewDNSPodServiceWithCredentials 使用指定的密钥创建DNSPod服务
func NewDNSPodServiceWithCredentials(domain, publicIP, secretId, secretKey string) (*DNSPodService, error) {
	if secretId == "" || secretKey == "" {
		return nil, fmt.Errorf("腾讯云密钥不能为空")
	}

	// 创建认证对象
	credential := common.NewCredential(secretId, secretKey)

	// 创建客户端配置
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = "dnspod.tencentcloudapi.com"

	// 创建客户端
	client, err := dnspod.NewClient(credential, "", cpf)
	if err != nil {
		return nil, fmt.Errorf("创建DNSPod客户端失败: %v", err)
	}

	return &DNSPodService{
		client:       client,
		domain:       domain,
		publicIP:     publicIP,
		subdomainMap: make(map[string]int),
		portMap:      make(map[int]string),
		recordMap:    make(map[string]string),
	}, nil
}

func NewDNSPodService(domain, publicIP string) (*DNSPodService, error) {
	// 从环境变量获取腾讯云密钥
	secretId := os.Getenv("TENCENTCLOUD_SECRET_ID")
	secretKey := os.Getenv("TENCENTCLOUD_SECRET_KEY")

	if secretId == "" || secretKey == "" {
		return nil, fmt.Errorf("腾讯云密钥未配置，请设置环境变量 TENCENTCLOUD_SECRET_ID 和 TENCENTCLOUD_SECRET_KEY")
	}

	// 创建认证对象
	credential := common.NewCredential(secretId, secretKey)

	// 创建客户端配置
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = "dnspod.tencentcloudapi.com"

	// 创建客户端
	client, err := dnspod.NewClient(credential, "", cpf)
	if err != nil {
		return nil, fmt.Errorf("创建DNSPod客户端失败: %v", err)
	}

	service := &DNSPodService{
		client:       client,
		domain:       domain,
		publicIP:     publicIP,
		subdomainMap: make(map[string]int),
		portMap:      make(map[int]string),
		recordMap:    make(map[string]string),
	}

	log.Printf("DNSPod服务初始化成功: domain=%s, publicIP=%s", domain, publicIP)
	return service, nil
}

// GenerateSubdomain 生成唯一的子域名
func (d *DNSPodService) GenerateSubdomain() (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	const length = 8

	for attempts := 0; attempts < 100; attempts++ {
		// 生成随机字符串
		b := make([]byte, length)
		for i := range b {
			n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
			if err != nil {
				return "", fmt.Errorf("生成随机数失败: %v", err)
			}
			b[i] = charset[n.Int64()]
		}

		subdomain := string(b)

		// 检查是否已存在
		if _, exists := d.subdomainMap[subdomain]; !exists {
			return subdomain, nil
		}
	}

	return "", fmt.Errorf("生成唯一子域名失败，尝试次数过多")
}

// CreateDNSRecord 创建DNS记录
func (d *DNSPodService) CreateDNSRecord(port int) (*DNSRecord, error) {
	// 生成子域名
	subdomain, err := d.GenerateSubdomain()
	if err != nil {
		return nil, fmt.Errorf("生成子域名失败: %v", err)
	}

	// 创建DNS记录请求
	request := dnspod.NewCreateRecordRequest()
	request.Domain = common.StringPtr(d.domain)
	request.RecordType = common.StringPtr("A")
	request.RecordLine = common.StringPtr("默认")
	request.Value = common.StringPtr(d.publicIP)
	request.SubDomain = common.StringPtr(subdomain)
	request.TTL = common.Uint64Ptr(600)
	request.Status = common.StringPtr("ENABLE")
	request.Remark = common.StringPtr(fmt.Sprintf("内网穿透代理端口%d", port))

	// 调用API创建记录
	response, err := d.client.CreateRecord(request)
	if err != nil {
		if sdkErr, ok := err.(*errors.TencentCloudSDKError); ok {
			return nil, fmt.Errorf("DNSPod API错误: %s", sdkErr.Message)
		}
		return nil, fmt.Errorf("创建DNS记录失败: %v", err)
	}

	recordID := fmt.Sprintf("%d", *response.Response.RecordId)
	fullDomain := fmt.Sprintf("%s.%s", subdomain, d.domain)

	// 更新映射关系
	d.mu.Lock()
	d.subdomainMap[subdomain] = port
	d.portMap[port] = subdomain
	d.recordMap[subdomain] = recordID
	d.mu.Unlock()

	record := &DNSRecord{
		SubDomain:  subdomain,
		Domain:     d.domain,
		RecordID:   recordID,
		Port:       port,
		FullDomain: fullDomain,
	}

	log.Printf("DNS记录创建成功: %s -> %s:%d (RecordID: %s)", fullDomain, d.publicIP, port, recordID)
	return record, nil
}

// DeleteDNSRecord 删除DNS记录
func (d *DNSPodService) DeleteDNSRecord(port int) error {
	d.mu.Lock()
	subdomain, exists := d.portMap[port]
	if !exists {
		d.mu.Unlock()
		return fmt.Errorf("端口 %d 对应的DNS记录不存在", port)
	}

	recordID, exists := d.recordMap[subdomain]
	if !exists {
		d.mu.Unlock()
		return fmt.Errorf("子域名 %s 对应的记录ID不存在", subdomain)
	}
	d.mu.Unlock()

	// 创建删除记录请求
	request := dnspod.NewDeleteRecordRequest()
	request.Domain = common.StringPtr(d.domain)
	request.RecordId = common.Uint64Ptr(parseUint64(recordID))

	// 调用API删除记录
	_, err := d.client.DeleteRecord(request)
	if err != nil {
		if sdkErr, ok := err.(*errors.TencentCloudSDKError); ok {
			return fmt.Errorf("DNSPod API错误: %s", sdkErr.Message)
		}
		return fmt.Errorf("删除DNS记录失败: %v", err)
	}

	// 更新映射关系
	d.mu.Lock()
	delete(d.subdomainMap, subdomain)
	delete(d.portMap, port)
	delete(d.recordMap, subdomain)
	d.mu.Unlock()

	fullDomain := fmt.Sprintf("%s.%s", subdomain, d.domain)
	log.Printf("DNS记录删除成功: %s (RecordID: %s)", fullDomain, recordID)
	return nil
}

// GetPortByDomain 根据域名获取端口
func (d *DNSPodService) GetPortByDomain(domain string) (int, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// 提取子域名
	subdomain := d.extractSubdomain(domain)
	if subdomain == "" {
		return 0, false
	}

	port, exists := d.subdomainMap[subdomain]
	return port, exists
}

// GetDomainByPort 根据端口获取域名
func (d *DNSPodService) GetDomainByPort(port int) (string, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	subdomain, exists := d.portMap[port]
	if !exists {
		return "", false
	}

	return fmt.Sprintf("%s.%s", subdomain, d.domain), true
}

// ListRecords 列出所有DNS记录
func (d *DNSPodService) ListRecords() []*DNSRecord {
	d.mu.RLock()
	defer d.mu.RUnlock()

	records := make([]*DNSRecord, 0, len(d.subdomainMap))
	for subdomain, port := range d.subdomainMap {
		recordID := d.recordMap[subdomain]
		records = append(records, &DNSRecord{
			SubDomain:  subdomain,
			Domain:     d.domain,
			RecordID:   recordID,
			Port:       port,
			FullDomain: fmt.Sprintf("%s.%s", subdomain, d.domain),
		})
	}

	return records
}

// extractSubdomain 从完整域名中提取子域名
func (d *DNSPodService) extractSubdomain(fullDomain string) string {
	// 移除协议前缀
	domain := strings.TrimPrefix(fullDomain, "http://")
	domain = strings.TrimPrefix(domain, "https://")

	// 移除端口号
	if colonIndex := strings.Index(domain, ":"); colonIndex != -1 {
		domain = domain[:colonIndex]
	}

	// 检查是否是我们的域名
	if !strings.HasSuffix(domain, "."+d.domain) && domain != d.domain {
		return ""
	}

	// 如果就是主域名，返回空
	if domain == d.domain {
		return ""
	}

	// 提取子域名
	subdomain := strings.TrimSuffix(domain, "."+d.domain)
	return subdomain
}

// parseUint64 字符串转uint64
func parseUint64(s string) uint64 {
	var result uint64
	fmt.Sscanf(s, "%d", &result)
	return result
}

// UpdatePublicIP 更新公网IP
func (d *DNSPodService) UpdatePublicIP(newIP string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	oldIP := d.publicIP
	d.publicIP = newIP

	log.Printf("公网IP已更新: %s -> %s", oldIP, newIP)

	// TODO: 这里可以批量更新所有DNS记录的IP地址
	// 为了简化，暂时只更新内存中的IP，新创建的记录会使用新IP

	return nil
}
