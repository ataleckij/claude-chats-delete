package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// writeMemory creates a memory file for a project and backdates it by age, so
// tests control the newest-first order.
func writeMemory(t *testing.T, project, name, content string, age time.Duration) string {
	t.Helper()
	dir := filepath.Join(projectsDir, project, memoryDirName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	when := time.Now().Add(-age)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
	return path
}

const memoryNote = `---
name: release-commits
description: "Bump the version: with the work, not \"after\""
metadata:
  node_type: memory
  type: feedback
---

Body text with a description: that must not be picked up.
`

func TestParseMemoryFrontmatter(t *testing.T) {
	tests := []struct {
		name            string
		content         string
		wantDescription string
		wantType        string
	}{
		{"quoted description, nested type", memoryNote, `Bump the version: with the work, not "after"`, "feedback"},
		{"plain values, top-level type", "---\ndescription: plain text\ntype: user\n---\nbody", "plain text", "user"},
		{"single quotes", "---\ndescription: 'it''s fine'\n---\n", "it's fine", ""},
		{"crlf line endings", "---\r\ndescription: windows\r\ntype: project\r\n---\r\n", "windows", "project"},
		{"no frontmatter", "- [Title](file.md) - hook\n", "", ""},
		{"unterminated frontmatter", "---\ndescription: never closed\n", "", ""},
		{"empty file", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			description, kind := parseMemoryFrontmatter([]byte(tt.content))
			if description != tt.wantDescription || kind != tt.wantType {
				t.Errorf("got (%q, %q), want (%q, %q)", description, kind, tt.wantDescription, tt.wantType)
			}
		})
	}
}

func TestFindAllMemoryFiles(t *testing.T) {
	setupStorageDirs(t)

	writeMemory(t, "-proj-a", "old.md", memoryNote, 3*time.Hour)
	newest := writeMemory(t, "-proj-b", "plain.md", "no frontmatter here\n", time.Hour)
	// The index is bookkeeping, not a note, and anything but markdown is ignored.
	writeMemory(t, "-proj-b", memoryIndexFile, "- [Plain](plain.md) - hook\n", time.Minute)
	writeMemory(t, "-proj-a", "notes.txt", "not markdown", time.Minute)
	// Not memory: a transcript next to memory/, a nested directory inside it,
	// and a project without a memory directory at all.
	writeTranscript(t, "-proj-a", "11111111-1111-1111-1111-111111111111")
	if err := os.MkdirAll(filepath.Join(projectsDir, "-proj-a", memoryDirName, "nested.md"), 0755); err != nil {
		t.Fatal(err)
	}
	writeTranscript(t, "-proj-c", "22222222-2222-2222-2222-222222222222")

	files := findAllMemoryFiles()
	if len(files) != 2 {
		t.Fatalf("found %d files, want 2: %+v", len(files), files)
	}
	if files[0].Path != newest || files[0].Project != "-proj-b" || files[0].Name != "plain.md" {
		t.Errorf("first file = %+v, want the newest one (-proj-b/plain.md)", files[0])
	}
	if files[0].Description != "" {
		t.Errorf("description = %q, want empty: the file has no frontmatter", files[0].Description)
	}
	if files[1].Name != "old.md" || files[1].Type != "feedback" || !strings.HasPrefix(files[1].Description, "Bump the version") {
		t.Errorf("second file = %+v, want old.md with its frontmatter parsed", files[1])
	}
	if files[1].Size != int64(len(memoryNote)) {
		t.Errorf("size = %d, want %d", files[1].Size, len(memoryNote))
	}
}

func TestFindAllMemoryFiles_NoProjectsDir(t *testing.T) {
	setupStorageDirs(t)
	projectsDir = filepath.Join(projectsDir, "missing")

	if files := findAllMemoryFiles(); len(files) != 0 {
		t.Errorf("found %d files, want none", len(files))
	}
}

func makeTestMemory(projects, perProject int) []MemoryFile {
	var files []MemoryFile
	for p := 0; p < projects; p++ {
		for i := 0; i < perProject; i++ {
			files = append(files, MemoryFile{
				Project:     "project-" + string(rune('a'+p)),
				Name:        "note-" + string(rune('a'+i)) + ".md",
				Description: "a note",
				Path:        "/tmp/project-" + string(rune('a'+p)) + "/memory/note-" + string(rune('a'+i)) + ".md",
				Modified:    "2026-01-01 00:00:00",
			})
		}
	}
	return files
}

