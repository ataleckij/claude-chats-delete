# Keyboard Shortcuts Guide

Complete reference for all keyboard shortcuts in Claude Code Chat Manager.

## Navigation Commands

### Basic Navigation
- **`↑` (Up Arrow)** or **`k`** - Move cursor up one item
- **`↓` (Down Arrow)** or **`j`** - Move cursor down one item

### Fast Scrolling
For users with large chat histories (hundreds of sessions):

- **`f`** or **`PgDn`** - Scroll down by one page
- **`b`** or **`PgUp`** - Scroll up by one page
- **`F`** - Scroll down by half page
- **`B`** - Scroll up by half page

### Jump Navigation
- **`g`** or **`Home`** - Jump to first chat (top of list)
- **`G`** or **`End`** - Jump to last chat (bottom of list)

## Selection Commands

- **`<Space>`** - Toggle selection for current chat
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
   - Press `g` to jump to first chat
   - Press `G` to jump to last chat

2. **Fast Scanning**
   - Use `f`/`b` to quickly scan through pages
   - Use `F`/`B` for half-page movements

3. **Bulk Selection Workflow**
   - Press `a` to select all chats
   - Use `<Space>` to deselect individual chats you want to keep
   - Press `d` to delete selection

### Vim Users

All standard vim navigation keys are supported:
- `j`/`k` - Line movement
- `f`/`b` - Full page scroll
- `F`/`B` - Half page scroll
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
