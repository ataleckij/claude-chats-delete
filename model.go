package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
)

// compactModeWidth is the terminal width threshold below which compact layout
// is used: shortened timestamp, no VERSION column, two-line help text.
const compactModeWidth = 110

const (
	tabChats    = 0
	tabSettings = 1
)

var tabs = []string{"Chats", "Settings"}

var (
	// Styles
	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Background(adaptiveColor("6", "6")).
			Foreground(adaptiveColor("0", "0")).
			Padding(0, 1)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(adaptiveColor("241", "8")).
				Padding(0, 1)

	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(adaptiveColor("226", "11"))

	cursorStyle = lipgloss.NewStyle().
			Reverse(true)

	dimStyle = lipgloss.NewStyle().
			Foreground(adaptiveColor("240", "8"))

	errorStyle = lipgloss.NewStyle().
			Foreground(adaptiveColor("196", "9")).
			Bold(true)

	successStyle = lipgloss.NewStyle().
			Foreground(adaptiveColor("46", "10")).
			Bold(true)

	helpStyle = lipgloss.NewStyle().
			Foreground(adaptiveColor("241", "8"))
)

// adaptiveColor returns a color that adapts to terminal capabilities
// Uses rich 256-color codes on modern terminals, falls back to basic 16 colors otherwise
func adaptiveColor(rich string, fallback string) lipgloss.TerminalColor {
	profile := lipgloss.ColorProfile()
	if profile == termenv.ANSI {
		return lipgloss.Color(fallback)
	}
	return lipgloss.Color(rich)
}

// Messages
type deleteCompleteMsg struct {
	count int
}

type errMsg string

type clearCopiedMsg struct {
	id int
}

type clearDeleteMsg struct {
	id int
}

// groupRow represents a single row in the grouped view.
// It is either a project header or a reference to a chat.
type groupRow struct {
	isHeader bool
	project  string // project name (set for both headers and chats)
	chatIdx  int    // index into m.chats (-1 for headers)
}

type model struct {
	tab           int
	cfg           *Config
	chats         []Chat
	cursor        int
	selected      map[int]bool
	confirmDelete bool
	deleting      bool
	deleted       int
	error         string
	width         int
	height        int
	scrollOffset  int
	copiedMsg     string
	deleteTimer   int // Track active delete message timer
	copyTimer     int // Track active copy message timer

	// True when the current m.selected was filled automatically by pressing
	// d with no prior selection. On confirm cancel we revert the auto-selection
	// so the selection state doesn't leak into the next d gesture.
	autoSelected bool

	// Settings tab
	settingsCursor int

	// Grouped view state
	grouped          bool
	expandedProjects map[string]bool
	groupRows        []groupRow // virtual row list built from chats + expanded state
}

func initialModel(cfg *Config) model {
	grouped := cfg != nil && cfg.GroupByProject
	m := model{
		cfg:              cfg,
		chats:            findAllChats(),
		selected:         make(map[int]bool),
		grouped:          grouped,
		expandedProjects: make(map[string]bool),
	}
	if m.grouped {
		m.rebuildGroupRows()
	}
	return m
}

// rebuildGroupRows creates the virtual row list from chats grouped by project.
// Projects are ordered by the most recent chat timestamp (newest first).
func (m *model) rebuildGroupRows() {
	// Collect unique projects in order of first appearance
	// (chats are already sorted by timestamp desc)
	seen := make(map[string]bool)
	var projects []string
	for _, chat := range m.chats {
		if !seen[chat.Project] {
			seen[chat.Project] = true
			projects = append(projects, chat.Project)
		}
	}

	// Build chat index groups
	chatsByProject := make(map[string][]int)
	for i, chat := range m.chats {
		chatsByProject[chat.Project] = append(chatsByProject[chat.Project], i)
	}

	var rows []groupRow
	for _, proj := range projects {
		rows = append(rows, groupRow{isHeader: true, project: proj, chatIdx: -1})
		if m.expandedProjects[proj] {
			for _, idx := range chatsByProject[proj] {
				rows = append(rows, groupRow{isHeader: false, project: proj, chatIdx: idx})
			}
		}
	}
	m.groupRows = rows
}

