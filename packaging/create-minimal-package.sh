#!/bin/bash
# Package minimal files needed for deb creation
# This creates a lightweight package to transfer to Linux machine

set -e

VERSION="1.0.0"
PACKAGE_NAME="opcuababy-deb-builder-${VERSION}"
OUTPUT_DIR="release/deb-builder"

echo "=== Creating minimal package for deb building ==="
echo ""

# Clean and create output directory
rm -rf "${OUTPUT_DIR}"
mkdir -p "${OUTPUT_DIR}/${PACKAGE_NAME}"

# Create directory structure
mkdir -p "${OUTPUT_DIR}/${PACKAGE_NAME}/packaging"
mkdir -p "${OUTPUT_DIR}/${PACKAGE_NAME}/release/linux"
mkdir -p "${OUTPUT_DIR}/${PACKAGE_NAME}/assets/icons"

# Copy build script
echo "Copying build script..."
cp packaging/build-deb.sh "${OUTPUT_DIR}/${PACKAGE_NAME}/packaging/"

# Copy executables
echo "Copying executables..."
if [ -f "release/linux/opcuababy-linux-amd64" ]; then
    cp release/linux/opcuababy-linux-amd64 "${OUTPUT_DIR}/${PACKAGE_NAME}/release/linux/"
    echo "  ✓ AMD64 executable"
fi

if [ -f "release/linux/opcuababy-linux-arm64" ]; then
    cp release/linux/opcuababy-linux-arm64 "${OUTPUT_DIR}/${PACKAGE_NAME}/release/linux/"
    echo "  ✓ ARM64 executable"
fi

# Copy icon
echo "Copying icon..."
cp assets/icons/icon.png "${OUTPUT_DIR}/${PACKAGE_NAME}/assets/icons/"

# Create README
cat > "${OUTPUT_DIR}/${PACKAGE_NAME}/README.txt" << 'EOF'
opcuaBaby Debian Package Builder
=================================

This package contains everything needed to build .deb packages.

Contents:
---------
packaging/build-deb.sh          - Build script
release/linux/opcuababy-linux-* - Executables
assets/icons/icon.png           - Application icon

Quick Start:
------------

1. Extract this package on your Linux machine:
   tar -xzf opcuababy-deb-builder-1.0.0.tar.gz
   cd opcuababy-deb-builder-1.0.0

2. Install required tools:
   sudo apt-get install dpkg-dev desktop-file-utils lintian

3. Build AMD64 package:
   chmod +x packaging/build-deb.sh
   ./packaging/build-deb.sh

4. Build ARM64 package:
   ARCH=arm64 ./packaging/build-deb.sh

5. Find your .deb files in:
   release/debian/

Installation on Debian/Ubuntu:
-------------------------------
   sudo dpkg -i release/debian/opcuababy_1.0.0_amd64.deb
   sudo apt-get install -f  # if dependencies missing

More info:
----------
   https://github.com/channono/opcuababy
EOF

# Create simple build instructions
cat > "${OUTPUT_DIR}/${PACKAGE_NAME}/BUILD.sh" << 'EOF'
#!/bin/bash
# Quick build script

echo "=== opcuaBaby Debian Package Builder ==="
echo ""
echo "Installing required tools..."
sudo apt-get update
sudo apt-get install -y dpkg-dev desktop-file-utils lintian

echo ""
echo "Making script executable..."
chmod +x packaging/build-deb.sh

echo ""
echo "Building package..."
./packaging/build-deb.sh

echo ""
echo "Done! Your .deb file is in: release/debian/"
EOF

chmod +x "${OUTPUT_DIR}/${PACKAGE_NAME}/BUILD.sh"

# Calculate size
TOTAL_SIZE=$(du -sh "${OUTPUT_DIR}/${PACKAGE_NAME}" | cut -f1)

echo ""
echo "=== Package contents ==="
find "${OUTPUT_DIR}/${PACKAGE_NAME}" -type f -exec ls -lh {} \; | awk '{print $5 "\t" $9}'

echo ""
echo "=== Creating tarball ==="
cd "${OUTPUT_DIR}"
tar -czf "${PACKAGE_NAME}.tar.gz" "${PACKAGE_NAME}"
cd - > /dev/null

TARBALL_SIZE=$(ls -lh "${OUTPUT_DIR}/${PACKAGE_NAME}.tar.gz" | awk '{print $5}')

echo ""
echo "=== ✅ Success! ==="
echo ""
echo "Package created: ${OUTPUT_DIR}/${PACKAGE_NAME}.tar.gz"
echo "Package size:    ${TARBALL_SIZE}"
echo "Uncompressed:    ${TOTAL_SIZE}"
echo ""
echo "Transfer to Linux machine:"
echo "  scp ${OUTPUT_DIR}/${PACKAGE_NAME}.tar.gz user@linux-machine:~/"
echo ""
echo "On Linux machine:"
echo "  tar -xzf ${PACKAGE_NAME}.tar.gz"
echo "  cd ${PACKAGE_NAME}"
echo "  ./BUILD.sh"
echo ""
