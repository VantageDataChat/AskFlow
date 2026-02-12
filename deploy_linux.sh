#!/bin/bash
# Helpdesk Linux 远程部署脚本

set -e

# 配置变量
SERVER="service.vantagedata.chat"
USER="root"
REMOTE_DIR="/opt/helpdesk"
DATA_DIR="/opt/helpdesk/data"
SERVICE_NAME="helpdesk"

echo "========================================="
echo "  Helpdesk Linux 远程部署"
echo "========================================="
echo ""

# 1. 测试 SSH 连接
echo "[1/8] 测试 SSH 连接..."
if ssh -o ConnectTimeout=5 -o StrictHostKeyChecking=no ${USER}@${SERVER} "echo '连接成功'" 2>/dev/null; then
    echo "      ✓ SSH 连接正常"
else
    echo "      ❌ SSH 连接失败，尝试配置 SSH 密钥..."
    if [ -f "setup_ssh.sh" ]; then
        bash setup_ssh.sh
    else
        echo "请检查服务器连接或手动配置 SSH"
        exit 1
    fi
fi
echo ""

# 2. 在服务器上安装依赖
echo "[2/8] 安装服务器依赖..."
ssh ${USER}@${SERVER} "
    apt-get update -qq
    apt-get install -y build-essential git wget curl sqlite3 > /dev/null 2>&1
    echo '      ✓ 依赖安装完成'
"
echo ""

# 3. 在服务器上编译程序
echo "[3/8] 在服务器上编译 Go 程序..."
ssh ${USER}@${SERVER} "
    # 检查 Go 是否安装
    if ! command -v go &> /dev/null; then
        echo '      - 安装 Go...'
        wget -q https://go.dev/dl/go1.23.5.linux-amd64.tar.gz
        rm -rf /usr/local/go
        tar -C /usr/local -xzf go1.23.5.linux-amd64.tar.gz
        rm go1.23.5.linux-amd64.tar.gz
        export PATH=\$PATH:/usr/local/go/bin
        echo 'export PATH=\$PATH:/usr/local/go/bin' >> ~/.bashrc
    fi

    GO_VERSION=\$(go version 2>/dev/null || echo 'not installed')
    echo \"      ✓ Go: \$GO_VERSION\"
"
echo ""

# 4. 上传源代码
echo "[4/8] 上传源代码到服务器..."
ssh ${USER}@${SERVER} "mkdir -p ${REMOTE_DIR}/build"

# 打包源代码（排除不必要的文件）
echo "      - 打包源代码..."
tar --exclude='build' \
    --exclude='node_modules' \
    --exclude='.git' \
    --exclude='*.exe' \
    --exclude='data' \
    -czf /tmp/helpdesk-src.tar.gz .

# 上传
echo "      - 上传到服务器..."
scp -q /tmp/helpdesk-src.tar.gz ${USER}@${SERVER}:${REMOTE_DIR}/
rm /tmp/helpdesk-src.tar.gz

ssh ${USER}@${SERVER} "
    cd ${REMOTE_DIR}
    tar -xzf helpdesk-src.tar.gz
    rm helpdesk-src.tar.gz
    echo '      ✓ 源代码上传完成'
"
echo ""

# 5. 在服务器上编译
echo "[5/8] 在服务器上编译..."
ssh ${USER}@${SERVER} "
    cd ${REMOTE_DIR}
    export PATH=\$PATH:/usr/local/go/bin
    export CGO_ENABLED=1

    echo '      - 编译中...'
    go build -ldflags '-s -w' -o helpdesk . 2>&1 | grep -v 'go: downloading' || true

    if [ -f 'helpdesk' ]; then
        chmod +x helpdesk
        SIZE=\$(du -h helpdesk | cut -f1)
        echo \"      ✓ 编译成功 (大小: \$SIZE)\"
    else
        echo '      ❌ 编译失败'
        exit 1
    fi
"
echo ""

# 6. 停止现有服务
echo "[6/8] 停止现有服务..."
ssh ${USER}@${SERVER} "
    if systemctl is-active --quiet ${SERVICE_NAME} 2>/dev/null; then
        echo '      - 停止服务...'
        systemctl stop ${SERVICE_NAME}
        echo '      ✓ 服务已停止'
    else
        echo '      - 无运行中的服务'
    fi
"
echo ""

# 7. 配置 systemd 服务
echo "[7/8] 配置 systemd 服务..."
ssh ${USER}@${SERVER} "
    # 创建数据目录
    mkdir -p ${DATA_DIR}/logs

    # 创建 systemd 服务文件
    cat > /etc/systemd/system/${SERVICE_NAME}.service << 'EOF'
[Unit]
Description=Helpdesk Support Service
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=${REMOTE_DIR}
ExecStart=${REMOTE_DIR}/helpdesk --datadir=${DATA_DIR}
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

    # 重载 systemd
    systemctl daemon-reload
    systemctl enable ${SERVICE_NAME}

    echo '      ✓ systemd 服务配置完成'
"
echo ""

# 8. 启动服务
echo "[8/8] 启动服务..."
ssh ${USER}@${SERVER} "
    systemctl start ${SERVICE_NAME}
    sleep 2

    if systemctl is-active --quiet ${SERVICE_NAME}; then
        echo '      ✓ 服务启动成功'
    else
        echo '      ❌ 服务启动失败'
        echo '      查看日志: journalctl -u ${SERVICE_NAME} -n 50'
        exit 1
    fi
"
echo ""

# 验证部署
echo "========================================="
echo "  部署完成！"
echo "========================================="
echo ""
echo "服务信息:"
echo "  - 服务器: ${SERVER}"
echo "  - 安装目录: ${REMOTE_DIR}"
echo "  - 数据目录: ${DATA_DIR}"
echo "  - 访问地址: http://${SERVER}:8080"
echo ""
echo "管理命令:"
echo "  - 查看状态: ssh ${USER}@${SERVER} 'systemctl status ${SERVICE_NAME}'"
echo "  - 停止服务: ssh ${USER}@${SERVER} 'systemctl stop ${SERVICE_NAME}'"
echo "  - 启动服务: ssh ${USER}@${SERVER} 'systemctl start ${SERVICE_NAME}'"
echo "  - 重启服务: ssh ${USER}@${SERVER} 'systemctl restart ${SERVICE_NAME}'"
echo "  - 查看日志: ssh ${USER}@${SERVER} 'journalctl -u ${SERVICE_NAME} -f'"
echo ""
echo "测试访问:"
echo "  curl http://${SERVER}:8080/api/health"
echo ""
