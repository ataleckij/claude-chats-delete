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

	// Load config; a missing one means this is the first run
	config, err := loadConfig()
	if err != nil {
		config = &Config{
			AutoUpdates:            true, // Enable by default
			UpdateCheckIntervalHrs: 1,    // Check every hour
			LastUpdateCheck:        0,
		}
	}

	// Reconcile the saved Claude directory with the one the environment points
	// at. Decisions taken without asking apply to this run only and are never
	// written back to the config.
	claudeDirForRun := config.ClaudeDir
	decision := resolveClaudeDir(config.ClaudeDir, systemClaudeDir(), isInteractive())
	switch decision.action {
	case dirAskFirstRun:
		dir, err := promptForClaudeDir(decision.dir)
		if err != nil {
			fmt.Printf("Error reading input: %v\n", err)
			os.Exit(1)
		}
		claudeDirForRun = dir
		config.ClaudeDir = dir
		if err := saveConfig(config); err != nil {
			fmt.Printf("Warning: Could not save config: %v\n", err)
		} else {
			fmt.Printf("\n✓ Configuration saved to: %s\n\n", configPath)
		}

	case dirAskSwitch:
		switchDir, err := promptToSwitchClaudeDir(config.ClaudeDir, decision.dir)
		if err != nil {
			fmt.Printf("Error reading input: %v\n", err)
			os.Exit(1)
		}
		if switchDir {
			claudeDirForRun = decision.dir
			config.ClaudeDir = decision.dir
			if err := saveConfig(config); err != nil {
				fmt.Printf("Warning: Could not save config: %v\n", err)
			}
		}

	case dirUseSystem:
		claudeDirForRun = decision.dir
	}

	// Set defaults for existing configs without update settings
	if config.UpdateCheckIntervalHrs == 0 {
		config.UpdateCheckIntervalHrs = 1
		config.AutoUpdates = true
	}

	// TODO: a Claude directory that is missing or empty - the default, a
	// CLAUDE_CONFIG_DIR path, or one typed at first run - currently just yields
	// an empty chat list. Decide how to report that instead of looking like a
	// history with no sessions.
	initializePaths(claudeDirForRun)

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
