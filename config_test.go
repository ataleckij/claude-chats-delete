package main

import (
	"os"
	"testing"
)

func TestResolveClaudeDir(t *testing.T) {
	const (
		saved  = "/home/me/.claude"
		system = "/custom/claude"
	)

	tests := []struct {
		name        string
		saved       string
		system      string
		interactive bool
		wantAction  dirAction
		wantDir     string
	}{
		{
			name:        "first run in a terminal asks, defaulting to the system path",
			saved:       "",
			system:      system,
			interactive: true,
			wantAction:  dirAskFirstRun,
			wantDir:     system,
		},
		{
			name:        "first run without a terminal follows the environment",
			saved:       "",
			system:      system,
			interactive: false,
			wantAction:  dirUseSystem,
			wantDir:     system,
		},
		{
			name:        "saved value already matches the environment",
			saved:       saved,
			system:      saved,
			interactive: true,
			wantAction:  dirUseSaved,
			wantDir:     saved,
		},
		{
			name:        "mismatch in a terminal offers the switch",
			saved:       saved,
			system:      system,
			interactive: true,
			wantAction:  dirAskSwitch,
			wantDir:     system,
		},
		{
			name:        "mismatch without a terminal follows the environment for this run",
			saved:       saved,
			system:      system,
			interactive: false,
			wantAction:  dirUseSystem,
			wantDir:     system,
		},
		{
			// CLAUDE_CONFIG_DIR was adopted and saved, then unset: the system
			// path falls back to the default and we offer to move back to it.
			name:        "env removed after being saved offers a move back to the default",
			saved:       system,
			system:      saved,
			interactive: true,
			wantAction:  dirAskSwitch,
			wantDir:     saved,
		},
		{
			name:        "env removed after being saved, no terminal: default for this run",
			saved:       system,
			system:      saved,
			interactive: false,
			wantAction:  dirUseSystem,
			wantDir:     saved,
		},
		{
			// Same directory spelled differently must not nag on every start.
			name:        "trailing slash is not a mismatch",
			saved:       saved,
			system:      saved + "/",
			interactive: true,
			wantAction:  dirUseSaved,
			wantDir:     saved,
		},
		{
			name:        "redundant separators are not a mismatch",
			saved:       saved,
			system:      "/home//me/./.claude",
			interactive: true,
			wantAction:  dirUseSaved,
			wantDir:     saved,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveClaudeDir(tt.saved, tt.system, tt.interactive)
			if got.action != tt.wantAction || got.dir != tt.wantDir {
				t.Errorf("resolveClaudeDir(%q, %q, %v) = {%v %q}, want {%v %q}",
					tt.saved, tt.system, tt.interactive, got.action, got.dir, tt.wantAction, tt.wantDir)
			}
		})
	}
}

func TestSystemDirLabel(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     string
	}{
		{"variable set", "/custom/claude", claudeConfigDirEnv},
		{"variable unset", "", "the default location"},
		{"variable blank", "   ", "the default location"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := systemDirLabel(tt.envValue); got != tt.want {
				t.Errorf("systemDirLabel(%q) = %q, want %q", tt.envValue, got, tt.want)
			}
		})
	}
}

func TestExpandHome(t *testing.T) {
	home := os.Getenv("HOME")

	tests := []struct {
		in   string
		want string
	}{
		{"~/.claude", home + "/.claude"},
		{"~", home},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"", ""},
		{"/has/~/inside", "/has/~/inside"}, // only a leading ~ expands
		{"/trailing/slash/", "/trailing/slash"},
		{"/double//separator", "/double/separator"},
		{"/dot/./segment", "/dot/segment"},
	}

	for _, tt := range tests {
		if got := expandHome(tt.in); got != tt.want {
			t.Errorf("expandHome(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSystemClaudeDir(t *testing.T) {
	orig, had := os.LookupEnv("CLAUDE_CONFIG_DIR")
	defer func() {
		if had {
			os.Setenv("CLAUDE_CONFIG_DIR", orig)
		} else {
			os.Unsetenv("CLAUDE_CONFIG_DIR")
		}
	}()

	os.Setenv("CLAUDE_CONFIG_DIR", "/custom/claude")
	if got := systemClaudeDir(); got != "/custom/claude" {
		t.Errorf("systemClaudeDir() = %q, want the CLAUDE_CONFIG_DIR value", got)
	}

	// A blank value counts as unset.
	os.Setenv("CLAUDE_CONFIG_DIR", "   ")
	if got := systemClaudeDir(); got != defaultClaudeDir() {
		t.Errorf("systemClaudeDir() = %q, want the default for a blank env value", got)
	}

	os.Unsetenv("CLAUDE_CONFIG_DIR")
	if got := systemClaudeDir(); got != defaultClaudeDir() {
		t.Errorf("systemClaudeDir() = %q, want the default when unset", got)
	}
}
