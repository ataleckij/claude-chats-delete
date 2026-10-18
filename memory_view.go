package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// memRow is a single row of the memory tab: a project header, or a reference
// to a memory file. The flat list is the same row list without headers.
type memRow struct {
	isHeader bool
	project  string
	fileIdx  int // index into m.memory (-1 for headers)
}

// memoryDeleteCompleteMsg reports a finished delete of memory notes. warning
// carries every failure, including an index that could not be updated.
type memoryDeleteCompleteMsg struct {
	count   int
	warning string
}

// loadMemory rereads the memory files from disk and resets the tab's viewport
// and selection.
func (m *model) loadMemory() {
	m.memory = findAllMemoryFiles()
	m.memCursor = 0
	m.memScroll = 0
	m.memSelected = make(map[int]bool)
	m.memAutoSelected = false
	m.rebuildMemRows()
}

// rebuildMemRows builds the row list from the memory files and, when grouped,
// the expanded state. Projects are ordered by their most recently modified
// file, which is the order the files are already sorted in.
func (m *model) rebuildMemRows() {
	var rows []memRow
	if !m.grouped {
		for i, f := range m.memory {
			rows = append(rows, memRow{project: f.Project, fileIdx: i})
		}
		m.memRows = rows
		return
	}

	var projects []string
	byProject := make(map[string][]int)
	for i, f := range m.memory {
		if _, seen := byProject[f.Project]; !seen {
			projects = append(projects, f.Project)
		}
		byProject[f.Project] = append(byProject[f.Project], i)
	}
	for _, project := range projects {
		rows = append(rows, memRow{isHeader: true, project: project, fileIdx: -1})
		if m.memExpanded[project] {
			for _, idx := range byProject[project] {
				rows = append(rows, memRow{project: project, fileIdx: idx})
			}
		}
	}
	m.memRows = rows
}

func (m model) memoryIndicesForProject(project string) []int {
	var indices []int
	for i, f := range m.memory {
		if f.Project == project {
			indices = append(indices, i)
		}
	}
	return indices
}

// toggleMemSelection selects every given note, or clears them all when every
// one is already selected.
func (m *model) toggleMemSelection(indices []int) {
	all := true
	for _, idx := range indices {
		if !m.memSelected[idx] {
			all = false
			break
		}
	}
	for _, idx := range indices {
		if all {
			delete(m.memSelected, idx)
		} else {
			m.memSelected[idx] = true
		}
	}
}

// updateMemoryConfirm handles keys while the delete confirmation is shown.
// Once confirmed, the dialog stays up and swallows every key, quitting
// included, until the delete reports back: quitting halfway would leave the
// index linking to notes already removed.
func (m model) updateMemoryConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.deleting {
		return m, nil
	}
	switch msg.String() {
	case "enter":
		m.deleting = true
		return m, m.deleteSelectedMemory()
	case "esc", "n":
		m.memConfirm = false
		if m.memAutoSelected {
			m.memSelected = make(map[int]bool)
			m.memAutoSelected = false
		}
	}
	return m, nil
}

func (m model) deleteSelectedMemory() tea.Cmd {
	var toDelete []MemoryFile
	for idx := range m.memSelected {
		if idx < len(m.memory) {
			toDelete = append(toDelete, m.memory[idx])
		}
	}
	return func() tea.Msg {
		count, err := deleteMemoryFiles(toDelete)
		msg := memoryDeleteCompleteMsg{count: count}
		if err != nil {
			msg.warning = err.Error()
		}
		return msg
	}
}

func (m model) memoryDeleteComplete(msg memoryDeleteCompleteMsg) (tea.Model, tea.Cmd) {
	m.deleting = false
	m.loadMemory()
	m.memConfirm = false
	m.memDeleted = msg.count
	m.error = msg.warning
	m.copiedMsg = ""
	m.deleteTimer++
	currentTimer := m.deleteTimer
	return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return clearDeleteMsg{id: currentTimer}
	})
}

func (m *model) adjustMemScroll() {
	visibleHeight := m.visibleHeight()
	if m.memCursor < m.memScroll {
		m.memScroll = m.memCursor
	} else if m.memCursor >= m.memScroll+visibleHeight {
		m.memScroll = m.memCursor - visibleHeight + 1
	}
}