// chatIndicesForProject returns all chat indices belonging to a project.
func (m model) chatIndicesForProject(project string) []int {
	var indices []int
	for i, chat := range m.chats {
		if chat.Project == project {
			indices = append(indices, i)
		}
	}
	return indices
}

func (m model) renderTabBar() string {
	appName := dimStyle.Render("Claude Code Manager")
	var tabParts []string
	for i, name := range tabs {
		if i == m.tab {
			tabParts = append(tabParts, activeTabStyle.Render(name))
		} else {
			tabParts = append(tabParts, inactiveTabStyle.Render(name))
		}
	}
	left := appName + "   " + strings.Join(tabParts, " ")
	if m.tab != tabChats {
		return left
	}
	stats := dimStyle.Render(fmt.Sprintf("Total: %d | Selected: %d", len(m.chats), len(m.selected)))
	width := m.width
	if width < 75 {
		width = 75
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(stats)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + stats
}

func (m model) visibleHeight() int {
	fixed := 9 // tabbar(1) + sep(1) + col-header(1) + sep(1) + bottom-sep(1) + help(1) + scroll(0-1) + status(0-1)
	if m.width < compactModeWidth {
		fixed = 10 // compact: +1 for extra help line
	}
	h := m.height - fixed
	if h < 1 {
		h = 10
	}
	return h
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// Confirmation dialog intercepts esc before global keys
		if m.confirmDelete {
			switch msg.String() {
			case "enter":
				return m, m.deleteSelectedChats()
			case "esc", "n":
				m.confirmDelete = false
				if m.autoSelected {
					m.selected = make(map[int]bool)
					m.autoSelected = false
				}
			}
			return m, nil
		}

		// Global keys
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		case "left":
			if m.tab > 0 {
				m.tab--
			}
			return m, nil
		case "right":
			if m.tab < len(tabs)-1 {
				m.tab++
			}
			return m, nil
		}

		// Settings tab
		if m.tab == tabSettings {
			switch msg.String() {
			case "up", "k":
				if m.settingsCursor > 0 {
					m.settingsCursor--
				}
			case "down", "j":
				if m.settingsCursor < settingsCount-1 {
					m.settingsCursor++
				}
			case "enter":
				if m.cfg != nil {
					switch m.settingsCursor {
					case settingAutoUpdates:
						m.cfg.AutoUpdates = !m.cfg.AutoUpdates
					case settingGroupByProject:
						m.cfg.GroupByProject = !m.cfg.GroupByProject
						m.grouped = m.cfg.GroupByProject
						if m.grouped {
							m.rebuildGroupRows()
							m.cursor = 0
							m.scrollOffset = 0
						} else {
							m.groupRows = nil
							m.cursor = 0
							m.scrollOffset = 0
						}
					}
					saveConfig(m.cfg)
				}
			}
			return m, nil
		}

		// Chats tab: grouped mode
		if m.grouped {
			return m.updateGrouped(msg)
		}

		// Chats tab normal mode
		switch msg.String() {

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.adjustScroll()
			}

		case "down", "j":
			if m.cursor < len(m.chats)-1 {
				m.cursor++
				m.adjustScroll()
			}

		case "f", "pgdown":
			visibleHeight := m.visibleHeight()
			m.cursor += visibleHeight
			if m.cursor >= len(m.chats) {
				m.cursor = len(m.chats) - 1
			}
			m.adjustScroll()

		case "b", "pgup":
			visibleHeight := m.visibleHeight()
			m.cursor -= visibleHeight
			if m.cursor < 0 {
				m.cursor = 0
			}
			m.adjustScroll()

		case "F":
			visibleHeight := m.visibleHeight()
			m.cursor += visibleHeight / 2
			if m.cursor >= len(m.chats) {
				m.cursor = len(m.chats) - 1
			}
			m.adjustScroll()

		case "B":
			visibleHeight := m.visibleHeight()
			m.cursor -= visibleHeight / 2
			if m.cursor < 0 {
				m.cursor = 0
			}
			m.adjustScroll()

		case "g", "home":
			m.cursor = 0
			m.adjustScroll()

		case "G", "end":
			if len(m.chats) > 0 {
				m.cursor = len(m.chats) - 1
			}
			m.adjustScroll()

		case " ":
			// Explicit toggle — user now owns the selection.
			m.autoSelected = false
			if m.selected[m.cursor] {
				delete(m.selected, m.cursor)
			} else {
				m.selected[m.cursor] = true
			}

		case "a":
			// Select all / deselect all toggle
			if len(m.chats) == 0 {
				return m, nil // Nothing to select
			}
			m.autoSelected = false
			if len(m.selected) == len(m.chats) {
				m.selected = make(map[int]bool)
			} else {
				for i := range m.chats {
					m.selected[i] = true
				}
			}

		case "d":
			// Explicit selection wins: if anything is already selected
			// (via Space or a), delete those. Otherwise auto-select the
			// chat under the cursor for this single gesture.
			if len(m.selected) == 0 && m.cursor < len(m.chats) {
				m.selected[m.cursor] = true
				m.autoSelected = true
			}
			if len(m.selected) > 0 {
				m.confirmDelete = true
			}

		case "r":
			// Refresh
			m.chats = findAllChats()
			m.selected = make(map[int]bool)
			m.autoSelected = false
			m.cursor = 0
			m.scrollOffset = 0
			m.error = ""
			m.deleted = 0
			m.copiedMsg = ""

		case "c":
			// Copy UUID to clipboard
			if m.cursor < len(m.chats) {
				uuid := m.chats[m.cursor].UUID
				if err := copyToClipboard(uuid); err != nil {
					m.error = fmt.Sprintf("Failed to copy: %v", err)
				} else {
					m.copyTimer++
					currentTimer := m.copyTimer
					m.copiedMsg = fmt.Sprintf("Chat UUID copied: %s", uuid)
					m.error = ""
					m.deleted = 0
					return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
						return clearCopiedMsg{id: currentTimer}
					})
				}
			}
		}

	case deleteCompleteMsg:
		m.deleting = false
		m.deleted = msg.count
		m.deleteTimer++
		currentTimer := m.deleteTimer
		m.chats = findAllChats()
		m.selected = make(map[int]bool)
		m.autoSelected = false
		m.cursor = 0
		m.scrollOffset = 0
		m.confirmDelete = false
		if m.grouped {
			m.rebuildGroupRows()
		}
		// Clear other status messages
		m.error = ""
		m.copiedMsg = ""
		if len(m.chats) == 0 {
			return m, tea.Quit
		}
		return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return clearDeleteMsg{id: currentTimer}
		})

	case errMsg:
		m.deleting = false
		m.error = string(msg)

	case clearCopiedMsg:
		if msg.id == m.copyTimer {
			m.copiedMsg = ""
		}

	case clearDeleteMsg:
		if msg.id == m.deleteTimer {
			m.deleted = 0
		}
	}

	return m, nil
}

