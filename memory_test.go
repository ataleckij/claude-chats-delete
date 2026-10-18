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

func TestUpdateMemory_IsReadOnly(t *testing.T) {
	m := makeMemoryModel(makeTestMemory(1, 2), false, normalWidth, 20)
	m.chats = makeTestChats(3)

	for _, r := range []rune{' ', 'a', 'd'} {
		m = send(m, keyRune(r))
	}
	if len(m.selected) != 0 || m.confirmDelete {
		t.Errorf("selection keys must do nothing on the memory tab: selected=%v confirm=%v", m.selected, m.confirmDelete)
	}
	if len(m.memory) != 2 {
		t.Errorf("memory list changed: %d files", len(m.memory))
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
