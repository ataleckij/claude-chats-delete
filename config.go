package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	tasksDir       string
	fileHistoryDir string
	plansDir       string
	agentsDir      string
)

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
	tasksDir = filepath.Join(claudeDir, "tasks")
	fileHistoryDir = filepath.Join(claudeDir, "file-history")
	plansDir = filepath.Join(claudeDir, "plans")
	agentsDir = filepath.Join(claudeDir, "agents")
}