// updateMemory handles key events on the memory tab.
func (m model) updateMemory(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.memSelected == nil {
		m.memSelected = make(map[int]bool)
	}
	rowCount := len(m.memRows)
	last := rowCount - 1
	if last < 0 {
		last = 0
	}

	switch msg.String() {
	case "up", "k":
		if m.memCursor > 0 {
			m.memCursor--
		}

	case "down", "j":
		if m.memCursor < last {
			m.memCursor++
		}

	case "f", "pgdown":
		m.memCursor += m.visibleHeight()

	case "b", "pgup":
		m.memCursor -= m.visibleHeight()

	case "F":
		m.memCursor += m.visibleHeight() / 2

	case "B":
		m.memCursor -= m.visibleHeight() / 2

	case "g", "home":
		m.memCursor = 0

	case "G", "end":
		m.memCursor = last

	case "enter":
		if m.memCursor < rowCount && m.memRows[m.memCursor].isHeader {
			project := m.memRows[m.memCursor].project
			if m.memExpanded == nil {
				m.memExpanded = make(map[string]bool)
			}
			m.memExpanded[project] = !m.memExpanded[project]
			m.rebuildMemRows()
			// Keep the cursor on the same header
			for i, row := range m.memRows {
				if row.isHeader && row.project == project {
					m.memCursor = i
					break
				}
			}
		}

	case " ":
		if m.memCursor < rowCount {
			m.memAutoSelected = false
			row := m.memRows[m.memCursor]
			if row.isHeader {
				m.toggleMemSelection(m.memoryIndicesForProject(row.project))
			} else {
				m.toggleMemSelection([]int{row.fileIdx})
			}
		}

	case "a":
		if len(m.memory) > 0 {
			m.memAutoSelected = false
			if len(m.memSelected) == len(m.memory) {
				m.memSelected = make(map[int]bool)
			} else {
				for i := range m.memory {
					m.memSelected[i] = true
				}
			}
		}

	case "d":
		// Explicit selection wins: only the row under the cursor is picked, and
		// only when nothing is selected. A project header picks all its notes.
		if len(m.memSelected) == 0 && m.memCursor < rowCount {
			row := m.memRows[m.memCursor]
			if row.isHeader {
				m.toggleMemSelection(m.memoryIndicesForProject(row.project))
			} else {
				m.toggleMemSelection([]int{row.fileIdx})
			}
			m.memAutoSelected = len(m.memSelected) > 0
		}
		if len(m.memSelected) > 0 {
			m.memConfirm = true
		}

	case "r":
		m.loadMemory()
		m.error = ""
		m.deleted = 0
		m.memDeleted = 0
		m.copiedMsg = ""

	case "c":
		if m.memCursor < rowCount && !m.memRows[m.memCursor].isHeader {
			path := m.memory[m.memRows[m.memCursor].fileIdx].Path
			if err := copyToClipboard(path); err != nil {
				m.error = fmt.Sprintf("Failed to copy: %v", err)
			} else {
				m.copyTimer++
				currentTimer := m.copyTimer
				m.copiedMsg = fmt.Sprintf("Path copied: %s", path)
				m.error = ""
				m.deleted = 0
				m.memDeleted = 0
				return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
					return clearCopiedMsg{id: currentTimer}
				})
			}
		}
	}

	if m.memCursor > last {
		m.memCursor = last
	}
	if m.memCursor < 0 {
		m.memCursor = 0
	}
	m.adjustMemScroll()
	return m, nil
}

