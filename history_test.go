package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	histDoomed = "11111111-1111-1111-1111-111111111111"
	histOther  = "22222222-2222-2222-2222-222222222222"
)

func historyLine(session, display, pasted string) string {
	if pasted == "" {
		pasted = "{}"
	}
	return `{"display":"` + display + `","pastedContents":` + pasted + `,"timestamp":1,"project":"/p","sessionId":"` + session + `"}`
}

func pasteRef(hash string) string {
	return `{"1":{"id":1,"type":"text","contentHash":"` + hash + `"}}`
}

func writeHistory(t *testing.T, content string) string {
	t.Helper()
	path := historyPath()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writePaste(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(claudeDir, pasteCacheDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("pasted"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestPruneHistory_RemovesOnlyMatchingLines(t *testing.T) {
	useTempClaudeDir(t, "")

	// Odd spacing, a broken line and a record without a session must all come
	// through byte for byte.
	keep1 := `{ "display": "keep one",  "sessionId":"` + histOther + `" }`
	broken := `{not json`
	noSession := `{"display":"no session"}`
	keep2 := historyLine(histOther, "keep two", "")

	content := strings.Join([]string{
		keep1,
		historyLine(histDoomed, "drop one", ""),
		broken,
		noSession,
		historyLine(histDoomed, "drop two", `{"1":{"id":1,"type":"text","content":"inline"}}`),
		keep2,
	}, "\n") + "\n"
	path := writeHistory(t, content)

	if err := pruneHistory(map[string]bool{histDoomed: true}); err != nil {
		t.Fatalf("pruneHistory: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{keep1, broken, noSession, keep2}, "\n") + "\n"
	if string(got) != want {
		t.Errorf("history =\n%s\nwant\n%s", got, want)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("permissions = %o, want 0600", perm)
	}
}

func TestPruneHistory_LastLineWithoutNewline(t *testing.T) {
	useTempClaudeDir(t, "")

	keep := historyLine(histOther, "keep", "")
	path := writeHistory(t, historyLine(histDoomed, "drop", "")+"\n"+keep)

	if err := pruneHistory(map[string]bool{histDoomed: true}); err != nil {
		t.Fatalf("pruneHistory: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != keep {
		t.Errorf("history = %q, want %q", got, keep)
	}
}

func TestPruneHistory_PasteCache(t *testing.T) {
	useTempClaudeDir(t, "")

	own := writePaste(t, "aaaa000000000001.txt")
	shared := writePaste(t, "bbbb000000000002.txt")
	foreign := writePaste(t, "cccc000000000003.txt")
	// A hash that is not a plain hex name must never be turned into a path.
	outside := filepath.Join(claudeDir, "outside.txt")
	if err := os.WriteFile(outside, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	writeHistory(t, strings.Join([]string{
		historyLine(histDoomed, "own", pasteRef("aaaa000000000001")),
		historyLine(histDoomed, "shared", pasteRef("bbbb000000000002")),
		historyLine(histDoomed, "missing", pasteRef("dddd000000000004")),
		historyLine(histDoomed, "escape", pasteRef("../outside")),
		historyLine(histOther, "shared", pasteRef("bbbb000000000002")),
		historyLine(histOther, "foreign", pasteRef("cccc000000000003")),
	}, "\n")+"\n")

	if err := pruneHistory(map[string]bool{histDoomed: true}); err != nil {
		t.Fatalf("pruneHistory: %v", err)
	}

	if exists(own) {
		t.Error("paste referenced only by the deleted session should be removed")
	}
	if !exists(shared) {
		t.Error("paste still referenced by another session should be kept")
	}
	if !exists(foreign) {
		t.Error("another session's paste should be kept")
	}
	if !exists(outside) {
		t.Error("a non-hex hash must not delete anything")
	}
}

func TestPruneHistory_NoWriteWhenNothingMatches(t *testing.T) {
	useTempClaudeDir(t, "")

	content := historyLine(histOther, "keep", "") + "\n"
	path := writeHistory(t, content)
	// A rewrite goes through a rename, which would swap in a different file.
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := pruneHistory(map[string]bool{histDoomed: true}); err != nil {
		t.Fatalf("pruneHistory: %v", err)
	}
	if err := pruneHistory(nil); err != nil {
		t.Fatalf("pruneHistory(nil): %v", err)
	}

	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Error("history file was replaced although no record matched")
	}
	got, _ := os.ReadFile(path)
	if string(got) != content {
		t.Errorf("history = %q, want it unchanged", got)
	}
}

func TestPruneHistory_MissingFile(t *testing.T) {
	useTempClaudeDir(t, "")

	if err := pruneHistory(map[string]bool{histDoomed: true}); err != nil {
		t.Fatalf("pruneHistory: %v", err)
	}
	if exists(historyPath()) {
		t.Error("a missing history file must not be created")
	}
}

func TestPruneHistory_UnreadableRecordProtectsPaste(t *testing.T) {
	useTempClaudeDir(t, "")

	odd := writePaste(t, "aaaa000000000001.txt")
	broken := writePaste(t, "bbbb000000000002.txt")

	// Neither kept line yields a hash when parsed: one has a paste entry of an
	// unexpected shape, the other is not JSON at all. Both still mention a hash
	// the deleted session used.
	writeHistory(t, strings.Join([]string{
		historyLine(histDoomed, "one", pasteRef("aaaa000000000001")),
		historyLine(histDoomed, "two", pasteRef("bbbb000000000002")),
		historyLine(histOther, "odd", `{"1":"aaaa000000000001"}`),
		`{"sessionId":"` + histOther + `","contentHash":"bbbb000000000002"`,
	}, "\n")+"\n")

	if err := pruneHistory(map[string]bool{histDoomed: true}); err != nil {
		t.Fatalf("pruneHistory: %v", err)
	}
	if !exists(odd) {
		t.Error("paste mentioned by a kept record of unexpected shape should be kept")
	}
	if !exists(broken) {
		t.Error("paste mentioned by a kept unparsable line should be kept")
	}
}

func TestPruneHistory_Lock(t *testing.T) {
	useTempClaudeDir(t, "")
	origWait := historyLockWait
	historyLockWait = 0
	t.Cleanup(func() { historyLockWait = origWait })

	content := historyLine(histDoomed, "drop", "") + "\n"
	path := writeHistory(t, content)
	lock := path + historyLockSuffix

	// Held by someone else: the history must stay as it is, and so must their lock.
	if err := os.Mkdir(lock, 0755); err != nil {
		t.Fatal(err)
	}
	err := pruneHistory(map[string]bool{histDoomed: true})
	if !errors.Is(err, errHistoryBusy) {
		t.Fatalf("err = %v, want errHistoryBusy", err)
	}
	if got, _ := os.ReadFile(path); string(got) != content {
		t.Errorf("history = %q, want it unchanged while locked", got)
	}
	if !exists(lock) {
		t.Error("a lock held by another process must not be removed")
	}

	// Free again: the prune goes through and leaves no lock behind.
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	if err := pruneHistory(map[string]bool{histDoomed: true}); err != nil {
		t.Fatalf("pruneHistory: %v", err)
	}
	if got, _ := os.ReadFile(path); len(got) != 0 {
		t.Errorf("history = %q, want it empty", got)
	}
	if exists(lock) {
		t.Error("own lock should be released")
	}
}

func TestPruneHistory_Symlink(t *testing.T) {
	useTempClaudeDir(t, "")

	keep := historyLine(histOther, "keep", "") + "\n"
	target := filepath.Join(t.TempDir(), "real-history.jsonl")
	if err := os.WriteFile(target, []byte(historyLine(histDoomed, "drop", "")+"\n"+keep), 0600); err != nil {
		t.Fatal(err)
	}
	link := historyPath()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := pruneHistory(map[string]bool{histDoomed: true}); err != nil {
		t.Fatalf("pruneHistory: %v", err)
	}

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("history.jsonl should still be a symlink")
	}
	if got, _ := os.ReadFile(target); string(got) != keep {
		t.Errorf("link target = %q, want %q", got, keep)
	}
}

// writeTranscript creates a minimal transcript for uuid in the given project.
func writeTranscript(t *testing.T, project, uuid string) string {
	t.Helper()
	dir := filepath.Join(projectsDir, project)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, uuid+transcriptExt)
	if err := os.WriteFile(path, []byte("{}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDeleteChats_PrunesHistory(t *testing.T) {
	setupStorageDirs(t)

	transcript := writeTranscript(t, "-p", histDoomed)
	keep := historyLine(histOther, "keep", "") + "\n"
	path := writeHistory(t, historyLine(histDoomed, "drop", "")+"\n"+keep)

	count, err := deleteChats([]Chat{{UUID: histDoomed, Path: transcript}})
	if err != nil || count != 1 {
		t.Fatalf("deleteChats = %d, %v; want 1, nil", count, err)
	}
	if exists(transcript) {
		t.Error("transcript should be deleted")
	}
	if got, _ := os.ReadFile(path); string(got) != keep {
		t.Errorf("history = %q, want only the other session's line", got)
	}
}

func TestDeleteChats_HistoryFailureKeepsCount(t *testing.T) {
	setupStorageDirs(t)

	transcript := writeTranscript(t, "-p", histDoomed)
	// A directory in place of the history file cannot be read as one.
	if err := os.Mkdir(historyPath(), 0755); err != nil {
		t.Fatal(err)
	}

	count, err := deleteChats([]Chat{{UUID: histDoomed, Path: transcript}})
	var pruneErr *historyPruneError
	if !errors.As(err, &pruneErr) {
		t.Fatalf("err = %v, want a *historyPruneError", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1: the chat itself was deleted", count)
	}
	if exists(transcript) {
		t.Error("transcript should be deleted despite the history failure")
	}
}

func TestDeleteChats_SharedUUIDKeepsHistory(t *testing.T) {
	setupStorageDirs(t)

	first := writeTranscript(t, "-a", histDoomed)
	second := writeTranscript(t, "-b", histDoomed)
	content := historyLine(histDoomed, "shared", "") + "\n"
	path := writeHistory(t, content)

	// The other project's transcript still carries the uuid: its prompts stay.
	if _, err := deleteChats([]Chat{{UUID: histDoomed, Path: first}}); err != nil {
		t.Fatalf("deleteChats: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != content {
		t.Errorf("history = %q, want it unchanged while another transcript uses the uuid", got)
	}

	// Once the last one goes, so do the prompts.
	if _, err := deleteChats([]Chat{{UUID: histDoomed, Path: second}}); err != nil {
		t.Fatalf("deleteChats: %v", err)
	}
	if got, _ := os.ReadFile(path); len(got) != 0 {
		t.Errorf("history = %q, want it empty", got)
	}
}

func TestDeleteChats_FailureAfterTranscriptStillPrunes(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	setupStorageDirs(t)

	transcript := writeTranscript(t, "-p", histDoomed)
	// A read-only index makes the deletion fail after the files are gone.
	index := filepath.Join(filepath.Dir(transcript), "sessions-index.json")
	if err := os.WriteFile(index, []byte(`{"version":1,"entries":[{"sessionId":"`+histDoomed+`"}]}`), 0444); err != nil {
		t.Fatal(err)
	}
	keep := historyLine(histOther, "keep", "") + "\n"
	path := writeHistory(t, historyLine(histDoomed, "drop", "")+"\n"+keep)

	_, err := deleteChats([]Chat{{UUID: histDoomed, Path: transcript}})
	if err == nil {
		t.Fatal("deleteChats should report the index failure")
	}
	var pruneErr *historyPruneError
	if errors.As(err, &pruneErr) {
		t.Errorf("err = %v, want the deletion failure, not a history warning", err)
	}
	if exists(transcript) {
		t.Fatal("transcript should already be deleted")
	}
	if got, _ := os.ReadFile(path); string(got) != keep {
		t.Errorf("history = %q, want the deleted transcript's prompts gone", got)
	}
}

func TestTranscriptLookup_PatternCharsInPath(t *testing.T) {
	tmp := setupStorageDirs(t)
	// Brackets, stars and question marks are legal in a directory name but
	// wildcards in a glob. setupStorageDirs restores projectsDir on cleanup.
	projectsDir = filepath.Join(tmp, "cl[au]de*?", "projects")

	first := writeTranscript(t, "-a", histDoomed)
	second := writeTranscript(t, "-b", histDoomed)
	content := historyLine(histDoomed, "shared", "") + "\n"
	path := writeHistory(t, content)

	if got := len(findAllChats()); got != 2 {
		t.Fatalf("findAllChats found %d chats, want 2", got)
	}
	if !transcriptExists(histDoomed) {
		t.Fatal("transcriptExists should see the transcripts")
	}

	if _, err := deleteChats([]Chat{{UUID: histDoomed, Path: first}}); err != nil {
		t.Fatalf("deleteChats: %v", err)
	}
	if !exists(second) {
		t.Fatal("the other project's transcript should be untouched")
	}
	if got, _ := os.ReadFile(path); string(got) != content {
		t.Errorf("history = %q, want it unchanged while another transcript uses the uuid", got)
	}
}
