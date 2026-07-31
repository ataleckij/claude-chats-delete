package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// jsonlScanTokenMax caps a single JSONL record. Records carry tool results and
// inlined attachments, so they routinely exceed bufio's 64 KiB default; a record
// above this cap stops the scan, which callers detect via scanner.Err().
// Overridable in tests.
var jsonlScanTokenMax = 16 * 1024 * 1024

// newJSONLScanner returns a scanner sized for transcript records. It starts
// small and grows on demand, so large caps cost nothing on ordinary files. The
// starting buffer never exceeds the cap: bufio uses the larger of the two, which
// would otherwise silently raise the effective limit.
func newJSONLScanner(r io.Reader) *bufio.Scanner {
	start := 64 * 1024
	if jsonlScanTokenMax < start {
		start = jsonlScanTokenMax
	}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, start), jsonlScanTokenMax)
	return scanner
}

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

			title, version, forkParentID, lineCount := scanChatMetadata(file)
			timestamp := getChatTimestamp(file)

			// TODO: Get messageCount from index if available
			// msgCount := messageCountMap[uuid]

			chats = append(chats, Chat{
				UUID:      uuid,
				Title:     title,
				Timestamp: timestamp,
				Project:   entry.Name(),
				Version:   version,
				// MessageCount: msgCount,
				LineCount:    lineCount,
				Path:         file,
				ForkParentID: forkParentID,
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
	// Ignore interrupt placeholder text used in some transcript variants.
	if cleaned == "[Request interrupted by user]" {
		return ""
	}

	return cleaned
}

// scanChatMetadata reads a chat JSONL file in a single pass and extracts
// display metadata (title, version, line count). Title priority matches the
// Claude Code --resume picker: customTitle (/rename) > aiTitle (auto-generated,
// v2.1.x) > agentName (readable session name) > first user message > summary
// fallback. Replaces three separate file scans.
//
// Scans the full file without an early exit: late /rename and ai-title records
// can appear at any line and lineCount needs the whole file, so any bail-out cap
// would silently break title detection on long sessions.
func scanChatMetadata(jsonlFile string) (title, version, forkParentID string, lineCount int) {
	file, err := os.Open(jsonlFile)
	if err != nil {
		return "[Error opening file]", "", "", 0
	}
	defer file.Close()

	scanner := newJSONLScanner(file)

	// Titles use last-wins (rewritten over the session, keep the latest);
	// firstUserMsg/firstSummary use first-wins (guarded by == "", keep the
	// earliest). Don't mix the two strategies up when editing the loop below.
	var firstUserMsg, firstSummary, lastCustomTitle, lastAiTitle, lastAgentName string

	for scanner.Scan() {
		lineCount++

		var msg JSONLMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}

		if version == "" && msg.Version != "" {
			version = msg.Version
		}

		if forkParentID == "" && msg.ForkedFrom.SessionID != "" {
			forkParentID = msg.ForkedFrom.SessionID
		}

		// /rename writes a dedicated record; last one wins.
		if msg.Type == "custom-title" && msg.CustomTitle != "" {
			lastCustomTitle = msg.CustomTitle
			continue
		}

		// Auto-generated title; rewritten as the conversation evolves, last wins.
		if msg.Type == "ai-title" && msg.AiTitle != "" {
			lastAiTitle = msg.AiTitle
			continue
		}

		// Readable session name (e.g. named background sessions); last wins.
		if msg.Type == "agent-name" && msg.AgentName != "" {
			lastAgentName = msg.AgentName
			continue
		}

		if firstSummary == "" && msg.Type == "summary" && msg.Summary != "" {
			firstSummary = msg.Summary
			continue
		}

		if firstUserMsg == "" && msg.Type == "user" && !msg.IsMeta {
			if c := cleanSystemTags(msg.Message.Content); c != "" {
				firstUserMsg = c
			}
		}
	}

	switch {
	case lastCustomTitle != "":
		title = lastCustomTitle
	case lastAiTitle != "":
		title = lastAiTitle
	case lastAgentName != "":
		title = lastAgentName
	case firstUserMsg != "":
		title = firstUserMsg
	case firstSummary != "":
		title = firstSummary
	default:
		title = "[No title]"
	}

	// A scan that stopped early (a record above the size limit, or an I/O error)
	// leaves the title and line count based on a partial read. Mark it so the
	// list does not present incomplete metadata as complete.
	if scanner.Err() != nil {
		title += " [partial scan]"
	}
	return
}

