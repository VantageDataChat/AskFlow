#!/bin/bash
# RapidSpeech.cpp 远程编译脚本

# 设置变量
WORK_DIR="/root/rapidspeech-build"
REPO_URL="https://github.com/RapidAI/RapidSpeech.cpp"

echo "========================================="
echo "RapidSpeech.cpp 远程编译配置"
echo "========================================="
echo ""

# 1. 检查系统信息
echo "[1/7] 检查系统信息..."
uname -a
cat /etc/os-release | grep PRETTY_NAME

# 2. 安装编译依赖
echo ""
echo "[2/7] 安装编译依赖..."
if command -v apt-get &> /dev/null; then
    apt-get update
    apt-get install -y build-essential cmake git wget curl
elif command -v yum &> /dev/null; then
    yum install -y gcc gcc-c++ cmake git wget curl
fi

# 3. 验证工具
echo ""
echo "[3/7] 验证编译工具..."
gcc --version | head -1
g++ --version | head -1
cmake --version | head -1
git --version

# 4. 创建工作目录
echo ""
echo "[4/7] 创建工作目录..."
mkdir -p ${WORK_DIR}
cd ${WORK_DIR}

# 5. 克隆代码仓库
echo ""
echo "[5/7] 克隆 RapidSpeech.cpp 仓库..."
if [ -d "RapidSpeech.cpp" ]; then
    echo "仓库已存在，更新代码..."
    cd RapidSpeech.cpp
    git pull
else
    git clone ${REPO_URL}
    cd RapidSpeech.cpp
fi

# 6. 初始化子模块
echo ""
echo "[6/7] 初始化子模块..."
git submodule sync
git submodule update --init --recursive

# 7. 编译
echo ""
echo "[7/7] 开始编译（这可能需要几分钟）..."
cmake -B build -DCMAKE_BUILD_TYPE=Release
cmake --build build --config Release -j$(nproc)

# 检查编译结果
echo ""
echo "========================================="
echo "编译完成！"
echo "========================================="
echo ""
echo "可执行文件位置:"
ls -lh build/examples/rs-asr-offline
echo ""
echo "完整路径: ${WORK_DIR}/RapidSpeech.cpp/build/examples/rs-asr-offline"
