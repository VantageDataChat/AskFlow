#!/bin/bash
# Helpdesk 远程部署脚本

set -e

# 配置变量
SERVER="service.vantagedata.chat"
USER="root"
REMOTE_DIR="/opt/helpdesk"
INSTALLER_PATH="build/installer/helpdesk-installer.exe"
DATA_DIR="/opt/helpdesk/data"

echo "========================================="
echo "  Helpdesk 远程部署脚本"
echo "========================================="
echo ""

# 检查安装包是否存在
if [ ! -f "$INSTALLER_PATH" ]; then
    echo "❌ 错误: 安装包不存在: $INSTALLER_PATH"
    echo "请先运行 build_local.cmd 构建安装包"
    exit 1
fi

INSTALLER_SIZE=$(du -h "$INSTALLER_PATH" | cut -f1)
echo "✓ 找到安装包: $INSTALLER_PATH ($INSTALLER_SIZE)"
echo ""

# 1. 测试 SSH 连接
echo "[1/6] 测试 SSH 连接..."
if ssh -o ConnectTimeout=5 -o StrictHostKeyChecking=no ${USER}@${SERVER} "echo '连接成功'" 2>/dev/null; then
    echo "      ✓ SSH 连接正常"
else
    echo "      ❌ SSH 连接失败"
    echo ""
    echo "尝试配置 SSH 密钥..."
    if [ -f "setup_ssh.sh" ]; then
        bash setup_ssh.sh
    else
        echo "请手动配置 SSH 或检查服务器连接"
        exit 1
    fi
fi
echo ""

# 2. 停止现有服务
echo "[2/6] 停止现有服务..."
ssh ${USER}@${SERVER} "
    if [ -f '${REMOTE_DIR}/helpdesk.exe' ]; then
        echo '      - 停止服务...'
        '${REMOTE_DIR}/helpdesk.exe' stop 2>/dev/null || true
        sleep 2
        echo '      - 卸载服务...'
        '${REMOTE_DIR}/helpdesk.exe' remove 2>/dev/null || true
        sleep 2
    fi
    echo '      ✓ 现有服务已停止'
"
echo ""

# 3. 备份现有数据（如果存在）
echo "[3/6] 备份现有数据..."
ssh ${USER}@${SERVER} "
    if [ -d '${DATA_DIR}' ]; then
        BACKUP_DIR='${REMOTE_DIR}/backup_\$(date +%Y%m%d_%H%M%S)'
        echo '      - 创建备份: \${BACKUP_DIR}'
        mkdir -p \${BACKUP_DIR}
        cp -r ${DATA_DIR}/* \${BACKUP_DIR}/ 2>/dev/null || true
        echo '      ✓ 数据已备份'
    else
        echo '      - 无需备份（首次部署）'
    fi
"
echo ""

# 4. 上传安装包
echo "[4/6] 上传安装包到服务器..."
ssh ${USER}@${SERVER} "mkdir -p ${REMOTE_DIR}/installer"
scp -o StrictHostKeyChecking=no "$INSTALLER_PATH" ${USER}@${SERVER}:${REMOTE_DIR}/installer/
echo "      ✓ 上传完成"
echo ""

# 5. 解压并安装
echo "[5/6] 在服务器上安装..."
ssh ${USER}@${SERVER} "
    cd ${REMOTE_DIR}

    # 使用 7z 或 unzip 解压 NSIS 安装包
    # NSIS 安装包实际上是自解压的，我们需要静默安装
    echo '      - 执行静默安装...'

    # 检查是否有 wine（用于在 Linux 上运行 Windows 程序）
    if command -v wine &> /dev/null; then
        wine installer/helpdesk-installer.exe /S /D=${REMOTE_DIR}
    else
        echo '      ⚠ 警告: 服务器上没有 wine，无法直接运行 Windows 安装包'
        echo '      请在 Windows 服务器上运行此脚本，或手动安装'
        exit 1
    fi

    echo '      ✓ 安装完成'
"
echo ""

# 6. 启动服务
echo "[6/6] 启动服务..."
ssh ${USER}@${SERVER} "
    cd ${REMOTE_DIR}

    # 安装并启动服务
    ./helpdesk.exe install --datadir='${DATA_DIR}'
    sleep 2
    ./helpdesk.exe start

    echo '      ✓ 服务已启动'
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
echo "  - 查看状态: ssh ${USER}@${SERVER} '${REMOTE_DIR}/helpdesk.exe help'"
echo "  - 停止服务: ssh ${USER}@${SERVER} '${REMOTE_DIR}/helpdesk.exe stop'"
echo "  - 启动服务: ssh ${USER}@${SERVER} '${REMOTE_DIR}/helpdesk.exe start'"
echo "  - 查看日志: ssh ${USER}@${SERVER} 'tail -f ${DATA_DIR}/logs/helpdesk.log'"
echo ""
