package main

import (
	"errors"
	"fmt"
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

// memoryIndexWriteAttempts bounds how often the index is reread when it keeps
// changing between the read and the write.
const memoryIndexWriteAttempts = 3

// deleteMemoryFiles removes the given notes and drops the lines that link to
// them from their project's index. A note already gone from disk counts as
// deleted. It returns how many notes are gone and every failure, joined; the
// index of a project is updated for the notes that were removed even when
// others in it could not be.
func deleteMemoryFiles(files []MemoryFile) (int, error) {
	var dirs []string
	removed := make(map[string]map[string]bool)
	var errs []error
	count := 0

	for _, f := range files {
		if err := os.Remove(f.Path); err != nil && !os.IsNotExist(err) {
			errs = append(errs, err)
			continue
		}
		count++
		dir := filepath.Dir(f.Path)
		if removed[dir] == nil {
			removed[dir] = make(map[string]bool)
			dirs = append(dirs, dir)
		}
		removed[dir][f.Name] = true
	}

	for _, dir := range dirs {
		if err := pruneMemoryIndex(dir, removed[dir]); err != nil {
			errs = append(errs, fmt.Errorf("memory index not updated in %s: %w", dir, err))
		}
	}
	return count, errors.Join(errs...)
}

// pruneMemoryIndex drops the lines of dir's index that link to any of names.
// A missing index is left missing.
//
// No lock is taken. Claude Code writes the index through its ordinary file
// tools, which take none and refuse an edit when the file changed since they
// read it. Its .consolidate-lock only keeps two autoDream runs apart and
// outlives each run as a timestamp, so it is neither read nor honoured here:
// honouring it would block deletes that Claude Code itself allows. Instead the
// index is reread when it changes under us, then replaced atomically.
func pruneMemoryIndex(dir string, names map[string]bool) error {
	if len(names) == 0 {
		return nil
	}
	path := resolveSymlink(filepath.Join(dir, memoryIndexFile))

	for attempt := 0; attempt < memoryIndexWriteAttempts; attempt++ {
		before, err := os.Stat(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		kept, dropped := filterMemoryIndex(data, names)
		if dropped == 0 {
			return nil
		}

		after, err := os.Stat(path)
		if err != nil {
			return err
		}
		if after.Size() != int64(len(data)) || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
			continue
		}
		return writeFileAtomic(path, kept)
	}
	return fmt.Errorf("%s kept changing, left unchanged", memoryIndexFile)
}

// filterMemoryIndex removes every line holding a markdown link to one of
// names, matched by file name rather than by title. Other lines, line endings
// included, are kept byte for byte.
func filterMemoryIndex(data []byte, names map[string]bool) (kept []byte, dropped int) {
	kept = make([]byte, 0, len(data))
	for _, line := range strings.SplitAfter(string(data), "\n") {
		if linksToAny(line, names) {
			dropped++
			continue
		}
		kept = append(kept, line...)
	}
	return kept, dropped
}

// linksToAny reports whether line has a link "[...](target)" whose target is
// one of names, written bare, as "./name", in angle brackets, or followed by a
// link title.
func linksToAny(line string, names map[string]bool) bool {
	rest := line
	for {
		i := strings.Index(rest, "](")
		if i < 0 {
			return false
		}
		rest = rest[i+2:]
		end := strings.IndexByte(rest, ')')
		if end < 0 {
			return false
		}
		target := strings.TrimSpace(rest[:end])
		if strings.HasPrefix(target, "<") {
			if close := strings.IndexByte(target, '>'); close > 0 {
				target = target[1:close]
			}
		} else if sp := strings.IndexAny(target, " \t"); sp >= 0 {
			target = target[:sp]
		}
		target = strings.TrimPrefix(target, "./")
		if names[target] {
			return true
		}
		rest = rest[end+1:]
	}
}