func makeMemoryModel(files []MemoryFile, grouped bool, width, height int) model {
	m := model{
		tab:              tabMemory,
		cfg:              &Config{GroupByProject: grouped},
		selected:         make(map[int]bool),
		width:            width,
		height:           height,
		grouped:          grouped,
		expandedProjects: make(map[string]bool),
		memExpanded:      make(map[string]bool),
		memory:           files,
		memSelected:      make(map[int]bool),
	}
	m.rebuildMemRows()
	return m
}

func TestTabs_OrderAndLabels(t *testing.T) {
	m := makeGroupedModel(makeTestChats(2), normalWidth, 20)
	bar := stripANSI(m.renderTabBar())
	if strings.Contains(bar, "Projects") {
		t.Errorf("the chats tab must keep its name in grouped view: %q", bar)
	}
	chats, memory, settings := strings.Index(bar, "Chats"), strings.Index(bar, "Memory"), strings.Index(bar, "Settings")
	if chats < 0 || !(chats < memory && memory < settings) {
		t.Errorf("tab bar = %q, want Chats, Memory, Settings in that order", bar)
	}

	m = send(m, tea.KeyMsg{Type: tea.KeyRight})
	if m.tab != tabMemory {
		t.Errorf("tab after one right = %d, want tabMemory", m.tab)
	}
	m = send(m, tea.KeyMsg{Type: tea.KeyRight})
	if m.tab != tabSettings {
		t.Errorf("tab after two rights = %d, want tabSettings", m.tab)
	}
}

func TestViewMemory_LineCounts(t *testing.T) {
	tests := []struct {
		name     string
		files    []MemoryFile
		grouped  bool
		width    int
		rows     int
		scroll   int
		footer   int
		expanded string
	}{
		{"flat", makeTestMemory(2, 3), false, normalWidth, 6, 0, fixedFooterLines, ""},
		{"flat compact", makeTestMemory(2, 3), false, compactWidth, 6, 0, fixedFooterLinesCompact, ""},
		{"flat empty", nil, false, normalWidth, 0, 0, fixedFooterLines, ""},
		{"flat with scroll", makeTestMemory(5, 6), false, normalWidth, 11, 1, fixedFooterLines, ""},
		{"grouped collapsed", makeTestMemory(2, 3), true, normalWidth, 2, 0, fixedFooterLines, ""},
		{"grouped expanded", makeTestMemory(2, 3), true, normalWidth, 5, 0, fixedFooterLines, "project-a"},
		{"grouped empty", nil, true, compactWidth, 0, 0, fixedFooterLinesCompact, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := makeMemoryModel(tt.files, tt.grouped, tt.width, 20)
			if tt.expanded != "" {
				m.memExpanded[tt.expanded] = true
				m.rebuildMemRows()
			}
			expected := fixedHeaderLines + tt.rows + tt.scroll + 0 + tt.footer
			if got := viewLineCount(m.View()); got != expected {
				t.Errorf("expected %d lines, got %d\noutput:\n%s", expected, got, stripANSI(m.View()))
			}
		})
	}
}

func TestViewMemory_ShowsError(t *testing.T) {
	m := makeMemoryModel(makeTestMemory(1, 1), false, normalWidth, 20)
	m.error = "boom"

	output := stripANSI(m.View())
	if !strings.Contains(output, "Error: boom") {
		t.Errorf("the memory tab must render m.error\noutput:\n%s", output)
	}
}

func TestViewMemory_StripsControlCharacters(t *testing.T) {
	// A quoted description is unescaped by the frontmatter parser, so "\x1b"
	// written as text must not reach the terminal as a real escape either.
	description, _ := parseMemoryFrontmatter([]byte("---\ndescription: \"\\x1b]0;pwned\\x07 quoted\"\n---\n"))
	files := makeTestMemory(1, 1)
	files[0].Description = description
	files[0].Name = "raw\x1b[2Jname.md"
	m := makeMemoryModel(files, false, normalWidth, 20)

	output := m.View()
	for _, seq := range []string{"\x1b]0;pwned", "\x07", "\x1b[2J"} {
		if strings.Contains(output, seq) {
			t.Errorf("view must not contain control sequence %q", seq)
		}
	}
	if !strings.Contains(output, "quoted") || !strings.Contains(output, "name.md") {
		t.Errorf("printable text around the control characters must stay\noutput:\n%s", stripANSI(output))
	}
}

func TestDisplayText(t *testing.T) {
	got := displayText("a\nb\r\tc\x1b[31md\u009be")
	if want := "a b  c [31md e"; got != want {
		t.Errorf("displayText = %q, want %q", got, want)
	}
}

