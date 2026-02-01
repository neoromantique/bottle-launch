// Permission settings: loading, saving, and toggling sandbox permissions for bottles.
package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// PermissionDef defines a permission with its metadata
type PermissionDef struct {
	Name  string // Variable name (e.g., "Network")
	Key   string // Shortcut key (e.g., "n")
	Label string // Display label (e.g., "Network")
}

var permissionDefs = []PermissionDef{
	{Name: "Network", Key: "n", Label: "Network"},
	{Name: "Audio", Key: "a", Label: "Audio"},
	{Name: "GPU", Key: "g", Label: "GPU"},
	{Name: "Wayland", Key: "w", Label: "Wayland"},
	{Name: "X11", Key: "x", Label: "X11"},
	{Name: "Camera", Key: "c", Label: "Camera"},
	{Name: "Portals", Key: "p", Label: "Portals"},
}

// Permissions holds the permission settings for a bottle
type Permissions struct {
	Network bool
	Audio   bool
	GPU     bool
	Wayland bool
	X11     bool
	Camera  bool
	Portals bool
	LastApp string

	// FIDO2 fields (all empty = password-based bottle)
	// BottleID is critical: random identifier generated at creation, used as clientDataHash
	FIDO2BottleID     string
	FIDO2CredentialID string
	FIDO2Salt         string
	FIDO2DeviceHint   string // hint only, re-enumerate on unlock
}

// defaultPermissions returns the default permission set
func defaultPermissions() *Permissions {
	return &Permissions{
		Network: true,
		Audio:   true,
		GPU:     true,
		Wayland: true,
		X11:     true,
		Camera:  false,
		Portals: false,
	}
}

// IsEnabled returns whether the permission at index is enabled
func (p *Permissions) IsEnabled(index int) bool {
	switch index {
	case 0:
		return p.Network
	case 1:
		return p.Audio
	case 2:
		return p.GPU
	case 3:
		return p.Wayland
	case 4:
		return p.X11
	case 5:
		return p.Camera
	case 6:
		return p.Portals
	}
	return false
}

// Toggle toggles the permission at index
func (p *Permissions) Toggle(index int) {
	switch index {
	case 0:
		p.Network = !p.Network
	case 1:
		p.Audio = !p.Audio
	case 2:
		p.GPU = !p.GPU
	case 3:
		p.Wayland = !p.Wayland
	case 4:
		p.X11 = !p.X11
	case 5:
		p.Camera = !p.Camera
	case 6:
		p.Portals = !p.Portals
	}
}

// Summary returns a string summary of enabled permissions
func (p *Permissions) Summary() string {
	var parts []string
	if p.Network {
		parts = append(parts, "Network")
	}
	if p.Audio {
		parts = append(parts, "Audio")
	}
	if p.GPU {
		parts = append(parts, "GPU")
	}
	if p.Wayland {
		parts = append(parts, "Wayland")
	}
	if p.X11 {
		parts = append(parts, "X11")
	}
	if p.Camera {
		parts = append(parts, "Camera")
	}
	if p.Portals {
		parts = append(parts, "Portals")
	}
	return strings.Join(parts, " ")
}

// loadPermissions loads permissions from a config file
func loadPermissions(path string) *Permissions {
	p := defaultPermissions()

	file, err := os.Open(path)
	if err != nil {
		return p
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		boolVal := val == "1" || strings.ToLower(val) == "true"

		switch key {
		case "PREF_NETWORK":
			p.Network = boolVal
		case "PREF_AUDIO":
			p.Audio = boolVal
		case "PREF_GPU":
			p.GPU = boolVal
		case "PREF_WAYLAND":
			p.Wayland = boolVal
		case "PREF_X11":
			p.X11 = boolVal
		case "PREF_CAMERA":
			p.Camera = boolVal
		case "PREF_PORTALS":
			p.Portals = boolVal
		case "PREF_LAST_APP":
			p.LastApp = strings.Trim(val, `"`)
		case "FIDO2_BOTTLE_ID":
			p.FIDO2BottleID = strings.Trim(val, `"`)
		case "FIDO2_CREDENTIAL_ID":
			p.FIDO2CredentialID = strings.Trim(val, `"`)
		case "FIDO2_SALT":
			p.FIDO2Salt = strings.Trim(val, `"`)
		case "FIDO2_DEVICE_HINT":
			p.FIDO2DeviceHint = strings.Trim(val, `"`)
		}
	}

	return p
}

// savePermissions saves permissions to a config file
func savePermissions(path string, p *Permissions) error {
	return savePermissionsAtomic(path, p)
}

// savePermissionsAtomic saves permissions atomically (write to temp, fsync, rename)
// This is critical for FIDO2 bottles to avoid data loss on crash
func savePermissionsAtomic(path string, p *Permissions) error {
	os.MkdirAll(filepath.Dir(path), 0755)

	boolToInt := func(b bool) string {
		if b {
			return "1"
		}
		return "0"
	}

	lines := []string{
		"PREF_NETWORK=" + boolToInt(p.Network),
		"PREF_AUDIO=" + boolToInt(p.Audio),
		"PREF_GPU=" + boolToInt(p.GPU),
		"PREF_WAYLAND=" + boolToInt(p.Wayland),
		"PREF_X11=" + boolToInt(p.X11),
		"PREF_CAMERA=" + boolToInt(p.Camera),
		"PREF_PORTALS=" + boolToInt(p.Portals),
		"PREF_LAST_APP=" + strconv.Quote(p.LastApp),
	}

	// Add FIDO2 fields if present
	if p.FIDO2BottleID != "" {
		lines = append(lines, "FIDO2_BOTTLE_ID="+strconv.Quote(p.FIDO2BottleID))
	}
	if p.FIDO2CredentialID != "" {
		lines = append(lines, "FIDO2_CREDENTIAL_ID="+strconv.Quote(p.FIDO2CredentialID))
	}
	if p.FIDO2Salt != "" {
		lines = append(lines, "FIDO2_SALT="+strconv.Quote(p.FIDO2Salt))
	}
	if p.FIDO2DeviceHint != "" {
		lines = append(lines, "FIDO2_DEVICE_HINT="+strconv.Quote(p.FIDO2DeviceHint))
	}

	// Write to temp file first
	tempFile, err := os.CreateTemp(filepath.Dir(path), ".bottle-config-*.tmp")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()

	for _, line := range lines {
		if _, err := tempFile.WriteString(line + "\n"); err != nil {
			tempFile.Close()
			os.Remove(tempPath)
			return err
		}
	}

	// Sync to disk
	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		os.Remove(tempPath)
		return err
	}
	tempFile.Close()

	// Atomic rename
	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath)
		return err
	}

	// Sync parent directory to ensure the rename is durable
	if dir, err := os.Open(filepath.Dir(path)); err == nil {
		dir.Sync()
		dir.Close()
	}

	return nil
}
