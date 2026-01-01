#!/bin/bash
# macOS DMG package creation script
# Creates installer DMG for opcuaBaby

set -e

# Configuration
APP_NAME="opcuaBaby"
VERSION="1.0.0"
DMG_NAME="opcuaBaby-${VERSION}"
DMG_DIR="release/macos/dmg"

# Source app bundles
ARM64_APP="release/macos/packages/opcuaBaby-arm64.app"
AMD64_APP="release/macos/packages/opcuaBaby-amd64.app"

echo "=== Creating macOS DMG Installers ==="
echo ""

# Create DMG directory
mkdir -p "${DMG_DIR}"

# Function to create DMG
create_dmg_package() {
    local APP_PATH=$1
    local ARCH=$2
    local OUTPUT_NAME="${DMG_NAME}-${ARCH}.dmg"
    local TEMP_DIR="${DMG_DIR}/temp-${ARCH}"
    
    echo "Creating DMG for ${ARCH}..."
    
    # Remove and recreate temporary directory
    rm -rf "${TEMP_DIR}"
    mkdir -p "${TEMP_DIR}"
    
    # Copy app to temp directory  
    cp -R "${APP_PATH}" "${TEMP_DIR}/${APP_NAME}.app"
    
    # Remove any existing DMG
    rm -f "${DMG_DIR}/${OUTPUT_NAME}"
    
    # Create DMG using create-dmg
    create-dmg \
        --volname "${APP_NAME}" \
        --volicon "assets/icons/app.icns" \
        --window-pos 200 120 \
        --window-size 800 400 \
        --icon-size 100 \
        --icon "${APP_NAME}.app" 200 190 \
        --hide-extension "${APP_NAME}.app" \
        --app-drop-link 600 185 \
        "${DMG_DIR}/${OUTPUT_NAME}" \
        "${TEMP_DIR}"
    
    # Clean up temp directory
    rm -rf "${TEMP_DIR}"
    
    echo "✅ Created: ${DMG_DIR}/${OUTPUT_NAME}"
    echo ""
}

# Check if apps exist
if [ ! -d "$ARM64_APP" ]; then
    echo "❌ Error: ARM64 app not found at ${ARM64_APP}"
    exit 1
fi

if [ ! -d "$AMD64_APP" ]; then
    echo "❌ Error: AMD64 app not found at ${AMD64_APP}"
    exit 1
fi

# Create DMG for ARM64
if [ -d "$ARM64_APP" ]; then
    create_dmg_package "$ARM64_APP" "arm64"
fi

# Create DMG for AMD64
if [ -d "$AMD64_APP" ]; then
    create_dmg_package "$AMD64_APP" "amd64"
fi

echo "=== Summary ==="
echo "Created DMG files:"
ls -lh "${DMG_DIR}"/*.dmg 2>/dev/null || echo "No DMG files created"
echo ""
echo "📦 DMG packages ready for distribution!"
echo ""
echo "Installation instructions:"
echo "1. Double-click the .dmg file"
echo "2. Drag opcuaBaby to Applications folder"
echo "3. Eject the DMG"
echo "4. Open opcuaBaby from Applications"
