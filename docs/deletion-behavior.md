# Deletion Behavior

This document describes what happens when you delete a chat with `claude-chats`
and which files on disk are touched.

## Confirmation Flow

When you press `d`:

1. If no chat is explicitly selected (via `Space`), the chat under the cursor
   is auto-selected for this single action. In grouped view, pressing `d` on a
   project header auto-selects every chat in that project.
2. A confirmation dialog appears.
3. Press `ENTER` to confirm, `ESC` or `n` to cancel.
4. If the selection was made automatically, cancelling reverts it so the next
   `d` acts on the new cursor position, not the stale one.

Explicit selection always wins: if you have chats selected via `Space`, `d`
deletes those regardless of where the cursor is, and cancelling does not wipe
your explicit selection.

## What Gets Deleted

For each chat UUID, the tool removes:

- Main chat file: `projects/<project>/<uuid>.jsonl`
- Subagents directory: `projects/<project>/<uuid>/subagents/`
- Tool results directory: `projects/<project>/<uuid>/tool-results/`
- File history: `file-history/<uuid>/`
- Debug logs: `debug/<uuid>.txt`
- Security warning state: `security_warnings_state_<uuid>.json`
- Todo files: `todos/<uuid>*.json`
- Session environment: `session-env/<uuid>/`
- Task state: `tasks/<uuid>/`
- Plan file: `plans/<slug>.md` (only when the slug is not referenced by any
  other chat)
- Agent memory: `agents/<agent-id>/memory-local.md` (session-specific only;
  project- and user-scope memories are preserved as they may be shared)

The tool also updates `projects/<project>/sessions-index.json` to drop the
entry for the deleted chat.

Debug logs are not written by default in recent Claude Code versions unless
`/debug` is enabled.

## Claude Directory Layout

The files above live in `~/.claude/`:

```
~/.claude/
├── projects/<project>/
│   ├── <uuid>.jsonl              # main chat file
│   ├── <uuid>/                   # chat directory
│   │   ├── subagents/            # subagent conversations
│   │   └── tool-results/         # tool execution results
│   └── sessions-index.json       # chat index (tool updates on delete)
├── file-history/<uuid>/          # file version history
├── debug/<uuid>.txt              # debug logs
├── todos/<uuid>-*.json           # todo lists
├── session-env/<uuid>/           # session environments
├── tasks/<uuid>/                 # background task state
├── plans/*.md                    # plan mode files (by slug)
└── agents/<agent-id>/            # agent memory (v2.1.33+)
    ├── memory-local.md           # session-specific (deleted)
    ├── memory-project.md         # project memory (preserved)
    └── memory-user.md            # global memory (preserved)
```
