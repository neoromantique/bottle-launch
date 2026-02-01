// Flatpak integration: application listing and sandboxed execution.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// FlatpakApp represents an installed Flatpak application
type FlatpakApp struct {
	ID   string
	Name string
}

// listFlatpakApps returns all installed Flatpak applications.
// Returns nil if flatpak is not available or the command fails.
func listFlatpakApps() []FlatpakApp {
	out, err := exec.Command("flatpak", "list", "--app", "--columns=application,name").Output()
	if err != nil {
		return nil
	}

	var apps []FlatpakApp
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Format: com.example.App\tApp Name
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) < 1 {
			continue
		}

		app := FlatpakApp{ID: parts[0]}
		if len(parts) >= 2 {
			app.Name = parts[1]
		} else {
			app.Name = parts[0]
		}
		apps = append(apps, app)
	}

	sort.Slice(apps, func(i, j int) bool {
		return apps[i].Name < apps[j].Name
	})

	return apps
}

// buildFlatpakArgs builds the flatpak run command arguments
func buildFlatpakArgs(appID, mountPoint string, perms *Permissions, extraArgs []string) []string {
	args := []string{
		"run",
		"--sandbox",
		"--filesystem=" + mountPoint,
	}

	// Permissions
	if perms.Network {
		args = append(args, "--share=network")
	}
	if perms.Audio {
		args = append(args, "--socket=pulseaudio")
	}
	if perms.GPU {
		args = append(args, "--device=dri")
	}
	if perms.Wayland {
		args = append(args, "--socket=wayland")
	}
	if perms.X11 {
		args = append(args, "--socket=fallback-x11")
	}
	if perms.Camera {
		args = append(args, "--device=video0")
	}
	if perms.Portals {
		args = append(args,
			"--talk-name=org.freedesktop.portal.Desktop",
			"--talk-name=org.freedesktop.portal.Notification",
			"--talk-name=org.freedesktop.portal.FileChooser",
		)
	}

	// Environment
	args = append(args,
		"--env=GTK_USE_PORTAL=0",
		"--env=HOME="+mountPoint,
		"--env=XDG_DATA_HOME="+filepath.Join(mountPoint, ".local", "share"),
		"--env=XDG_CONFIG_HOME="+filepath.Join(mountPoint, ".config"),
		"--env=XDG_CACHE_HOME="+filepath.Join(mountPoint, ".cache"),
		"--env=XDG_DOWNLOAD_DIR="+filepath.Join(mountPoint, "Downloads"),
	)

	args = append(args, appID)
	args = append(args, extraArgs...)

	return args
}

// runFlatpakApp runs a Flatpak app (blocking)
func runFlatpakApp(appID, mountPoint string, perms *Permissions, extraArgs []string) error {
	// Create standard directories
	dirs := []string{
		"Downloads",
		".config",
		".local/share",
		".cache",
	}
	for _, dir := range dirs {
		os.MkdirAll(filepath.Join(mountPoint, dir), 0755)
	}

	args := buildFlatpakArgs(appID, mountPoint, perms, extraArgs)
	cmd := exec.Command("flatpak", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
