# 邮箱服务部署指南

## 环境信息
- 服务器: 124.156.188.238
- 用户: root
- 密码: a1039385286
- 域名: niuma946.com
- 部署路径: /opt/mail-server

## 快速部署步骤

### 方法一：使用自动部署脚本 (推荐)

1. **安装 sshpass (可选，用于自动输入密码)**
   ```bash
   # macOS
   brew install hudochenkov/sshpass/sshpass
   
   # Ubuntu/Debian
   apt-get install sshpass
   ```

2. **运行部署脚本**
   ```bash
   ./deploy.sh
   ```
   
   如果未安装 sshpass，需要手动输入密码 3-4 次。

### 方法二：手动部署

#### 1. 编译项目
```bash
cd /Users/shengye/qoder/mail
GOOS=linux GOARCH=amd64 go build -o mail-server
```

#### 2. 上传文件到服务器
```bash
# 创建远程目录
ssh root@124.156.188.238 "mkdir -p /opt/mail-server"

# 上传可执行文件
scp mail-server root@124.156.188.238:/opt/mail-server/

# 上传web目录
scp -r web root@124.156.188.238:/opt/mail-server/
```

#### 3. 配置systemd服务
```bash
# 创建服务文件
cat > mail-server.service << 'EOF'
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
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

# 上传并安装服务
scp mail-server.service root@124.156.188.238:/tmp/
ssh root@124.156.188.238 << 'EOF'
mv /tmp/mail-server.service /etc/systemd/system/
systemctl daemon-reload
EOF
```

#### 4. 配置防火墙
```bash
ssh root@124.156.188.238 << 'EOF'
# CentOS/RHEL (firewalld)
firewall-cmd --permanent --add-port=25/tcp
firewall-cmd --permanent --add-port=8080/tcp
firewall-cmd --reload

# 或 Ubuntu/Debian (ufw)
# ufw allow 25/tcp
# ufw allow 8080/tcp
EOF
```

#### 5. 启动服务
```bash
ssh root@124.156.188.238 << 'EOF'
chmod +x /opt/mail-server/mail-server
systemctl enable mail-server
systemctl start mail-server
systemctl status mail-server
EOF
```

## DNS配置

### 必须配置的DNS记录

登录腾讯云DNSPod控制台，为域名 `niuma946.com` 添加以下记录：

1. **A记录** (指向SMTP服务器)
   - 主机记录: `mail`
   - 记录类型: `A`
   - 记录值: `124.156.188.238`
   - TTL: `600`

2. **MX记录** (主域名邮件交换)
   - 主机记录: `@`
   - 记录类型: `MX`
   - 记录值: `mail.niuma946.com`
   - 优先级: `10`
   - TTL: `600`

3. **SPF记录** (可选，提高邮件投递率)
   - 主机记录: `@`
   - 记录类型: `TXT`
   - 记录值: `v=spf1 ip4:124.156.188.238 ~all`
   - TTL: `600`

## 服务管理命令

### 查看服务状态
```bash
ssh root@124.156.188.238 'systemctl status mail-server'
```

### 查看实时日志
```bash
ssh root@124.156.188.238 'journalctl -u mail-server -f'
```

### 重启服务
```bash
ssh root@124.156.188.238 'systemctl restart mail-server'
```

### 停止服务
```bash
ssh root@124.156.188.238 'systemctl stop mail-server'
```

## 访问服务

### 管理界面
```
http://124.156.188.238:8080/
```

### API接口
```bash
# 获取邮件列表
curl http://124.156.188.238:8080/api/mails

# 获取统计信息
curl http://124.156.188.238:8080/api/stats

# 获取邮箱域名列表
curl http://124.156.188.238:8080/api/domains

# 创建邮箱域名
curl -X POST http://124.156.188.238:8080/api/domains \
  -H "Content-Type: application/json" \
  -d '{"email":"test@niuma946.com"}'
```

## 测试邮件接收

### 使用telnet测试SMTP
```bash
telnet mail.niuma946.com 25

# 输入以下命令:
HELO test.com
MAIL FROM:<test@test.com>
RCPT TO:<admin@niuma946.com>
DATA
Subject: Test Email

This is a test email.
.
QUIT
```

### 使用mail命令发送
```bash
echo "Test email body" | mail -s "Test Subject" admin@niuma946.com
```

## 功能说明

### 1. 邮件接收
- SMTP服务监听25端口
- 自动接收发往 @niuma946.com 的邮件
- 所有邮件存储在SQLite数据库

### 2. DNS自动解析
- 通过管理界面创建邮箱时，自动创建MX记录
- 为每个邮箱生成唯一的子域名
- 使用腾讯云DNSPod API管理

### 3. Web管理界面
- 查看所有接收的邮件
- 管理邮箱域名
- 创建新邮箱并自动解析
- 实时统计信息

## 常见问题

### 1. 端口25被占用
```bash
# 查看占用端口的进程
ssh root@124.156.188.238 'netstat -tlnp | grep :25'
```

### 2. 防火墙阻止
```bash
# 检查防火墙状态
ssh root@124.156.188.238 'firewall-cmd --list-all'
```

### 3. DNS解析不生效
- 等待DNS传播（最多10分钟）
- 使用 `dig mail.niuma946.com` 检查

### 4. 查看详细日志
```bash
ssh root@124.156.188.238 'journalctl -u mail-server --no-pager -n 100'
```

## 安全建议

1. **修改默认端口** (可选)
   - 将SMTP端口从25改为587
   - 将HTTP端口从8080改为其他端口

2. **配置SSL/TLS** (推荐)
   - 为SMTP添加TLS加密
   - 为HTTP API添加HTTPS

3. **限制访问**
   - 使用防火墙限制API访问IP
   - 配置Nginx反向代理

4. **定期备份**
   ```bash
   # 备份数据库
   ssh root@124.156.188.238 'cp /opt/mail-server/mails.db /backup/mails_$(date +%Y%m%d).db'
   ```

## 更新部署

如果需要更新代码：
```bash
# 重新编译并部署
./deploy.sh

# 或手动更新
GOOS=linux GOARCH=amd64 go build -o mail-server
scp mail-server root@124.156.188.238:/opt/mail-server/
ssh root@124.156.188.238 'systemctl restart mail-server'
```