// getChatTitle returns just the title. Retained for test compatibility.
func getChatTitle(jsonlFile string) string {
	title, _, _, _ := scanChatMetadata(jsonlFile)
	return title
}

// getChatVersion returns just the version. Retained for test compatibility.
func getChatVersion(jsonlFile string) string {
	_, version, _, _ := scanChatMetadata(jsonlFile)
	return version
}

func getChatTimestamp(jsonlFile string) string {
	info, err := os.Stat(jsonlFile)
	if err != nil {
		return "Unknown"
	}
	return info.ModTime().Format("2006-01-02 15:04:05")
}

// getSlugFromChat returns the plan slug referenced by a transcript. ok is false
// when the file could not be read to the end, so callers can tell "no slug" from
// "unknown" and avoid deleting a plan on incomplete evidence.
func getSlugFromChat(jsonlFile string) (slug string, ok bool) {
	file, err := os.Open(jsonlFile)
	if err != nil {
		return "", false
	}
	defer file.Close()

	scanner := newJSONLScanner(file)

	// Scan all lines to find slug (it can be in any message)
	for scanner.Scan() {
		var msg JSONLMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}
		if msg.Slug != "" {
			return msg.Slug, true
		}
	}

	return "", scanner.Err() == nil
}

// isSlugUsedInOtherChats checks whether slug is still referenced by chats other
// than the one currently being deleted.
func isSlugUsedInOtherChats(slug string, excludeUUID string) bool {
	if slug == "" {
		return false
	}

	matches, err := filepath.Glob(filepath.Join(projectsDir, "*", "*.jsonl"))
	if err != nil {
		return true // safe default: keep plan file if we cannot verify
	}

	for _, path := range matches {
		uuid := strings.TrimSuffix(filepath.Base(path), ".jsonl")
		if uuid == excludeUUID {
			continue
		}
		other, ok := getSlugFromChat(path)
		if !ok {
			return true // safe default: a transcript we could not read may use it
		}
		if other == slug {
			return true
		}
	}

	return false
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
	// Guard against an empty uuid: the lookups below build paths and globs from
	// it, and an empty one would match unrelated sessions' state.
	if uuid == "" {
		return nil
	}

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

	// Debug file
	debugFile := filepath.Join(debugDir, uuid+".txt")
	if _, err := os.Stat(debugFile); err == nil {
		files = append(files, debugFile)
	}

	// Session-scoped security warning dedupe state (security-guidance hook),
	// including the sidecar lock file written next to it.
	for _, ext := range []string{".json", ".lock"} {
		p := filepath.Join(securityDir, "security_warnings_state_"+uuid+ext)
		if _, err := os.Stat(p); err == nil {
			files = append(files, p)
		}
	}

	// Failed telemetry events, named 1p_failed_events.<session-uuid>.<id>.json.
	// The uuid must be a whole dot-separated component: a bare substring match
	// would also hit files that merely carry it as their trailing id.
	telemetryMatches, _ := filepath.Glob(filepath.Join(telemetryDir, "*."+uuid+".*.json"))
	files = append(files, telemetryMatches...)

	files = append(files, jobRelatedFiles(uuid)...)

	// Session directory
	sessionPath := filepath.Join(sessionDir, uuid)
	if _, err := os.Stat(sessionPath); err == nil {
		files = append(files, sessionPath)
	}

	// Task state directory
	tasksPath := filepath.Join(tasksDir, uuid)
	if _, err := os.Stat(tasksPath); err == nil {
		files = append(files, tasksPath)
	}

	// File history directory
	fileHistoryPath := filepath.Join(fileHistoryDir, uuid)
	if _, err := os.Stat(fileHistoryPath); err == nil {
		files = append(files, fileHistoryPath)
	}

	files = append(files, legacyRelatedFiles(uuid, chatJSONLPath)...)

	return files
}

// jobState is the part of jobs/<id>/state.json this tool relies on.
type jobState struct {
	SessionID           string `json:"sessionId"`
	ForkParentSessionID string `json:"forkParentSessionId"`
}

func readJobState(dir string) (jobState, bool) {
	data, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		return jobState{}, false
	}
	var s jobState
	if err := json.Unmarshal(data, &s); err != nil {
		return jobState{}, false
	}
	return s, true
}

