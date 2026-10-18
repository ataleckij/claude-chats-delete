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

// loadMemory rereads the memory files from disk and resets the tab's viewport.
func (m *model) loadMemory() {
	m.memory = findAllMemoryFiles()
	m.memCursor = 0
	m.memScroll = 0
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

func (m model) memoryCountForProject(project string) int {
	count := 0
	for _, f := range m.memory {
		if f.Project == project {
			count++
		}
	}
	return count
}

func (m *model) adjustMemScroll() {
	visibleHeight := m.visibleHeight()
	if m.memCursor < m.memScroll {
		m.memScroll = m.memCursor
	} else if m.memCursor >= m.memScroll+visibleHeight {
		m.memScroll = m.memCursor - visibleHeight + 1
	}
}

// updateMemory handles key events on the memory tab, which is read-only: there
// is no selection and nothing here deletes.
func (m model) updateMemory(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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

	case "r":
		m.loadMemory()
		m.error = ""
		m.deleted = 0
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
		remaining := width - 4 - timestampWidth - 4 // indent + ts + gaps
		nameWidth = remaining * 35 / 100
		descWidth = remaining - nameWidth
	} else {
		remaining := width - 2 - timestampWidth - 6 // indent + ts + gaps
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
		header = fmt.Sprintf("    %-*s  %-*s  %-*s", timestampWidth, "MODIFIED", nameWidth, "FILE", descWidth, "DESCRIPTION")
	} else {
		header = fmt.Sprintf("  %-*s  %-*s  %-*s  %-*s", timestampWidth, "MODIFIED", nameWidth, "FILE", descWidth, "DESCRIPTION", projectWidth, "PROJECT")
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
		if row.isHeader {
			arrow := "▸"
			if m.memExpanded[row.project] {
				arrow = "▾"
			}
			projectClean := displayText(row.project)
			countInfo := dimStyle.Render(fmt.Sprintf("(%d files)", m.memoryCountForProject(row.project)))
			left := fmt.Sprintf("%s %s", arrow, projectClean)

			// Anchor the count info to the right edge (same gap trick as the tab bar).
			gap := width - lipgloss.Width(left) - lipgloss.Width(countInfo)
			if gap < 1 {
				gap = 1
			}
			line = left + strings.Repeat(" ", gap) + countInfo
		} else {
			f := m.memory[row.fileIdx]

			timestamp := f.Modified
			if timestampWidth == 11 && len(timestamp) >= 16 {
				timestamp = timestamp[5:16] // "MM-DD HH:MM"
			}
			timestamp = runewidth.Truncate(timestamp, timestampWidth, "")
			name := runewidth.Truncate(displayText(f.Name), nameWidth, "..")
			description := runewidth.Truncate(displayText(f.Description), descWidth, "..")

			if m.grouped {
				line = fmt.Sprintf("    %-*s  %-*s  %-*s", timestampWidth, timestamp, nameWidth, name, descWidth, description)
			} else {
				projectClean := displayText(f.Project)
				project := truncateLeft(projectClean, projectWidth-2)
				line = fmt.Sprintf("  %-*s  %-*s  %-*s  %-*s", timestampWidth, timestamp, nameWidth, name, descWidth, description, projectWidth, project)
			}
		}

		style := lipgloss.NewStyle()
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
		s.WriteString(errorStyle.Render("Error: " + m.error))
		s.WriteString("\n")
	} else if m.copiedMsg != "" {
		s.WriteString(successStyle.Render("✓ " + m.copiedMsg))
		s.WriteString("\n")
	}

	// Help
	expand := ""
	if m.grouped {
		expand = "Enter: Expand | "
	}
	if compact {
		actionsLine := "Actions:    " + expand + "c: Copy Path | r: Refresh | q: Quit"
		navLine := "Navigation: ↑/↓: Items | ←/→: Tabs | f/b: PgDn/PgUp | F/B: Half | g/G: Home/End"
		s.WriteString(helpStyle.Render(actionsLine))
		s.WriteString("\n")
		s.WriteString(helpStyle.Render(navLine))
		s.WriteString("\n")
	} else {
		help := "↑/↓:Items | ←/→:Tabs | " + strings.ReplaceAll(expand, ": ", ":") + "c:Copy Path | r:Refresh | f/b:PgUp/PgDn | g/G:Home/End | q/esc:Quit"
		s.WriteString(helpStyle.Render(help))
		s.WriteString("\n")
	}

	return s.String()
}
