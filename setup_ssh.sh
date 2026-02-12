#!/bin/bash
# SSH 密钥自动配置脚本

SERVER="service.vantagedata.chat"
USER="root"
PASSWORD="sunion123"
PUBKEY=$(cat ~/.ssh/id_rsa.pub)

echo "正在配置 SSH 密钥到服务器..."

# 使用 expect 自动输入密码（如果没有 expect 则需要手动输入）
if command -v expect &> /dev/null; then
    expect << EOF
spawn ssh -o StrictHostKeyChecking=no ${USER}@${SERVER} "mkdir -p ~/.ssh && chmod 700 ~/.ssh && echo '${PUBKEY}' >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys"
expect {
    "password:" { send "${PASSWORD}\r"; exp_continue }
    eof
}
EOF
else
    # 没有 expect，使用交互式方式
    echo "请输入服务器密码: ${PASSWORD}"
    ssh -o StrictHostKeyChecking=no ${USER}@${SERVER} "mkdir -p ~/.ssh && chmod 700 ~/.ssh && echo '${PUBKEY}' >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys"
fi

echo "配置完成！测试连接..."
ssh -o StrictHostKeyChecking=no ${USER}@${SERVER} "echo '✅ SSH 密钥认证成功！'"
