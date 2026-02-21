package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
)

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
			visibleHeight := m.height - 8
			if visibleHeight < 1 {
				visibleHeight = 10
			}
			m.cursor += visibleHeight
			if m.cursor >= len(m.chats) {
				m.cursor = len(m.chats) - 1
			}
			m.adjustScroll()

		case "b", "pgup":
			visibleHeight := m.height - 8
			if visibleHeight < 1 {
				visibleHeight = 10
			}
			m.cursor -= visibleHeight
			if m.cursor < 0 {
				m.cursor = 0
			}
			m.adjustScroll()

		case "F":
			visibleHeight := m.height - 8
			if visibleHeight < 1 {
				visibleHeight = 10
			}
			m.cursor += visibleHeight / 2
			if m.cursor >= len(m.chats) {
				m.cursor = len(m.chats) - 1
			}
			m.adjustScroll()

		case "B":
			visibleHeight := m.height - 8
			if visibleHeight < 1 {
				visibleHeight = 10
			}
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
	visibleHeight := m.height - 8 // Account for header/footer
	// confirmDelete dialog replaces help text, no additional space needed
	if visibleHeight < 1 {
		visibleHeight = 10
	}

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
	// Fixed: indicator(4) + timestamp(19) + version(8) + lines(6) + gaps(8) = 45
	width := m.width
	if width < 75 {
		width = 75 // minimum width
	}

	versionWidth := 8
	// msgWidth := 5 // TODO: maybe re-enable later
	linesWidth := 6
	remaining := width - 45
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
	headerFmt := fmt.Sprintf("    %%-19s  %%-%ds  %%-%ds  %%-%ds  %%-%ds", versionWidth, linesWidth, titleWidth, projectWidth)
	header := fmt.Sprintf(headerFmt, "TIMESTAMP", "VERSION", "LINES", "TITLE", "PROJECT")
	s.WriteString(dimStyle.Render(header))
	s.WriteString("\n")
	s.WriteString(strings.Repeat("─", width))
	s.WriteString("\n")

	// Chat list
	visibleHeight := m.height - 8
	// confirmDelete dialog replaces help text, no additional space needed
	if visibleHeight < 1 {
		visibleHeight = 10
	}

	start := m.scrollOffset
	end := start + visibleHeight
	if end > len(m.chats) {
		end = len(m.chats)
	}

	for i := start; i < end; i++ {
		chat := m.chats[i]

		// Truncate fields using visual width
		timestamp := runewidth.Truncate(chat.Timestamp, 19, "")
		version := runewidth.Truncate(chat.Version, versionWidth-1, "")
		// TODO: msg column - maybe re-enable later
		// msg := fmt.Sprintf("%d", chat.MessageCount)
		// if chat.MessageCount == 0 {
		// 	msg = "-"
		// }
		lines := fmt.Sprintf("%d", chat.LineCount)
		if chat.LineCount == 0 {
			lines = "-"
		}

		// Clean title from newlines
		titleCleaned := strings.ReplaceAll(chat.Title, "\n", " ")
		titleCleaned = strings.ReplaceAll(titleCleaned, "\r", "")
		titleCleaned = strings.Join(strings.Fields(titleCleaned), " ")
		title := runewidth.Truncate(titleCleaned, titleWidth-2, "..")

		// Clean project from newlines
		projectCleaned := strings.ReplaceAll(chat.Project, "\n", " ")
		projectCleaned = strings.ReplaceAll(projectCleaned, "\r", "")
		projectCleaned = strings.Join(strings.Fields(projectCleaned), " ")
		project := runewidth.Truncate(projectCleaned, projectWidth-2, "..")

		// Selection indicator
		indicator := "[ ]"
		if m.selected[i] {
			indicator = "[✓]"
		}

		lineFmt := fmt.Sprintf("%%s %%-19s  %%-%ds  %%-%ds  %%-%ds  %%-%ds", versionWidth, linesWidth, titleWidth, projectWidth)
		line := fmt.Sprintf(lineFmt, indicator, timestamp, version, lines, title, project)

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

	// Confirmation dialog
	if m.confirmDelete {
		s.WriteString(errorStyle.Render(fmt.Sprintf("Delete %d chat(s)?", len(m.selected))))
		s.WriteString(" ")
		s.WriteString(helpStyle.Render("[ENTER=Yes] [ESC=No]"))
		s.WriteString("\n")
	} else {
		// Help
		help := "↑/↓:Nav | <Space>:Toggle (a:All) | c:Copy ID | d:Delete | r:Refresh | f/b:PgUp/PgDn | g/G:Home/End | q/esc:Quit"
		s.WriteString(helpStyle.Render(help))
		s.WriteString("\n")
	}

	return s.String()
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

func copyToClipboard(text string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		// Try xclip first, then xsel
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.Command("wl-copy")
		} else {
			return fmt.Errorf("no clipboard utility found (install xclip, xsel, or wl-copy)")
		}
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func (m model) deleteSelectedChats() tea.Cmd {
	return func() tea.Msg {
		count := 0
		for idx := range m.selected {
			if idx < len(m.chats) {
				chat := m.chats[idx]
				files := findRelatedFiles(chat.UUID)
				for _, file := range files {
					if err := os.RemoveAll(file); err != nil {
						return errMsg(fmt.Sprintf("Failed to delete %s: %v", file, err))
					}
				}

				// Update sessions-index.json
				if err := updateSessionsIndex(chat.UUID); err != nil {
					return errMsg(fmt.Sprintf("Failed to update index: %v", err))
				}

				count++
			}
		}
		return deleteCompleteMsg{count: count}
	}
}

func main() {
	// Parse command-line flags
	updateFlag := flag.Bool("update", false, "Check for updates and install if available")
	versionFlag := flag.Bool("version", false, "Show current version")
	flag.Parse()

	// Show version
	if *versionFlag {
		fmt.Printf("claude-chats v%s\n", CurrentVersion)
		os.Exit(0)
	}

	// Load or create config
	config, err := loadConfig()
	if err != nil {
		// First run - prompt for directory
		dir, err := promptForClaudeDir()
		if err != nil {
			fmt.Printf("Error reading input: %v\n", err)
			os.Exit(1)
		}

		// Validate directory exists
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			fmt.Printf("Error: Directory does not exist: %s\n", dir)
			fmt.Println("Please create the directory or specify a different path.")
			os.Exit(1)
		}

		// Save config with defaults
		config = &Config{
			ClaudeDir:              dir,
			AutoUpdates:            true, // Enable by default
			UpdateCheckIntervalHrs: 1,    // Check every hour
			LastUpdateCheck:        0,
		}
		if err := saveConfig(config); err != nil {
			fmt.Printf("Warning: Could not save config: %v\n", err)
		} else {
			fmt.Printf("\n✓ Configuration saved to: %s\n\n", configPath)
		}
	}

	// Set defaults for existing configs without update settings
	if config.UpdateCheckIntervalHrs == 0 {
		config.UpdateCheckIntervalHrs = 1
		config.AutoUpdates = true
	}

	// Initialize paths from config
	initializePaths(config.ClaudeDir)

	// Manual update check
	if *updateFlag {
		fmt.Printf("Checking for updates...\n")
		if newVersion := checkForUpdate(); newVersion != "" {
			if promptAndUpdate(newVersion) {
				// User declined or update failed
				config.LastUpdateCheck = time.Now().Unix()
				saveConfig(config)
			}
		} else {
			fmt.Printf("You're up to date (v%s)\n", CurrentVersion)
		}
		return
	}

	// Automatic update check (on startup)
	if config.AutoUpdates &&
		os.Getenv("CLAUDE_CHATS_DISABLE_AUTOUPDATER") != "1" &&
		shouldCheckUpdate(config.LastUpdateCheck, config.UpdateCheckIntervalHrs) {

		if newVersion := checkForUpdate(); newVersion != "" {
			// Prompt for update
			if promptAndUpdate(newVersion) {
				// User declined or update failed, save check time
				config.LastUpdateCheck = time.Now().Unix()
				saveConfig(config)
			}
			// If update succeeded, program exits in promptAndUpdate
		} else {
			// No update available, save check time
			config.LastUpdateCheck = time.Now().Unix()
			saveConfig(config)
		}
	}

	// Run TUI
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
