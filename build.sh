#!/bin/bash

# Build script for bbctl
# Builds bbctl for multiple platforms and creates distribution archives

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
PROJECT_NAME="bbctl"
VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo 'dev')}"
BUILD_DIR="dist"
TEMP_DIR=$(mktemp -d)

# Cleanup function
cleanup() {
    echo -e "${YELLOW}Cleaning up temporary files...${NC}"
    rm -rf "$TEMP_DIR"
}
trap cleanup EXIT

# Print header 
echo -e "${BLUE}================================${NC}"
echo -e "${BLUE}  bbctl Build Script${NC}"
echo -e "${BLUE}================================${NC}"
echo -e "${BLUE}Version: $VERSION${NC}"
echo -e "${BLUE}Build directory: $BUILD_DIR${NC}"
echo

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo -e "${RED}Error: Go is not installed or not in PATH${NC}"
    exit 1
fi

# Check Go version
GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
echo -e "${GREEN}Go version: $GO_VERSION${NC}"

# Create build directory
echo -e "${YELLOW}Creating build directory...${NC}"
mkdir -p "$BUILD_DIR"

# Build flags - remove 'v' prefix if it exists
VERSION_CLEAN=$(echo "$VERSION" | sed 's/^v//')
LDFLAGS="-X 'github.com/vinisman/bbctl/cmd.Version=$VERSION_CLEAN' -X 'github.com/vinisman/bbctl/cmd.Commit=$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown')'"

# Platforms to build for
PLATFORMS=(
    "linux/amd64"
    "windows/amd64"
)

# Build function
build_for_platform() {
    local platform=$1
    local os=$(echo $platform | cut -d'/' -f1)
    local arch=$(echo $platform | cut -d'/' -f2)
    
    echo -e "${YELLOW}Building for $os/$arch...${NC}"
    
    # Set output filename
    local output_name="$PROJECT_NAME"
    if [[ "$os" == "windows" ]]; then
        output_name="${output_name}.exe"
    fi
    
    # Set environment variables for cross-compilation
    export GOOS="$os"
    export GOARCH="$arch"
    export CGO_ENABLED=0
    
    # Build the binary
    local output_path="$TEMP_DIR/${os}-${arch}/$output_name"
    mkdir -p "$(dirname "$output_path")"
    
    go build -ldflags "$LDFLAGS" -o "$output_path" .
    
    if [[ $? -eq 0 ]]; then
        echo -e "${GREEN}✓ Built successfully: $output_path${NC}"
    else
        echo -e "${RED}✗ Build failed for $os/$arch${NC}"
        return 1
    fi
}

# Package function
package_for_platform() {
    local platform=$1
    local os=$(echo $platform | cut -d'/' -f1)
    local arch=$(echo $platform | cut -d'/' -f2)
    
    echo -e "${YELLOW}Packaging for $os/$arch...${NC}"
    
    local package_name="${PROJECT_NAME}-${os}-${arch}-v${VERSION_CLEAN}"
    local package_dir="$TEMP_DIR/$package_name"
    local output_name="$PROJECT_NAME"
    
    if [[ "$os" == "windows" ]]; then
        output_name="${output_name}.exe"
    fi
    
    # Create archive directly from binary
    cd "$TEMP_DIR/${os}-${arch}"
    
    if [[ "$os" == "windows" ]]; then
        # Create ZIP archive for Windows
        local archive_name="${package_name}.zip"
        zip "$archive_name" "$output_name" > /dev/null
        echo -e "${GREEN}✓ Created ZIP archive: $archive_name${NC}"
    else
        # Create TAR.GZ archive for Linux
        local archive_name="${package_name}.tar.gz"
        tar --exclude='._*' --exclude='.DS_Store' -czf "$archive_name" "$output_name" 2>/dev/null
        echo -e "${GREEN}✓ Created TAR.GZ archive: $archive_name${NC}"
    fi
    
    # Move archive to build directory
    mv "$archive_name" "$OLDPWD/$BUILD_DIR/"
    
    cd "$OLDPWD"
}

# Main build process
echo -e "${YELLOW}Starting build process...${NC}"

# Clean previous builds
echo -e "${YELLOW}Cleaning previous builds...${NC}"
rm -rf "$BUILD_DIR"/*

# Build for each platform
for platform in "${PLATFORMS[@]}"; do
    if build_for_platform "$platform"; then
        package_for_platform "$platform"
    else
        echo -e "${RED}Failed to build for $platform${NC}"
        exit 1
    fi
done

# Create checksums
echo -e "${YELLOW}Creating checksums...${NC}"
cd "$BUILD_DIR"
for file in *.tar.gz *.zip; do
    if [[ -f "$file" ]]; then
        sha256sum "$file" > "${file}.sha256"
        echo -e "${GREEN}✓ Created checksum: ${file}.sha256${NC}"
    fi
done
cd ..

# Print summary
echo
echo -e "${BLUE}================================${NC}"
echo -e "${BLUE}  Build Summary${NC}"
echo -e "${BLUE}================================${NC}"
echo -e "${GREEN}Version: $VERSION${NC}"
echo -e "${GREEN}Build directory: $BUILD_DIR${NC}"
echo
echo -e "${YELLOW}Created files:${NC}"
ls -la "$BUILD_DIR"/
echo
echo -e "${GREEN}Build completed successfully!${NC}"

# Optional: Show file sizes
echo
echo -e "${YELLOW}File sizes:${NC}"
for file in "$BUILD_DIR"/*.tar.gz "$BUILD_DIR"/*.zip; do
    if [[ -f "$file" ]]; then
        size=$(du -h "$file" | cut -f1)
        echo -e "${BLUE}$(basename "$file"): $size${NC}"
    fi
done