func TestUpdateMemory_NavigationAndExpand(t *testing.T) {
	m := makeMemoryModel(makeTestMemory(2, 3), true, normalWidth, 20)

	// Paging past either end stays inside the list.
	m = send(m, keyRune('f'))
	if m.memCursor != 1 {
		t.Errorf("cursor after page down = %d, want 1 (last header)", m.memCursor)
	}
	m = send(m, keyRune('b'))
	if m.memCursor != 0 {
		t.Errorf("cursor after page up = %d, want 0", m.memCursor)
	}

	m = send(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.memExpanded["project-a"] || len(m.memRows) != 5 {
		t.Fatalf("enter on a header should expand it: expanded=%v rows=%d", m.memExpanded, len(m.memRows))
	}
	if m.memCursor != 0 {
		t.Errorf("cursor = %d, want it to stay on the header", m.memCursor)
	}
	m = send(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.memExpanded["project-a"] || len(m.memRows) != 2 {
		t.Errorf("a second enter should collapse it: expanded=%v rows=%d", m.memExpanded, len(m.memRows))
	}
}

func TestUpdateMemory_SelectAndConfirm(t *testing.T) {
	m := makeMemoryModel(makeTestMemory(1, 3), false, normalWidth, 20)
	m.chats = makeTestChats(3)

	m = send(m, keyRune(' '))
	if !m.memSelected[0] || len(m.memSelected) != 1 {
		t.Fatalf("space should select the note under the cursor: %v", m.memSelected)
	}
	m = send(m, keyRune('a'))
	if len(m.memSelected) != 3 {
		t.Fatalf("a should select every note: %v", m.memSelected)
	}
	m = send(m, keyRune('a'))
	if len(m.memSelected) != 0 {
		t.Fatalf("a with everything selected should clear: %v", m.memSelected)
	}

	// d with nothing selected picks the cursor row; esc drops that pick again.
	m = send(m, keyRune('j'))
	m = send(m, keyRune('d'))
	if !m.memConfirm || !m.memSelected[1] || len(m.memSelected) != 1 {
		t.Fatalf("d should confirm the note under the cursor: confirm=%v selected=%v", m.memConfirm, m.memSelected)
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(model)
	if m.memConfirm || len(m.memSelected) != 0 {
		t.Errorf("esc should cancel and drop the automatic pick: confirm=%v selected=%v", m.memConfirm, m.memSelected)
	}
	if cmd != nil || len(m.memory) != 3 {
		t.Errorf("esc in the dialog must neither quit nor delete")
	}

	if len(m.selected) != 0 || m.confirmDelete {
		t.Errorf("the chat selection must stay untouched: selected=%v confirm=%v", m.selected, m.confirmDelete)
	}
}

func TestUpdateMemory_GroupedHeaderSelectsProject(t *testing.T) {
	m := makeMemoryModel(makeTestMemory(2, 2), true, normalWidth, 20)

	m = send(m, keyRune(' '))
	if len(m.memSelected) != 2 || !m.memSelected[0] || !m.memSelected[1] {
		t.Fatalf("space on a header should select its project's notes: %v", m.memSelected)
	}
	if !strings.Contains(stripANSI(m.View()), "(2 files, 2 selected)") {
		t.Errorf("header should count the selected notes")
	}
	m = send(m, keyRune(' '))
	if len(m.memSelected) != 0 {
		t.Fatalf("space on a fully selected header should clear it: %v", m.memSelected)
	}

	m = send(m, keyRune('j'))
	m = send(m, keyRune('d'))
	if !m.memConfirm || len(m.memSelected) != 2 || !m.memSelected[2] || !m.memSelected[3] {
		t.Errorf("d on a header should pick its whole project: %v", m.memSelected)
	}
}

func TestUpdateMemory_DeleteRemovesNotesAndIndexLines(t *testing.T) {
	setupStorageDirs(t)
	doomed := writeMemory(t, "-proj-a", "doomed.md", memoryNote, time.Hour)
	kept := writeMemory(t, "-proj-a", "kept.md", memoryNote, 2*time.Hour)
	index := writeMemory(t, "-proj-a", memoryIndexFile, "- [Doomed](doomed.md) - hook\n- [Kept](kept.md) - hook\n", time.Minute)

	m := makeMemoryModel(nil, false, normalWidth, 20)
	m.loadMemory()
	if len(m.memory) != 2 || m.memory[0].Path != doomed {
		t.Fatalf("unexpected memory list: %+v", m.memory)
	}

	m = send(m, keyRune('d'))
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter in the dialog should start the delete")
	}
	next, _ = next.(model).Update(cmd())
	m = next.(model)

	if _, err := os.Stat(doomed); !os.IsNotExist(err) {
		t.Errorf("deleted note still on disk: %v", err)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Errorf("other note touched: %v", err)
	}
	data, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "- [Kept](kept.md) - hook\n"; got != want {
		t.Errorf("index = %q, want %q", got, want)
	}
	if m.memDeleted != 1 || m.error != "" || len(m.memory) != 1 || len(m.memSelected) != 0 {
		t.Errorf("after delete: deleted=%d error=%q files=%d selected=%v", m.memDeleted, m.error, len(m.memory), m.memSelected)
	}
	if !strings.Contains(stripANSI(m.View()), "Deleted 1 memory file(s)") {
		t.Errorf("the delete count should be shown")
	}
}

func TestUpdateMemory_KeysIgnoredWhileDeleting(t *testing.T) {
	m := makeMemoryModel(makeTestMemory(1, 2), false, normalWidth, 20)
	m = send(m, keyRune('d'))
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if cmd == nil || !m.deleting || !m.memConfirm {
		t.Fatalf("enter should start the delete and keep the dialog up: cmd=%v deleting=%v confirm=%v", cmd != nil, m.deleting, m.memConfirm)
	}
	if !strings.Contains(stripANSI(m.View()), "Deleting 1 memory file(s)...") {
		t.Errorf("the dialog should show the delete in progress")
	}

	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyEnter}, {Type: tea.KeyEsc}, {Type: tea.KeyCtrlC}, keyRune('q'), keyRune('n'),
	} {
		next, cmd := m.Update(key)
		m = next.(model)
		if cmd != nil {
			t.Errorf("%q during a delete returned a command (quit or a second delete)", key.String())
		}
		if !m.memConfirm || !m.deleting {
			t.Errorf("%q during a delete closed the dialog", key.String())
		}
	}

	next, _ = m.Update(memoryDeleteCompleteMsg{count: 1})
	m = next.(model)
	if m.memConfirm || m.deleting {
		t.Errorf("the dialog should close once the delete reports back")
	}
}

