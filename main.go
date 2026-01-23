package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Config stores application configuration
type Config struct {
	ClaudeDir string `json:"claude_dir"`
}

// Chat represents a single chat session
type Chat struct {
	UUID      string
	Title     string
	Timestamp string
	Project   string
	Version   string
	Path      string
	Files     []string // related files for deletion
}

// JSONLMessage represents a message in the JSONL file
type JSONLMessage struct {
	Type    string `json:"type"`
	Version string `json:"version"`
	Slug    string `json:"slug"`
	IsMeta  bool   `json:"isMeta"`
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
}

// SessionsIndex represents the sessions-index.json structure
type SessionsIndex struct {
	Version      int            `json:"version"`
	Entries      []SessionEntry `json:"entries"`
	OriginalPath string         `json:"originalPath"`
}

type SessionEntry struct {
	SessionID    string `json:"sessionId"`
	FullPath     string `json:"fullPath"`
	FileMtime    int64  `json:"fileMtime"`
	FirstPrompt  string `json:"firstPrompt"`
	Summary      string `json:"summary"`
	MessageCount int    `json:"messageCount"`
	Created      string `json:"created"`
	Modified     string `json:"modified"`
	GitBranch    string `json:"gitBranch"`
	ProjectPath  string `json:"projectPath"`
	IsSidechain  bool   `json:"isSidechain"`
}

var (
	configPath     = filepath.Join(os.Getenv("HOME"), ".config", "claude-chats", "config.json")
	claudeDir      string
	projectsDir    string
	debugDir       string
	todosDir       string
	sessionDir     string
	fileHistoryDir string
	plansDir       string

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
	width         int
	height        int
	scrollOffset  int
	copiedMsg     string
	copyId        int
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

		case "c":
			// Copy UUID to clipboard
			if m.cursor < len(m.chats) {
				uuid := m.chats[m.cursor].UUID
				if err := copyToClipboard(uuid); err != nil {
					m.error = fmt.Sprintf("Failed to copy: %v", err)
				} else {
					m.copyId++
					currentId := m.copyId
					m.copiedMsg = fmt.Sprintf("Chat UUID copied: %s", uuid)
					return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
						return clearCopiedMsg{id: currentId}
					})
				}
			}
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

	case clearCopiedMsg:
		if msg.id == m.copyId {
			m.copiedMsg = ""
		}
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

	// Calculate column widths based on terminal width
	// Fixed: indicator(4) + timestamp(19) + version(8) + gaps(6) = 37
	width := m.width
	if width < 70 {
		width = 70 // minimum width
	}

	versionWidth := 8
	remaining := width - 37
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
	s.WriteString("\n\n")

	// Column headers
	headerFmt := fmt.Sprintf("    %%-19s  %%-%ds  %%-%ds  %%-%ds", versionWidth, titleWidth, projectWidth)
	header := fmt.Sprintf(headerFmt, "TIMESTAMP", "VERSION", "TITLE", "PROJECT")
	s.WriteString(dimStyle.Render(header))
	s.WriteString("\n")
	s.WriteString(strings.Repeat("─", width))
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
		if len(title) > titleWidth-2 {
			title = title[:titleWidth-2] + ".."
		}

		project := chat.Project
		if len(project) > projectWidth-2 {
			project = project[:projectWidth-2] + ".."
		}

		version := chat.Version
		if len(version) > versionWidth-1 {
			version = version[:versionWidth-1]
		}

		// Selection indicator
		indicator := "[ ]"
		if m.selected[i] {
			indicator = "[✓]"
		}

		lineFmt := fmt.Sprintf("%%s %%-19s  %%-%ds  %%-%ds  %%-%ds", versionWidth, titleWidth, projectWidth)
		line := fmt.Sprintf(lineFmt, indicator, timestamp, version, title, project)

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
	if m.copiedMsg != "" {
		s.WriteString(successStyle.Render("✓ " + m.copiedMsg))
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
		help := "↑/↓:Navigate | SPACE:Select/Deselect | C:Copy UUID | D:Delete | R:Refresh | Q:Quit"
		s.WriteString(helpStyle.Render(help))
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
			version := getChatVersion(file)

			chats = append(chats, Chat{
				UUID:      uuid,
				Title:     title,
				Timestamp: timestamp,
				Project:   entry.Name(),
				Version:   version,
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

func cleanSystemTags(content string) string {
	// Remove content within system tags (including the tags themselves)
	systemTagPairs := [][2]string{
		{"<local-command-caveat>", "</local-command-caveat>"},
		{"<command-name>", "</command-name>"},
		{"<command-message>", "</command-message>"},
		{"<command-args>", "</command-args>"},
		{"<local-command-stdout>", "</local-command-stdout>"},
		{"<system-reminder>", "</system-reminder>"},
	}

	cleaned := content
	for _, pair := range systemTagPairs {
		start := strings.Index(cleaned, pair[0])
		for start >= 0 {
			end := strings.Index(cleaned[start:], pair[1])
			if end >= 0 {
				end += start + len(pair[1])
				cleaned = cleaned[:start] + cleaned[end:]
			} else {
				// No closing tag, remove from start tag onwards
				cleaned = cleaned[:start]
				break
			}
			start = strings.Index(cleaned, pair[0])
		}
	}

	// Trim whitespace and newlines
	cleaned = strings.TrimSpace(cleaned)

	// If content is empty or only contains tags/whitespace, return empty
	if cleaned == "" || strings.HasPrefix(cleaned, "<") {
		return ""
	}

	return cleaned
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
		if lineNum == 1 {
			continue // Skip first line (file-history-snapshot)
		}

		var msg JSONLMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}

		// Skip meta messages and find first real user message
		if msg.Type == "user" && !msg.IsMeta {
			content := msg.Message.Content
			// Clean up system tags
			content = cleanSystemTags(content)
			if content != "" {
				return content
			}
		}

		// Stop after checking reasonable number of lines
		if lineNum > 20 {
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

func getChatVersion(jsonlFile string) string {
	file, err := os.Open(jsonlFile)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		if lineNum == 2 { // Second line (first user message)
			var msg JSONLMessage
			if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
				return ""
			}
			return msg.Version
		}
		if lineNum > 2 {
			break
		}
	}

	return ""
}

func getSlugFromChat(jsonlFile string) string {
	file, err := os.Open(jsonlFile)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	// Scan all lines to find slug (it can be in any message)
	for scanner.Scan() {
		var msg JSONLMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}
		if msg.Slug != "" {
			return msg.Slug
		}
	}

	return ""
}

func updateSessionsIndex(uuid string) error {
	// Find all sessions-index.json files in project directories
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		indexPath := filepath.Join(projectsDir, entry.Name(), "sessions-index.json")
		if _, err := os.Stat(indexPath); os.IsNotExist(err) {
			continue
		}

		// Read the index
		data, err := os.ReadFile(indexPath)
		if err != nil {
			continue
		}

		var index SessionsIndex
		if err := json.Unmarshal(data, &index); err != nil {
			continue
		}

		// Filter out the deleted session
		originalLen := len(index.Entries)
		var newEntries []SessionEntry
		for _, entry := range index.Entries {
			if entry.SessionID != uuid {
				newEntries = append(newEntries, entry)
			}
		}

		// Only write if something was removed
		if len(newEntries) < originalLen {
			index.Entries = newEntries

			// Write back
			data, err := json.MarshalIndent(index, "", "  ")
			if err != nil {
				return err
			}

			if err := os.WriteFile(indexPath, data, 0644); err != nil {
				return err
			}
		}
	}

	return nil
}