func (m model) viewMemory() string {
	width := m.width
	if width < 75 {
		width = 75
	}
	compact := width < compactModeWidth

	// The grouped list drops the PROJECT column (it is the group header) and so
	// always has room for the full timestamp; the flat one shortens it to
	// "MM-DD HH:MM" on narrow terminals, like the chat list.
	timestampWidth := 19
	if compact && !m.grouped {
		timestampWidth = 11
	}

	var nameWidth, descWidth, projectWidth int
	if m.grouped {
		remaining := width - 5 - timestampWidth - 4 // indicator + indent + ts + gaps
		nameWidth = remaining * 35 / 100
		descWidth = remaining - nameWidth
	} else {
		remaining := width - 4 - timestampWidth - 6 // indicator + ts + gaps
		nameWidth = remaining * 30 / 100
		projectWidth = remaining * 25 / 100
		if projectWidth < 10 {
			projectWidth = 10
		}
		descWidth = remaining - nameWidth - projectWidth
	}
	if nameWidth < 20 {
		nameWidth = 20
	}
	if descWidth < 20 {
		descWidth = 20
	}

	var s strings.Builder

	// Header
	s.WriteString(m.renderTabBar())
	s.WriteString("\n")
	s.WriteString(dimStyle.Render(strings.Repeat("─", width)))
	s.WriteString("\n")

	// Column headers
	var header string
	if m.grouped {
		header = fmt.Sprintf("     %-*s  %-*s  %-*s", timestampWidth, "MODIFIED", nameWidth, "FILE", descWidth, "DESCRIPTION")
	} else {
		header = fmt.Sprintf("    %-*s  %-*s  %-*s  %-*s", timestampWidth, "MODIFIED", nameWidth, "FILE", descWidth, "DESCRIPTION", projectWidth, "PROJECT")
	}
	s.WriteString(dimStyle.Render(header))
	s.WriteString("\n")
	s.WriteString(dimStyle.Render(strings.Repeat("─", width)))
	s.WriteString("\n")

	// Rows
	visibleHeight := m.visibleHeight()
	rowCount := len(m.memRows)

	start := m.memScroll
	end := start + visibleHeight
	if end > rowCount {
		end = rowCount
	}

	for i := start; i < end; i++ {
		row := m.memRows[i]

		var line string
		selected := false
		if row.isHeader {
			arrow := "▸"
			if m.memExpanded[row.project] {
				arrow = "▾"
			}
			indices := m.memoryIndicesForProject(row.project)
			sel := 0
			for _, idx := range indices {
				if m.memSelected[idx] {
					sel++
				}
			}
			indicator := "[ ]"
			if sel > 0 && sel == len(indices) {
				indicator = "[✓]"
				selected = true
			} else if sel > 0 {
				indicator = "[~]"
			}
			projectClean := displayText(row.project)
			countInfo := dimStyle.Render(fmt.Sprintf("(%d files, %d selected)", len(indices), sel))
			left := fmt.Sprintf("%s %s %s", indicator, arrow, projectClean)

			// Anchor the count info to the right edge (same gap trick as the tab bar).
			gap := width - lipgloss.Width(left) - lipgloss.Width(countInfo)
			if gap < 1 {
				gap = 1
			}
			line = left + strings.Repeat(" ", gap) + countInfo
		} else {
			f := m.memory[row.fileIdx]
			selected = m.memSelected[row.fileIdx]
			indicator := "[ ]"
			if selected {
				indicator = "[✓]"
			}

			timestamp := f.Modified
			if timestampWidth == 11 && len(timestamp) >= 16 {
				timestamp = timestamp[5:16] // "MM-DD HH:MM"
			}
			timestamp = runewidth.Truncate(timestamp, timestampWidth, "")
			name := runewidth.Truncate(displayText(f.Name), nameWidth, "..")
			description := runewidth.Truncate(displayText(f.Description), descWidth, "..")

			if m.grouped {
				line = fmt.Sprintf("%s  %-*s  %-*s  %-*s", indicator, timestampWidth, timestamp, nameWidth, name, descWidth, description)
			} else {
				projectClean := displayText(f.Project)
				project := truncateLeft(projectClean, projectWidth-2)
				line = fmt.Sprintf("%s %-*s  %-*s  %-*s  %-*s", indicator, timestampWidth, timestamp, nameWidth, name, descWidth, description, projectWidth, project)
			}
		}

		style := lipgloss.NewStyle()
		if selected {
			style = selectedStyle
		}
		if i == m.memCursor {
			style = cursorStyle
		}
		s.WriteString(style.Render(line))
		s.WriteString("\n")
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
		// A delete can report several failures with file paths in them; keep
		// them on the one status line the layout reserves.
		s.WriteString(errorStyle.Render(runewidth.Truncate("Error: "+displayText(m.error), width, "..")))
		s.WriteString("\n")
	} else if m.memDeleted > 0 {
		s.WriteString(successStyle.Render(fmt.Sprintf("✓ Deleted %d memory file(s)", m.memDeleted)))
		s.WriteString("\n")
	} else if m.copiedMsg != "" {
		s.WriteString(successStyle.Render("✓ " + m.copiedMsg))
		s.WriteString("\n")
	}

	// Help / Confirmation dialog
	expand := ""
	if m.grouped {
		expand = "Enter: Expand | "
	}
	if m.memConfirm && m.deleting {
		s.WriteString(errorStyle.Render(fmt.Sprintf("Deleting %d memory file(s)...", len(m.memSelected))))
		s.WriteString("\n")
	} else if m.memConfirm {
		s.WriteString(errorStyle.Render(fmt.Sprintf("Delete %d memory file(s)?", len(m.memSelected))))
		s.WriteString(" ")
		s.WriteString(helpStyle.Render("[ENTER=Yes] [ESC=No]"))
		s.WriteString("\n")
	} else if compact {
		actionsLine := "Actions:    <Space>: Toggle | a: Toggle All | " + expand + "d: Delete | c: Copy Path | r: Refresh | q: Quit"
		navLine := "Navigation: ↑/↓: Items | ←/→: Tabs | f/b: PgDn/PgUp | F/B: Half | g/G: Home/End"
		s.WriteString(helpStyle.Render(actionsLine))
		s.WriteString("\n")
		s.WriteString(helpStyle.Render(navLine))
		s.WriteString("\n")
	} else {
		help := "↑/↓:Items | ←/→:Tabs | <Space>:Toggle | a:Toggle All | " + strings.ReplaceAll(expand, ": ", ":") + "c:Copy Path | d:Delete | r:Refresh | f/b:PgUp/PgDn | g/G:Home/End | q/esc:Quit"
		s.WriteString(helpStyle.Render(help))
		s.WriteString("\n")
	}

	return s.String()
}