func (m *model) adjustScroll() {
	visibleHeight := m.visibleHeight()
	// confirmDelete dialog replaces help text, no additional space needed

	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	} else if m.cursor >= m.scrollOffset+visibleHeight {
		m.scrollOffset = m.cursor - visibleHeight + 1
	}
}

const (
	settingAutoUpdates   = 0
	settingGroupByProject = 1
	settingsCount        = 2
)

func (m model) viewSettings() string {
	width := m.width
	if width < 75 {
		width = 75
	}

	var s strings.Builder
	s.WriteString(m.renderTabBar())
	s.WriteString("\n")
	s.WriteString(dimStyle.Render(strings.Repeat("─", width)))
	s.WriteString("\n\n")

	// Auto-updates setting
	autoVal := "OFF"
	autoStyle := errorStyle
	if m.cfg != nil && m.cfg.AutoUpdates {
		autoVal = "ON"
		autoStyle = successStyle
	}
	autoHint := ""
	if m.cfg == nil || !m.cfg.AutoUpdates {
		autoHint = "  " + dimStyle.Render("(use `claude-chats --update` for manual update)")
	}
	autoLine := fmt.Sprintf("  Auto-updates      %s%s", autoStyle.Render(autoVal), autoHint)
	if m.settingsCursor == settingAutoUpdates {
		s.WriteString(cursorStyle.Render(autoLine))
	} else {
		s.WriteString(autoLine)
	}
	s.WriteString("\n")

	// Group by project setting
	groupVal := "OFF"
	groupStyle := errorStyle
	if m.cfg != nil && m.cfg.GroupByProject {
		groupVal = "ON"
		groupStyle = successStyle
	}
	groupLine := fmt.Sprintf("  Group by project  %s", groupStyle.Render(groupVal))
	if m.settingsCursor == settingGroupByProject {
		s.WriteString(cursorStyle.Render(groupLine))
	} else {
		s.WriteString(groupLine)
	}
	s.WriteString("\n")

	s.WriteString("\n")
	s.WriteString(dimStyle.Render(strings.Repeat("─", width)))
	s.WriteString("\n")
	s.WriteString(helpStyle.Render("↑/↓:Navigate | Enter:Toggle | ←/→:Switch tabs | q:Quit"))
	s.WriteString("\n")
	return s.String()
}

