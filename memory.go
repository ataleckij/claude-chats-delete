package main

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Claude Code's per-project auto-memory: projects/<project>/memory/ holds one
// markdown file per note plus an index of them. The index is bookkeeping over
// the notes rather than a note itself, so it is not listed.
const (
	memoryDirName   = "memory"
	memoryIndexFile = "MEMORY.md"
	memoryExt       = ".md"

	// A note's frontmatter sits at the very top, so only the head of the file
	// is read to find it.
	memoryHeadBytes = 8 * 1024
)

// MemoryFile is one file of a project's auto-memory.
type MemoryFile struct {
	Project     string
	Name        string
	Description string // from the frontmatter; empty when the file has none
	Type        string // from the frontmatter; empty when the file has none
	Path        string
	Modified    string // same layout as Chat.Timestamp
	Size        int64
}

// findAllMemoryFiles lists the memory notes of every project, newest first.
// The index file is left out, and subdirectories of memory/ are not descended
// into.
func findAllMemoryFiles() []MemoryFile {
	var files []MemoryFile

	projects, err := os.ReadDir(projectsDir)
	if err != nil {
		return files
	}

	for _, project := range projects {
		if !project.IsDir() {
			continue
		}
		dir := filepath.Join(projectsDir, project.Name(), memoryDirName)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), memoryExt) || e.Name() == memoryIndexFile {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			path := filepath.Join(dir, e.Name())
			description, kind := parseMemoryFrontmatter(readHead(path, memoryHeadBytes))
			files = append(files, MemoryFile{
				Project:     project.Name(),
				Name:        e.Name(),
				Description: description,
				Type:        kind,
				Path:        path,
				Modified:    info.ModTime().Format("2006-01-02 15:04:05"),
				Size:        info.Size(),
			})
		}
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].Modified != files[j].Modified {
			return files[i].Modified > files[j].Modified
		}
		return files[i].Path < files[j].Path
	})
	return files
}

// readHead returns up to limit bytes from the start of a file, or nil when it
// cannot be read.
func readHead(path string, limit int64) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit))
	if err != nil {
		return nil
	}
	return data
}

// parseMemoryFrontmatter pulls the description and type out of a note's
// frontmatter. It reads "key: value" lines between the opening and closing
// "---" and nothing more of YAML: the type may sit at the top level or nested
// under metadata, so indentation is ignored. Content without frontmatter, or
// with an unterminated one, yields empty strings.
func parseMemoryFrontmatter(data []byte) (description, kind string) {
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r") != "---" {
		return "", ""
	}

	closed := false
	for _, line := range lines[1:] {
		line = strings.TrimRight(line, "\r")
		if line == "---" {
			closed = true
			break
		}
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		switch key {
		case "description":
			description = unquoteYAMLScalar(value)
		case "type":
			kind = unquoteYAMLScalar(value)
		}
	}
	if !closed {
		return "", ""
	}
	return description, kind
}

// unquoteYAMLScalar trims a scalar and strips one layer of matching quotes.
func unquoteYAMLScalar(s string) string {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return s
	}
	switch {
	case s[0] == '"' && s[len(s)-1] == '"':
		if unquoted, err := strconv.Unquote(s); err == nil {
			return unquoted
		}
		return s[1 : len(s)-1]
	case s[0] == '\'' && s[len(s)-1] == '\'':
		return strings.ReplaceAll(s[1:len(s)-1], "''", "'")
	}
	return s
}
