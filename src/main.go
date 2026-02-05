// Main entry point: CLI argument parsing, signal handling, and TUI initialization.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Global state for signal handler cleanup
var (
	currentMountInfo *MountInfo
	currentRunningCmd *exec.Cmd
	mountMutex       sync.Mutex
	cleanupOnce      sync.Once
)

// SetCurrentMountInfo updates the global mount info (for signal handler cleanup)
func SetCurrentMountInfo(info *MountInfo) {
	mountMutex.Lock()
	currentMountInfo = info
	mountMutex.Unlock()
}

// SetCurrentRunningCmd updates the global running command (for signal handler cleanup)
func SetCurrentRunningCmd(cmd *exec.Cmd) {
	mountMutex.Lock()
	currentRunningCmd = cmd
	mountMutex.Unlock()
}

// setupSignalHandler sets up signal handling to unmount on abnormal exit.
// Handles SIGTERM, SIGHUP, and SIGQUIT. SIGINT is handled by Bubbletea in TUI mode.
func setupSignalHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	go func() {
		sig := <-c
		performCleanup()
		// Use appropriate exit code based on signal
		switch sig {
		case syscall.SIGTERM:
			os.Exit(128 + 15) // SIGTERM = 15
		case syscall.SIGHUP:
			os.Exit(128 + 1) // SIGHUP = 1
		case syscall.SIGQUIT:
			os.Exit(128 + 3) // SIGQUIT = 3
		default:
			os.Exit(ExitSIGINT)
		}
	}()
}

// setupSignalHandlerCLI sets up signal handling for CLI mode.
// Also handles SIGINT since there's no TUI to intercept it.
func setupSignalHandlerCLI() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	go func() {
		sig := <-c
		performCleanup()
		// Use appropriate exit code based on signal
		switch sig {
		case syscall.SIGINT:
			os.Exit(ExitSIGINT)
		case syscall.SIGTERM:
			os.Exit(128 + 15) // SIGTERM = 15
		case syscall.SIGHUP:
			os.Exit(128 + 1) // SIGHUP = 1
		case syscall.SIGQUIT:
			os.Exit(128 + 3) // SIGQUIT = 3
		default:
			os.Exit(ExitSIGINT)
		}
	}()
}

// performCleanup stops any running process and unmounts the bottle.
// Safe to call multiple times due to sync.Once.
func performCleanup() {
	cleanupOnce.Do(func() {
		mountMutex.Lock()
		defer mountMutex.Unlock()

		// Stop running Flatpak process first
		if currentRunningCmd != nil && currentRunningCmd.Process != nil {
			_ = currentRunningCmd.Process.Signal(syscall.SIGTERM)
			// Give it a moment to terminate gracefully
			time.Sleep(200 * time.Millisecond)
			// Force kill if still running
			_ = currentRunningCmd.Process.Kill()
			currentRunningCmd = nil
		}

		// Unmount the bottle
		if currentMountInfo != nil {
			_ = udisksUnmountBottle(currentMountInfo)
			currentMountInfo = nil
		}
	})
}

func main() {
	// Parse CLI args - default to TUI mode
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-h", "--help", "help":
			printUsage()
			return
		case "create":
			if len(os.Args) < 4 {
				fmt.Fprintln(os.Stderr, "Usage: bottle-launch create <bottle> <size>")
				os.Exit(1)
			}
			if err := cmdCreate(os.Args[2], os.Args[3]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "run":
			if len(os.Args) < 4 {
				fmt.Fprintln(os.Stderr, "Usage: bottle-launch run <bottle> <app_id> [-- args...]")
				os.Exit(1)
			}
			bottle := os.Args[2]
			appID := os.Args[3]
			var extraArgs []string
			for i := 4; i < len(os.Args); i++ {
				if os.Args[i] == "--" {
					extraArgs = os.Args[i+1:]
					break
				}
			}
			if err := cmdRun(bottle, appID, extraArgs); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "list":
			cmdList()
			return
		case "tui":
			// Fall through to TUI mode
		default:
			fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
			printUsage()
			os.Exit(1)
		}
	}

	// TUI mode
	setupSignalHandler()

	// Ensure cleanup happens on panic or unexpected exit
	defer func() {
		if r := recover(); r != nil {
			performCleanup()
			panic(r) // Re-panic after cleanup
		}
	}()

	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		performCleanup()
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`Usage: bottle-launch <command> [options]

Commands:
    tui                       Interactive TUI mode (default)
    create <bottle> <size>    Create a new encrypted bottle
    run <bottle> <app_id> [-- extra_args...]
                              Run Flatpak app with data in bottle
    list                      List currently mounted bottles

Examples:
    bottle-launch
    bottle-launch tui
    bottle-launch create myapp.bottle 2G
    bottle-launch run firefox.bottle org.mozilla.firefox
    bottle-launch run firefox.bottle org.mozilla.firefox -- --private-window

Bottle storage: ~/.local/share/bottles/
Config storage: ~/.config/bottle-launch/
`)
}

// cmdCreate creates a new bottle from CLI
func cmdCreate(bottle, size string) error {
	return createBottleBase(bottle, size, "", false)
}

// cmdRun runs an app in CLI mode
func cmdRun(bottle, appID string, extraArgs []string) error {
	// Load default permissions
	configPath := getConfigPath(bottle)
	perms := loadPermissions(configPath)

	// Mount bottle (will prompt for password via polkit)
	mountInfo, err := udisksMountBottle(bottle, "")
	if err != nil {
		return err
	}
	SetCurrentMountInfo(mountInfo)
	setupSignalHandlerCLI()
	defer func() {
		SetCurrentRunningCmd(nil)
		SetCurrentMountInfo(nil)
		udisksUnmountBottle(mountInfo)
	}()

	// Build and run the app, tracking the command for signal cleanup
	cmd := buildFlatpakCommand(appID, mountInfo.MountPoint, perms, extraArgs)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	SetCurrentRunningCmd(cmd)
	return cmd.Run()
}

// cmdList lists mounted bottles
func cmdList() {
	bottles := listBottles()
	fmt.Println("Currently mounted bottles:")
	fmt.Println()

	found := false
	for _, bottle := range bottles {
		loopDev := findLoopForFile(bottle)
		if loopDev == "" {
			continue
		}

		found = true
		fmt.Printf("  Bottle: %s\n", bottleName(bottle))
		fmt.Printf("  File:   %s\n", bottle)
		fmt.Printf("  Loop:   %s\n", loopDev)

		cleartext := findCleartextForLoop(loopDev)
		if cleartext != "" {
			fmt.Printf("  Crypt:  %s\n", cleartext)
			mount := findMountForDevice(cleartext)
			if mount != "" {
				fmt.Printf("  Mount:  %s\n", mount)
			} else {
				fmt.Printf("  Mount:  (unlocked but not mounted)\n")
			}
		} else {
			fmt.Printf("  Status: (locked)\n")
		}
		fmt.Println()
	}

	if !found {
		fmt.Println("  (none)")
	}
}
