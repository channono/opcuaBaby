#!/bin/bash
#
# 使用 fyne-cross 构建 Linux 平台版本 (amd64, arm64)
# 需要 Docker 运行
#

# --- 应用元数据 ---
APP_NAME="opcuaBaby"
APP_ID="com.giantbaby.opcua"
ICON_FILE="assets/icons/app.icns"
# --------------------

echo "=========================================="
echo "  使用 fyne-cross 构建 Linux 版本"
echo "=========================================="
echo ""
echo "前提条件检查:"
echo "  1. Docker Desktop 正在运行"
echo "  2. fyne-cross 已安装"
echo ""

# 检查 Docker 是否运行
if ! docker info > /dev/null 2>&1; then
    echo "[✗] Docker 未运行，请启动 Docker Desktop"
    exit 1
fi
echo "[✓] Docker 正在运行"

# 检查 fyne-cross 是否安装
if ! command -v fyne-cross &> /dev/null; then
    echo "[✗] fyne-cross 未安装"
    echo "    请运行: go install github.com/fyne-io/fyne-cross@latest"
    exit 1
fi
echo "[✓] fyne-cross 已安装"

echo ""
echo "=========================================="
echo "  开始构建..."
echo "=========================================="

# Linux 平台和架构列表
LINUX_ARCHS=("amd64" "arm64")

for arch in "${LINUX_ARCHS[@]}"; do
    echo ""
    echo "--------------------------------------------------"
    echo "正在构建 Linux ($arch)..."
    echo "--------------------------------------------------"
    
    # 注意: Linux 构建通常生成一个 tar.xz 包，或者直接是二进制文件
    fyne-cross linux \
        -arch="$arch" \
        -app-id="$APP_ID" \
        -icon="$ICON_FILE" \
        -name="$APP_NAME"
    
    if [ $? -eq 0 ]; then
        echo "[✓] Linux ($arch) 构建成功"
        
        # 输出路径通常是 fyne-cross/dist/linux-$arch
        OUTPUT_DIR="fyne-cross/dist/linux-$arch"
        RELEASE_DIR="release/linux"
        
        # 确保 release 目录存在
        mkdir -p "$RELEASE_DIR"
        
        # 复制二进制文件到 release 目录，使用统一的命名格式
        # Fyne cross 默认生成的 linux 二进制文件名通常是 APP_NAME (无扩展名)
        if [ -f "$OUTPUT_DIR/$APP_NAME" ]; then
            cp "$OUTPUT_DIR/$APP_NAME" "$RELEASE_DIR/opcuababy-linux-$arch"
            chmod +x "$RELEASE_DIR/opcuababy-linux-$arch"
            SIZE=$(du -h "$RELEASE_DIR/opcuababy-linux-$arch" | cut -f1)
            echo "    ✓ 已复制到: $RELEASE_DIR/opcuababy-linux-$arch ($SIZE)"
        elif [ -f "$OUTPUT_DIR/$APP_NAME.tar.xz" ]; then
             # 有些 Fyne 版本或配置可能会打包
             cp "$OUTPUT_DIR/$APP_NAME.tar.xz" "$RELEASE_DIR/opcuababy-linux-$arch.tar.xz"
             echo "    ✓ 已复制归档: $RELEASE_DIR/opcuababy-linux-$arch.tar.xz"
        fi
    else
        echo "[✗] Linux ($arch) 构建失败"
    fi
done

echo ""
echo "=========================================="
echo "  构建完成"
echo "=========================================="
echo ""
echo "发布目录内容:"
ls -lh release/linux/

echo ""
echo "✓ 所有 Linux 可执行文件已复制到: release/linux/"
echo "  - opcuababy-linux-amd64"
echo "  - opcuababy-linux-arm64"
