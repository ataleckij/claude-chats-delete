package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/mattn/go-runewidth"
)

// truncateLeft truncates s from the left to fit within maxWidth visual columns,
// prepending ".." if truncation occurs. Shows the end of the string (most specific part).
func truncateLeft(s string, maxWidth int) string {
	if runewidth.StringWidth(s) <= maxWidth {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 {
		candidate := ".." + string(runes)
		if runewidth.StringWidth(candidate) <= maxWidth {
			return candidate
		}
		runes = runes[1:]
	}
	return ".."
}

// justifyItems distributes items evenly across totalWidth.
// prefix is printed as-is before the items (e.g. "Actions:    ").
// Items are separated by " | " stretched to fill the remaining width.
func justifyItems(prefix string, items []string, totalWidth int) string {
	prefixWidth := runewidth.StringWidth(prefix)
	available := totalWidth - prefixWidth

	if len(items) <= 1 {
		return prefix + strings.Join(items, "")
	}

	// Calculate total content width with minimum separators " | "
	contentWidth := 0
	for _, item := range items {
		contentWidth += runewidth.StringWidth(item)
	}
	minSepWidth := 3 // " | "
	gaps := len(items) - 1
	extra := available - contentWidth - minSepWidth*gaps
	if extra < 0 {
		extra = 0
	}

	// Distribute extra spaces: each gap gets base + 1 for the first `rem` gaps
	base := extra / gaps
	rem := extra % gaps

	var sb strings.Builder
	sb.WriteString(prefix)
	for i, item := range items {
		sb.WriteString(item)
		if i < gaps {
			extra := base
			if i < rem {
				extra = base + 1
			}
			left := extra / 2
			right := extra - left
			sb.WriteString(" " + strings.Repeat(" ", left) + "|" + strings.Repeat(" ", right) + " ")
		}
	}
	return sb.String()
}

func copyToClipboard(text string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		// Try xclip first, then xsel
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.Command("wl-copy")
		} else {
			return fmt.Errorf("no clipboard utility found (install xclip, xsel, or wl-copy)")
		}
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}
