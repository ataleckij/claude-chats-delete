package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanSystemTags(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plain text unchanged",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "removes system-reminder tag",
			input: "before<system-reminder>secret</system-reminder>after",
			want:  "beforeafter",
		},
		{
			name:  "removes local-command-caveat tag",
			input: "text<local-command-caveat>caveat content</local-command-caveat>more",
			want:  "textmore",
		},
		{
			name:  "newlines replaced with spaces",
			input: "line one\nline two\nline three",
			want:  "line one line two line three",
		},
		{
			name:  "carriage returns removed",
			input: "line one\r\nline two",
			want:  "line one line two",
		},
		{
			name:  "multiple spaces collapsed",
			input: "too   many   spaces",
			want:  "too many spaces",
		},
		{
			name:  "empty string returns empty",
			input: "",
			want:  "",
		},
		{
			name:  "only tags returns empty",
			input: "<system-reminder>hidden</system-reminder>",
			want:  "",
		},
		{
			name:  "output never contains newline",
			input: "first\nsecond\nthird",
			want:  "first second third",
		},
		{
			name:  "unclosed tag removes everything after it",
			input: "before<system-reminder>no closing tag",
			want:  "before",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanSystemTags(tt.input)
			if got != tt.want {
				t.Errorf("cleanSystemTags(%q) = %q, want %q", tt.input, got, tt.want)
			}
			// Critical: output must never contain newlines (would break UI layout)
			if strings.Contains(got, "\n") {
				t.Errorf("cleanSystemTags(%q) contains newline in output: %q", tt.input, got)
			}
			if strings.Contains(got, "\r") {
				t.Errorf("cleanSystemTags(%q) contains carriage return in output: %q", tt.input, got)
			}
		})
	}
}

// writeTempJSONL writes JSONL lines to a temp file and returns its path.
func writeTempJSONL(t *testing.T, lines []string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "*.jsonl")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer f.Close()
	for _, line := range lines {
		f.WriteString(line + "\n")
	}
	return f.Name()
}

func TestGetChatTitle(t *testing.T) {
	// line1 is always skipped (file-history-snapshot)
	const line1 = `{"type":"snapshot","content":"file-history"}`

	tests := []struct {
		name  string
		lines []string
		want  string
	}{
		{
			name: "returns first user message content",
			lines: []string{
				line1,
				`{"type":"user","message":{"content":"what is the meaning of life"},"isMeta":false,"version":"2.1.49"}`,
			},
			want: "what is the meaning of life",
		},
		{
			name: "skips meta messages",
			lines: []string{
				line1,
				`{"type":"user","message":{"content":"meta stuff"},"isMeta":true}`,
				`{"type":"user","message":{"content":"real question"},"isMeta":false}`,
			},
			want: "real question",
		},
		{
			name: "falls back to summary when no user message",
			lines: []string{
				line1,
				`{"type":"summary","summary":"this is the summary","leafUuid":"abc"}`,
				`{"type":"assistant","message":{"content":"response"}}`,
			},
			want: "this is the summary",
		},
		{
			name: "returns [No title] when file has only line1",
			lines: []string{
				line1,
			},
			want: "[No title]",
		},
		{
			name: "strips system tags from content",
			lines: []string{
				line1,
				`{"type":"user","message":{"content":"<system-reminder>hidden</system-reminder>real content"},"isMeta":false}`,
			},
			want: "real content",
		},
		{
			name: "title with newlines is cleaned — output has no newlines",
			lines: []string{
				line1,
				`{"type":"user","message":{"content":"line one\nline two\nline three"},"isMeta":false}`,
			},
			want: "line one line two line three",
		},
		{
			name: "content that is only system tags falls through to [No title]",
			lines: []string{
				line1,
				`{"type":"user","message":{"content":"<system-reminder>secret</system-reminder>"},"isMeta":false}`,
			},
			want: "[No title]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempJSONL(t, tt.lines)
			got := getChatTitle(path)
			if got != tt.want {
				t.Errorf("getChatTitle() = %q, want %q", got, tt.want)
			}
			// Critical regression: title must never contain newline (breaks UI layout)
			if strings.Contains(got, "\n") {
				t.Errorf("getChatTitle() returned title with newline: %q", got)
			}
		})
	}
}

// setupStorageDirs creates a temp ~/.claude-like structure and wires global path vars.
// All globals are restored automatically when the test ends.
func setupStorageDirs(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()

	// save originals
	origProjects := projectsDir
	origDebug := debugDir
	origTodos := todosDir
	origSession := sessionDir
	origTasks := tasksDir
	origFileHistory := fileHistoryDir
	origPlans := plansDir
	origAgents := agentsDir

	projectsDir = filepath.Join(tmp, "projects")
	debugDir = filepath.Join(tmp, "debug")
	todosDir = filepath.Join(tmp, "todos")
	sessionDir = filepath.Join(tmp, "session-env")
	tasksDir = filepath.Join(tmp, "tasks")
	fileHistoryDir = filepath.Join(tmp, "file-history")
	plansDir = filepath.Join(tmp, "plans")
	agentsDir = filepath.Join(tmp, "agents")

	for _, d := range []string{projectsDir, debugDir, todosDir, sessionDir, tasksDir, fileHistoryDir, plansDir, agentsDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}

	t.Cleanup(func() {
		projectsDir = origProjects
		debugDir = origDebug
		todosDir = origTodos
		sessionDir = origSession
		tasksDir = origTasks
		fileHistoryDir = origFileHistory
		plansDir = origPlans
		agentsDir = origAgents
	})

	return tmp
}