func (m model) View() string {
	if m.tab == tabSettings {
		return m.viewSettings()
	}

	if m.grouped {
		return m.viewGrouped()
	}

	if len(m.chats) == 0 {
		return activeTabStyle.Render("No chats found.") + "\n\nPress q to quit.\n"
	}

	// Calculate column widths based on terminal width
	width := m.width
	if width < 75 {
		width = 75 // minimum width
	}

	compact := width < compactModeWidth

	// In compact mode: hide VERSION, shorten TIMESTAMP to "MM-DD HH:MM" (11 chars)
	// Fixed cols: indicator(4) + timestamp + version + lines(6) + gaps
	var timestampWidth, versionWidth int
	var fixedWidth int
	if compact {
		timestampWidth = 11 // "01-15 14:32"
		versionWidth = 0
		fixedWidth = 4 + timestampWidth + 5 + 5 // indicator + ts + lines + gaps
	} else {
		timestampWidth = 19 // "2025-01-15 14:32:10"
		versionWidth = 8
		fixedWidth = 44 // indicator(4) + ts(19) + version(8) + lines(5) + gaps(8)
	}

	linesWidth := 5
	remaining := width - fixedWidth
	titleWidth := remaining * 60 / 100 // 60% for title
	projectWidth := remaining - titleWidth

	if titleWidth < 30 {
		titleWidth = 30
	}
	if projectWidth < 10 {
		projectWidth = 10
	}

	var s strings.Builder

	// Header
	s.WriteString(m.renderTabBar())
	s.WriteString("\n")




	s.WriteString(dimStyle.Render(strings.Repeat("─", width)))
	s.WriteString("\n")

	// Column headers
	var header string
	if compact {
		headerFmt := fmt.Sprintf("    %%-*s  %%-%ds  %%-%ds  %%-%ds", linesWidth, titleWidth, projectWidth)
		header = fmt.Sprintf(headerFmt, timestampWidth, "TIMESTAMP", "LINES", "TITLE", "PROJECT")
	} else {
		headerFmt := fmt.Sprintf("    %%-*s  %%-%ds  %%-%ds  %%-%ds  %%-%ds", versionWidth, linesWidth, titleWidth, projectWidth)
		header = fmt.Sprintf(headerFmt, timestampWidth, "TIMESTAMP", "VERSION", "LINES", "TITLE", "PROJECT")
	}
	s.WriteString(dimStyle.Render(header))
	s.WriteString("\n")
	s.WriteString(dimStyle.Render(strings.Repeat("─", width)))
	s.WriteString("\n")

	// Chat list
	visibleHeight := m.visibleHeight()
	// confirmDelete dialog replaces help text, no additional space needed

	start := m.scrollOffset
	end := start + visibleHeight
	if end > len(m.chats) {
		end = len(m.chats)
	}

	for i := start; i < end; i++ {
		chat := m.chats[i]

		// Truncate fields using visual width
		var timestamp string
		if compact {
			// "2025-01-15 14:32:10" -> "01-15 14:32"
			if len(chat.Timestamp) >= 16 {
				timestamp = chat.Timestamp[5:16] // "MM-DD HH:MM"
			} else {
				timestamp = runewidth.Truncate(chat.Timestamp, timestampWidth, "")
			}
		} else {
			timestamp = runewidth.Truncate(chat.Timestamp, timestampWidth, "")
		}
		version := runewidth.Truncate(chat.Version, versionWidth-1, "")
		// TODO: msg column - maybe re-enable later
		// msg := fmt.Sprintf("%d", chat.MessageCount)
		// if chat.MessageCount == 0 {
		// 	msg = "-"
		// }
		var lines string
		switch {
		case chat.LineCount == 0:
			lines = "-"
		case chat.LineCount >= 10000:
			lines = fmt.Sprintf("%dk", chat.LineCount/1000)
		default:
			lines = fmt.Sprintf("%d", chat.LineCount)
		}

		titleClean := strings.NewReplacer("\n", " ").Replace(chat.Title)
		title := runewidth.Truncate(titleClean, titleWidth, "..")
		projectClean := strings.NewReplacer("\n", " ").Replace(chat.Project)
		project := truncateLeft(projectClean, projectWidth-2)

		// Selection indicator
		indicator := "[ ]"
		if m.selected[i] {
			indicator = "[✓]"
		}

		var line string
		if compact {
			lineFmt := fmt.Sprintf("%%s %%-*s  %%-%ds  %%-%ds  %%-%ds", linesWidth, titleWidth, projectWidth)
			line = fmt.Sprintf(lineFmt, indicator, timestampWidth, timestamp, lines, title, project)
		} else {
			lineFmt := fmt.Sprintf("%%s %%-*s  %%-%ds  %%-%ds  %%-%ds  %%-%ds", versionWidth, linesWidth, titleWidth, projectWidth)
			line = fmt.Sprintf(lineFmt, indicator, timestampWidth, timestamp, version, lines, title, project)
		}

		// Apply styles
		style := lipgloss.NewStyle()
		if m.selected[i] {
			style = selectedStyle
		}
		if i == m.cursor {
			style = cursorStyle
		}

		s.WriteString(style.Render(line))
		s.WriteString("\n")
	}

	// Scroll indicator
	if len(m.chats) > visibleHeight {
		scrollInfo := fmt.Sprintf("[%d-%d/%d]", start+1, end, len(m.chats))
		s.WriteString(dimStyle.Render(scrollInfo))
		s.WriteString("\n")
	}

	// Bottom separator
	s.WriteString(dimStyle.Render(strings.Repeat("─", width)))
	s.WriteString("\n")

	// Status messages (below separator)
	if m.error != "" {
		s.WriteString(errorStyle.Render("Error: " + m.error))
		s.WriteString("\n")
	} else if m.deleted > 0 {
		s.WriteString(successStyle.Render(fmt.Sprintf("✓ Deleted %d chat(s)", m.deleted)))
		s.WriteString("\n")
	} else if m.copiedMsg != "" {
		s.WriteString(successStyle.Render("✓ " + m.copiedMsg))
		s.WriteString("\n")
	}

	// Help / Confirmation dialog
	if m.confirmDelete {
		s.WriteString(errorStyle.Render(fmt.Sprintf("Delete %d chat(s)?", len(m.selected))))
		s.WriteString(" ")
		s.WriteString(helpStyle.Render("[ENTER=Yes] [ESC=No]"))
		s.WriteString("\n")
	} else if compact {
		actionsLine := "Actions:    <Space>: Toggle | a: Toggle All | d: Delete | c: Copy | r: Refresh | q: Quit"
		navLine := "Navigation: ↑/↓: Chats | ←/→: Tabs | f/b: PgDn/PgUp | F/B: Half | g/G: Home/End"
		s.WriteString(helpStyle.Render(actionsLine))
		s.WriteString("\n")
		s.WriteString(helpStyle.Render(navLine))
		s.WriteString("\n")
	} else {
		help := "↑/↓:Chats | ←/→:Tabs | <Space>:Toggle | a:Toggle All | c:Copy ID | d:Delete | r:Refresh | f/b:PgUp/PgDn | g/G:Home/End | q/esc:Quit"
		s.WriteString(helpStyle.Render(help))
		s.WriteString("\n")
	}

	return s.String()
}

