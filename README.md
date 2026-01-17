# Claude Code Chats Delete TUI

**Delete and remove Claude Code chat sessions** with an interactive terminal UI.

Browse, select, and bulk delete chat histories stored in `~/.claude` directory.

Tested with Claude Code v2.1.7.

<img src="./demo.gif" />

## Features

- Browse all chat sessions across projects
- View chat titles, timestamps, versions, and projects
- Copy chat UUID to clipboard
- Multiple selection with visual indicators
- Delete chats with all related files (subagents, file-history, debug, todos, session-env)
- Keyboard-driven navigation (vim-style support)
- Color-coded interface

## Installation

### Quick Install

```bash
curl -sSL https://raw.githubusercontent.com/ataleckij/claude-chats-delete/main/install.sh | sh
```

This will:
- Detect your platform (Linux/macOS, x64/ARM)
- Download the latest release binary
- Verify checksum (SHA256)
- Install to `~/.local/bin/claude-chats`

**Requirements:** curl or wget (usually pre-installed on Linux/macOS)

### Build from Source

See [docs/install-from-source.md](docs/install-from-source.md) for manual build instructions (requires Go 1.21+).

## Usage

```bash
claude-chats
```

### Keyboard Controls

| Key | Action |
|-----|--------|
| `↑/↓` or `k/j` | Navigate up/down |
| `SPACE` | Select/deselect current chat |
| `c` | Copy chat UUID to clipboard |
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
- Subagents directory (`<uuid>/subagents/`)
- File history (`file-history/<uuid>/`)
- Debug logs (`debug/*.txt`)
- Todo files (`todos/*.json`)
- Session environment (`session-env/*/`)

## Configuration

On first run, you'll be prompted to specify your Claude directory. Configuration is saved to `~/.config/claude-chats/config.json`.

## Claude Directory Structure

The tool manages files in `~/.claude/`:

```
~/.claude/
├── projects/<project>/
│   ├── <uuid>.jsonl              # Main chat file
│   └── <uuid>/                   # Chat directory
│       └── subagents/            # Subagent conversations
├── file-history/<uuid>/          # File version history
├── debug/<uuid>.txt              # Debug logs
├── todos/<uuid>-*.json           # Todo lists
└── session-env/<uuid>/           # Session environments
```

## License

MIT
