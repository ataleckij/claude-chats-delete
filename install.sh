#!/bin/bash
set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

REPO_URL="https://github.com/ataleckij/claude-chats-delete"
BINARY_NAME="claude-chats"
INSTALL_DIR="$HOME/.local/bin"

echo -e "${BLUE}Claude Code Chats Delete - Installer${NC}"
echo ""

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo -e "${RED}Error: Go is not installed!${NC}"
    echo ""
    echo "This installer requires Go 1.21 or later to build from source."
    echo ""
    echo "Please install Go first:"
    echo ""
    echo -e "${YELLOW}Linux (using package manager):${NC}"
    echo "  Ubuntu/Debian:  sudo apt install golang-go"
    echo "  Fedora/RHEL:    sudo dnf install golang"
    echo "  Arch Linux:     sudo pacman -S go"
    echo ""
    echo -e "${YELLOW}macOS:${NC}"
    echo "  brew install go"
    echo ""
    echo -e "${YELLOW}Or download from:${NC}"
    echo "  https://go.dev/dl/"
    echo ""
    exit 1
fi

# Check Go version
GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
echo -e "${GREEN}✓${NC} Found Go ${GO_VERSION}"

# Create temporary directory
TMP_DIR=$(mktemp -d)
cd "$TMP_DIR"

echo -e "${BLUE}Cloning repository...${NC}"
git clone --depth 1 "$REPO_URL" . 2>/dev/null || {
    echo -e "${RED}Error: Failed to clone repository${NC}"
    echo "Make sure you have git installed and internet connection"
    rm -rf "$TMP_DIR"
    exit 1
}

echo -e "${GREEN}✓${NC} Repository cloned"

echo -e "${BLUE}Building binary...${NC}"
go build -o "$BINARY_NAME" . || {
    echo -e "${RED}Error: Build failed${NC}"
    rm -rf "$TMP_DIR"
    exit 1
}

echo -e "${GREEN}✓${NC} Build successful"

# Create install directory if it doesn't exist
mkdir -p "$INSTALL_DIR"

# Install binary
echo -e "${BLUE}Installing to ${INSTALL_DIR}...${NC}"
mv "$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
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