// updateGrouped handles key events in grouped view mode.
// cursor indexes into m.groupRows (the virtual row list).
func (m model) updateGrouped(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	rowCount := len(m.groupRows)

	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.adjustScrollGrouped()
		}

	case "down", "j":
		if m.cursor < rowCount-1 {
			m.cursor++
			m.adjustScrollGrouped()
		}

	case "f", "pgdown":
		m.cursor += m.visibleHeight()
		if m.cursor >= rowCount {
			m.cursor = rowCount - 1
		}
		m.adjustScrollGrouped()

	case "b", "pgup":
		m.cursor -= m.visibleHeight()
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.adjustScrollGrouped()

	case "F":
		m.cursor += m.visibleHeight() / 2
		if m.cursor >= rowCount {
			m.cursor = rowCount - 1
		}
		m.adjustScrollGrouped()

	case "B":
		m.cursor -= m.visibleHeight() / 2
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.adjustScrollGrouped()

	case "g", "home":
		m.cursor = 0
		m.adjustScrollGrouped()

	case "G", "end":
		if rowCount > 0 {
			m.cursor = rowCount - 1
		}
		m.adjustScrollGrouped()

	case "enter":
		// Expand/collapse project header
		if m.cursor < rowCount && m.groupRows[m.cursor].isHeader {
			proj := m.groupRows[m.cursor].project
			m.expandedProjects[proj] = !m.expandedProjects[proj]
			m.rebuildGroupRows()
			// Keep cursor on the same header
			for i, row := range m.groupRows {
				if row.isHeader && row.project == proj {
					m.cursor = i
					break
				}
			}
			m.adjustScrollGrouped()
		}

	case " ":
		if m.cursor < rowCount {
			m.autoSelected = false
			row := m.groupRows[m.cursor]
			if row.isHeader {
				// Toggle all chats in this project
				indices := m.chatIndicesForProject(row.project)
				allSelected := true
				for _, idx := range indices {
					if !m.selected[idx] {
						allSelected = false
						break
					}
				}
				if allSelected {
					for _, idx := range indices {
						delete(m.selected, idx)
					}
				} else {
					for _, idx := range indices {
						m.selected[idx] = true
					}
				}
			} else {
				// Toggle individual chat
				chatIdx := row.chatIdx
				if m.selected[chatIdx] {
					delete(m.selected, chatIdx)
				} else {
					m.selected[chatIdx] = true
				}
			}
		}

	case "a":
		if len(m.chats) == 0 {
			return m, nil
		}
		m.autoSelected = false
		if len(m.selected) == len(m.chats) {
			m.selected = make(map[int]bool)
		} else {
			for i := range m.chats {
				m.selected[i] = true
			}
		}

	case "d":
		// Explicit selection wins: only auto-select when nothing is selected.
		// On a project header we pick every chat in that project (works for
		// both expanded and collapsed groups). On a chat row we pick just it.
		if len(m.selected) == 0 && m.cursor < len(m.groupRows) {
			row := m.groupRows[m.cursor]
			if row.isHeader {
				for _, idx := range m.chatIndicesForProject(row.project) {
					m.selected[idx] = true
				}
			} else {
				m.selected[row.chatIdx] = true
			}
			if len(m.selected) > 0 {
				m.autoSelected = true
			}
		}
		if len(m.selected) > 0 {
			m.confirmDelete = true
		}

	case "r":
		m.chats = findAllChats()
		m.selected = make(map[int]bool)
		m.autoSelected = false
		m.cursor = 0
		m.scrollOffset = 0
		m.error = ""
		m.deleted = 0
		m.copiedMsg = ""
		m.rebuildGroupRows()

	case "c":
		if m.cursor < rowCount && !m.groupRows[m.cursor].isHeader {
			chatIdx := m.groupRows[m.cursor].chatIdx
			if chatIdx < len(m.chats) {
				uuid := m.chats[chatIdx].UUID
				if err := copyToClipboard(uuid); err != nil {
					m.error = fmt.Sprintf("Failed to copy: %v", err)
				} else {
					m.copyTimer++
					currentTimer := m.copyTimer
					m.copiedMsg = fmt.Sprintf("Chat UUID copied: %s", uuid)
					m.error = ""
					m.deleted = 0
					return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
						return clearCopiedMsg{id: currentTimer}
					})
				}
			}
		}
	}

	return m, nil
}

