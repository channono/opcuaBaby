#!/bin/bash
# Debian/Ubuntu .deb package creation script
# Package opcuaBaby as a deb file

set -e

# Configuration variables
APP_NAME="opcuababy"
VERSION="1.0.0"
ARCH="amd64"  # or arm64
MAINTAINER="Big Giantbaby <466719205@qq.com>"
DESCRIPTION="OPC UA Client with Modern Encryption Support"

# Create packaging directory structure
WORK_DIR="deb-package"
PACKAGE_DIR="${WORK_DIR}/${APP_NAME}_${VERSION}_${ARCH}"

echo "=== Creating Debian package directory structure ==="
mkdir -p "${PACKAGE_DIR}/DEBIAN"
mkdir -p "${PACKAGE_DIR}/usr/bin"
mkdir -p "${PACKAGE_DIR}/usr/share/applications"
mkdir -p "${PACKAGE_DIR}/usr/share/icons/hicolor/256x256/apps"
mkdir -p "${PACKAGE_DIR}/usr/share/pixmaps"
mkdir -p "${PACKAGE_DIR}/usr/share/doc/${APP_NAME}"

# 1. Create control file
echo "Creating control file..."
cat > "${PACKAGE_DIR}/DEBIAN/control" << EOF
Package: ${APP_NAME}
Version: ${VERSION}
Section: net
Priority: optional
Architecture: ${ARCH}
Depends: libgl1, libxi6, libxcursor1, libxrandr2, libxinerama1, libx11-6
Maintainer: ${MAINTAINER}
Description: ${DESCRIPTION}
 opcuaBaby is a modern OPC UA client application supporting all
 modern encryption standards including:
 - Basic256Sha256
 - Aes128_Sha256_RsaOaep
 - Aes128_Sha256_RsaPss
 - Aes256_Sha256_RsaOaep
 - Aes256_Sha256_RsaPss
 .
 Features include:
 - Full address space browsing
 - Read/Write operations
 - REST API support
 - WebSocket streaming
 - Certificate management
Homepage: https://github.com/channono/opcuababy
EOF

# 2. Create postinst script (run after installation)
echo "Creating postinst script..."
cat > "${PACKAGE_DIR}/DEBIAN/postinst" << 'EOF'
#!/bin/bash
set -e

# Update desktop database
if command -v update-desktop-database > /dev/null 2>&1; then
    update-desktop-database /usr/share/applications || true
fi

# Update icon cache
if command -v gtk-update-icon-cache > /dev/null 2>&1; then
    gtk-update-icon-cache -f -t /usr/share/icons/hicolor || true
fi

# Update MIME database
if command -v update-mime-database > /dev/null 2>&1; then
    update-mime-database /usr/share/mime || true
fi

echo "opcuaBaby installation completed!"
echo "Run 'opcuababy' to start the application, or find it in the Applications menu."

exit 0
EOF

chmod 755 "${PACKAGE_DIR}/DEBIAN/postinst"

# 3. Create prerm script (run before uninstallation)
echo "Creating prerm script..."
cat > "${PACKAGE_DIR}/DEBIAN/prerm" << 'EOF'
#!/bin/bash
set -e

# Cleanup before uninstallation

exit 0
EOF

chmod 755 "${PACKAGE_DIR}/DEBIAN/prerm"

# 4. Create postrm script (run after uninstallation)
echo "Creating postrm script..."
cat > "${PACKAGE_DIR}/DEBIAN/postrm" << 'EOF'
#!/bin/bash
set -e

