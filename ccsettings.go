package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Claude Code's own settings file and the retention key this tool exposes.
const (
	claudeSettingsFile = "settings.json"
	cleanupPeriodKey   = "cleanupPeriodDays"

	// defaultCleanupPeriod is what Claude Code applies when the key is absent.
	defaultCleanupPeriod = 30

	// Claude Code rejects 0 with a validation error, so 1 is the floor.
	minCleanupPeriod = 1
	maxCleanupPeriod = 3650
)

// cleanupPeriodPresets are the values the settings row cycles through. A value
// set elsewhere is kept as is and snaps to the next larger preset.
var cleanupPeriodPresets = []int{7, 14, 30, 60, 90, 180, 365}

// claudeSettingsPath is the user-level settings file inside the Claude
// directory the tool is pointed at.
func claudeSettingsPath() string {
	return filepath.Join(claudeDir, claudeSettingsFile)
}

// readCleanupPeriod returns the retention period from Claude Code's settings.
// set reports whether the key is present: when it is not, Claude Code applies
// defaultCleanupPeriod and the caller can say so. A missing file is not an
// error - it just means nothing is configured yet.
func readCleanupPeriod() (days int, set bool, err error) {
	data, err := os.ReadFile(claudeSettingsPath())
	if os.IsNotExist(err) {
		return defaultCleanupPeriod, false, nil
	}
	if err != nil {
		return defaultCleanupPeriod, false, err
	}
	return parseCleanupPeriod(data)
}

// parseCleanupPeriod reads the retention value out of settings content already
// in hand, so a caller that must both inspect and rewrite the file can do it
// from a single snapshot instead of reading twice.
func parseCleanupPeriod(data []byte) (days int, set bool, err error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return defaultCleanupPeriod, false, nil
	}

	var settings map[string]json.RawMessage
	if err := json.Unmarshal(data, &settings); err != nil {
		return defaultCleanupPeriod, false, fmt.Errorf("%s is not valid JSON: %w", claudeSettingsFile, err)
	}

	raw, ok := settings[cleanupPeriodKey]
	if !ok {
		return defaultCleanupPeriod, false, nil
	}
	if err := json.Unmarshal(raw, &days); err != nil {
		return defaultCleanupPeriod, false, fmt.Errorf("%s holds %s, not a number: %w", cleanupPeriodKey, raw, err)
	}
	return days, true, nil
}

// nextCleanupPeriod returns the preset following current, wrapping around. A
// value that is not a preset advances to the next larger one.
func nextCleanupPeriod(current int) int {
	for _, p := range cleanupPeriodPresets {
		if p > current {
			return p
		}
	}
	return cleanupPeriodPresets[0]
}

// prevCleanupPeriod returns the preset before current, wrapping around. A value
// that is not a preset falls back to the next smaller one.
func prevCleanupPeriod(current int) int {
	for i := len(cleanupPeriodPresets) - 1; i >= 0; i-- {
		if cleanupPeriodPresets[i] < current {
			return cleanupPeriodPresets[i]
		}
	}
	return cleanupPeriodPresets[len(cleanupPeriodPresets)-1]
}

// cleanupState is a retention value as it was observed: set reports whether the
// key was present at all.
type cleanupState struct {
	days int
	set  bool
}

// cleanupConflictError says the file no longer holds the value the caller based
// its edit on, so someone else changed it in the meantime. The caller decides
// whether to overwrite.
type cleanupConflictError struct {
	current cleanupState
}

func (e *cleanupConflictError) Error() string {
	if !e.current.set {
		return fmt.Sprintf("%s was removed from %s in the meantime", cleanupPeriodKey, claudeSettingsFile)
	}
	return fmt.Sprintf("%s changed to %d in %s in the meantime", cleanupPeriodKey, e.current.days, claudeSettingsFile)
}

// writeCleanupPeriod stores days in Claude Code's settings file, editing only
// that key: the surrounding bytes, key order and formatting are preserved, so
// the file stays as its owner wrote it. Unparsable content is left untouched.
//
// When expect is non-nil the file must still hold that value, otherwise a
// *cleanupConflictError is returned and nothing is written; pass nil to
// overwrite regardless. The check and the edit share one read of the file, so a
// write landing between them cannot slip past the check and still be discarded.
func writeCleanupPeriod(days int, expect *cleanupState) error {
	if days < minCleanupPeriod || days > maxCleanupPeriod {
		return fmt.Errorf("cleanup period must be between %d and %d days", minCleanupPeriod, maxCleanupPeriod)
	}

	path := claudeSettingsPath()
	original, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		original = nil
	} else if err != nil {
		return err
	}

	if expect != nil {
		current, set, err := parseCleanupPeriod(original)
		if err != nil {
			return err
		}
		if current != expect.days || set != expect.set {
			return &cleanupConflictError{current: cleanupState{days: current, set: set}}
		}
	}

	updated, err := setJSONNumber(original, cleanupPeriodKey, days)
	if err != nil {
		return err
	}
	if err := verifyOnlyKeyChanged(original, updated, cleanupPeriodKey, days); err != nil {
		return err
	}
	return writeFileAtomic(path, updated)
}

// setJSONNumber returns content with key set to value at the top level. An
// existing key has only its value bytes replaced; a new key is inserted first,
// matching the indentation already in the file. Empty content becomes a new
// object.
func setJSONNumber(content []byte, key string, value int) ([]byte, error) {
	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return []byte(fmt.Sprintf("{\n  %q: %d\n}\n", key, value)), nil
	}

	start, end, found, err := topLevelValueRange(content, key)
	if err != nil {
		return nil, err
	}

	if found {
		var out []byte
		out = append(out, content[:start]...)
		out = append(out, []byte(fmt.Sprintf("%d", value))...)
		out = append(out, content[end:]...)
		return out, nil
	}
	return insertFirstKey(content, key, value)
}

