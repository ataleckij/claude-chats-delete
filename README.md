# Claude Code Chats Delete

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

**Requirements:** Go 1.21+ and git must be installed.

### Quick Install

```bash
curl -sSL https://raw.githubusercontent.com/ataleckij/claude-chats-delete/main/install.sh | sh
```

This will:
- Clone the repository
- Build the binary
- Install to `~/.local/bin/claude-chats`

### Manual Installation

See [docs/install-from-source.md](docs/install-from-source.md) for manual build instructions.

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

## Configuration

On first run, you'll be prompted to specify your Claude directory. Configuration is saved to `~/.config/claude-chats/config.json`.

## Claude Directory Structure

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

## License

MIT