func (m *model) adjustScrollGrouped() {
	visibleHeight := m.visibleHeight()
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	} else if m.cursor >= m.scrollOffset+visibleHeight {
		m.scrollOffset = m.cursor - visibleHeight + 1
	}
}

// selectedCountForProject returns how many chats in a project are selected.
func (m model) selectedCountForProject(project string) (selected, total int) {
	for i, chat := range m.chats {
		if chat.Project == project {
			total++
			if m.selected[i] {
				selected++
			}
		}
	}
	return
}

func (m model) viewGrouped() string {
	if len(m.chats) == 0 {
		return activeTabStyle.Render("No chats found.") + "\n\nPress q to quit.\n"
	}

	width := m.width
	if width < 75 {
		width = 75
	}

	compact := width < compactModeWidth

	// Column widths for chat rows (indented by 2 for nesting)
	var timestampWidth, versionWidth, fixedWidth int
	if compact {
		timestampWidth = 11
		versionWidth = 0
		fixedWidth = 4 + 2 + timestampWidth + 5 + 5 // indicator + indent + ts + lines + gaps
	} else {
		timestampWidth = 19
		versionWidth = 8
		fixedWidth = 46 // indicator(4) + indent(2) + ts(19) + version(8) + lines(5) + gaps(8)
	}

	linesWidth := 5
	remaining := width - fixedWidth
	titleWidth := remaining * 65 / 100 // more title space since project is in header
	if titleWidth < 30 {
		titleWidth = 30
	}

	var s strings.Builder

	// Header
	s.WriteString(m.renderTabBar())
	s.WriteString("\n")
	s.WriteString(dimStyle.Render(strings.Repeat("─", width)))
	s.WriteString("\n")

	// Thin column header for grouped view
	header := fmt.Sprintf("    %-*s", width-4, "PROJECT / CHAT")
	s.WriteString(dimStyle.Render(header))
	s.WriteString("\n")
	s.WriteString(dimStyle.Render(strings.Repeat("─", width)))
	s.WriteString("\n")

	// Rows
	visibleHeight := m.visibleHeight()
	rowCount := len(m.groupRows)

	start := m.scrollOffset
	end := start + visibleHeight
	if end > rowCount {
		end = rowCount
	}

	for i := start; i < end; i++ {
		row := m.groupRows[i]

		if row.isHeader {
			// Project header row
			sel, total := m.selectedCountForProject(row.project)
			arrow := "▸"
			if m.expandedProjects[row.project] {
				arrow = "▾"
			}

			// Selection indicator for project
			indicator := "[ ]"
			if sel == total && total > 0 {
				indicator = "[✓]"
			} else if sel > 0 {
				indicator = "[~]"
			}

			projectClean := strings.NewReplacer("\n", " ").Replace(row.project)
			countInfo := dimStyle.Render(fmt.Sprintf("(%d chats, %d selected)", total, sel))
			line := fmt.Sprintf("%s %s %s  %s", indicator, arrow, projectClean, countInfo)

			// Pad to full width
			lineWidth := lipgloss.Width(line)
			if lineWidth < width {
				line += strings.Repeat(" ", width-lineWidth)
			}

			style := lipgloss.NewStyle()
			if sel > 0 && sel == total {
				style = selectedStyle
			}
			if i == m.cursor {
				style = cursorStyle
			}
			s.WriteString(style.Render(line))
			s.WriteString("\n")
		} else {
			// Chat row (indented under project)
			chat := m.chats[row.chatIdx]

			var timestamp string
			if compact {
				if len(chat.Timestamp) >= 16 {
					timestamp = chat.Timestamp[5:16]
				} else {
					timestamp = runewidth.Truncate(chat.Timestamp, timestampWidth, "")
				}
			} else {
				timestamp = runewidth.Truncate(chat.Timestamp, timestampWidth, "")
			}

			var version string
			if versionWidth > 0 {
				version = runewidth.Truncate(chat.Version, versionWidth-1, "")
			}
			var lines string
			switch {
			case chat.LineCount == 0:
				lines = "-"
			case chat.LineCount >= 10000:
				lines = fmt.Sprintf("%dk", chat.LineCount/1000)
			default:
				lines = fmt.Sprintf("%d", chat.LineCount)
			}

			titleClean := strings.NewReplacer("\n", " ").Replace(chat.Title)
			title := runewidth.Truncate(titleClean, titleWidth, "..")

			indicator := "[ ]"
			if m.selected[row.chatIdx] {
				indicator = "[✓]"
			}

			var line string
			if compact {
				lineFmt := fmt.Sprintf("%%s  %%-*s  %%-%ds  %%-%ds", linesWidth, titleWidth)
				line = fmt.Sprintf(lineFmt, indicator, timestampWidth, timestamp, lines, title)
			} else {
				lineFmt := fmt.Sprintf("%%s  %%-*s  %%-%ds  %%-%ds  %%-%ds", versionWidth, linesWidth, titleWidth)
				line = fmt.Sprintf(lineFmt, indicator, timestampWidth, timestamp, version, lines, title)
			}

			style := lipgloss.NewStyle()
			if m.selected[row.chatIdx] {
				style = selectedStyle
			}
			if i == m.cursor {
				style = cursorStyle
			}
			s.WriteString(style.Render(line))
			s.WriteString("\n")
		}
	}

	// Scroll indicator
	if rowCount > visibleHeight {
		scrollInfo := fmt.Sprintf("[%d-%d/%d]", start+1, end, rowCount)
		s.WriteString(dimStyle.Render(scrollInfo))
		s.WriteString("\n")
	}

	// Bottom separator
	s.WriteString(dimStyle.Render(strings.Repeat("─", width)))
	s.WriteString("\n")

	// Status messages
	if m.error != "" {
		s.WriteString(errorStyle.Render("Error: " + m.error))
		s.WriteString("\n")
	} else if m.deleted > 0 {
		s.WriteString(successStyle.Render(fmt.Sprintf("✓ Deleted %d chat(s)", m.deleted)))
		s.WriteString("\n")
	} else if m.copiedMsg != "" {
		s.WriteString(successStyle.Render("✓ " + m.copiedMsg))
		s.WriteString("\n")
	}

	// Help / Confirmation dialog
	if m.confirmDelete {
		s.WriteString(errorStyle.Render(fmt.Sprintf("Delete %d chat(s)?", len(m.selected))))
		s.WriteString(" ")
		s.WriteString(helpStyle.Render("[ENTER=Yes] [ESC=No]"))
		s.WriteString("\n")
	} else if compact {
		actionsLine := "Actions:    <Space>: Toggle | Enter: Expand | a: Toggle All | d: Delete | c: Copy | r: Refresh | q: Quit"
		navLine := "Navigation: ↑/↓: Items | ←/→: Tabs | f/b: PgDn/PgUp | F/B: Half | g/G: Home/End"
		s.WriteString(helpStyle.Render(actionsLine))
		s.WriteString("\n")
		s.WriteString(helpStyle.Render(navLine))
		s.WriteString("\n")
	} else {
		help := "↑/↓:Items | ←/→:Tabs | Enter:Expand | <Space>:Toggle | a:Toggle All | c:Copy ID | d:Delete | r:Refresh | q/esc:Quit"
		s.WriteString(helpStyle.Render(help))
		s.WriteString("\n")
	}

	return s.String()
}

func (m model) deleteSelectedChats() tea.Cmd {
	return func() tea.Msg {
		var toDelete []Chat
		for idx := range m.selected {
			if idx < len(m.chats) {
				toDelete = append(toDelete, m.chats[idx])
			}
		}
		count, err := deleteChats(toDelete)
		if err != nil {
			return errMsg(err.Error())
		}
		return deleteCompleteMsg{count: count}
	}
}