// jobRelatedFiles returns background-job state belonging to a session
// (v2.1.212+). Job directories are named after the first 8 hex chars of the
// session uuid, so every candidate is confirmed against the sessionId recorded
// inside its state.json before being scheduled for deletion. jobs/pins.json is
// global and never returned.
//
// A fork keeps a copy of its parent's transcript in tmp/parent-transcript.jsonl.
// That copy is returned when the parent is the session being deleted, so the
// parent's content does not survive in the fork's job directory; the fork's own
// transcript is self-contained and stays usable.
func jobRelatedFiles(uuid string) []string {
	// A job whose state.json omits sessionId reads back as an empty string, so
	// an empty uuid would match it.
	if uuid == "" {
		return nil
	}

	entries, err := os.ReadDir(jobsDir)
	if err != nil {
		return nil
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(jobsDir, e.Name())
		state, ok := readJobState(dir)
		if !ok {
			continue
		}
		switch uuid {
		case state.SessionID:
			files = append(files, dir)
		case state.ForkParentSessionID:
			parentCopy := filepath.Join(dir, "tmp", "parent-transcript.jsonl")
			if _, err := os.Stat(parentCopy); err == nil {
				files = append(files, parentCopy)
			}
		}
	}
	return files
}

// legacyRelatedFiles returns session files from the pre-v2.1.220 layout. These
// paths are no longer created by current Claude Code versions, but histories
// written by older ones still carry them, so they stay probed during the
// transition. Each entry is a no-op when the directory does not exist.
func legacyRelatedFiles(uuid, chatJSONLPath string) []string {
	var files []string

	// Security warning state used to live directly in the Claude dir before it
	// moved into security/.
	securityWarningsStateFile := filepath.Join(claudeDir, "security_warnings_state_"+uuid+".json")
	if _, err := os.Stat(securityWarningsStateFile); err == nil {
		files = append(files, securityWarningsStateFile)
	}

	// Todo files
	todoMatches, _ := filepath.Glob(filepath.Join(todosDir, uuid+"*.json"))
	files = append(files, todoMatches...)

	// Plan file (via slug), deleted only when no other chat references the slug
	if chatJSONLPath != "" {
		slug, ok := getSlugFromChat(chatJSONLPath)
		if ok && slug != "" && !isSlugUsedInOtherChats(slug, uuid) {
			planFile := filepath.Join(plansDir, slug+".md")
			if _, err := os.Stat(planFile); err == nil {
				files = append(files, planFile)
			}
		}
	}

	// Session-scoped agent memory (v2.1.33+). Agent memory now lives in
	// agent-memory/<agent-name>/ and is project-scoped, so it is preserved on
	// delete; only the old per-agent memory-local.md was session-scoped.
	if chatJSONLPath != "" {
		// An incomplete scan is safe to ignore here: every id it did find is
		// genuinely this chat's, and ids it missed only mean their memory is
		// left behind. Unlike the plan file, a partial result cannot point at
		// another chat's data.
		agentIDs, _ := parseAgentIDs(chatJSONLPath)
		for _, agentID := range agentIDs {
			localMemory := filepath.Join(agentsDir, agentID, "memory-local.md")
			if _, err := os.Stat(localMemory); err == nil {
				files = append(files, localMemory)
			}

			// memory-project.md and memory-user.md may be shared across chats
			// and are intentionally kept.
		}
	}

	return files
}

// parseAgentIDs extracts agent IDs from a chat JSONL file. ok is false when the
// file could not be read to the end, in which case the list may be incomplete.
func parseAgentIDs(chatFile string) (ids []string, ok bool) {
	var agentIDs []string
	seen := make(map[string]bool)

	file, err := os.Open(chatFile)
	if err != nil {
		return agentIDs, false
	}
	defer file.Close()

	scanner := newJSONLScanner(file)
	for scanner.Scan() {
		var msg struct {
			AgentID string `json:"agentId"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &msg); err == nil {
			if msg.AgentID != "" && !seen[msg.AgentID] {
				agentIDs = append(agentIDs, msg.AgentID)
				seen[msg.AgentID] = true
			}
		}
	}

	return agentIDs, scanner.Err() == nil
}

// deleteChats deletes all files related to the given chats and updates sessions index.
// Returns count of deleted chats or an error.
func deleteChats(chats []Chat) (int, error) {
	count := 0
	for _, chat := range chats {
		files := findRelatedFiles(chat.UUID)
		for _, file := range files {
			if err := os.RemoveAll(file); err != nil {
				return 0, fmt.Errorf("failed to delete %s: %w", file, err)
			}
		}
		if err := updateSessionsIndex(chat.UUID); err != nil {
			return 0, fmt.Errorf("failed to update index: %w", err)
		}
		count++
	}
	return count, nil
}
