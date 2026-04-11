package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

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

			title, version, lineCount := scanChatMetadata(file)
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
	// Ignore interrupt placeholder text used in some transcript variants.
	if cleaned == "[Request interrupted by user]" {
		return ""
	}

	return cleaned
}

// scanChatMetadata reads a chat JSONL file in a single pass and extracts
// display metadata (title, version, line count). Title priority matches the
// Claude Code --resume picker: customTitle (/rename) > first user message >
// summary fallback. Replaces three separate file scans.
//
// Scans the full file without an early exit: late /rename records can appear
// at any line and lineCount needs the whole file, so any bail-out cap would
// silently break rename detection on long sessions.
func scanChatMetadata(jsonlFile string) (title, version string, lineCount int) {
	file, err := os.Open(jsonlFile)
	if err != nil {
		return "[Error opening file]", "", 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 1024*1024) // 1MB buffer for large JSONL lines
	scanner.Buffer(buf, len(buf))

	var firstUserMsg, firstSummary, lastCustomTitle string

	for scanner.Scan() {
		lineCount++

		var msg JSONLMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}

		if version == "" && msg.Version != "" {
			version = msg.Version
		}

		// /rename writes a dedicated record; last one wins.
		if msg.Type == "custom-title" && msg.CustomTitle != "" {
			lastCustomTitle = msg.CustomTitle
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
	case firstUserMsg != "":
		title = firstUserMsg
	case firstSummary != "":
		title = firstSummary
	default:
		title = "[No title]"
	}
	return
}

// getChatTitle returns just the title. Retained for test compatibility.
func getChatTitle(jsonlFile string) string {
	title, _, _ := scanChatMetadata(jsonlFile)
	return title
}

// getChatVersion returns just the version. Retained for test compatibility.
func getChatVersion(jsonlFile string) string {
	_, version, _ := scanChatMetadata(jsonlFile)
	return version
}

func getChatTimestamp(jsonlFile string) string {
	info, err := os.Stat(jsonlFile)
	if err != nil {
		return "Unknown"
	}
	return info.ModTime().Format("2006-01-02 15:04:05")
}

func getSlugFromChat(jsonlFile string) string {
	file, err := os.Open(jsonlFile)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 1024*1024) // 1MB buffer for large JSONL lines
	scanner.Buffer(buf, len(buf))

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
		if getSlugFromChat(path) == slug {
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
		if slug != "" && !isSlugUsedInOtherChats(slug, uuid) {
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

	// Session-scoped security warning dedupe state (security-guidance hook)
	securityWarningsStateFile := filepath.Join(claudeDir, "security_warnings_state_"+uuid+".json")
	if _, err := os.Stat(securityWarningsStateFile); err == nil {
		files = append(files, securityWarningsStateFile)
	}

	// Todo files
	todoMatches, _ := filepath.Glob(filepath.Join(todosDir, uuid+"*.json"))
	files = append(files, todoMatches...)

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
	buf := make([]byte, 1024*1024) // 1MB buffer for large JSONL lines
	scanner.Buffer(buf, len(buf))
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

	return agentIDs
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
