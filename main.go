package main

import (
	"bufio"
	"encoding/json"
	"flag"
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
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
)

// Config stores application configuration
type Config struct {
	ClaudeDir              string `json:"claude_dir"`
	AutoUpdates            bool   `json:"auto_updates"`
	LastUpdateCheck        int64  `json:"last_update_check"`
	UpdateCheckIntervalHrs int    `json:"update_check_interval_hours"`
}

// Chat represents a single chat session
type Chat struct {
	UUID      string
	Title     string
	Timestamp string
	Project   string
	Version   string
	// MessageCount int // TODO: maybe re-enable later
	LineCount int
	Path      string
	Files     []string // related files for deletion
}

// JSONLMessage represents a message in the JSONL file
type JSONLMessage struct {
	Type    string `json:"type"`
	Version string `json:"version"`
	Slug    string `json:"slug"`
	IsMeta  bool   `json:"isMeta"`
	Summary string `json:"summary"`
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
	agentsDir      string

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

		case "pgdown", "ctrl+f":
			visibleHeight := m.height - 8
			if visibleHeight < 1 {
				visibleHeight = 10
			}
			m.cursor += visibleHeight
			if m.cursor >= len(m.chats) {
				m.cursor = len(m.chats) - 1
			}
			m.adjustScroll()

		case "pgup", "ctrl+b":
			visibleHeight := m.height - 8
			if visibleHeight < 1 {
				visibleHeight = 10
			}
			m.cursor -= visibleHeight
			if m.cursor < 0 {
				m.cursor = 0
			}
			m.adjustScroll()

		case "home", "g":
			m.cursor = 0
			m.adjustScroll()

		case "end", "G":
			if len(m.chats) > 0 {
				m.cursor = len(m.chats) - 1
			}
			m.adjustScroll()

		case "ctrl+d":
			visibleHeight := m.height - 8
			if visibleHeight < 1 {
				visibleHeight = 10
			}
			m.cursor += visibleHeight / 2
			if m.cursor >= len(m.chats) {
				m.cursor = len(m.chats) - 1
			}
			m.adjustScroll()

		case "ctrl+u":
			visibleHeight := m.height - 8
			if visibleHeight < 1 {
				visibleHeight = 10
			}
			m.cursor -= visibleHeight / 2
			if m.cursor < 0 {
				m.cursor = 0
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
		help := "↑/↓/PgUp/PgDn:Nav | Home/End:Jump | Ctrl+U/D:Half | SPACE:Toggle (A:All) | C:Copy ID | D:Delete | R:Refresh UI | Q:Quit"
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

		// TODO: Build a map of UUID -> messageCount from sessions-index.json if it exists
		// messageCountMap := make(map[string]int)
		// indexPath := filepath.Join(projectPath, "sessions-index.json")
		// if data, err := os.ReadFile(indexPath); err == nil {
		// 	var index SessionsIndex
		// 	if err := json.Unmarshal(data, &index); err == nil {
		// 		for _, sessionEntry := range index.Entries {
		// 			messageCountMap[sessionEntry.SessionID] = sessionEntry.MessageCount
		// 		}
		// 	}
		// }

		// Scan all JSONL files (original behavior)
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
			lineCount := countLines(file)

			// TODO: Get messageCount from index if available
			// msgCount := messageCountMap[uuid]

			chats = append(chats, Chat{
				UUID:      uuid,
				Title:     title,
				Timestamp: timestamp,
				Project:   entry.Name(),
				Version:   version,
				// MessageCount: msgCount,
				LineCount: lineCount,
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

	// Remove ALL newline characters from content
	cleaned = strings.ReplaceAll(cleaned, "\n", " ")
	cleaned = strings.ReplaceAll(cleaned, "\r", "")

	// Remove multiple spaces
	cleaned = strings.Join(strings.Fields(cleaned), " ")

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
	var firstSummary string

	for scanner.Scan() {
		lineNum++
		if lineNum == 1 {
			continue // Skip first line (file-history-snapshot)
		}

		var msg JSONLMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}

		// Save first summary as fallback
		if msg.Type == "summary" && msg.Summary != "" && firstSummary == "" {
			firstSummary = msg.Summary
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
		if lineNum > 100 {
			break
		}
	}

	// Fallback to summary if no user message found
	if firstSummary != "" {
		return firstSummary
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

func countLines(jsonlFile string) int {
	file, err := os.Open(jsonlFile)
	if err != nil {
		return 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		count++
	}

	return count
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

		// Tool results directory (within chat directory)
		chatDir := strings.TrimSuffix(m, ".jsonl")
		toolResultsDir := filepath.Join(chatDir, "tool-results")
		if _, err := os.Stat(toolResultsDir); err == nil {
			files = append(files, toolResultsDir)
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

	// Agent memory files (v2.1.33+)
	// Parse agent IDs from chat JSONL and delete local scope memory
	if chatJSONLPath != "" {
		agentIDs := parseAgentIDs(chatJSONLPath)
		for _, agentID := range agentIDs {
			// Delete local scope memory (always tied to this chat session)
			localMemory := filepath.Join(agentsDir, agentID, "memory-local.md")
			if _, err := os.Stat(localMemory); err == nil {
				files = append(files, localMemory)
			}

			// Note: We don't delete memory-project.md or memory-user.md as they may be
			// shared across multiple chats. Consider implementing reference counting
			// in a future version if needed.
		}
	}

	return files
}

// parseAgentIDs extracts agent IDs from chat JSONL file
func parseAgentIDs(chatFile string) []string {
	var agentIDs []string
	seen := make(map[string]bool)

	file, err := os.Open(chatFile)
	if err != nil {
		return agentIDs
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var msg struct {
			AgentID string `json:"agent_id"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &msg); err == nil {
			if msg.AgentID != "" && !seen[msg.AgentID] {
				agentIDs = append(agentIDs, msg.AgentID)
				seen[msg.AgentID] = true
			}
		}
	}

	return agentIDs
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
	agentsDir = filepath.Join(claudeDir, "agents")
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
