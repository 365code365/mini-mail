#!/bin/bash

# 邮箱服务部署脚本
# 目标服务器: 124.156.188.238

set -e

SERVER_IP="124.156.188.238"
SERVER_USER="root"
SERVER_PASSWORD="a1039385286."
REMOTE_DIR="/opt/mail-server"
LOCAL_DIR="/Users/shengye/qoder/mail"
SERVER="${SERVER_USER}@${SERVER_IP}"

# 检查sshpass是否安装
if ! command -v sshpass &> /dev/null; then
    echo "⚠️  未安装 sshpass，正在尝试安装..."
    if command -v brew &> /dev/null; then
        brew install sshpass 2>/dev/null || echo "请手动安装: brew install sshpass"
    else
        echo "❌ 请先安装 sshpass:"
        echo "   macOS: brew install sshpass"
        echo "   Linux: apt-get install sshpass 或 yum install sshpass"
        exit 1
    fi
fi

# SSH命令别名（自动输入密码）
SSH_CMD="sshpass -p ${SERVER_PASSWORD} ssh -o StrictHostKeyChecking=no"
SCP_CMD="sshpass -p ${SERVER_PASSWORD} scp -o StrictHostKeyChecking=no"

echo "====================================="
echo "      邮箱服务部署脚本"
echo "====================================="
echo "目标服务器: ${SERVER}"
echo "部署目录: ${REMOTE_DIR}"
echo "端口配置: SMTP=25, HTTP=9989"
echo ""
echo "✅ 服务访问地址:"
echo "   管理界面: http://${SERVER_IP}:9989/"
echo "   SMTP服务: mail.niuma946.com:25"
echo "====================================="
echo ""

# 1. 上传源代码
echo "[1/6] 上传源代码到服务器..."
cd ${LOCAL_DIR}

# 创建临时目录
mkdir -p /tmp/mail-server-src
cp -r *.go go.mod go.sum smtp storage api services web /tmp/mail-server-src/ 2>/dev/null || true

