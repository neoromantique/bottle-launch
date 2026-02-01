// Main entry point: CLI argument parsing, signal handling, and TUI initialization.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
)

// Global mount state for signal handler cleanup
var (
	currentMountInfo *MountInfo
	mountMutex       sync.Mutex
)

// SetCurrentMountInfo updates the global mount info (for signal handler cleanup)
func SetCurrentMountInfo(info *MountInfo) {
	mountMutex.Lock()
	currentMountInfo = info
	mountMutex.Unlock()
}

// setupSignalHandler sets up SIGTERM/SIGINT handling to unmount on abnormal exit
func setupSignalHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-c
		mountMutex.Lock()
		if currentMountInfo != nil {
			udisksUnmountBottle(currentMountInfo)
		}
		mountMutex.Unlock()
		os.Exit(ExitSIGINT)
	}()
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
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
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
	defer udisksUnmountBottle(mountInfo)

	// Run the app
	return runFlatpakApp(appID, mountInfo.MountPoint, perms, extraArgs)
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
