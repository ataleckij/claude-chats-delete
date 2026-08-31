package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// useTempClaudeDir points claudeDir at a scratch directory and optionally seeds
// a settings file, returning its path.
func useTempClaudeDir(t *testing.T, settings string) string {
	t.Helper()
	orig := claudeDir
	dir := t.TempDir()
	claudeDir = dir
	t.Cleanup(func() { claudeDir = orig })

	path := filepath.Join(dir, claudeSettingsFile)
	if settings != "" {
		if err := os.WriteFile(path, []byte(settings), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestReadCleanupPeriod(t *testing.T) {
	tests := []struct {
		name     string
		settings string
		want     int
		wantSet  bool
		wantErr  bool
	}{
		{name: "missing file falls back to the default", settings: "", want: defaultCleanupPeriod},
		{name: "key absent falls back to the default", settings: `{"model":"opus"}`, want: defaultCleanupPeriod},
		{name: "key present", settings: `{"cleanupPeriodDays":90}`, want: 90, wantSet: true},
		{name: "empty file is treated as unset", settings: "   \n", want: defaultCleanupPeriod},
		{name: "broken JSON reports an error", settings: `{not json`, want: defaultCleanupPeriod, wantErr: true},
		{name: "non-numeric value reports an error", settings: `{"cleanupPeriodDays":"90"}`, want: defaultCleanupPeriod, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useTempClaudeDir(t, tt.settings)

			days, set, err := readCleanupPeriod()
			if tt.wantErr && err == nil {
				t.Fatalf("expected an error, got days=%d set=%v", days, set)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if days != tt.want {
				t.Errorf("days = %d, want %d", days, tt.want)
			}
			if set != tt.wantSet {
				t.Errorf("set = %v, want %v", set, tt.wantSet)
			}
		})
	}
}

func TestNextCleanupPeriod(t *testing.T) {
	tests := []struct {
		current, want int
	}{
		{7, 14},
		{30, 60},
		{365, 7},  // wraps around
		{45, 60},  // custom value advances to the next larger preset
		{1000, 7}, // above every preset wraps around
		{1, 7},    // below every preset picks the smallest
	}

	for _, tt := range tests {
		if got := nextCleanupPeriod(tt.current); got != tt.want {
			t.Errorf("nextCleanupPeriod(%d) = %d, want %d", tt.current, got, tt.want)
		}
	}
}

// The whole point of the surgical edit: everything except the one value stays
// byte for byte as its owner wrote it.
func TestWriteCleanupPeriodPreservesTheFile(t *testing.T) {
	const settings = `{
  "model": "opus[1m]",
  "hooks": {
    "Notification": [
      {"matcher": "permission_prompt", "hooks": [{"type": "command"}]}
    ]
  },
  "cleanupPeriodDays": 30,
  "voiceEnabled": true
}
`
	path := useTempClaudeDir(t, settings)

	if err := writeCleanupPeriod(90, nil); err != nil {
		t.Fatalf("writeCleanupPeriod: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(settings, `"cleanupPeriodDays": 30`, `"cleanupPeriodDays": 90`, 1)
	if string(got) != want {
		t.Errorf("file changed beyond the value:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestWriteCleanupPeriodInserts(t *testing.T) {
	tests := []struct {
		name     string
		settings string
		want     string
	}{
		{
			name:     "multi-line object gets the key first, with matching indentation",
			settings: "{\n  \"model\": \"opus\"\n}\n",
			want:     "{\n  \"cleanupPeriodDays\": 45,\n  \"model\": \"opus\"\n}\n",
		},
		{
			name:     "empty object takes no trailing comma",
			settings: "{}\n",
			want:     "{\"cleanupPeriodDays\": 45}\n",
		},
		{
			name:     "single-line object stays on one line",
			settings: `{"model": "opus"}`,
			want:     `{"cleanupPeriodDays": 45, "model": "opus"}`,
		},
		{
			name:     "four-space indentation is reused",
			settings: "{\n    \"model\": \"opus\"\n}\n",
			want:     "{\n    \"cleanupPeriodDays\": 45,\n    \"model\": \"opus\"\n}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := useTempClaudeDir(t, tt.settings)

			if err := writeCleanupPeriod(45, nil); err != nil {
				t.Fatalf("writeCleanupPeriod: %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("got:\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

func TestWriteCleanupPeriodCreatesFile(t *testing.T) {
	path := useTempClaudeDir(t, "")

	if err := writeCleanupPeriod(60, nil); err != nil {
		t.Fatalf("writeCleanupPeriod: %v", err)
	}

	days, set, err := readCleanupPeriod()
	if err != nil {
		t.Fatalf("readCleanupPeriod: %v", err)
	}
	if !set || days != 60 {
		t.Errorf("read back days=%d set=%v, want 60/true", days, set)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("settings file was not created: %v", err)
	}
}

// A nested key of the same name must not be mistaken for the top-level one.
func TestWriteCleanupPeriodIgnoresNestedKey(t *testing.T) {
	const settings = `{
  "somePlugin": {
    "cleanupPeriodDays": 5
  },
  "model": "opus"
}
`
	path := useTempClaudeDir(t, settings)

	if err := writeCleanupPeriod(90, nil); err != nil {
		t.Fatalf("writeCleanupPeriod: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"cleanupPeriodDays": 5`) {
		t.Errorf("the nested value was modified:\n%s", got)
	}
	days, _, err := readCleanupPeriod()
	if err != nil {
		t.Fatal(err)
	}
	if days != 90 {
		t.Errorf("top-level value = %d, want 90", days)
	}
}

func TestWriteCleanupPeriodRefusesBadInput(t *testing.T) {
	t.Run("unparsable settings are left untouched", func(t *testing.T) {
		const broken = `{"model": "opus"`
		path := useTempClaudeDir(t, broken)

		if err := writeCleanupPeriod(90, nil); err == nil {
			t.Fatal("expected an error for unparsable settings")
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != broken {
			t.Errorf("file was modified:\n%s", got)
		}
	})

	t.Run("out of range values are rejected", func(t *testing.T) {
		useTempClaudeDir(t, `{"model":"opus"}`)

		for _, days := range []int{0, -1, maxCleanupPeriod + 1} {
			if err := writeCleanupPeriod(days, nil); err == nil {
				t.Errorf("writeCleanupPeriod(%d) should have failed", days)
			}
		}
	})
}

// The guard must catch an edit that damaged a neighbouring key.
func TestVerifyOnlyKeyChanged(t *testing.T) {
	original := []byte(`{"a": 1, "cleanupPeriodDays": 30}`)

	tests := []struct {
		name    string
		updated string
		wantErr bool
	}{
		{name: "only the value changed", updated: `{"a": 1, "cleanupPeriodDays": 90}`},
		{name: "invalid JSON", updated: `{"a": 1, "cleanupPeriodDays": 90`, wantErr: true},
		{name: "a neighbour changed", updated: `{"a": 2, "cleanupPeriodDays": 90}`, wantErr: true},
		{name: "a neighbour was dropped", updated: `{"cleanupPeriodDays": 90}`, wantErr: true},
		{name: "an extra key appeared", updated: `{"a": 1, "b": 2, "cleanupPeriodDays": 90}`, wantErr: true},
		{name: "the value was not applied", updated: `{"a": 1, "cleanupPeriodDays": 30}`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyOnlyKeyChanged(original, []byte(tt.updated), cleanupPeriodKey, 90)
			if tt.wantErr && err == nil {
				t.Error("expected an error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestPrevCleanupPeriod(t *testing.T) {
	tests := []struct{ current, want int }{
		{14, 7},
		{60, 30},
		{7, 365},    // wraps around
		{45, 30},    // custom value falls back to the next smaller preset
		{1, 365},    // below every preset wraps around
		{1000, 365}, // above every preset picks the largest
	}
	for _, tt := range tests {
		if got := prevCleanupPeriod(tt.current); got != tt.want {
			t.Errorf("prevCleanupPeriod(%d) = %d, want %d", tt.current, got, tt.want)
		}
	}
}

// The value may sit last in a compact object, with no whitespace between the
// colon, the number and the closing brace - the tightest case for the byte
// offsets the edit relies on.
func TestWriteCleanupPeriodCompactTrailingKey(t *testing.T) {
	const settings = `{"a":1,"cleanupPeriodDays":30}`
	path := useTempClaudeDir(t, settings)

	if err := writeCleanupPeriod(90, nil); err != nil {
		t.Fatalf("writeCleanupPeriod: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"a":1,"cleanupPeriodDays":90}` {
		t.Errorf("got %q", got)
	}
}

func TestWriteCleanupPeriodDetectsConflict(t *testing.T) {
	useTempClaudeDir(t, `{"cleanupPeriodDays": 30}`)

	// Someone else moved it to 90 while we still believed it was 30.
	if err := writeCleanupPeriod(90, nil); err != nil {
		t.Fatal(err)
	}

	err := writeCleanupPeriod(180, &cleanupState{days: 30, set: true})
	conflict, ok := err.(*cleanupConflictError)
	if !ok {
		t.Fatalf("expected a conflict, got %v", err)
	}
	if conflict.current.days != 90 || !conflict.current.set {
		t.Errorf("conflict reports days=%d set=%v, want 90/true", conflict.current.days, conflict.current.set)
	}

	// The refused write must not have touched the file.
	days, _, err := readCleanupPeriod()
	if err != nil {
		t.Fatal(err)
	}
	if days != 90 {
		t.Errorf("value = %d, want the file left at 90", days)
	}

	// Passing no expectation overwrites deliberately.
	if err := writeCleanupPeriod(180, nil); err != nil {
		t.Fatalf("forced write: %v", err)
	}
	if days, _, _ := readCleanupPeriod(); days != 180 {
		t.Errorf("value = %d, want 180 after the forced write", days)
	}
}

func TestWriteCleanupPeriodConflictOnRemovedKey(t *testing.T) {
	useTempClaudeDir(t, `{"model": "opus"}`)

	// We believed the key held 30; in fact it is absent.
	err := writeCleanupPeriod(60, &cleanupState{days: 30, set: true})
	if _, ok := err.(*cleanupConflictError); !ok {
		t.Fatalf("expected a conflict, got %v", err)
	}
}

func TestWriteCleanupPeriodKeepsFileMode(t *testing.T) {
	path := useTempClaudeDir(t, `{"cleanupPeriodDays": 30}`)
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}

	if err := writeCleanupPeriod(90, nil); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("mode = %o, want the original 0600 preserved", got)
	}
}

func TestWriteCleanupPeriodKeepsCRLF(t *testing.T) {
	settings := "{\r\n  \"model\": \"opus\"\r\n}\r\n"
	path := useTempClaudeDir(t, settings)

	if err := writeCleanupPeriod(45, nil); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\r\n  \"cleanupPeriodDays\": 45,\r\n  \"model\": \"opus\"\r\n}\r\n"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// An object with nothing but a closing brace gives no indentation to copy, so
// the conventional two spaces are used rather than the brace's own column.
func TestWriteCleanupPeriodEmptyMultilineObject(t *testing.T) {
	path := useTempClaudeDir(t, "{\n}\n")

	if err := writeCleanupPeriod(45, nil); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"cleanupPeriodDays\": 45\n}\n"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// The conflict check and the patch must come from the same bytes: a value read
// for the check but patched from a later read would drop a concurrent write
// without reporting a conflict.
func TestParseCleanupPeriodSharesTheSnapshot(t *testing.T) {
	data := []byte(`{"cleanupPeriodDays": 30, "model": "opus"}`)

	days, set, err := parseCleanupPeriod(data)
	if err != nil {
		t.Fatal(err)
	}
	if !set || days != 30 {
		t.Errorf("parsed days=%d set=%v, want 30/true", days, set)
	}

	if _, set, err := parseCleanupPeriod([]byte(`{"model":"opus"}`)); err != nil || set {
		t.Errorf("absent key: set=%v err=%v, want false/nil", set, err)
	}
	if _, _, err := parseCleanupPeriod([]byte("  \n")); err != nil {
		t.Errorf("empty content should not be an error: %v", err)
	}
	if _, _, err := parseCleanupPeriod([]byte("{oops")); err == nil {
		t.Error("malformed content should report an error")
	}
}