// insertFirstKey adds key right after the opening brace, so no existing entry
// has to be located or rewritten.
func insertFirstKey(content []byte, key string, value int) ([]byte, error) {
	open := strings.IndexByte(string(content), '{')
	if open < 0 {
		return nil, fmt.Errorf("%s does not contain a JSON object", claudeSettingsFile)
	}

	rest := strings.TrimLeft(string(content[open+1:]), " \t")
	empty := strings.HasPrefix(strings.TrimLeft(rest, "\r\n"), "}")

	entry := fmt.Sprintf("%q: %d", key, value)
	var insert string
	switch {
	case strings.HasPrefix(rest, "\n") || strings.HasPrefix(rest, "\r\n"):
		// Multi-line object: reuse the indentation and line ending already used.
		newline := "\n"
		if strings.HasPrefix(rest, "\r\n") {
			newline = "\r\n"
		}
		indent := leadingIndent(rest)
		if empty {
			insert = newline + indent + entry
		} else {
			insert = newline + indent + entry + ","
		}
	case empty:
		insert = entry
	default:
		insert = entry + ", "
	}

	var out []byte
	out = append(out, content[:open+1]...)
	out = append(out, []byte(insert)...)
	out = append(out, content[open+1:]...)
	return out, nil
}

// leadingIndent returns the whitespace starting the first non-empty line of s,
// falling back to two spaces, which is what Claude Code writes.
func leadingIndent(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		// The closing brace of an otherwise empty object sits at the object's
		// own level and says nothing about how its entries are indented.
		if strings.TrimSpace(line) == "}" {
			break
		}
		return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	}
	return "  "
}

// topLevelValueRange locates the byte range of key's value in a JSON object,
// looking only at the top level so a nested occurrence of the same name is
// ignored.
func topLevelValueRange(content []byte, key string) (start, end int, found bool, err error) {
	dec := json.NewDecoder(bytes.NewReader(content))
	tok, err := dec.Token()
	if err != nil {
		return 0, 0, false, fmt.Errorf("%s is not valid JSON: %w", claudeSettingsFile, err)
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return 0, 0, false, fmt.Errorf("%s does not contain a JSON object", claudeSettingsFile)
	}

	for dec.More() {
		nameTok, err := dec.Token()
		if err != nil {
			return 0, 0, false, err
		}
		name, _ := nameTok.(string)

		// InputOffset after reading the name is where the value begins,
		// modulo the whitespace and colon the decoder has consumed.
		before := dec.InputOffset()
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return 0, 0, false, err
		}
		after := dec.InputOffset()

		if name == key {
			valueStart := before + int64(indexOfValue(content[before:after]))
			return int(valueStart), int(after), true, nil
		}
	}
	return 0, 0, false, nil
}

// indexOfValue skips the colon and whitespace that separate a key from its
// value inside the span the decoder consumed.
func indexOfValue(span []byte) int {
	i := 0
	for i < len(span) && (span[i] == ' ' || span[i] == '\t' || span[i] == '\n' || span[i] == '\r' || span[i] == ':') {
		i++
	}
	return i
}

// verifyOnlyKeyChanged rejects an edit that did anything beyond setting key to
// value: the result must be valid JSON, carry the new value, and keep exactly
// the original top-level keys plus this one. A byte-offset edit that went wrong
// is caught here rather than on disk.
func verifyOnlyKeyChanged(original, updated []byte, key string, value int) error {
	if !json.Valid(updated) {
		return fmt.Errorf("edit would have produced invalid JSON, %s left unchanged", claudeSettingsFile)
	}

	var after map[string]json.RawMessage
	if err := json.Unmarshal(updated, &after); err != nil {
		return err
	}

	var got int
	raw, ok := after[key]
	if !ok {
		return fmt.Errorf("edit did not set %s", key)
	}
	if err := json.Unmarshal(raw, &got); err != nil || got != value {
		return fmt.Errorf("edit did not store %d in %s", value, key)
	}

	before := map[string]json.RawMessage{}
	if len(strings.TrimSpace(string(original))) > 0 {
		if err := json.Unmarshal(original, &before); err != nil {
			return err
		}
	}
	for name, raw := range before {
		newRaw, ok := after[name]
		if !ok {
			return fmt.Errorf("edit would have dropped %q, %s left unchanged", name, claudeSettingsFile)
		}
		if name != key && string(newRaw) != string(raw) {
			return fmt.Errorf("edit would have changed %q, %s left unchanged", name, claudeSettingsFile)
		}
	}
	for name := range after {
		if _, ok := before[name]; !ok && name != key {
			return fmt.Errorf("edit would have added %q, %s left unchanged", name, claudeSettingsFile)
		}
	}
	return nil
}

// writeFileAtomic writes data through a temp file in the same directory, so a
// crash cannot leave the settings half-written.
//
// TODO: the rename replaces a symlinked settings.json with a regular file
// instead of writing through the link. Resolve the link first if users turn up
// keeping this file in a dotfiles setup.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".settings-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	// Keep the file's own permissions: forcing 0644 would loosen a settings
	// file the user deliberately restricted. A new file gets 0644.
	mode := os.FileMode(0644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}
