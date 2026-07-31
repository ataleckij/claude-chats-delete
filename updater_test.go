package main

import (
	"strings"
	"testing"
)

// Valid 64-char digests, distinguishable by their first characters.
const (
	digestLinux     = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	digestDarwinUp  = "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
	digestDarwinOld = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
)

func TestParseChecksum(t *testing.T) {
	content := digestLinux + `  claude-chats-linux-amd64
` + digestDarwinUp + ` *claude-chats-darwin-arm64
` + digestDarwinOld + `  claude-chats-darwin-arm64-old
`

	tests := []struct {
		name    string
		binary  string
		want    string
		wantErr bool
	}{
		{
			name:   "plain entry",
			binary: "claude-chats-linux-amd64",
			want:   digestLinux,
		},
		{
			name:   "binary-mode marker is stripped and digest lowercased",
			binary: "claude-chats-darwin-arm64",
			want:   strings.ToLower(digestDarwinUp),
		},
		{
			name:   "name is matched exactly, not by prefix",
			binary: "claude-chats-darwin-arm64-old",
			want:   digestDarwinOld,
		},
		{
			name:    "unknown asset",
			binary:  "claude-chats-windows-amd64",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseChecksum(content, tt.binary)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseChecksum(%q) = %q, want error", tt.binary, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseChecksum(%q) returned error: %v", tt.binary, err)
			}
			if got != tt.want {
				t.Errorf("parseChecksum(%q) = %q, want %q", tt.binary, got, tt.want)
			}
		})
	}
}

func TestParseChecksumEmptyFile(t *testing.T) {
	if _, err := parseChecksum("", "claude-chats-linux-amd64"); err == nil {
		t.Error("parseChecksum on empty content should fail, not return an empty digest")
	}
}

// A digest that is not 64 hex characters must be rejected outright, so a
// corrupted checksums file reports itself instead of failing later as a
// mismatch against a bogus expected value.
func TestParseChecksumRejectsMalformedDigest(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"too short", "abc123  claude-chats-linux-amd64"},
		{"not hex", strings.Repeat("z", 64) + "  claude-chats-linux-amd64"},
		{"too long", strings.Repeat("a", 65) + "  claude-chats-linux-amd64"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseChecksum(tt.content, "claude-chats-linux-amd64")
			if err == nil {
				t.Errorf("parseChecksum accepted a malformed digest, returned %q", got)
			}
		})
	}
}
