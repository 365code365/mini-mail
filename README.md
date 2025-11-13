# 邮箱服务器 (Mail Server)

使用Go语言开发的邮箱服务,支持接收邮件(SMTP)和通过HTTP API获取邮件。

## 功能特性

- ✅ SMTP服务器接收邮件 (端口25)
- ✅ HTTP API获取邮件列表和详情
- ✅ SQLite数据库存储邮件
- ✅ 支持自定义域名配置
- ✅ RESTful API接口

## 项目结构

```
mail/
├── main.go           # 主程序入口
├── smtp/             # SMTP服务器模块
│   └── server.go     # SMTP协议实现
├── api/              # HTTP API模块
│   └── server.go     # REST API实现
├── storage/          # 数据存储模块
│   └── storage.go    # SQLite存储实现
├── config.example.yaml  # 配置文件示例
└── README.md         # 说明文档
```

## 快速开始

### 1. 安装依赖

```bash
go mod download
```

### 2. 配置域名

编辑 `main.go` 中的配置:

```go
config := Config{
    Domain:      "mail.example.com", // 修改为你的域名
    SMTPPort:    25,                  // SMTP端口
    HTTPPort:    8080,                // HTTP API端口
    DatabasePath: "./mails.db",       // 数据库路径
}
```

### 3. 运行服务

```bash
# 普通模式运行
go run main.go

# 或编译后运行
go build -o mail-server
./mail-server
```

**注意**: SMTP服务默认使用25端口,需要root权限。如果在测试环境,可以修改为2525等非特权端口。

```bash
# 使用root权限运行(生产环境)
sudo ./mail-server

# 或修改SMTPPort为2525后普通权限运行(测试环境)
```

## API接口说明

### 1. 获取邮件列表

```bash
GET /api/mails?limit=20&offset=0
```

**参数**:
- `limit`: 每页数量 (默认20, 最大100)
- `offset`: 偏移量 (默认0)

**响应示例**:
```json
{
  "total": 100,
  "limit": 20,
  "offset": 0,
  "mails": [
    {
      "id": 1,
      "from": "sender@example.com",
      "to": "[\"recipient@example.com\"]",
      "subject": "测试邮件",
      "body": "邮件正文内容",
      "raw_data": "原始邮件数据...",
      "received_at": "2025-11-13T10:30:00Z"
    }
  ]
}
```

### 2. 获取单个邮件

```bash
GET /api/mails/{id}
```

**响应示例**:
```json
{
  "id": 1,
  "from": "sender@example.com",
  "to": "[\"recipient@example.com\"]",
  "subject": "测试邮件",
  "body": "邮件正文内容",
  "raw_data": "原始邮件数据...",
  "received_at": "2025-11-13T10:30:00Z"
}
```

### 3. 获取统计信息

```bash
GET /api/stats
```

**响应示例**:
```json
{
  "total_mails": 100
}
```

## 域名配置说明

### DNS配置

要让其他邮件服务器能够向你的服务器发送邮件,需要配置DNS记录:

#### 1. A记录
将域名指向你的服务器IP:
```
mail.example.com.  IN  A  124.156.188.238
```

#### 2. MX记录
配置邮件交换记录:
```
example.com.  IN  MX  10  mail.example.com.
```

#### 3. SPF记录 (可选,提高邮件投递率)
```
example.com.  IN  TXT  "v=spf1 ip4:124.156.188.238 ~all"
```

### 防火墙配置

确保服务器防火墙开放以下端口:
- TCP 25 (SMTP)
- TCP 8080 (HTTP API)

```bash
# CentOS/RHEL
sudo firewall-cmd --permanent --add-port=25/tcp
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --reload

# Ubuntu/Debian
sudo ufw allow 25/tcp
sudo ufw allow 8080/tcp
```

### 测试邮件接收

使用telnet测试SMTP服务:

```bash
telnet mail.example.com 25

# 输入以下命令:
HELO test.com
MAIL FROM:<test@test.com>
RCPT TO:<admin@example.com>
DATA
Subject: Test Email

This is a test email.
.
QUIT
```

或使用命令行发送邮件:

```bash
echo "This is a test email" | mail -s "Test Subject" admin@example.com
```

## 测试API

### 使用curl测试

```bash
# 获取邮件列表
curl http://localhost:8080/api/mails?limit=10

# 获取单个邮件
curl http://localhost:8080/api/mails/1

# 获取统计信息
curl http://localhost:8080/api/stats
```

### 使用浏览器

直接访问: `http://localhost:8080/api/mails`

## 生产环境部署

### 1. 使用systemd管理服务

创建服务文件 `/etc/systemd/system/mail-server.service`:

```ini
[Unit]
Description=Mail Server
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/mail-server
ExecStart=/opt/mail-server/mail-server
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

启动服务:

```bash
sudo systemctl daemon-reload
sudo systemctl enable mail-server
sudo systemctl start mail-server
sudo systemctl status mail-server
```

### 2. 日志管理

查看日志:
```bash
sudo journalctl -u mail-server -f
```

### 3. 数据备份

定期备份数据库文件:
```bash
cp /opt/mail-server/mails.db /backup/mails_$(date +%Y%m%d).db
```

## 开发说明

### 编译

```bash
# 本地编译
go build -o mail-server

# 交叉编译 (Linux)
GOOS=linux GOARCH=amd64 go build -o mail-server-linux

# 交叉编译 (Windows)
GOOS=windows GOARCH=amd64 go build -o mail-server.exe
```

### 运行测试

```bash
go test ./...
```

## 注意事项

1. **端口25权限**: 使用25端口需要root权限,测试时可改用2525等高端口
2. **反垃圾邮件**: 生产环境建议添加SPF、DKIM、DMARC等反垃圾邮件机制
3. **TLS加密**: 建议添加TLS/SSL支持以加密邮件传输
4. **邮件队列**: 大量邮件场景建议使用消息队列
5. **存储优化**: 大量邮件建议使用PostgreSQL或MySQL替代SQLite

## 技术栈

- Go 1.21+
- SQLite3 (数据库)
- Gorilla Mux (HTTP路由)
- 标准库 net/mail (邮件解析)

## 许可证

MIT License

## 支持

如有问题,请提交Issue或联系开发者。
