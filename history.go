package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Claude Code's prompt history: one JSONL file shared by every session, plus
// the directory holding pastes too large to be inlined into it.
const (
	historyFile   = "history.jsonl"
	pasteCacheDir = "paste-cache"

	// Claude Code serializes its own writes with a lock directory next to the
	// file (the proper-lockfile convention, seen on v2.1.296).
	historyLockSuffix = ".lock"

	historyLockAttempts  = 5
	historyWriteAttempts = 3
)

// historyLockWait is the pause between attempts to take the lock. Overridable
// in tests.
var historyLockWait = 100 * time.Millisecond

var errHistoryBusy = fmt.Errorf("%s is busy, left unchanged", historyFile)

// historyPruneError reports that chats were deleted but their prompt history
// could not be cleaned up, so callers can tell it from a failed deletion.
type historyPruneError struct{ err error }

func (e *historyPruneError) Error() string {
	return fmt.Sprintf("prompt history not cleaned up: %v", e.err)
}

func (e *historyPruneError) Unwrap() error { return e.err }

func historyPath() string {
	return filepath.Join(claudeDir, historyFile)
}

// historyEntry is the part of a history record this tool relies on.
type historyEntry struct {
	SessionID      string          `json:"sessionId"`
	PastedContents json.RawMessage `json:"pastedContents"`
}

// pasteHashes returns the paste-cache files a record points at. Short pastes
// are inlined into the record and carry no hash.
func (e historyEntry) pasteHashes() []string {
	var pasted map[string]json.RawMessage
	if json.Unmarshal(e.PastedContents, &pasted) != nil {
		return nil
	}
	var hashes []string
	for _, raw := range pasted {
		var p struct {
			ContentHash string `json:"contentHash"`
		}
		if json.Unmarshal(raw, &p) == nil && isHexName(p.ContentHash) {
			hashes = append(hashes, p.ContentHash)
		}
	}
	return hashes
}

// isHexName reports whether s is a non-empty hex string. Hashes read from the
// history file become file names, so anything else is rejected rather than
// joined into a path.
func isHexName(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

// filterHistory drops the records that belong to the given sessions. Every
// other line, including ones that do not parse, is carried over byte for byte.
//
// orphaned lists the paste hashes of dropped records that appear nowhere in
// what is kept. The check is a plain substring search rather than a parse of
// the kept records, so a record this tool cannot read still protects the pastes
// it mentions.
func filterHistory(data []byte, uuids map[string]bool) (kept []byte, dropped int, orphaned []string) {
	droppedHashes := make(map[string]bool)

	kept = make([]byte, 0, len(data))
	for len(data) > 0 {
		line := data
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			line = data[:i+1]
		}
		data = data[len(line):]

		var entry historyEntry
		if json.Unmarshal(bytes.TrimSpace(line), &entry) != nil ||
			entry.SessionID == "" || !uuids[entry.SessionID] {
			kept = append(kept, line...)
			continue
		}
		dropped++
		for _, h := range entry.pasteHashes() {
			droppedHashes[h] = true
		}
	}

	for h := range droppedHashes {
		if !bytes.Contains(kept, []byte(h)) {
			orphaned = append(orphaned, h)
		}
	}
	return kept, dropped, orphaned
}

// lockHistory takes the lock Claude Code holds while it writes the history
// file, so running sessions wait instead of appending to a file that is being
// replaced. A lock that stays held is reported as busy rather than broken: it
// belongs to another process.
func lockHistory(path string) (unlock func(), err error) {
	lock := path + historyLockSuffix
	for attempt := 1; ; attempt++ {
		err := os.Mkdir(lock, 0755)
		if err == nil {
			return func() { os.Remove(lock) }, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		if attempt >= historyLockAttempts {
			return nil, errHistoryBusy
		}
		time.Sleep(historyLockWait)
	}
}

// pruneHistory removes the given sessions' prompts from the history file, along
// with the cached pastes no remaining record refers to. A missing file and a
// file without matching records are both left untouched.
//
// The rewrite happens under Claude Code's own history lock. A writer that
// ignores the lock is still caught by comparing the file before and after the
// read, in which case the rewrite is retried and finally given up.
func pruneHistory(uuids map[string]bool) error {
	if len(uuids) == 0 {
		return nil
	}
	// The lock sits next to the real file, so a symlinked history is resolved
	// first.
	path := resolveSymlink(historyPath())

	// Checked before locking, so a history with nothing to remove never needs
	// the Claude directory to be writable.
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, dropped, _ := filterHistory(data, uuids); dropped == 0 {
		return nil
	}

	unlock, err := lockHistory(path)
	if err != nil {
		return err
	}
	defer unlock()

	for attempt := 0; attempt < historyWriteAttempts; attempt++ {
		before, err := os.Stat(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		kept, dropped, orphaned := filterHistory(data, uuids)
		if dropped == 0 {
			return nil
		}

		after, err := os.Stat(path)
		if err != nil {
			return err
		}
		if after.Size() != int64(len(data)) || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
			continue
		}

		if err := writeFileAtomic(path, kept); err != nil {
			return err
		}
		return removePastes(orphaned)
	}
	return errHistoryBusy
}

func removePastes(hashes []string) error {
	var errs []error
	for _, h := range hashes {
		err := os.Remove(filepath.Join(claudeDir, pasteCacheDir, h+".txt"))
		if err != nil && !os.IsNotExist(err) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