# 上传到服务器
${SSH_CMD} ${SERVER} "mkdir -p ${REMOTE_DIR}/src"
${SCP_CMD} -r /tmp/mail-server-src/* ${SERVER}:${REMOTE_DIR}/src/

echo "✓ 源代码上传成功"
echo ""

# 2. 在服务器上编译
echo "[2/6] 在服务器上编译..."
${SSH_CMD} ${SERVER} << 'COMPILE'
set -x
cd /opt/mail-server/src

# 设置Go环境
export PATH=$PATH:/usr/local/go/bin
export GOPROXY=https://goproxy.cn,direct

# 检查Go是否安装
if command -v go &> /dev/null; then
    echo "Go已安装: $(go version)"
else
    echo "安装Go..."
    cd /tmp
    wget https://go.dev/dl/go1.21.0.linux-amd64.tar.gz
    rm -rf /usr/local/go 
    tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz
    echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
    cd /opt/mail-server/src
fi

# 编译
echo "正在编译..."
echo "下载依赖..."
go mod download
echo "开始编译程序..."
CGO_ENABLED=1 go build -v -o ../mail-server
BUILD_STATUS=$?
if [ $BUILD_STATUS -eq 0 ]; then
    echo "✓ 编译成功"
    chmod +x ../mail-server
    ls -lh ../mail-server
else
    echo "❌ 编译失败，exit code: $BUILD_STATUS"
    exit 1
fi
COMPILE

if [ $? -ne 0 ]; then
    echo "❌ 服务器编译失败!"
    exit 1
fi
echo "✓ 编译完成"
echo ""

# 3. 上传web目录和配置文件
echo "[3/6] 上传web目录..."
${SCP_CMD} -r web ${SERVER}:${REMOTE_DIR}/
if [ $? -ne 0 ]; then
    echo "❌ Web目录上传失败!"
    exit 1
fi
echo "✓ Web目录上传成功"

# 上传配置文件
echo "上传配置文件..."
if [ -f "config.yaml" ]; then
    ${SCP_CMD} config.yaml ${SERVER}:${REMOTE_DIR}/
    if [ $? -ne 0 ]; then
        echo "❌ 配置文件上传失败!"
        exit 1
    fi
    echo "✓ 配置文件上传成功"
else
    echo "⚠️  配置文件config.yaml不存在，使用默认配置"
fi
echo ""

# 4. 配置systemd服务
echo "[4/6] 配置systemd服务..."

# 创建systemd服务文件
cat > /tmp/mail-server.service << 'EOF'
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

${SCP_CMD} /tmp/mail-server.service ${SERVER}:/tmp/
${SSH_CMD} ${SERVER} "mv /tmp/mail-server.service /etc/systemd/system/ && systemctl daemon-reload"
if [ $? -ne 0 ]; then
    echo "❌ 服务配置失败!"
    exit 1
fi
echo "✓ Systemd服务配置成功"
echo ""

# 5. 配置防火墙和启动服务
echo "[5/6] 配置防火墙和启动服务..."
${SSH_CMD} ${SERVER} << 'ENDSSH'
# 设置权限
chmod +x /opt/mail-server/mail-server

# 配置防火墙
if systemctl is-active --quiet firewalld; then
    firewall-cmd --permanent --add-port=25/tcp
    firewall-cmd --permanent --add-port=9989/tcp
    firewall-cmd --reload
    echo "✓ 防火墙已配置 (firewalld)"
elif command -v ufw &> /dev/null; then
    ufw allow 25/tcp
    ufw allow 9989/tcp
    echo "✓ 防火墙已配置 (ufw)"
else
    echo "⚠ 未检测到防火墙"
fi

# 启动服务
systemctl enable mail-server
systemctl restart mail-server

echo ""
echo "等待服务启动..."
sleep 3
ENDSSH

echo "✓ 服务配置完成"
echo ""

# 6. 查看服务状态
echo "[6/6] 查看服务状态..."
${SSH_CMD} ${SERVER} 'systemctl status mail-server --no-pager -l' || true
echo ""

echo "====================================="
echo "        部署完成！"
echo "====================================="
echo ""
echo "✅ 服务信息:"
echo "   管理界面: http://${SERVER_IP}:9989/"
echo "   SMTP服务: mail.niuma946.com:25"
echo ""

# 检查服务是否启动成功
echo "检查服务启动状态..."
if ${SSH_CMD} ${SERVER} 'systemctl is-active --quiet mail-server'; then
    echo "✓ 服务运行正常"
    echo ""
    echo "📋 常用命令:"
    echo "   查看实时日志: ${SSH_CMD} ${SERVER} 'journalctl -u mail-server -f'"
    echo "   查看服务状态: ${SSH_CMD} ${SERVER} 'systemctl status mail-server'"
    echo "   重启服务:     ${SSH_CMD} ${SERVER} 'systemctl restart mail-server'"
    echo "   停止服务:     ${SSH_CMD} ${SERVER} 'systemctl stop mail-server'"
    echo ""
    echo "====================================="
    echo ""
    echo "正在查看实时日志 (按 Ctrl+C 退出)..."
    echo ""
    sleep 2
    ${SSH_CMD} ${SERVER} 'journalctl -u mail-server -f'
else
    echo "❌ 服务启动失败！查看错误日志:"
    echo ""
    echo "==================== 错误日志 ===================="
    ${SSH_CMD} ${SERVER} 'journalctl -u mail-server -n 50 --no-pager'
    echo ""
    echo "==================== 手动测试 ===================="
    echo "尝试手动运行程序查看错误:"
    ${SSH_CMD} ${SERVER} 'cd /opt/mail-server && ./mail-server 2>&1' &
    PID=$!
    sleep 5
    kill $PID 2>/dev/null || true
    echo ""
    echo "====================================="
    echo "排查建议:"
    echo "1. 检查程序依赖是否完整"
    echo "2. 检查端口是否被占用: ${SSH_CMD} ${SERVER} 'netstat -tlnp | grep -E \"(25|9989)\"'"
    echo "3. 手动运行查看详细错误: ${SSH_CMD} ${SERVER} 'cd /opt/mail-server && ./mail-server'"
    echo "====================================="
    exit 1
fi
