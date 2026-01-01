#!/bin/bash
#
# 使用 fyne-cross 构建所有 Windows 平台版本
# 需要 Docker 运行
#

# --- 应用元数据 ---
APP_NAME="opcuaBaby"
APP_ID="com.giantbaby.opcua"
ICON_FILE="assets/icons/app.icns"
# --------------------

echo "=========================================="
echo "  使用 fyne-cross 构建 Windows 版本"
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

# Windows 平台和架构列表
WINDOWS_ARCHS=("amd64" "arm64")

for arch in "${WINDOWS_ARCHS[@]}"; do
    echo ""
    echo "--------------------------------------------------"
    echo "正在构建 Windows ($arch)..."
    echo "--------------------------------------------------"
    
    fyne-cross windows \
        -arch="$arch" \
        -app-id="$APP_ID" \
        -icon="$ICON_FILE" \
        -name="$APP_NAME"
    
    if [ $? -eq 0 ]; then
        echo "[✓] Windows ($arch) 构建成功"
        
        # 解压并复制到 release 目录
        OUTPUT_DIR="fyne-cross/dist/windows-$arch"
        RELEASE_DIR="release/windows"
        
        if [ -f "$OUTPUT_DIR/$APP_NAME.zip" ]; then
            # 解压 ZIP 文件
            unzip -o -q "$OUTPUT_DIR/$APP_NAME.zip" -d "$OUTPUT_DIR/"
            
            # 确保 release 目录存在
            mkdir -p "$RELEASE_DIR"
            
            # 复制到 release 目录，使用统一的命名格式
            if [ -f "$OUTPUT_DIR/opcuababy.exe" ]; then
                cp "$OUTPUT_DIR/opcuababy.exe" "$RELEASE_DIR/opcuababy-windows-$arch.exe"
                SIZE=$(du -h "$RELEASE_DIR/opcuababy-windows-$arch.exe" | cut -f1)
                echo "    ✓ 已复制到: $RELEASE_DIR/opcuababy-windows-$arch.exe ($SIZE)"
            fi
        fi
    else
        echo "[✗] Windows ($arch) 构建失败"
    fi
done

echo ""
echo "=========================================="
echo "  构建完成"
echo "=========================================="
echo ""
echo "发布目录内容:"
ls -lh release/windows/

echo ""
echo "✓ 所有 Windows 可执行文件已复制到: release/windows/"
echo "  - opcuababy-windows-amd64.exe"
echo "  - opcuababy-windows-arm64.exe"

