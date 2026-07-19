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
	GroupByProject         bool   `json:"group_by_project"`
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

	// ForkParentID identifies the parent session referenced by a fork, or is empty
	// for a regular session. Populated from transcript-level forkedFrom
	// back-references (written v2.1.118 through ~v2.1.206). Verified on v2.1.215:
	// fork transcripts are full self-contained copies (deleting the parent keeps
	// the fork's history intact), forkedFrom is no longer written, and the parent
	// linkage lives in ~/.claude/jobs/<id>/state.json (forkParentSessionId) —
	// so this field stays empty for new forks. Kept for older transcripts and
	// future "(Branch of …)" labels.
	ForkParentID string
}

// JSONLMessage represents a message in the JSONL file
type JSONLMessage struct {
	Type        string `json:"type"`
	Version     string `json:"version"`
	Slug        string `json:"slug"`
	IsMeta      bool   `json:"isMeta"`
	Summary     string `json:"summary"`
	CustomTitle string `json:"customTitle"`
	AiTitle     string `json:"aiTitle"`
	AgentName   string `json:"agentName"`
	Message     struct {
		Content string `json:"content"`
	} `json:"message"`
	ForkedFrom struct {
		SessionID string `json:"sessionId"`
	} `json:"forkedFrom"`
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

// TODO: directories under ~/.claude that are not yet covered. For each, decide
// whether per-chat cleanup applies (UUID-keyed -> include in findRelatedFiles)
// or whether it belongs to a global retention pool (timestamp-keyed -> ignore).
//   - worktrees/  (v2.1.157+) background-agent git worktrees
//   - workflows/  (v2.1.154+) dynamic-workflow run state
//   - jobs/       (v2.1.212+) background-session job state, keyed by the first
//                 8 hex chars of the session UUID (state.json, timeline.jsonl).
//                 A fork's tmp/parent-transcript.jsonl holds a COPY of the
//                 parent session's transcript, so deleting the parent leaves
//                 its content behind here. pins.json in the same dir is global.
// Inspect on-disk layout before adding; do not guess UUID-keyed-ness from the name.

// TODO: fork/parent handling (verified on v2.1.215): fork JSONLs are full
// self-contained copies, so deleting a parent does not break a fork's history.
// Remaining work: fork detection for v2.1.212+ must read
// ~/.claude/jobs/<id>/state.json (forkParentSessionId) since transcripts no
// longer carry forkedFrom; and deleting a parent still leaves a copy of its
// transcript in the fork's jobs tmp dir (see the jobs/ entry above).

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
