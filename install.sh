#!/bin/bash
set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

REPO_OWNER="ataleckij"
REPO_NAME="claude-chats-delete"
BINARY_NAME="claude-chats"
INSTALL_DIR="$HOME/.local/bin"

echo -e "${BLUE}Claude Code Chats Delete - Installer${NC}"
echo ""

# Detect OS
OS="$(uname -s)"
case "$OS" in
    Linux*)     OS_TYPE="linux";;
    Darwin*)    OS_TYPE="darwin";;
    *)
        echo -e "${RED}Error: Unsupported operating system: $OS${NC}"
        echo "Supported: Linux, macOS"
        exit 1
        ;;
esac

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64)     ARCH_TYPE="amd64";;
    aarch64)    ARCH_TYPE="arm64";;
    arm64)      ARCH_TYPE="arm64";;
    *)
        echo -e "${RED}Error: Unsupported architecture: $ARCH${NC}"
        echo "Supported: x86_64, arm64, aarch64"
        exit 1
        ;;
esac

PLATFORM="${OS_TYPE}-${ARCH_TYPE}"
echo -e "${GREEN}✓${NC} Detected platform: ${PLATFORM}"

# Check for curl or wget
if command -v curl &> /dev/null; then
    DOWNLOADER="curl"
elif command -v wget &> /dev/null; then
    DOWNLOADER="wget"
else
    echo -e "${RED}Error: Neither curl nor wget is installed${NC}"
    echo "Please install one of them and try again"
    exit 1
fi

# Function to download file
download() {
    local url=$1
    local output=$2

    if [ "$DOWNLOADER" = "curl" ]; then
        curl -fsSL "$url" -o "$output"
    else
        wget -q -O "$output" "$url"
    fi
}

# Get latest release version
echo -e "${BLUE}Fetching latest release...${NC}"
LATEST_RELEASE=$(download "https://api.github.com/repos/$REPO_OWNER/$REPO_NAME/releases/latest" - | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' || echo "")

if [ -z "$LATEST_RELEASE" ]; then
    echo -e "${RED}Error: Could not fetch latest release${NC}"
    echo "Please check your internet connection or try again later"
    exit 1
fi

echo -e "${GREEN}✓${NC} Latest version: ${LATEST_RELEASE}"

# Construct download URLs
BINARY_FILENAME="claude-chats-${PLATFORM}"
DOWNLOAD_URL="https://github.com/$REPO_OWNER/$REPO_NAME/releases/download/$LATEST_RELEASE/$BINARY_FILENAME"
CHECKSUMS_URL="https://github.com/$REPO_OWNER/$REPO_NAME/releases/download/$LATEST_RELEASE/checksums.txt"

# Create temporary directory
TMP_DIR=$(mktemp -d)
cd "$TMP_DIR"

# Download binary
echo -e "${BLUE}Downloading ${BINARY_FILENAME}...${NC}"
download "$DOWNLOAD_URL" "$BINARY_FILENAME" || {
    echo -e "${RED}Error: Failed to download binary${NC}"
    echo "URL: $DOWNLOAD_URL"
    rm -rf "$TMP_DIR"
    exit 1
}

echo -e "${GREEN}✓${NC} Downloaded"

# Download and verify checksum
echo -e "${BLUE}Verifying checksum...${NC}"
download "$CHECKSUMS_URL" "checksums.txt" || {
    echo -e "${YELLOW}Warning: Could not download checksums file${NC}"
    echo "Skipping checksum verification"
}

if [ -f "checksums.txt" ]; then
    # Extract expected checksum for this binary
    EXPECTED_CHECKSUM=$(grep "$BINARY_FILENAME" checksums.txt | awk '{print $1}')

    if [ -z "$EXPECTED_CHECKSUM" ]; then
        echo -e "${YELLOW}Warning: Checksum not found for $BINARY_FILENAME${NC}"
    else
        # Calculate actual checksum
        if command -v sha256sum &> /dev/null; then
            ACTUAL_CHECKSUM=$(sha256sum "$BINARY_FILENAME" | awk '{print $1}')
        elif command -v shasum &> /dev/null; then
            ACTUAL_CHECKSUM=$(shasum -a 256 "$BINARY_FILENAME" | awk '{print $1}')
        else
            echo -e "${YELLOW}Warning: No checksum tool found (sha256sum/shasum)${NC}"
            ACTUAL_CHECKSUM=""
        fi

        if [ -n "$ACTUAL_CHECKSUM" ]; then
            if [ "$EXPECTED_CHECKSUM" = "$ACTUAL_CHECKSUM" ]; then
                echo -e "${GREEN}✓${NC} Checksum verified"
            else
                echo -e "${RED}Error: Checksum mismatch!${NC}"
                echo "Expected: $EXPECTED_CHECKSUM"
                echo "Got:      $ACTUAL_CHECKSUM"
                rm -rf "$TMP_DIR"
                exit 1
            fi
        fi
    fi
fi

# Create install directory if it doesn't exist
mkdir -p "$INSTALL_DIR"

# Check if binary already exists
if [ -f "$INSTALL_DIR/$BINARY_NAME" ]; then
    echo ""
    echo -e "${YELLOW}Found existing installation at ${INSTALL_DIR}/${BINARY_NAME}${NC}"
    read -p "Overwrite? [y/N] " -n 1 -r </dev/tty
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Installation cancelled"
        cd ~
        rm -rf "$TMP_DIR"
        exit 0
    fi
fi

# Install binary
echo -e "${BLUE}Installing to ${INSTALL_DIR}...${NC}"
mv "$BINARY_FILENAME" "$INSTALL_DIR/$BINARY_NAME"
chmod +x "$INSTALL_DIR/$BINARY_NAME"

echo -e "${GREEN}✓${NC} Installed to ${INSTALL_DIR}/${BINARY_NAME}"

# Cleanup
cd ~
rm -rf "$TMP_DIR"

echo ""
echo -e "${GREEN}Installation complete!${NC}"
echo ""

# Check if install directory is in PATH
if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
    echo -e "${YELLOW}Warning: $INSTALL_DIR is not in your PATH${NC}"
    echo ""
    echo "Add this line to your shell config (~/.bashrc, ~/.zshrc, etc.):"
    echo ""
    echo -e "  ${BLUE}export PATH=\"\$HOME/.local/bin:\$PATH\"${NC}"
    echo ""
    echo "Then restart your shell or run:"
    echo ""
    echo -e "  ${BLUE}source ~/.bashrc${NC}  # or ~/.zshrc"
    echo ""
else
    echo "Run the tool with:"
    echo ""
    echo -e "  ${BLUE}$BINARY_NAME${NC}"
    echo ""
fi
