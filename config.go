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
	sessionDir     string
	tasksDir       string
	fileHistoryDir string
	securityDir    string
	telemetryDir   string
	jobsDir        string

	// Legacy layout, still probed for histories written by older Claude Code
	// versions. Verified absent on v2.1.220: todos/ and plans/ are no longer
	// created, security warning state moved into security/, and agents/ now
	// holds only agent definitions (memory moved to agent-memory/, which is
	// project-scoped and therefore preserved on delete).
	todosDir  string
	plansDir  string
	agentsDir string
)

// Directories under ~/.claude surveyed on v2.1.220, with their cleanup status:
//   - sessions/    runtime registry of live sessions, keyed by PID (holds
//                  sessionId, status). Owned by the running process, not the
//                  history: not deleted per chat.
//   - daemon/      supervisor state (roster.json lists live workers, control
//                  key, dispatch dir). Session ids appear only while a worker
//                  runs: runtime state, not deleted per chat.
//   - agent-memory/<agent>/  per-agent memory, project-scoped: preserved.
//   - worktrees/, workflows/  absent at user level; worktrees live in a
//                  project's own .claude/, so they are out of scope here.
//   - jobs/        background-session state, covered by findRelatedFiles.
//                  pins.json in that directory is global and never removed.
//
// TODO: verify later, purpose unknown - downloads/ has stayed empty since it
// appeared, so there is nothing to judge yet. Re-check whether it becomes
// session-keyed, and re-survey after Claude Code layout changes.

// TODO: fork detection is not implemented. Since v2.1.212 transcripts no longer
// carry forkedFrom, so a fork's parent can only be found by reading
// jobs/<id>/state.json (forkParentSessionId). Needed for "(Branch of …)" labels;
// not needed for safe deletion, since fork transcripts are self-contained
// (verified on v2.1.215).

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
	sessionDir = filepath.Join(claudeDir, "session-env")
	tasksDir = filepath.Join(claudeDir, "tasks")
	fileHistoryDir = filepath.Join(claudeDir, "file-history")
	securityDir = filepath.Join(claudeDir, "security")
	telemetryDir = filepath.Join(claudeDir, "telemetry")
	jobsDir = filepath.Join(claudeDir, "jobs")

	// Legacy layout (see the var block above)
	todosDir = filepath.Join(claudeDir, "todos")
	plansDir = filepath.Join(claudeDir, "plans")
	agentsDir = filepath.Join(claudeDir, "agents")
}
