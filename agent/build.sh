#!/bin/bash
# Enterprise RAT Agent - Cross-Platform Build Script
# This script builds the agent for Windows and Linux platforms

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUILD_DIR="${SCRIPT_DIR}/dist"
PLATFORMS=("linux/amd64" "linux/arm64" "windows/amd64")

echo "=============================================="
echo " Enterprise RAT Agent - Cross-Platform Build"
echo "=============================================="
echo ""

# Clean build directory
echo "Cleaning build directory..."
rm -rf "${BUILD_DIR}"
mkdir -p "${BUILD_DIR}"

# Check if buildx is available
if ! docker buildx version >/dev/null 2>&1; then
    echo "Docker buildx not available. Using standard Go builds..."
    USE_GO_BUILD=true
else
    USE_GO_BUILD=false
    echo "Creating buildx builder..."
    docker buildx create --name rat-builder --use || true
    docker buildx inspect rat-builder --bootstrap || true
fi

if [ "$USE_GO_BUILD" = true ]; then
    echo ""
    echo "Building with standard Go..."

    echo "Building Linux AMD64..."
    GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-w -s" -o "${BUILD_DIR}/rat-agent-linux-amd64" ./cmd/agent

    echo "Building Linux ARM64..."
    GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-w -s" -o "${BUILD_DIR}/rat-agent-linux-arm64" ./cmd/agent

    echo "Building Windows AMD64..."
    GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-w -s" -o "${BUILD_DIR}/rat-agent-windows-amd64.exe" ./cmd/agent

else
    echo ""
    echo "Building with Docker buildx for multiple platforms..."

    # Build all platforms
    docker buildx build \
        --platform linux/amd64,linux/arm64,windows/amd64 \
        --target linux-builder \
        --output type=local,dest="${BUILD_DIR}" \
        --progress=plain \
        "${SCRIPT_DIR}"

    # Rename files
    cd "${BUILD_DIR}"
    mv agent-linux-amd64 rat-agent-linux-amd64 2>/dev/null || true
    mv agent-linux-arm64 rat-agent-linux-arm64 2>/dev/null || true
    mv agent-windows-amd64.exe rat-agent-windows-amd64.exe 2>/dev/null || true
fi

echo ""
echo "=============================================="
echo " Build Complete! "
echo "=============================================="
echo ""
echo "Artifacts in: ${BUILD_DIR}"
echo ""
ls -la "${BUILD_DIR}"
echo ""

# Create checksums
echo "Creating checksums..."
cd "${BUILD_DIR}"
for f in *; do
    if [ -f "$f" ]; then
        echo "$f: $(sha256sum "$f" | cut -d' ' -f1)" >> checksums.txt
    fi
done

echo "Checksums:"
cat checksums.txt
echo ""

# Create deployment packages
echo "Creating deployment packages..."

if command -v zip >/dev/null 2>&1; then
    cd "${BUILD_DIR}"
    
    if [ -f rat-agent-linux-amd64 ]; then
        zip -j rat-agent-linux-amd64.zip rat-agent-linux-amd64
    fi
    
    if [ -f rat-agent-linux-arm64 ]; then
        zip -j rat-agent-linux-arm64.zip rat-agent-linux-arm64
    fi
    
    if [ -f rat-agent-windows-amd64.exe ]; then
        mv rat-agent-windows-amd64.exe rat-agent-windows-amd64.exe
        zip -j rat-agent-windows-amd64.zip rat-agent-windows-amd64.exe
    fi
fi

echo ""
echo "=============================================="
echo " Build Summary "
echo "=============================================="
ls -la "${BUILD_DIR}"
echo ""
echo "To deploy agents:"
echo ""
echo "Linux AMD64:"
echo "  scp rat-agent-linux-amd64 user@target:/opt/rat/agent"
echo "  chmod +x /opt/rat/agent"
echo "  /opt/rat/agent"
echo ""
echo "Linux ARM64:"
echo "  scp rat-agent-linux-arm64 user@target:/opt/rat/agent"
echo "  chmod +x /opt/rat/agent"
echo "  /opt/rat/agent"
echo ""
echo "Windows:"
echo "  Copy rat-agent-windows-amd64.exe to target"
echo "  Start-Process -FilePath ./rat-agent-windows-amd64.exe"
echo ""
