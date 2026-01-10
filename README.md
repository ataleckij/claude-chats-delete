# Claude Chats Delete

**Delete and remove Claude Code chat sessions** with an interactive terminal UI.

Browse, select, and bulk delete chat histories stored in `~/.claude` directory.

## Features

- Browse all chat sessions across projects
- View chat titles, timestamps, and projects
- Multiple selection with visual indicators
- Delete chats with all related files (debug logs, todos, session-env)
- Keyboard-driven navigation (vim-style support)
- Color-coded interface

## Installation

### From Source

```bash
# Clone or download the repository
git clone https://github.com/ataleckij/claude-chats-delete.git
cd claude-chats-delete

# Install dependencies
go mod download

# Build
go build -o claude-chats

# Install to /usr/local/bin (optional)
sudo mv claude-chats /usr/local/bin/

# Or install to ~/.local/bin (no sudo)
mkdir -p ~/.local/bin
mv claude-chats ~/.local/bin/
# Make sure ~/.local/bin is in your PATH
```

### Quick Install (one-liner)

```bash
# Build and install to ~/.local/bin
go build -o ~/.local/bin/claude-chats
```

## Usage

```bash
claude-chats
```

### Keyboard Controls

| Key | Action |
|-----|--------|
| `↑/↓` or `k/j` | Navigate up/down |
| `SPACE` | Select/deselect current chat |
| `d` | Delete selected chats (with confirmation) |
| `r` | Refresh chat list |
| `q` or `Ctrl+C` | Quit |

### Deletion Confirmation

When you press `d`:
1. Confirmation dialog appears
2. Press `ENTER` to confirm deletion
3. Press `ESC` or `n` to cancel

All related files are deleted:
- Main chat file (`.jsonl`)
- Debug logs (`debug/*.txt`)
- Todo files (`todos/*.json`)
- Session environment (`session-env/*/`)

## Development

### Requirements

- Go 1.21+
- Dependencies:
  - `github.com/charmbracelet/bubbletea` - TUI framework
  - `github.com/charmbracelet/lipgloss` - Terminal styling

### Building

```bash
# Download dependencies
go mod download

# Build
go build -o claude-chats

# Run
./claude-chats
```

### Cross-compilation

```bash
# macOS (ARM)
GOOS=darwin GOARCH=arm64 go build -o claude-chats-darwin-arm64

# macOS (Intel)
GOOS=darwin GOARCH=amd64 go build -o claude-chats-darwin-amd64

# Linux
GOOS=linux GOARCH=amd64 go build -o claude-chats-linux-amd64
```

## Structure

The tool manages files in `~/.claude/`:

```
~/.claude/
├── projects/
│   └── <project-name>/
│       └── <uuid>.jsonl          # Chat sessions
├── debug/
│   └── <uuid>.txt                # Debug logs
├── todos/
│   └── <uuid>*.json              # Todo lists
└── session-env/
    └── <uuid>/                   # Session environments
```

## Comparison with Python Version

| Feature | Python | Go |
|---------|--------|-----|
| Dependencies | curses (stdlib) | bubbletea (external) |
| Binary size | N/A (interpreted) | ~5-10 MB |
| Startup time | ~100ms | ~10ms |
| Installation | Copy script | Single binary |
| Cross-platform | Requires Python | Compile once, run anywhere |

## License

MIT

## Contributing

Pull requests welcome!
