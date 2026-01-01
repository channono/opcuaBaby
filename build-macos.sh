#!/bin/bash
set -e

APP_NAME="opcuaBaby"
APP_ID="com.giantbaby.opcua"
ICON_FILE="assets/icons/app.icns"
RELEASE_DIR="release/macos/packages"

echo "=========================================="
echo "  Building macOS Applications (AMD64 & ARM64)"
echo "=========================================="

mkdir -p "$RELEASE_DIR"

# Build ARM64
echo ""
echo "--------------------------------------------------"
echo "Building for darwin/arm64 (Apple Silicon)..."
echo "--------------------------------------------------"
# Clear any existing build artifacts
rm -rf "${APP_NAME}.app"

CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 fyne package -os darwin -name "$APP_NAME" -appID "$APP_ID" -icon "$ICON_FILE"

if [ -d "${APP_NAME}.app" ]; then
    DEST="${RELEASE_DIR}/${APP_NAME}-arm64.app"
    rm -rf "$DEST"
    mv "${APP_NAME}.app" "$DEST"
    echo "✅ Success: $DEST"
else
    echo "❌ Failed to build arm64"
    exit 1
fi

# Build AMD64
echo ""
echo "--------------------------------------------------"
echo "Building for darwin/amd64 (Intel)..."
echo "--------------------------------------------------"
rm -rf "${APP_NAME}.app"

CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 fyne package -os darwin -name "$APP_NAME" -appID "$APP_ID" -icon "$ICON_FILE"

if [ -d "${APP_NAME}.app" ]; then
    DEST="${RELEASE_DIR}/${APP_NAME}-amd64.app"
    rm -rf "$DEST"
    mv "${APP_NAME}.app" "$DEST"
    echo "✅ Success: $DEST"
else
    echo "❌ Failed to build amd64"
    exit 1
fi

echo ""
echo "=========================================="
echo "  Calling DMG Packager..."
echo "=========================================="
echo ""

# Make sure the packaging script is executable
chmod +x packaging/build-dmg.sh
./packaging/build-dmg.sh