func TestFindRelatedFiles(t *testing.T) {
	setupStorageDirs(t)

	uuid := "deadbeef-1234-5678-abcd-000000000001"
	project := "my-project"

	projDir := filepath.Join(projectsDir, project)
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}

	// JSONL with a slug so the plan file can be discovered
	jsonlContent := "{\"type\":\"snapshot\"}\n" +
		"{\"type\":\"user\",\"message\":{\"content\":\"hi\"},\"slug\":\"my-slug\",\"isMeta\":false}\n"
	jsonlPath := filepath.Join(projDir, uuid+".jsonl")
	if err := os.WriteFile(jsonlPath, []byte(jsonlContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create all known artifact types
	wantFiles := map[string]string{
		"jsonl":        jsonlPath,
		"debug":        filepath.Join(debugDir, uuid+".txt"),
		"todo":         filepath.Join(todosDir, uuid+"-todos.json"),
		"session-env":  filepath.Join(sessionDir, uuid),
		"tasks":        filepath.Join(tasksDir, uuid),
		"file-history": filepath.Join(fileHistoryDir, uuid),
		"subagents":    filepath.Join(projDir, uuid),
		"plan":         filepath.Join(plansDir, "my-slug.md"),
	}

	for key, path := range wantFiles {
		if key == "jsonl" {
			continue
		}
		if key == "session-env" || key == "tasks" || key == "file-history" || key == "subagents" {
			if err := os.MkdirAll(path, 0755); err != nil {
				t.Fatalf("mkdir %s: %v", key, err)
			}
		} else {
			if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
				t.Fatalf("write %s: %v", key, err)
			}
		}
	}

	got := findRelatedFiles(uuid)
	found := make(map[string]bool)
	for _, f := range got {
		found[f] = true
	}

	for key, path := range wantFiles {
		if !found[path] {
			t.Errorf("findRelatedFiles missing %s artifact: %s", key, path)
		}
	}
}

func TestFindRelatedFiles_AgentMemory(t *testing.T) {
	setupStorageDirs(t)

	uuid := "deadbeef-0000-0000-0000-000000000002"
	agentID := "agent-abc123"

	projDir := filepath.Join(projectsDir, "agent-project")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}

	// JSONL referencing an agent_id
	jsonlContent := "{\"type\":\"snapshot\"}\n" +
		"{\"type\":\"user\",\"message\":{\"content\":\"hi\"},\"isMeta\":false,\"agent_id\":\"" + agentID + "\"}\n"
	if err := os.WriteFile(filepath.Join(projDir, uuid+".jsonl"), []byte(jsonlContent), 0644); err != nil {
		t.Fatal(err)
	}

	// memory-local.md — should be deleted with the chat
	localMemory := filepath.Join(agentsDir, agentID, "memory-local.md")
	if err := os.MkdirAll(filepath.Dir(localMemory), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(localMemory, []byte("local memory"), 0644); err != nil {
		t.Fatal(err)
	}

	// memory-project.md and memory-user.md — must NOT be deleted
	projectMemory := filepath.Join(agentsDir, agentID, "memory-project.md")
	userMemory := filepath.Join(agentsDir, agentID, "memory-user.md")
	if err := os.WriteFile(projectMemory, []byte("project memory"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userMemory, []byte("user memory"), 0644); err != nil {
		t.Fatal(err)
	}

	got := findRelatedFiles(uuid)
	found := make(map[string]bool)
	for _, f := range got {
		found[f] = true
	}

	if !found[localMemory] {
		t.Errorf("findRelatedFiles must include memory-local.md: %s", localMemory)
	}
	if found[projectMemory] {
		t.Errorf("findRelatedFiles must NOT include memory-project.md: %s", projectMemory)
	}
	if found[userMemory] {
		t.Errorf("findRelatedFiles must NOT include memory-user.md: %s", userMemory)
	}
}

func TestUpdateSessionsIndex(t *testing.T) {
	setupStorageDirs(t)

	projDir := filepath.Join(projectsDir, "test-project")
	os.MkdirAll(projDir, 0755)

	targetUUID := "uuid-to-delete"
	keepUUID := "uuid-to-keep"

	indexContent := `{
		"version": 1,
		"entries": [
			{"sessionId": "` + targetUUID + `", "firstPrompt": "delete me"},
			{"sessionId": "` + keepUUID + `", "firstPrompt": "keep me"}
		]
	}`
	indexPath := filepath.Join(projDir, "sessions-index.json")
	if err := os.WriteFile(indexPath, []byte(indexContent), 0644); err != nil {
		t.Fatal(err)
	}

	if err := updateSessionsIndex(targetUUID); err != nil {
		t.Fatalf("updateSessionsIndex error: %v", err)
	}

	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	if strings.Contains(content, targetUUID) {
		t.Errorf("sessions-index still contains deleted UUID %q", targetUUID)
	}
	if !strings.Contains(content, keepUUID) {
		t.Errorf("sessions-index lost the kept UUID %q", keepUUID)
	}
}
