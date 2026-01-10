# Install from Source

## Requirements

- Go 1.21 or later
- git

## Steps

```bash
# Clone the repository
git clone https://github.com/ataleckij/claude-chats-delete.git
cd claude-chats-delete

# Build
go build -o claude-chats

# Install to ~/.local/bin
mkdir -p ~/.local/bin
mv claude-chats ~/.local/bin/

# Make sure ~/.local/bin is in your PATH
export PATH="$HOME/.local/bin:$PATH"
```

## Cross-compilation

Build for different platforms:

```bash
# macOS ARM (M1/M2/M3)
GOOS=darwin GOARCH=arm64 go build -o claude-chats-darwin-arm64

# macOS Intel
GOOS=darwin GOARCH=amd64 go build -o claude-chats-darwin-amd64

# Linux x86_64
GOOS=linux GOARCH=amd64 go build -o claude-chats-linux-amd64
```
