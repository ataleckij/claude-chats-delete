package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	// Parse command-line flags
	updateFlag := flag.Bool("update", false, "Check for updates and install if available")
	versionFlag := flag.Bool("version", false, "Show current version")
	flag.Parse()

	// Show version
	if *versionFlag {
		fmt.Printf("claude-chats v%s\n", CurrentVersion)
		os.Exit(0)
	}

	// Load or create config
	config, err := loadConfig()
	if err != nil {
		// First run - prompt for directory
		dir, err := promptForClaudeDir()
		if err != nil {
			fmt.Printf("Error reading input: %v\n", err)
			os.Exit(1)
		}

		// Validate directory exists
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			fmt.Printf("Error: Directory does not exist: %s\n", dir)
			fmt.Println("Please create the directory or specify a different path.")
			os.Exit(1)
		}

		// Save config with defaults
		config = &Config{
			ClaudeDir:              dir,
			AutoUpdates:            true, // Enable by default
			UpdateCheckIntervalHrs: 1,    // Check every hour
			LastUpdateCheck:        0,
		}
		if err := saveConfig(config); err != nil {
			fmt.Printf("Warning: Could not save config: %v\n", err)
		} else {
			fmt.Printf("\n✓ Configuration saved to: %s\n\n", configPath)
		}
	}

	// Set defaults for existing configs without update settings
	if config.UpdateCheckIntervalHrs == 0 {
		config.UpdateCheckIntervalHrs = 1
		config.AutoUpdates = true
	}

	// Initialize paths from config
	initializePaths(config.ClaudeDir)

	// Manual update check
	if *updateFlag {
		fmt.Printf("Checking for updates...\n")
		if newVersion := checkForUpdate(); newVersion != "" {
			if promptAndUpdate(newVersion) {
				// User declined or update failed
				config.LastUpdateCheck = time.Now().Unix()
				saveConfig(config)
			}
		} else {
			fmt.Printf("You're up to date (v%s)\n", CurrentVersion)
		}
		return
	}

	// Automatic update check (on startup)
	if config.AutoUpdates &&
		os.Getenv("CLAUDE_CHATS_DISABLE_AUTOUPDATER") != "1" &&
		shouldCheckUpdate(config.LastUpdateCheck, config.UpdateCheckIntervalHrs) {

		if newVersion := checkForUpdate(); newVersion != "" {
			// Prompt for update
			if promptAndUpdate(newVersion) {
				// User declined or update failed, save check time
				config.LastUpdateCheck = time.Now().Unix()
				saveConfig(config)
			}
			// If update succeeded, program exits in promptAndUpdate
		} else {
			// No update available, save check time
			config.LastUpdateCheck = time.Now().Unix()
			saveConfig(config)
		}
	}

	// Run TUI
	p := tea.NewProgram(initialModel(config), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
