#!/bin/bash
#
#
# 前提条件:
# 1. Go 语言环境已安装。
# 2. Docker 已安装并正在运行。

# --- 应用元数据 ---
# 应用最终名称
APP_NAME="opcuaBaby"
# 应用的唯一标识符 (通常是反向域名)
APP_ID="com.giantbaby.opcua"
# 应用图标文件 (确保文件名正确)
ICON_FILE="opcuababy.icns"
# --------------------

# 定义目标平台列表
PLATFORMS=(
    "windows/amd64"
    "darwin/amd64"
    #"darwin/arm64"
)

echo "开始使用 fyne-cross 进行构建和打包..."
echo "请确保 Docker 正在运行。"

# 循环遍历平台列表并构建
for platform in "${PLATFORMS[@]}"; do
    # 从 "os/arch" 格式中分离 os 和 arch
    os="${platform%/*}"
    arch="${platform#*/}"

    echo "--------------------------------------------------"
    echo "正在为 $os ($arch) 构建..."
    echo "--------------------------------------------------"

    # 执行 fyne-cross 命令，使用独立的 os 子命令和 -arch 标志
    # 这会创建一个更完整的应用程序包，而不仅仅是可执行文件
    if [ "$os" == "windows" ]; then
       CGO_ENABLED=1 GOOS="$os"   GOARCH="$arch"   CC=x86_64-w64-mingw32-gcc fyne package    -name="$APP_NAME" -app-id="$APP_ID" -icon="$ICON_FILE"
    fi
    if [ "$os" == "darwin" ]; then
      GOARCH="$arch"  fyne package  -target "$os"  -name="$APP_NAME" -app-id="$APP_ID" -icon="$ICON_FILE"
    fi

    # 检查构建是否成功
    if [ $? -eq 0 ]; then
        echo "成功为 $os ($arch) 构建和打包。"
    else
        echo "为 $os ($arch) 构建失败。"
    fi
done

echo "--------------------------------------------------"
echo "所有构建已完成。"
echo "--------------------------------------------------"