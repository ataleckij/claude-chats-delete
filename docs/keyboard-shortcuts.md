# Keyboard Shortcuts Guide

Complete reference for all keyboard shortcuts in Claude Code Chat Manager.

## Navigation Commands

### Basic Navigation
- **`↑` (Up Arrow)** or **`k`** - Move cursor up one item
- **`↓` (Down Arrow)** or **`j`** - Move cursor down one item

### Fast Scrolling (NEW)
For users with large chat histories (hundreds of sessions), these shortcuts provide faster navigation:

- **`PgUp` (Page Up)** or **`Ctrl+B`** - Scroll up by one page (visible height)
- **`PgDn` (Page Down)** or **`Ctrl+F`** - Scroll down by one page (visible height)
- **`Ctrl+U`** - Scroll up by half page (vim-style)
- **`Ctrl+D`** - Scroll down by half page (vim-style)

### Jump Navigation
- **`Home`** or **`g`** - Jump to first chat (top of list)
- **`End`** or **`G`** - Jump to last chat (bottom of list)

## Selection Commands

- **`SPACE`** - Toggle selection for current chat
- **`a`** - Toggle select/deselect all chats

## Action Commands

- **`c`** - Copy current chat UUID to clipboard
- **`d`** - Delete selected chats (shows confirmation dialog)
- **`r`** - Refresh chat list (reload from disk)
- **`q`** or **`Ctrl+C`** - Quit application

## Confirmation Dialog

When deleting (after pressing `d`):
- **`ENTER`** - Confirm deletion
- **`ESC`** or **`n`** - Cancel deletion

## Tips for Power Users

### Working with Large Chat Histories

If you have hundreds of chat sessions:

1. **Quick Jump to Recent/Old Chats**
   - Press `Home` or `g` to jump to newest chats
   - Press `End` or `G` to jump to oldest chats

2. **Fast Scanning**
   - Use `PgDn`/`PgUp` to quickly scan through pages
   - Use `Ctrl+D`/`Ctrl+U` for more precise half-page movements

3. **Bulk Selection Workflow**
   - Press `Home` to start at top
   - Use `PgDn` + `SPACE` to select chats as you scroll
   - Press `a` to select all remaining chats
   - Press `d` to delete selection

### Vim Users

All standard vim navigation keys are supported:
- `j`/`k` - Line movement
- `Ctrl+D`/`Ctrl+U` - Half page scroll
- `Ctrl+F`/`Ctrl+B` - Full page scroll
- `g`/`G` - Jump to top/bottom

## Scroll Indicator

When the chat list is longer than the visible area, a scroll indicator appears at the bottom showing:
```
[1-20/150]
```
This means you're viewing items 1-20 out of 150 total chats.

## Performance Notes

- Page scrolling automatically adapts to your terminal height
- Minimum page size is 10 items (for very small terminals)
- All navigation commands work instantly regardless of chat count
