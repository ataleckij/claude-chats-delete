package main

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func init() {
	// Disable ANSI colors so View() output is plain text.
	// Without this, lipgloss wraps strings in escape codes that interfere
	// with line counting.
	lipgloss.SetColorProfile(termenv.Ascii)
}

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[mK]`)

func stripANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}

func viewLineCount(s string) int {
	return strings.Count(stripANSI(s), "\n")
}

func makeTestModel(chats []Chat, width, height int) model {
	return model{
		chats:    chats,
		selected: make(map[int]bool),
		width:    width,
		height:   height,
	}
}

func makeTestChats(n int) []Chat {
	chats := make([]Chat, n)
	for i := range chats {
		chats[i] = Chat{
			UUID:      fmt.Sprintf("uuid-%d", i),
			Title:     fmt.Sprintf("Chat number %d", i),
			Timestamp: "2026-01-01 00:00:00",
			Project:   "test-project",
			Version:   "2.1.49",
			LineCount: 5,
		}
	}
	return chats
}

// Layout constants — update these if the View() layout changes.
// If a test fails, it means View() gained or lost a fixed line.
const (
	fixedHeaderLines        = 4 // tabbar(+stats) + top-separator + col-headers + separator
	fixedFooterLines        = 2 // bottom-separator + help (OR confirmation, same count)
	fixedFooterLinesCompact = 3 // bottom-separator + 2 help lines (actions + navigation)

	normalWidth  = compactModeWidth + 10 // full mode: timestamp 19, version visible, 1 help line
	compactWidth = compactModeWidth - 30 // compact mode: timestamp 11, no version, 2 help lines
)

func TestView_EmptyChats(t *testing.T) {
	m := makeTestModel(nil, normalWidth, 20)
	output := stripANSI(m.View())

	// Empty state is a special path returning "No chats found.\n\nPress q to quit.\n"
	got := strings.Count(output, "\n")
	if got != 3 {
		t.Errorf("empty state: expected 3 lines, got %d\noutput: %q", got, output)
	}
}

func TestView_FewChats_NoScroll(t *testing.T) {
	// 5 chats, height=20 → visibleHeight=12 → all 5 fit, no scroll indicator
	chats := makeTestChats(5)
	m := makeTestModel(chats, normalWidth, 20)
	// expected: header(4) + chats(5) + scroll(0) + status(0) + footer(2) = 11
	expected := fixedHeaderLines + 5 + 0 + 0 + fixedFooterLines
	got := viewLineCount(m.View())
	if got != expected {
		t.Errorf("few chats no scroll: expected %d lines, got %d", expected, got)
	}
}

func TestView_ManyChats_WithScroll(t *testing.T) {
	// 30 chats, height=20 → visibleHeight=11 → scroll indicator shown
	chats := makeTestChats(30)
	m := makeTestModel(chats, normalWidth, 20)
	// expected: header(4) + chats(11) + scroll(1) + status(0) + footer(2) = 18
	expected := fixedHeaderLines + 11 + 1 + 0 + fixedFooterLines

	got := viewLineCount(m.View())
	if got != expected {
		t.Errorf("many chats with scroll: expected %d lines, got %d", expected, got)
	}
}

func TestView_WithErrorMessage(t *testing.T) {
	// Error message occupies the status slot
	chats := makeTestChats(5)
	m := makeTestModel(chats, normalWidth, 20)
	m.error = "something went wrong"
	// expected: header(4) + chats(5) + scroll(0) + status(1) + footer(2) = 12
	expected := fixedHeaderLines + 5 + 0 + 1 + fixedFooterLines
	got := viewLineCount(m.View())
	if got != expected {
		t.Errorf("with error: expected %d lines, got %d", expected, got)
	}
}

func TestView_WithDeletedMessage(t *testing.T) {
	// deleted > 0 occupies the status slot
	chats := makeTestChats(5)
	m := makeTestModel(chats, normalWidth, 20)
	m.deleted = 3
	// expected: header(4) + chats(5) + scroll(0) + status(1) + footer(2) = 12
	expected := fixedHeaderLines + 5 + 0 + 1 + fixedFooterLines
	got := viewLineCount(m.View())
	if got != expected {
		t.Errorf("with deleted msg: expected %d lines, got %d", expected, got)
	}
}

func TestView_ConfirmDialog_ReplacesHelp(t *testing.T) {
	// confirmDelete replaces help line — total count must be SAME as normal
	chats := makeTestChats(5)
	m := makeTestModel(chats, normalWidth, 20)
	m.selected[0] = true
	m.confirmDelete = true
	// expected: header(4) + chats(5) + scroll(0) + status(0) + footer(2) = 11
	// same as TestView_FewChats_NoScroll — confirmation replaces help but separator stays
	expected := fixedHeaderLines + 5 + 0 + 0 + fixedFooterLines
	got := viewLineCount(m.View())
	if got != expected {
		t.Errorf("confirm dialog: expected %d lines (same as help), got %d", expected, got)
	}
}

func TestView_SmallTerminal_UsesMinVisibleHeight(t *testing.T) {
	// height=5 → visibleHeight falls back to 10 (not negative)
	// 3 chats < 10 (fallback visible height) → no scroll indicator
	chats := makeTestChats(3)
	m := makeTestModel(chats, normalWidth, 5)
	// expected: header(4) + chats(3) + scroll(0) + status(0) + footer(2) = 9
	expected := fixedHeaderLines + 3 + 0 + 0 + fixedFooterLines
	got := viewLineCount(m.View())
	if got != expected {
		t.Errorf("small terminal: expected %d lines, got %d", expected, got)
	}
}

func TestView_TitleWithNewline_DoesNotBreakLayout(t *testing.T) {
	// Regression: if chat.Title contains \n it used to create extra lines,
	// breaking the entire UI layout. getChatTitle cleans this via cleanSystemTags,
	// but this test documents the contract at the View() level.
	chats := []Chat{
		{
			UUID:      "uuid-regression",
			Title:     "first line\nsecond line",
			Timestamp: "2026-01-01 00:00:00",
			Project:   "proj",
			Version:   "2.1.49",
			LineCount: 1,
		},
	}
	m := makeTestModel(chats, normalWidth, 20)
	output := stripANSI(m.View())

	// 1 chat, no scroll, no status: header(4) + chat(1) + footer(2) = 7
	expected := fixedHeaderLines + 1 + 0 + 0 + fixedFooterLines
	got := strings.Count(output, "\n")
	if got != expected {
		t.Errorf("title with newline broke layout: expected %d lines, got %d\noutput:\n%s",
			expected, got, output)
	}
}

func TestView_ProjectWithNewline_DoesNotBreakLayout(t *testing.T) {
	// Regression: project name with \n used to break layout.
	chats := []Chat{
		{
			UUID:      "uuid-proj-regression",
			Title:     "normal title",
			Timestamp: "2026-01-01 00:00:00",
			Project:   "project\nwith newline",
			Version:   "2.1.49",
			LineCount: 1,
		},
	}
	m := makeTestModel(chats, normalWidth, 20)
	output := stripANSI(m.View())

	expected := fixedHeaderLines + 1 + 0 + 0 + fixedFooterLines
	got := strings.Count(output, "\n")
	if got != expected {
		t.Errorf("project with newline broke layout: expected %d lines, got %d\noutput:\n%s",
			expected, got, output)
	}
}

func TestView_CompactMode_TwoHelpLines(t *testing.T) {
	// width < compactModeWidth: compact mode renders 2 help lines instead of 1
	chats := makeTestChats(5)
	m := makeTestModel(chats, compactWidth, 20)
	// expected: header(4) + chats(5) + scroll(0) + status(0) + footer(3) = 12
	expected := fixedHeaderLines + 5 + 0 + 0 + fixedFooterLinesCompact
	got := viewLineCount(m.View())
	if got != expected {
		t.Errorf("compact mode two help lines: expected %d lines, got %d", expected, got)
	}
}

// makeTestChatsMultiProject creates chats spread across multiple projects.
// Pattern: chats 0..perProject-1 in "project-A", next perProject in "project-B", etc.
func makeTestChatsMultiProject(projectCount, perProject int) []Chat {
	projects := []string{"project-A", "project-B", "project-C", "project-D", "project-E"}
	var chats []Chat
	for p := 0; p < projectCount && p < len(projects); p++ {
		for c := 0; c < perProject; c++ {
			idx := p*perProject + c
			chats = append(chats, Chat{
				UUID:      fmt.Sprintf("uuid-%d", idx),
				Title:     fmt.Sprintf("Chat %d in %s", c, projects[p]),
				Timestamp: fmt.Sprintf("2026-01-%02d 00:00:00", 10-p), // newer projects first
				Project:   projects[p],
				Version:   "2.1.49",
				LineCount: 5,
			})
		}
	}
	return chats
}

func makeGroupedModel(chats []Chat, width, height int) model {
	m := model{
		chats:            chats,
		selected:         make(map[int]bool),
		width:            width,
		height:           height,
		grouped:          true,
		expandedProjects: make(map[string]bool),
	}
	m.rebuildGroupRows()
	return m
}

// --- Grouped view tests ---

func TestViewGrouped_EmptyChats(t *testing.T) {
	m := makeGroupedModel(nil, normalWidth, 20)
	output := stripANSI(m.View())
	got := strings.Count(output, "\n")
	if got != 3 {
		t.Errorf("grouped empty state: expected 3 lines, got %d\noutput: %q", got, output)
	}
}

func TestViewGrouped_CollapsedProjects_LineCount(t *testing.T) {
	// 3 projects, all collapsed = 3 header rows
	chats := makeTestChatsMultiProject(3, 4)
	m := makeGroupedModel(chats, normalWidth, 30)
	// Layout: header(4) + rows(3) + scroll(0) + status(0) + footer(2) = 9
	expected := fixedHeaderLines + 3 + 0 + 0 + fixedFooterLines
	got := viewLineCount(m.View())
	if got != expected {
		t.Errorf("grouped collapsed: expected %d lines, got %d", expected, got)
	}
}

func TestViewGrouped_ExpandedProject_LineCount(t *testing.T) {
	// 2 projects with 3 chats each, one expanded
	chats := makeTestChatsMultiProject(2, 3)
	m := makeGroupedModel(chats, normalWidth, 30)
	m.expandedProjects["project-A"] = true
	m.rebuildGroupRows()
	// Rows: header-A(1) + 3 chats + header-B(1) = 5 rows
	expected := fixedHeaderLines + 5 + 0 + 0 + fixedFooterLines
	got := viewLineCount(m.View())
	if got != expected {
		t.Errorf("grouped expanded: expected %d lines, got %d", expected, got)
	}
}

func TestViewGrouped_WithScroll(t *testing.T) {
	// Many projects to trigger scroll
	chats := makeTestChatsMultiProject(5, 4)
	m := makeGroupedModel(chats, normalWidth, 15)
	// Expand all projects: 5 headers + 20 chats = 25 rows
	for _, name := range []string{"project-A", "project-B", "project-C", "project-D", "project-E"} {
		m.expandedProjects[name] = true
	}
	m.rebuildGroupRows()
	vh := m.visibleHeight() // 15 - 9 = 6
	// 25 rows > 6 visible → scroll indicator
	expected := fixedHeaderLines + vh + 1 + 0 + fixedFooterLines
	got := viewLineCount(m.View())
	if got != expected {
		t.Errorf("grouped scroll: expected %d lines, got %d (vh=%d, rows=%d)",
			expected, got, vh, len(m.groupRows))
	}
}

func TestViewGrouped_ConfirmDialog_ReplacesHelp(t *testing.T) {
	chats := makeTestChatsMultiProject(2, 3)
	m := makeGroupedModel(chats, normalWidth, 30)
	m.selected[0] = true
	m.confirmDelete = true
	// Same line count as without confirm (dialog replaces help)
	expected := fixedHeaderLines + 2 + 0 + 0 + fixedFooterLines
	got := viewLineCount(m.View())
	if got != expected {
		t.Errorf("grouped confirm: expected %d lines, got %d", expected, got)
	}
}

func TestUpdateGrouped_SpaceOnHeaderTogglesProject(t *testing.T) {
	chats := makeTestChatsMultiProject(2, 3)
	m := makeGroupedModel(chats, normalWidth, 30)

	// Cursor is on first project header (index 0)
	// Press space to select all chats in project-A
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}}
	result, _ := m.updateGrouped(keyMsg)
	m = result.(model)

	// project-A has chats at indices 0, 1, 2
	for _, idx := range []int{0, 1, 2} {
		if !m.selected[idx] {
			t.Errorf("chat %d should be selected after space on project header", idx)
		}
	}
	// project-B chats should NOT be selected
	for _, idx := range []int{3, 4, 5} {
		if m.selected[idx] {
			t.Errorf("chat %d should NOT be selected (different project)", idx)
		}
	}

	// Press space again to deselect all in project-A
	result, _ = m.updateGrouped(keyMsg)
	m = result.(model)
	if len(m.selected) != 0 {
		t.Errorf("expected all deselected, got %d selected", len(m.selected))
	}
}

func TestUpdateGrouped_EnterExpandCollapse(t *testing.T) {
	chats := makeTestChatsMultiProject(2, 3)
	m := makeGroupedModel(chats, normalWidth, 30)

	// Initially 2 rows (2 collapsed headers)
	if len(m.groupRows) != 2 {
		t.Fatalf("expected 2 collapsed rows, got %d", len(m.groupRows))
	}

	// Press enter to expand first project
	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	result, _ := m.updateGrouped(enterMsg)
	m = result.(model)

	// Now: header-A(1) + 3 chats + header-B(1) = 5 rows
	if len(m.groupRows) != 5 {
		t.Errorf("expected 5 rows after expand, got %d", len(m.groupRows))
	}
	// Cursor should still be on the project-A header (index 0)
	if m.cursor != 0 {
		t.Errorf("cursor should be 0 after expand, got %d", m.cursor)
	}

	// Press enter again to collapse
	result, _ = m.updateGrouped(enterMsg)
	m = result.(model)

	if len(m.groupRows) != 2 {
		t.Errorf("expected 2 rows after collapse, got %d", len(m.groupRows))
	}
}

func TestUpdateGrouped_EnterOnChatRowDoesNothing(t *testing.T) {
	chats := makeTestChatsMultiProject(1, 3)
	m := makeGroupedModel(chats, normalWidth, 30)
	m.expandedProjects["project-A"] = true
	m.rebuildGroupRows()

	// Move cursor to first chat row (index 1)
	m.cursor = 1
	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	result, _ := m.updateGrouped(enterMsg)
	m = result.(model)

	// Should still have 4 rows (1 header + 3 chats), nothing changed
	if len(m.groupRows) != 4 {
		t.Errorf("enter on chat row should not change rows, got %d", len(m.groupRows))
	}
}

func TestViewGrouped_ProjectWithNewline(t *testing.T) {
	chats := []Chat{
		{UUID: "u1", Title: "test", Timestamp: "2026-01-01 00:00:00",
			Project: "proj\nwith newline", Version: "2.1.49", LineCount: 1},
	}
	m := makeGroupedModel(chats, normalWidth, 20)
	output := stripANSI(m.View())
	// 1 collapsed header row
	expected := fixedHeaderLines + 1 + 0 + 0 + fixedFooterLines
	got := strings.Count(output, "\n")
	if got != expected {
		t.Errorf("grouped project with newline: expected %d lines, got %d\noutput:\n%s",
			expected, got, output)
	}
}

// send drives the model through m.Update with the given key message and
// returns the resulting model, panicking on the (unused) command.
func send(m model, msg tea.KeyMsg) model {
	next, _ := m.Update(msg)
	return next.(model)
}

func keyRune(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// Regression: d → esc must fully clear auto-selection so the next d
// acts on the chat under the new cursor position, not the stale one.
func TestUpdateD_AutoSelectClearsOnCancel(t *testing.T) {
	chats := makeTestChats(3)
	m := makeTestModel(chats, normalWidth, 20)

	// Press d on chat 0: auto-select + confirm dialog.
	m = send(m, keyRune('d'))
	if !m.confirmDelete {
		t.Fatal("confirm dialog not shown after d")
	}
	if !m.selected[0] {
		t.Fatal("chat 0 not auto-selected after d")
	}
	if !m.autoSelected {
		t.Fatal("autoSelected flag not set after auto-select")
	}

	// Esc cancels and must revert the auto-selection.
	m = send(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.confirmDelete {
		t.Fatal("confirm still shown after esc")
	}
	if len(m.selected) != 0 {
		t.Fatalf("auto-selection leaked after esc: %v", m.selected)
	}
	if m.autoSelected {
		t.Fatal("autoSelected flag not cleared after esc")
	}

	// Move cursor to chat 1.
	m = send(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Fatalf("cursor = %d after down, want 1", m.cursor)
	}

	// Press d again: must auto-select the new chat, not the stale one.
	m = send(m, keyRune('d'))
	if !m.confirmDelete {
		t.Fatal("second confirm not shown")
	}
	if m.selected[0] {
		t.Fatal("chat 0 still selected on second d (regression)")
	}
	if !m.selected[1] {
		t.Fatal("chat 1 not auto-selected on second d")
	}
}

// Explicit selection via Space must win over cursor position: d with an
// existing selection deletes the explicit set, ignoring where the cursor
// is, and esc must not wipe the explicit selection.
func TestUpdateD_ExplicitSelectionWinsOverCursor(t *testing.T) {
	chats := makeTestChats(3)
	m := makeTestModel(chats, normalWidth, 20)

	// Space on chat 0: explicit selection, flag stays false.
	m = send(m, keyRune(' '))
	if !m.selected[0] {
		t.Fatal("chat 0 not selected by space")
	}
	if m.autoSelected {
		t.Fatal("autoSelected must not be set by space")
	}

	// Move cursor to chat 2.
	m = send(m, tea.KeyMsg{Type: tea.KeyDown})
	m = send(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 2 {
		t.Fatalf("cursor = %d, want 2", m.cursor)
	}

	// d should open confirm for the already-selected chat 0, not add chat 2.
	m = send(m, keyRune('d'))
	if !m.confirmDelete {
		t.Fatal("confirm not shown")
	}
	if !m.selected[0] {
		t.Fatal("chat 0 dropped from selection on d")
	}
	if m.selected[2] {
		t.Fatal("chat 2 auto-added despite explicit selection — rule broken")
	}
	if m.autoSelected {
		t.Fatal("autoSelected flag must not be set when explicit selection exists")
	}

	// Esc must not wipe the explicit selection.
	m = send(m, tea.KeyMsg{Type: tea.KeyEsc})
	if !m.selected[0] {
		t.Fatal("explicit selection wiped on esc — rule broken")
	}
}

// Grouped view: d on a collapsed project header must auto-select every
// chat in that project and open the confirm dialog.
func TestUpdateGroupedD_HeaderAutoSelectsProjectChats(t *testing.T) {
	// 2 projects, 3 chats each; cursor starts on header-A (row 0).
	chats := makeTestChatsMultiProject(2, 3)
	m := makeGroupedModel(chats, normalWidth, 30)

	m = send(m, keyRune('d'))

	if !m.confirmDelete {
		t.Fatal("confirm dialog not shown after d on project header")
	}
	if !m.autoSelected {
		t.Fatal("autoSelected flag not set after header auto-select")
	}
	// project-A owns chat indices 0, 1, 2.
	for _, idx := range []int{0, 1, 2} {
		if !m.selected[idx] {
			t.Errorf("chat %d (project-A) not auto-selected", idx)
		}
	}
	// project-B chats must NOT be selected.
	for _, idx := range []int{3, 4, 5} {
		if m.selected[idx] {
			t.Errorf("chat %d (project-B) auto-selected; header should only pick its own project", idx)
		}
	}
}

// Grouped view: d on a header → esc must revert the auto-selection,
// so the next d on another header acts on the new project only.
func TestUpdateGroupedD_HeaderAutoSelectClearsOnCancel(t *testing.T) {
	chats := makeTestChatsMultiProject(2, 3)
	m := makeGroupedModel(chats, normalWidth, 30)

	// d on header-A → auto-select project-A.
	m = send(m, keyRune('d'))
	if len(m.selected) != 3 || !m.autoSelected {
		t.Fatalf("setup failed: selected=%v autoSelected=%v", m.selected, m.autoSelected)
	}

	// Esc cancels.
	m = send(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.confirmDelete {
		t.Fatal("confirm still shown after esc")
	}
	if len(m.selected) != 0 {
		t.Fatalf("header auto-selection leaked after esc: %v", m.selected)
	}
	if m.autoSelected {
		t.Fatal("autoSelected flag not cleared after esc")
	}

	// Move cursor to header-B and press d again.
	m = send(m, tea.KeyMsg{Type: tea.KeyDown})
	m = send(m, keyRune('d'))

	// Only project-B chats (3, 4, 5) should be selected now.
	for _, idx := range []int{0, 1, 2} {
		if m.selected[idx] {
			t.Errorf("project-A chat %d still selected on second d (regression)", idx)
		}
	}
	for _, idx := range []int{3, 4, 5} {
		if !m.selected[idx] {
			t.Errorf("project-B chat %d not auto-selected on second d", idx)
		}
	}
}

// Grouped view: explicit Space-selection on one project header must survive
// pressing d on a different project's header. d with non-empty selection
// ignores the cursor and shows confirm for the existing selection.
func TestUpdateGroupedD_ExplicitWinsOnHeader(t *testing.T) {
	chats := makeTestChatsMultiProject(2, 3)
	m := makeGroupedModel(chats, normalWidth, 30)

	// Space on header-A toggles all of project-A's chats explicitly.
	m = send(m, keyRune(' '))
	for _, idx := range []int{0, 1, 2} {
		if !m.selected[idx] {
			t.Fatalf("setup: chat %d not selected by space", idx)
		}
	}
	if m.autoSelected {
		t.Fatal("space must not set autoSelected")
	}

	// Move cursor to header-B.
	m = send(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", m.cursor)
	}

	// d opens confirm for the already-selected project-A chats.
	m = send(m, keyRune('d'))
	if !m.confirmDelete {
		t.Fatal("confirm not shown")
	}
	if m.autoSelected {
		t.Fatal("autoSelected must not be set when explicit selection exists")
	}
	for _, idx := range []int{0, 1, 2} {
		if !m.selected[idx] {
			t.Errorf("project-A chat %d dropped on d", idx)
		}
	}
	for _, idx := range []int{3, 4, 5} {
		if m.selected[idx] {
			t.Errorf("project-B chat %d auto-added despite explicit selection — rule broken", idx)
		}
	}

	// Esc must not wipe the explicit selection.
	m = send(m, tea.KeyMsg{Type: tea.KeyEsc})
	for _, idx := range []int{0, 1, 2} {
		if !m.selected[idx] {
			t.Errorf("explicit project-A chat %d wiped on esc — rule broken", idx)
		}
	}
}

func TestVisibleHeight(t *testing.T) {
	tests := []struct {
		width  int
		height int
		want   int
	}{
		{width: normalWidth, height: 20, want: 11},  // 20 - 9 = 11
		{width: normalWidth, height: 40, want: 31},  // 40 - 9 = 31
		{width: normalWidth, height: 10, want: 1},   // 10 - 9 = 1
		{width: normalWidth, height: 9, want: 10},   // 9 - 9 = 0 < 1 → fallback 10
		{width: compactWidth, height: 20, want: 10}, // compact: 20 - 10 = 10
		{width: compactWidth, height: 5, want: 10},  // compact: 5 - 10 < 1 → fallback 10
	}
	for _, tt := range tests {
		m := model{width: tt.width, height: tt.height}
		got := m.visibleHeight()
		if got != tt.want {
			t.Errorf("visibleHeight() with width=%d height=%d: got %d, want %d",
				tt.width, tt.height, got, tt.want)
		}
	}
}