func findRelatedFiles(uuid string) []string {
	var files []string
	var chatJSONLPath string

	// Main JSONL file and subagents directory
	matches, _ := filepath.Glob(filepath.Join(projectsDir, "*", uuid+".jsonl"))
	for _, m := range matches {
		files = append(files, m)
		chatJSONLPath = m // Save for slug extraction

		// Subagents directory (same name as jsonl but without extension)
		subagentsDir := strings.TrimSuffix(m, ".jsonl")
		if _, err := os.Stat(subagentsDir); err == nil {
			files = append(files, subagentsDir)
		}
	}

	// Plan file (via slug)
	if chatJSONLPath != "" {
		slug := getSlugFromChat(chatJSONLPath)
		if slug != "" {
			planFile := filepath.Join(plansDir, slug+".md")
			if _, err := os.Stat(planFile); err == nil {
				files = append(files, planFile)
			}
		}
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

	// File history directory
	fileHistoryPath := filepath.Join(fileHistoryDir, uuid)
	if _, err := os.Stat(fileHistoryPath); err == nil {
		files = append(files, fileHistoryPath)
	}

	return files
}

// Config management

func loadConfig() (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

func saveConfig(config *Config) error {
	// Create config directory if it doesn't exist
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

func promptForClaudeDir() (string, error) {
	defaultDir := filepath.Join(os.Getenv("HOME"), ".claude")

	fmt.Println("Claude Chat Manager - First Run Setup")
	fmt.Println()
	fmt.Printf("Enter the path to your Claude directory (default: %s)\n", defaultDir)
	fmt.Print("Path [press Enter for default]: ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return defaultDir, nil
	}

	// Expand ~ to home directory
	if strings.HasPrefix(input, "~") {
		input = filepath.Join(os.Getenv("HOME"), input[1:])
	}

	return input, nil
}

func initializePaths(dir string) {
	claudeDir = dir
	projectsDir = filepath.Join(claudeDir, "projects")
	debugDir = filepath.Join(claudeDir, "debug")
	todosDir = filepath.Join(claudeDir, "todos")
	sessionDir = filepath.Join(claudeDir, "session-env")
	fileHistoryDir = filepath.Join(claudeDir, "file-history")
	plansDir = filepath.Join(claudeDir, "plans")
}

func main() {
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

		// Save config
		config = &Config{ClaudeDir: dir}
		if err := saveConfig(config); err != nil {
			fmt.Printf("Warning: Could not save config: %v\n", err)
		} else {
			fmt.Printf("\n✓ Configuration saved to: %s\n\n", configPath)
		}
	}

	// Initialize paths from config
	initializePaths(config.ClaudeDir)

	// Run TUI
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
