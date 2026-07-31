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
- Security warning state: `security/security_warnings_state_<uuid>.json` and its
  `.lock` sidecar
- Failed telemetry events: `telemetry/*<uuid>*.json`
- Session environment: `session-env/<uuid>/`
- Task state: `tasks/<uuid>/`
- Background job state: `jobs/<uuid-prefix>/` (matched on the `sessionId` inside
  its `state.json`, since the directory name is only an 8-character prefix)
- A fork's copy of this chat's transcript:
  `jobs/<fork-prefix>/tmp/parent-transcript.jsonl` (the fork's own transcript is
  self-contained and is left alone)

### Older Layouts

These paths are no longer created by current Claude Code versions (verified
absent on v2.1.220), but histories written by older ones still carry them, so
they are removed when present:

- Security warning state in the Claude directory root:
  `security_warnings_state_<uuid>.json`
- Todo files: `todos/<uuid>*.json`
- Plan file: `plans/<slug>.md` (only when the slug is not referenced by any
  other chat)
- Agent memory: `agents/<agent-id>/memory-local.md` (session-specific only)

The tool also updates `projects/<project>/sessions-index.json` to drop the
entry for the deleted chat. Recent Claude Code versions may not write this
index at all; when it is absent the update is a no-op.

Project- and user-scoped state is preserved: `projects/<project>/memory/`
(project memory notes) and `agent-memory/<agent>/` (per-agent memory, which is
project-scoped in the current layout) are not tied to a single chat and are left
untouched on delete.

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
│   ├── memory/                   # project memory (preserved)
│   └── sessions-index.json       # chat index (updated on delete, may be absent)
├── file-history/<uuid>/          # file version history
├── debug/<uuid>.txt              # debug logs
├── session-env/<uuid>/           # session environments
├── tasks/<uuid>/                 # background task state
├── security/                     # security-hook state
│   ├── security_warnings_state_<uuid>.json
│   └── security_warnings_state_<uuid>.lock
├── telemetry/*<uuid>*.json       # failed telemetry events
├── jobs/<uuid-prefix>/           # background session state
│   ├── state.json                # carries sessionId / forkParentSessionId
│   ├── timeline.jsonl
│   └── tmp/parent-transcript.jsonl  # a fork's copy of its parent transcript
├── jobs/pins.json                # global, never removed
├── agent-memory/<agent>/         # per-agent memory (preserved)
├── sessions/<pid>.json           # live-session registry (preserved)
├── daemon/roster.json            # live background workers (preserved)
│
│   # older layouts, removed when present:
├── todos/<uuid>-*.json           # todo lists
├── plans/*.md                    # plan mode files (by slug)
└── agents/<agent-id>/            # agent memory (v2.1.33+)
    ├── memory-local.md           # session-specific (deleted)
    ├── memory-project.md         # project memory (preserved)
    └── memory-user.md            # global memory (preserved)
```
