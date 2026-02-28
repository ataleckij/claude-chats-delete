package main

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

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
