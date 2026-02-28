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

var (
	// Styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("6"))

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

type model struct {
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
}

func initialModel() model {
	chats := findAllChats()
	return model{
		chats:    chats,
		selected: make(map[int]bool),
	}
}

func (m model) visibleHeight() int {
	fixed := 9 // header(4) + bottom-separator(1) + help(1) + scroll(0-1) + status(0-1) = 9 base
	if m.width < compactModeWidth {
		fixed = 10 // compact: +1 for extra help line (separator already in base)
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
		// Confirmation dialog mode
		if m.confirmDelete {
			switch msg.String() {
			case "enter":
				return m, m.deleteSelectedChats()
			case "esc", "n":
				m.confirmDelete = false
			}
			return m, nil
		}

		// Normal mode
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit

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
			if len(m.selected) == len(m.chats) {
				m.selected = make(map[int]bool)
			} else {
				for i := range m.chats {
					m.selected[i] = true
				}
			}

		case "d":
			if len(m.selected) > 0 {
				m.confirmDelete = true
			}

		case "r":
			// Refresh
			m.chats = findAllChats()
			m.selected = make(map[int]bool)
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
		m.cursor = 0
		m.scrollOffset = 0
		m.confirmDelete = false
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

func (m model) View() string {
	if len(m.chats) == 0 {
		return titleStyle.Render("No chats found.") + "\n\nPress q to quit.\n"
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
	s.WriteString(titleStyle.Render("Claude Code Chat Manager"))
	s.WriteString("\n")

	// Stats
	stats := fmt.Sprintf("Total: %d | Selected: %d", len(m.chats), len(m.selected))
	s.WriteString(dimStyle.Render(stats))
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
		actionsLine := justifyItems("Actions:    ",
			[]string{"<Space>:Toggle (a:All)", "d:Delete", "c:Copy", "r:Refresh", "q:Quit"},
			width)
		navLine := justifyItems("Navigation: ",
			[]string{"↑/↓", "f/b:PgDn/PgUp", "F/B:Half", "g/G:Home/End"},
			width)
		s.WriteString(helpStyle.Render(actionsLine))
		s.WriteString("\n")
		s.WriteString(helpStyle.Render(navLine))
		s.WriteString("\n")
	} else {
		help := "↑/↓:Nav | <Space>:Toggle (a:All) | c:Copy ID | d:Delete | r:Refresh | f/b:PgUp/PgDn | g/G:Home/End | q/esc:Quit"
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
