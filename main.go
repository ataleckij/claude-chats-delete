package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Chat represents a single chat session
type Chat struct {
	UUID      string
	Title     string
	Timestamp string
	Project   string
	Path      string
	Files     []string // related files for deletion
}

// JSONLMessage represents a message in the JSONL file
type JSONLMessage struct {
	Type    string `json:"type"`
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
}

var (
	claudeDir    = filepath.Join(os.Getenv("HOME"), ".claude")
	projectsDir  = filepath.Join(claudeDir, "projects")
	debugDir     = filepath.Join(claudeDir, "debug")
	todosDir     = filepath.Join(claudeDir, "todos")
	sessionDir   = filepath.Join(claudeDir, "session-env")

	// Styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("86")).
			MarginBottom(1)

	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("226"))

	cursorStyle = lipgloss.NewStyle().
			Reverse(true)

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("46")).
			Bold(true)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
)

type model struct {
	chats         []Chat
	cursor        int
	selected      map[int]bool
	confirmDelete bool
	deleting      bool
	deleted       int
	error         string
	height        int
	scrollOffset  int
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
		case "ctrl+c", "q":
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

		case " ":
			if m.selected[m.cursor] {
				delete(m.selected, m.cursor)
			} else {
				m.selected[m.cursor] = true
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
		}

	case deleteCompleteMsg:
		m.deleting = false
		m.deleted = msg.count
		m.chats = findAllChats()
		m.selected = make(map[int]bool)
		m.cursor = 0
		m.scrollOffset = 0
		m.confirmDelete = false
		if len(m.chats) == 0 {
			return m, tea.Quit
		}

	case errMsg:
		m.deleting = false
		m.error = string(msg)
	}

	return m, nil
}

func (m *model) adjustScroll() {
	visibleHeight := m.height - 8 // Account for header/footer
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

	var s strings.Builder

	// Header
	s.WriteString(titleStyle.Render("Claude Code Chat Manager"))
	s.WriteString("\n")

	// Stats
	stats := fmt.Sprintf("Total: %d | Selected: %d", len(m.chats), len(m.selected))
	s.WriteString(dimStyle.Render(stats))
	s.WriteString("\n\n")

	// Column headers
	header := fmt.Sprintf("%-20s %-45s %-30s", "TIMESTAMP", "TITLE", "PROJECT")
	s.WriteString(dimStyle.Render(header))
	s.WriteString("\n")
	s.WriteString(strings.Repeat("─", 100))
	s.WriteString("\n")

	// Chat list
	visibleHeight := m.height - 8
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

		// Truncate fields
		timestamp := chat.Timestamp
		if len(timestamp) > 19 {
			timestamp = timestamp[:19]
		}

		title := chat.Title
		if len(title) > 43 {
			title = title[:43] + ".."
		}

		project := chat.Project
		if len(project) > 28 {
			project = project[:28] + ".."
		}

		// Selection indicator
		indicator := "[ ]"
		if m.selected[i] {
			indicator = "[✓]"
		}

		line := fmt.Sprintf("%s %-19s %-45s %-30s",
			indicator, timestamp, title, project)

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
		s.WriteString("\n")
		scrollInfo := fmt.Sprintf("[%d-%d/%d]", start+1, end, len(m.chats))
		s.WriteString(dimStyle.Render(scrollInfo))
	}

	// Status messages
	s.WriteString("\n\n")
	if m.error != "" {
		s.WriteString(errorStyle.Render("Error: " + m.error))
		s.WriteString("\n")
	}
	if m.deleted > 0 {
		s.WriteString(successStyle.Render(fmt.Sprintf("✓ Deleted %d chat(s)", m.deleted)))
		s.WriteString("\n")
	}

	// Confirmation dialog
	if m.confirmDelete {
		s.WriteString("\n")
		s.WriteString(errorStyle.Render(fmt.Sprintf("Delete %d chat(s)?", len(m.selected))))
		s.WriteString(" ")
		s.WriteString(helpStyle.Render("[ENTER=Yes] [ESC=No]"))
		s.WriteString("\n")
	} else {
		// Help
		help := "↑/↓:Navigate | SPACE:Select/Deselect | D:Delete | R:Refresh | Q:Quit"
		s.WriteString(helpStyle.Render(help))
	}

	return s.String()
}

// Messages
type deleteCompleteMsg struct {
	count int
}

type errMsg string

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
					count++
				}
			}
		}
		return deleteCompleteMsg{count: count}
	}
}

// File operations

func findAllChats() []Chat {
	var chats []Chat

	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return chats
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		projectPath := filepath.Join(projectsDir, entry.Name())
		files, err := filepath.Glob(filepath.Join(projectPath, "*.jsonl"))
		if err != nil {
			continue
		}

		for _, file := range files {
			basename := filepath.Base(file)
			uuid := strings.TrimSuffix(basename, ".jsonl")

			// Skip agent files
			if strings.HasPrefix(uuid, "agent-") {
				continue
			}

			title := getChatTitle(file)
			timestamp := getChatTimestamp(file)

			chats = append(chats, Chat{
				UUID:      uuid,
				Title:     title,
				Timestamp: timestamp,
				Project:   entry.Name(),
				Path:      file,
			})
		}
	}

	// Sort by timestamp (newest first)
	sort.Slice(chats, func(i, j int) bool {
		return chats[i].Timestamp > chats[j].Timestamp
	})

	return chats
}

func getChatTitle(jsonlFile string) string {
	file, err := os.Open(jsonlFile)
	if err != nil {
		return "[Error opening file]"
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		if lineNum == 2 { // Second line (first user message)
			var msg JSONLMessage
			if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
				return "[Error parsing JSON]"
			}

			if msg.Type == "user" {
				content := msg.Message.Content
				if len(content) > 60 {
					return content[:60] + "..."
				}
				return content
			}
		}
		if lineNum > 2 {
			break
		}
	}

	return "[No title]"
}

func getChatTimestamp(jsonlFile string) string {
	info, err := os.Stat(jsonlFile)
	if err != nil {
		return "Unknown"
	}
	return info.ModTime().Format("2006-01-02 15:04:05")
}

func findRelatedFiles(uuid string) []string {
	var files []string

	// Main JSONL file
	matches, _ := filepath.Glob(filepath.Join(projectsDir, "*", uuid+".jsonl"))
	for _, m := range matches {
		files = append(files, m)
	}

	// Debug file
	debugFile := filepath.Join(debugDir, uuid+".txt")
	if _, err := os.Stat(debugFile); err == nil {
		files = append(files, debugFile)
	}

	// Todo files
	todoMatches, _ := filepath.Glob(filepath.Join(todosDir, uuid+"*.json"))
	files = append(files, todoMatches...)

	// Session directory
	sessionPath := filepath.Join(sessionDir, uuid)
	if _, err := os.Stat(sessionPath); err == nil {
		files = append(files, sessionPath)
	}

	return files
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
