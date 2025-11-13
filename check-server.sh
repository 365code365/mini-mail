#!/bin/bash

SERVER_PASSWORD="a1039385286."
SERVER="root@124.156.188.238"

echo "正在查看服务器状态..."
echo ""

sshpass -p "${SERVER_PASSWORD}" ssh -o StrictHostKeyChecking=no ${SERVER} << 'ENDSSH'
echo "=== 服务状态 ==="
systemctl status mail-server --no-pager

echo ""
echo "=== 最近30条日志 ==="
journalctl -u mail-server -n 30 --no-pager

echo ""
echo "=== 检查文件 ==="
ls -lh /opt/mail-server/

echo ""
echo "=== 测试运行 ==="
cd /opt/mail-server && ./mail-server 2>&1 | head -20
ENDSSH
