package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const (
	CurrentVersion = "0.2.2"
	GitHubAPIURL   = "https://api.github.com/repos/ataleckij/claude-chats-delete/releases/latest"
)

// GitHubRelease represents the GitHub API response for a release
type GitHubRelease struct {
	TagName string `json:"tag_name"` // e.g. "v0.1.6"
	HTMLURL string `json:"html_url"`
}

// shouldCheckUpdate returns true if enough time has passed since last check
func shouldCheckUpdate(lastCheck int64, intervalHours int) bool {
	if lastCheck == 0 {
		return true // First run
	}

	hoursSinceCheck := time.Since(time.Unix(lastCheck, 0)).Hours()
	return hoursSinceCheck >= float64(intervalHours)
}

// checkForUpdate queries GitHub API for the latest release
// Returns the new version string (without 'v' prefix) if update is available, empty string otherwise
func checkForUpdate() string {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(GitHubAPIURL)
	if err != nil {
		return "" // Silently fail on network errors
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return ""
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	if isNewerVersion(latestVersion, CurrentVersion) {
		return latestVersion
	}

	return ""
}

// isNewerVersion compares two semantic version strings
// Returns true if latest > current
func isNewerVersion(latest, current string) bool {
	latestParts := strings.Split(latest, ".")
	currentParts := strings.Split(current, ".")

	// Compare each version component
	for i := 0; i < 3; i++ {
		var latestNum, currentNum int
		if i < len(latestParts) {
			fmt.Sscanf(latestParts[i], "%d", &latestNum)
		}
		if i < len(currentParts) {
			fmt.Sscanf(currentParts[i], "%d", &currentNum)
		}

		if latestNum > currentNum {
			return true
		}
		if latestNum < currentNum {
			return false
		}
	}

	return false
}

// promptAndUpdate asks user if they want to update and performs the update if yes
// Returns true if user declined the update (or update failed), false if update succeeded
func promptAndUpdate(newVersion string) bool {
	fmt.Printf("\n")
	fmt.Printf("Update available: v%s → v%s\n", CurrentVersion, newVersion)
	fmt.Print("Download and install? [y/N]: ")

	var response string
	fmt.Scanln(&response)

	if strings.ToLower(strings.TrimSpace(response)) == "y" {
		fmt.Printf("\nDownloading v%s...\n", newVersion)
		if err := downloadAndInstall(newVersion); err != nil {
			fmt.Printf("Update failed: %v\n", err)
			fmt.Println("Please update manually:")
			fmt.Println("  curl -sSL https://raw.githubusercontent.com/ataleckij/claude-chats-delete/main/install.sh | sh\n")
			time.Sleep(2 * time.Second)
			return true // Update failed, remember check time
		} else {
			fmt.Println("\n✓ Update successful! Restarting...\n")

			// Get current executable path
			exePath, err := os.Executable()
			if err != nil {
				fmt.Println("Failed to get executable path, please restart manually")
				os.Exit(0)
			}

			// Replace current process with new version (automatic restart)
			// This preserves PID and doesn't require manual restart
			if err := syscall.Exec(exePath, os.Args, os.Environ()); err != nil {
				fmt.Printf("Failed to restart automatically: %v\n", err)
				fmt.Println("Please restart claude-chats manually to use the new version.\n")
				os.Exit(0)
			}
		}
	}

	fmt.Println()
	return true // User declined
}

// downloadAndInstall downloads the binary and replaces the current executable
func downloadAndInstall(version string) error {
	// Determine platform-specific binary name
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	binaryName := fmt.Sprintf("claude-chats-%s-%s", goos, goarch)

	// Download URL
	url := fmt.Sprintf("https://github.com/ataleckij/claude-chats-delete/releases/download/v%s/%s", version, binaryName)

	// Download to temporary file
	tmpFile, err := os.CreateTemp("", "claude-chats-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath) // Clean up on error

	// Download binary
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	// Write to temp file
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write binary: %w", err)
	}
	tmpFile.Close()

	// Make executable
	if err := os.Chmod(tmpPath, 0755); err != nil {
		return fmt.Errorf("failed to chmod: %w", err)
	}

	// Get current executable path
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Backup current binary (optional safety measure)
	backupPath := exePath + ".backup"
	if err := copyFile(exePath, backupPath); err != nil {
		return fmt.Errorf("failed to backup current binary: %w", err)
	}

	// Try atomic rename first (works for same-device and avoids "text file busy")
	if err := os.Rename(tmpPath, exePath); err != nil {
		// Rename failed (likely cross-device link), try remove + copy approach
		// In Linux, we can remove a running executable - process continues until exit
		if removeErr := os.Remove(exePath); removeErr != nil {
			copyFile(backupPath, exePath)
			return fmt.Errorf("failed to remove old binary: %w", removeErr)
		}

		// Copy new binary to destination
		if copyErr := copyFile(tmpPath, exePath); copyErr != nil {
			copyFile(backupPath, exePath)
			return fmt.Errorf("failed to install new binary: %w", copyErr)
		}
	}

	// Remove backup
	os.Remove(backupPath)

	return nil
}

// copyFile copies a file from src to dst, preserving permissions
func copyFile(src, dst string) error {
	// Read source file
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// Get source file info for permissions
	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	// Create destination file
	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	// Copy contents
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	return dstFile.Sync()
}
