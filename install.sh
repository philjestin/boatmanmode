#!/bin/bash
# BoatmanMode installation script
# Downloads and installs the latest release binary

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Default installation directory
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# GitHub repository
REPO="philjestin/boatmanmode"

# Detect OS and architecture
detect_platform() {
    local os=$(uname -s | tr '[:upper:]' '[:lower:]')
    local arch=$(uname -m)

    case "$os" in
        linux*)
            OS="Linux"
            ;;
        darwin*)
            OS="Darwin"
            ;;
        *)
            echo -e "${RED}Unsupported OS: $os${NC}"
            exit 1
            ;;
    esac

    case "$arch" in
        x86_64|amd64)
            ARCH="x86_64"
            ;;
        arm64|aarch64)
            ARCH="arm64"
            ;;
        *)
            echo -e "${RED}Unsupported architecture: $arch${NC}"
            exit 1
            ;;
    esac

    echo -e "${GREEN}Detected platform: $OS $ARCH${NC}"
}

# Get latest release version
get_latest_version() {
    echo -e "${YELLOW}Fetching latest release...${NC}"

    LATEST_VERSION=$(curl -s "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

    if [ -z "$LATEST_VERSION" ]; then
        echo -e "${RED}Failed to fetch latest version${NC}"
        exit 1
    fi

    echo -e "${GREEN}Latest version: $LATEST_VERSION${NC}"
}

# Download and install
install_binary() {
    local version="${1:-$LATEST_VERSION}"
    local filename="boatmanmode_${version}_${OS}_${ARCH}.tar.gz"
    local download_url="https://github.com/$REPO/releases/download/${version}/${filename}"

    echo -e "${YELLOW}Downloading $filename...${NC}"

    local tmp_dir=$(mktemp -d)
    cd "$tmp_dir"

    if ! curl -sL "$download_url" -o "$filename"; then
        echo -e "${RED}Failed to download binary${NC}"
        echo -e "${YELLOW}URL: $download_url${NC}"
        exit 1
    fi

    echo -e "${YELLOW}Extracting...${NC}"
    tar -xzf "$filename"

    if [ ! -f "boatman" ]; then
        echo -e "${RED}Binary not found in archive${NC}"
        exit 1
    fi

    echo -e "${YELLOW}Installing to $INSTALL_DIR...${NC}"

    # Check if we need sudo
    if [ -w "$INSTALL_DIR" ]; then
        mv boatman "$INSTALL_DIR/boatman"
    else
        echo -e "${YELLOW}Installing to $INSTALL_DIR requires sudo${NC}"
        sudo mv boatman "$INSTALL_DIR/boatman"
    fi

    chmod +x "$INSTALL_DIR/boatman"

    # Cleanup
    cd - > /dev/null
    rm -rf "$tmp_dir"

    echo -e "${GREEN}Successfully installed boatman to $INSTALL_DIR/boatman${NC}"
}

# Verify installation
verify_installation() {
    echo -e "${YELLOW}Verifying installation...${NC}"

    if command -v boatman &> /dev/null; then
        local installed_version=$(boatman version | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' || echo "unknown")
        echo -e "${GREEN}✓ boatman is installed: $installed_version${NC}"
        return 0
    else
        echo -e "${RED}✗ boatman not found in PATH${NC}"
        echo -e "${YELLOW}You may need to add $INSTALL_DIR to your PATH${NC}"
        return 1
    fi
}

# Main installation flow
main() {
    echo ""
    echo "BoatmanMode Installation Script"
    echo "================================"
    echo ""

    # Parse arguments
    VERSION=""
    while [[ $# -gt 0 ]]; do
        case $1 in
            --version)
                VERSION="$2"
                shift 2
                ;;
            --dir)
                INSTALL_DIR="$2"
                shift 2
                ;;
            -h|--help)
                echo "Usage: $0 [OPTIONS]"
                echo ""
                echo "Options:"
                echo "  --version VERSION    Install specific version (e.g., v1.0.0)"
                echo "  --dir DIR           Installation directory (default: /usr/local/bin)"
                echo "  -h, --help          Show this help message"
                echo ""
                exit 0
                ;;
            *)
                echo -e "${RED}Unknown option: $1${NC}"
                echo "Use --help for usage information"
                exit 1
                ;;
        esac
    done

    detect_platform

    if [ -z "$VERSION" ]; then
        get_latest_version
        VERSION="$LATEST_VERSION"
    else
        echo -e "${GREEN}Installing version: $VERSION${NC}"
    fi

    install_binary "$VERSION"
    verify_installation

    echo ""
    echo -e "${GREEN}Installation complete! 🚣${NC}"
    echo ""
    echo "Get started with:"
    echo "  boatman --help"
    echo "  boatman work --help"
    echo ""
}

main "$@"