func TestViewMemory_MultiLineErrorStaysOnOneLine(t *testing.T) {
	m := makeMemoryModel(makeTestMemory(1, 1), false, normalWidth, 20)
	clean := strings.Count(stripANSI(m.View()), "\n")
	m.error = "remove /a/one.md: denied\nremove /a/two.md: denied\nremove /a/three.md: denied"

	output := stripANSI(m.View())
	if got := strings.Count(output, "\n"); got != clean+1 {
		t.Errorf("view has %d lines, want %d: the error must take exactly one line\noutput:\n%s", got, clean+1, output)
	}
	if !strings.Contains(output, "Error: remove /a/one.md: denied remove") {
		t.Errorf("errors should be joined on one line\noutput:\n%s", output)
	}
}

func TestDeleteMemoryFiles(t *testing.T) {
	setupStorageDirs(t)
	a1 := writeMemory(t, "-proj-a", "one.md", memoryNote, time.Hour)
	writeMemory(t, "-proj-a", "two.md", memoryNote, time.Hour)
	indexA := writeMemory(t, "-proj-a", memoryIndexFile, "# Memory\r\n- [One](one.md) - hook\r\n- [Two](two.md) - hook\r\n", time.Hour)
	// A project without an index must not get one.
	b1 := writeMemory(t, "-proj-b", "one.md", memoryNote, time.Hour)
	gone := MemoryFile{Project: "-proj-a", Name: "gone.md", Path: filepath.Join(filepath.Dir(a1), "gone.md")}

	files := []MemoryFile{
		{Project: "-proj-a", Name: "one.md", Path: a1},
		{Project: "-proj-b", Name: "one.md", Path: b1},
		gone,
	}
	count, err := deleteMemoryFiles(files)
	if err != nil || count != 3 {
		t.Fatalf("deleteMemoryFiles = %d, %v; want 3, nil (a note already gone counts)", count, err)
	}

	data, err := os.ReadFile(indexA)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "# Memory\r\n- [Two](two.md) - hook\r\n"; got != want {
		t.Errorf("index = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(b1), memoryIndexFile)); !os.IsNotExist(err) {
		t.Errorf("an index was created where there was none: %v", err)
	}
}

func TestFilterMemoryIndex(t *testing.T) {
	names := map[string]bool{"note.md": true, "other.md": true}
	input := strings.Join([]string{
		"# Index",
		"- [Note](note.md) - bare",
		"- [Note](./note.md) - dot slash",
		"- [Note](<note.md>) - angle brackets",
		`- [Note](note.md "title") - with a title`,
		"- [Two](keep.md) and [other](other.md) - any link counts",
		"- [Similar](my-note.md) - only the exact name matches",
		"- [Sub](sub/note.md) - another directory",
		"note.md mentioned without a link",
		"- [Last](note.md) - no trailing newline",
	}, "\n")

	kept, dropped := filterMemoryIndex([]byte(input), names)
	want := strings.Join([]string{
		"# Index",
		"- [Similar](my-note.md) - only the exact name matches",
		"- [Sub](sub/note.md) - another directory",
		"note.md mentioned without a link",
		"",
	}, "\n")
	if string(kept) != want || dropped != 6 {
		t.Errorf("filterMemoryIndex dropped %d\ngot:\n%q\nwant:\n%q", dropped, kept, want)
	}
}

func TestUpdateMemory_EmptyListKeysAreSafe(t *testing.T) {
	for _, grouped := range []bool{false, true} {
		m := makeMemoryModel(nil, grouped, normalWidth, 20)
		for _, r := range []rune{'j', 'k', 'f', 'b', 'F', 'B', 'g', 'G', 'c'} {
			m = send(m, keyRune(r))
		}
		m = send(m, tea.KeyMsg{Type: tea.KeyEnter})
		if m.memCursor != 0 || m.memScroll != 0 {
			t.Errorf("grouped=%v: cursor=%d scroll=%d, want both 0", grouped, m.memCursor, m.memScroll)
		}
		m.View()
	}
}

func TestUpdateChats_EmptyListKeysAreSafe(t *testing.T) {
	for _, grouped := range []bool{false, true} {
		var m model
		if grouped {
			m = makeGroupedModel(nil, normalWidth, 20)
		} else {
			m = makeTestModel(nil, normalWidth, 20)
		}
		for _, r := range []rune{'j', 'k', 'f', 'b', 'F', 'B', 'g', 'G', ' ', 'a', 'c', 'd'} {
			m = send(m, keyRune(r))
		}
		if m.cursor != 0 || m.scrollOffset != 0 {
			t.Errorf("grouped=%v: cursor=%d scroll=%d, want both 0", grouped, m.cursor, m.scrollOffset)
		}
		if len(m.selected) != 0 || m.confirmDelete {
			t.Errorf("grouped=%v: nothing to select or delete, got selected=%v confirm=%v", grouped, m.selected, m.confirmDelete)
		}
		m.View()
	}
}

func TestDeleteComplete_EmptyListDoesNotQuit(t *testing.T) {
	setupStorageDirs(t)
	m := makeTestModel(nil, normalWidth, 20)

	updated, cmd := m.Update(deleteCompleteMsg{count: 1, warning: "prompt history not cleaned up: busy"})
	m = updated.(model)
	if cmd == nil {
		t.Fatal("want the timer that clears the status message")
	}
	// A quit command returns at once; the status timer takes seconds, so it is
	// not waited for.
	result := make(chan tea.Msg, 1)
	go func() { result <- cmd() }()
	select {
	case msg := <-result:
		if _, quit := msg.(tea.QuitMsg); quit {
			t.Error("deleting the last chat must not quit the application")
		}
	case <-time.After(100 * time.Millisecond):
	}
	if output := stripANSI(m.View()); !strings.Contains(output, "prompt history not cleaned up") {
		t.Errorf("the warning should stay visible\noutput:\n%s", output)
	}
}

func TestSettings_GroupToggleAppliesToMemory(t *testing.T) {
	m := settingsModel(t, t.TempDir())
	m.settingsCursor = settingGroupByProject
	m.expandedProjects = make(map[string]bool)
	m.memory = makeTestMemory(2, 3)
	m.rebuildMemRows()
	if len(m.memRows) != 6 {
		t.Fatalf("flat rows = %d, want 6", len(m.memRows))
	}

	m = send(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.grouped || len(m.memRows) != 2 || !m.memRows[0].isHeader {
		t.Errorf("after turning grouping on: grouped=%v rows=%d, want 2 project headers", m.grouped, len(m.memRows))
	}
	m = send(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.grouped || len(m.memRows) != 6 {
		t.Errorf("after turning grouping off: grouped=%v rows=%d, want 6 files", m.grouped, len(m.memRows))
	}
}
