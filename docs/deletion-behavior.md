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

### Prompt History

Claude Code keeps every prompt you type in `history.jsonl`, one file shared by
all sessions, where each record carries the `sessionId` it was typed in. Deleting
a chat removes that chat's records from the file. Every other line, including
ones that do not parse, is carried over byte for byte.

Pastes too large to be stored inside a record live in `paste-cache/<hash>.txt`
and are referenced from the record by that hash. Such a file is removed only
when no remaining record in `history.jsonl` points at it, so a paste used in
another chat survives.

Running Claude Code sessions append to `history.jsonl` at any time. The tool
takes the same lock Claude Code uses for its own writes (a `history.jsonl.lock`
directory next to the file, as seen on v2.1.296), so a running session waits
and appends its prompt afterwards. The file is rewritten through a temporary
file and a rename. If the lock stays held by another process, or the file keeps
changing anyway, the history is left as it was. A symlinked `history.jsonl` is
written through to its target.

Records are matched by session id alone, so they are removed only once no
transcript with that id is left in any project. Two transcripts sharing an id
(a restored backup, a hand-copied project) keep their prompts until the last
one is deleted.

When the history cannot be cleaned up, the chat is deleted anyway and the tool
reports that the prompt history was left behind.

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
untouched on chat delete. Memory notes can be deleted from the **Memory** tab,
see below.

Debug logs are not written by default in recent Claude Code versions unless
`/debug` is enabled.

## Deleting Memory Notes

The **Memory** tab deletes project memory notes with the same selection and
confirmation flow as chats. For each deleted note:

- The note file `projects/<project>/memory/<name>.md` is removed.
- Every line of that project's `MEMORY.md` index that links to the note
  (`[...](<name>.md)`, also written as `./<name>.md` or `<<name>.md>`) is
  removed; the rest of the index is kept byte for byte. A project without an
  index does not get one.

The index is rewritten atomically, and it is reread if it changes while being
read. Claude Code takes no lock on it, so a line Claude Code adds in the moment
between that check and the rewrite can still be lost. Claude Code's
`memory/.consolidate-lock` is not consulted: it only keeps two automatic memory
consolidations from running at once, and Claude Code's own memory writes ignore
it. If the index cannot be updated, the notes stay deleted and a warning is
shown.

## Claude Directory Layout

The files above live in `~/.claude/`:

```
~/.claude/
├── projects/<project>/
│   ├── <uuid>.jsonl              # main chat file
│   ├── <uuid>/                   # chat directory
│   │   ├── subagents/            # subagent conversations
│   │   └── tool-results/         # tool execution results
│   ├── memory/                   # project memory (deleted only from the Memory tab)
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
├── history.jsonl                 # prompt history (the chat's records removed)
├── paste-cache/<hash>.txt        # large pastes (removed when unreferenced)
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