if [ "$1" = "purge" ]; then
    # Clean up configuration files
    rm -rf /home/*/.opcuababy 2>/dev/null || true
fi

# 更新桌面数据库
if command -v update-desktop-database > /dev/null 2>&1; then
    update-desktop-database /usr/share/applications || true
fi

# 更新图标缓存
if command -v gtk-update-icon-cache > /dev/null 2>&1; then
    gtk-update-icon-cache -f -t /usr/share/icons/hicolor || true
fi

exit 0
EOF

chmod 755 "${PACKAGE_DIR}/DEBIAN/postrm"

# 5. Copy executable
echo "Copying executable..."
if [ "$ARCH" = "amd64" ]; then
    cp release/linux/opcuababy-linux-amd64 "${PACKAGE_DIR}/usr/bin/opcuababy"
else
    cp release/linux/opcuababy-linux-arm64 "${PACKAGE_DIR}/usr/bin/opcuababy"
fi
chmod 755 "${PACKAGE_DIR}/usr/bin/opcuababy"

# 6. Create .desktop file (with correct Categories)
echo "Creating .desktop file..."
cat > "${PACKAGE_DIR}/usr/share/applications/opcuababy.desktop" << 'EOF'
[Desktop Entry]
Version=1.0
Type=Application
Name=opcuaBaby
GenericName=OPC UA Client
Comment=OPC UA Client with Modern Encryption Support

Exec=opcuababy %U
Icon=opcuababy
Terminal=false
StartupNotify=true
Categories=Network;Development;Monitor;
Keywords=OPC;UA;OPCUA;Industrial;Automation;SCADA;PLC;IIoT;Client;
MimeType=
X-GNOME-UsesNotifications=true
EOF

chmod 644 "${PACKAGE_DIR}/usr/share/applications/opcuababy.desktop"

# 7. Copy icon
echo "Processing icon..."
if [ -f "assets/icons/icon.png" ]; then
    cp assets/icons/icon.png "${PACKAGE_DIR}/usr/share/icons/hicolor/256x256/apps/opcuababy.png"
    cp assets/icons/icon.png "${PACKAGE_DIR}/usr/share/pixmaps/opcuababy.png"
elif [ -f "Icon.png" ]; then
    cp Icon.png "${PACKAGE_DIR}/usr/share/icons/hicolor/256x256/apps/opcuababy.png"
    cp Icon.png "${PACKAGE_DIR}/usr/share/pixmaps/opcuababy.png"
else
    echo "Warning: Icon file not found"
fi

# 8. Create documentation
echo "Creating documentation..."
cat > "${PACKAGE_DIR}/usr/share/doc/${APP_NAME}/README.Debian" << 'EOF'
opcuaBaby for Debian
====================

opcuaBaby is a modern OPC UA client application.

Quick Start
-----------

1. Launch the application:
   $ opcuababy

2. Or launch from the Applications menu:
   Applications → Network → opcuaBaby

Configuration File Locations
----------------------------

- Config: ~/.opcuababy/opcuababy_config.json
- Certificates: ~/.opcuababy/certs/

Help
----

Detailed documentation: https://github.com/channono/opcuababy

Report Issues
-------------

GitHub Issues: https://github.com/channono/opcuababy/issues
EOF

# Create copyright file
cat > "${PACKAGE_DIR}/usr/share/doc/${APP_NAME}/copyright" << 'EOF'
Format: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/
Upstream-Name: opcuaBaby
Source: https://github.com/channono/opcuababy

Files: *
Copyright: 2025 opcuaBaby Authors
License: MIT
 MIT License
 .
 Permission is hereby granted, free of charge, to any person obtaining a copy
 of this software and associated documentation files (the "Software"), to deal
 in the Software without restriction, including without limitation the rights
 to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 copies of the Software, and to permit persons to whom the Software is
 furnished to do so, subject to the following conditions:
 .
 The above copyright notice and this permission notice shall be included in all
 copies or substantial portions of the Software.
 .
 THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 SOFTWARE.
EOF

# Create changelog
cat > "${PACKAGE_DIR}/usr/share/doc/${APP_NAME}/changelog.Debian" << EOF
${APP_NAME} (${VERSION}) unstable; urgency=low

  * Initial release with full modern encryption support
  * Support for all 9 OPC UA security policies
  * ARM64 and AMD64 architecture support

 -- ${MAINTAINER}  $(date -R)
EOF

gzip -9 "${PACKAGE_DIR}/usr/share/doc/${APP_NAME}/changelog.Debian"

# 9. Validate .desktop file
echo "Validating .desktop file..."
if command -v desktop-file-validate > /dev/null 2>&1; then
    desktop-file-validate "${PACKAGE_DIR}/usr/share/applications/opcuababy.desktop" || echo "Warning: .desktop file validation failed"
fi

# 10. Set correct permissions
echo "Setting permissions..."
find "${PACKAGE_DIR}" -type d -exec chmod 755 {} \;
find "${PACKAGE_DIR}" -type f -exec chmod 644 {} \;
chmod 755 "${PACKAGE_DIR}/usr/bin/opcuababy"
chmod 755 "${PACKAGE_DIR}/DEBIAN/postinst"
chmod 755 "${PACKAGE_DIR}/DEBIAN/prerm"
chmod 755 "${PACKAGE_DIR}/DEBIAN/postrm"

# 11. Build deb package
echo "=== Building .deb package ==="
dpkg-deb --build --root-owner-group "${PACKAGE_DIR}"

# 12. Move to release directory
mkdir -p release/debian
mv "${WORK_DIR}/${APP_NAME}_${VERSION}_${ARCH}.deb" "release/debian/"

echo ""
echo "=== ✅ .deb package created successfully! ==="
echo "File location: release/debian/${APP_NAME}_${VERSION}_${ARCH}.deb"
echo ""
echo "Installation command:"
echo "  sudo dpkg -i release/debian/${APP_NAME}_${VERSION}_${ARCH}.deb"
echo "  sudo apt-get install -f  # if dependencies are missing"
echo ""
echo "Uninstallation commands:"
echo "  sudo apt-get remove ${APP_NAME}"
echo "  sudo apt-get purge ${APP_NAME}  # remove config files"
echo ""

# 13. Verify package
echo "=== Verifying package information ==="
dpkg-deb --info "release/debian/${APP_NAME}_${VERSION}_${ARCH}.deb"
echo ""
dpkg-deb --contents "release/debian/${APP_NAME}_${VERSION}_${ARCH}.deb"

# 14. Run lintian check (if available)
if command -v lintian > /dev/null 2>&1; then
    echo ""
    echo "=== Running lintian check ==="
    lintian "release/debian/${APP_NAME}_${VERSION}_${ARCH}.deb" || true
fi

echo ""
echo "🎉 Done!"
