// Bottle management: listing, creation, deletion, and path handling for encrypted containers.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

var (
	bottleDir string
	configDir string
)

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		// Fall back to /tmp if home directory is unavailable
		home = "/tmp"
	}

	// BOTTLE_DIR environment variable or default
	bottleDir = os.Getenv("BOTTLE_DIR")
	if bottleDir == "" {
		bottleDir = filepath.Join(home, ".local", "share", "bottles")
	}

	// Config dir follows XDG
	xdgConfig := os.Getenv("XDG_CONFIG_HOME")
	if xdgConfig == "" {
		xdgConfig = filepath.Join(home, ".config")
	}
	configDir = filepath.Join(xdgConfig, "bottle-launch")
}

// listBottles returns all .bottle files in the bottle directory
func listBottles() []string {
	os.MkdirAll(bottleDir, 0755)

	entries, err := os.ReadDir(bottleDir)
	if err != nil {
		return nil
	}

	var bottles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".bottle") {
			bottles = append(bottles, filepath.Join(bottleDir, e.Name()))
		}
	}

	sort.Strings(bottles)
	return bottles
}

// bottleName returns just the filename of a bottle path
func bottleName(path string) string {
	return filepath.Base(path)
}

// getBottleHash returns a 12-char hash of the bottle's real path
func getBottleHash(bottle string) string {
	realPath, err := filepath.Abs(bottle)
	if err != nil {
		realPath = bottle
	}

	hash := sha256.Sum256([]byte(realPath))
	return hex.EncodeToString(hash[:])[:12]
}

// getMapperName returns the dm-crypt mapper name for a bottle
func getMapperName(bottle string) string {
	return "bottle-" + getBottleHash(bottle)
}

// getConfigPath returns the config file path for a bottle
func getConfigPath(bottle string) string {
	return filepath.Join(configDir, getBottleHash(bottle)+".conf")
}

// getFSLabel returns a filesystem label derived from the bottle name.
// ext4 labels are limited to 16 characters.
func getFSLabel(bottle string) string {
	name := filepath.Base(bottle)
	name = strings.TrimSuffix(name, ".bottle")
	if len(name) > 16 {
		name = name[:16]
	}
	return name
}

// findLoopForFile finds the loop device associated with a file
func findLoopForFile(bottle string) string {
	realPath, err := filepath.Abs(bottle)
	if err != nil {
		realPath = bottle
	}
	out, err := exec.Command("losetup", "-j", realPath).Output()
	if err != nil || len(out) == 0 {
		return ""
	}

	// Format: /dev/loop0: [2049]:12345 (/path/to/file)
	line := strings.TrimSpace(string(out))
	if idx := strings.Index(line, ":"); idx > 0 {
		return line[:idx]
	}
	return ""
}

// findCleartextForLoop finds the dm-crypt device under a loop device
func findCleartextForLoop(loopDev string) string {
	out, err := exec.Command("lsblk", "-nlo", "NAME,TYPE", loopDev).Output()
	if err != nil {
		return ""
	}

	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == "crypt" {
			return "/dev/" + fields[0]
		}
	}
	return ""
}

// findMountForDevice finds the mount point for a device
func findMountForDevice(device string) string {
	out, err := exec.Command("lsblk", "-nlo", "MOUNTPOINT", device).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// createBottleBase creates a new bottle file with LUKS encryption
func createBottleBase(bottle, size, password string, interactive bool) error {
	// Ensure bottle directory exists (for CLI create on fresh install)
	os.MkdirAll(bottleDir, 0755)

	if bottle == "" {
		return errBottlePathRequired
	}
	if size == "" {
		return errSizeRequired
	}

	// Ensure .bottle extension
	if !strings.HasSuffix(bottle, ".bottle") {
		bottle += ".bottle"
	}

	// If just a name, put in bottle dir
	if !strings.Contains(bottle, string(os.PathSeparator)) {
		bottle = filepath.Join(bottleDir, bottle)
	}

	if _, err := os.Stat(bottle); err == nil {
		return errBottleExists
	}

	realPath, err := filepath.Abs(bottle)
	if err != nil {
		return &bottleError{op: "path", msg: err.Error()}
	}
	mapperName := getMapperName(realPath)

	// Create sparse file
	cmd := exec.Command("truncate", "-s", size, realPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return &bottleError{op: "create file", msg: string(out)}
	}

	// LUKS format
	var luksCmd *exec.Cmd
	if password != "" {
		luksCmd = cryptsetupCmd("luksFormat", "--type", "luks2", "--batch-mode", realPath, "-")
		luksCmd.Stdin = strings.NewReader(password)
	} else {
		luksCmd = cryptsetupCmd("luksFormat", "--type", "luks2", realPath)
	}
	if out, err := luksCmd.CombinedOutput(); err != nil {
		os.Remove(realPath)
		return &bottleError{op: "LUKS format", msg: string(out)}
	}

	// Setup loop device
	loopOut, err := privCmd("losetup", "--find", "--show", "--", realPath).Output()
	if err != nil {
		os.Remove(realPath)
		return &bottleError{op: "loop setup", msg: err.Error()}
	}
	loopDev := strings.TrimSpace(string(loopOut))

	// Open LUKS
	var openCmd *exec.Cmd
	if password != "" {
		openCmd = cryptsetupCmd("open", "--key-file=-", loopDev, mapperName)
		openCmd.Stdin = strings.NewReader(password)
	} else {
		openCmd = cryptsetupCmd("open", loopDev, mapperName)
	}
	if out, err := openCmd.CombinedOutput(); err != nil {
		privCmd("losetup", "-d", loopDev).Run()
		os.Remove(realPath)
		return &bottleError{op: "LUKS open", msg: string(out)}
	}

	// Create filesystem with label for consistent mount point naming
	if out, err := privCmd("mkfs.ext4", "-q", "-L", getFSLabel(realPath), "/dev/mapper/"+mapperName).CombinedOutput(); err != nil {
		cryptsetupCmd("close", mapperName).Run()
		privCmd("losetup", "-d", loopDev).Run()
		os.Remove(realPath)
		return &bottleError{op: "mkfs", msg: string(out)}
	}

	// Cleanup
	cryptsetupCmd("close", mapperName).Run()
	privCmd("losetup", "-d", loopDev).Run()

	return nil
}

// deleteBottle removes a bottle file and its config
func deleteBottle(bottle string) error {
	// Check if mounted
	loopDev := findLoopForFile(bottle)
	if loopDev != "" {
		return errBottleMounted
	}

	if err := os.Remove(bottle); err != nil {
		return err
	}

	// Also remove config
	os.Remove(getConfigPath(bottle))
	return nil
}

// Errors
type bottleError struct {
	op  string
	msg string
}

func (e *bottleError) Error() string {
	return e.op + ": " + e.msg
}

var (
	errBottlePathRequired = &bottleError{op: "bottle", msg: "path required"}
	errSizeRequired       = &bottleError{op: "bottle", msg: "size required"}
	errBottleExists       = &bottleError{op: "bottle", msg: "already exists"}
	errBottleMounted      = &bottleError{op: "bottle", msg: "currently mounted - close any running apps first"}
)

// CreateBottleWithYubiKey creates a new bottle encrypted with FIDO2/YubiKey
// The FIDO2 secret is the ONLY LUKS passphrase - no password is ever set
func CreateBottleWithYubiKey(bottle, size string, fido2Secret []byte, bottleID, credID, salt, deviceHint string) error {
	if bottle == "" {
		return errBottlePathRequired
	}
	if size == "" {
		return errSizeRequired
	}
	if len(fido2Secret) != 32 {
		return &bottleError{op: "fido2", msg: "invalid secret length"}
	}

	// Ensure .bottle extension
	if !strings.HasSuffix(bottle, ".bottle") {
		bottle += ".bottle"
	}

	// If just a name, put in bottle dir
	if !strings.Contains(bottle, string(os.PathSeparator)) {
		bottle = filepath.Join(bottleDir, bottle)
	}

	if _, err := os.Stat(bottle); err == nil {
		return errBottleExists
	}

	realPath, err := filepath.Abs(bottle)
	if err != nil {
		return &bottleError{op: "path", msg: err.Error()}
	}
	mapperName := getMapperName(realPath)
	configPath := getConfigPath(realPath)

	// Create sparse file
	cmd := exec.Command("truncate", "-s", size, realPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return &bottleError{op: "create file", msg: string(out)}
	}

	// CRITICAL: Save config FIRST with FIDO2 fields (atomic write + fsync)
	// This ensures recovery data exists BEFORE destructive operations
	perms := defaultPermissions()
	perms.FIDO2BottleID = bottleID
	perms.FIDO2CredentialID = credID
	perms.FIDO2Salt = salt
	perms.FIDO2DeviceHint = deviceHint

	if err := savePermissionsAtomic(configPath, perms); err != nil {
		os.Remove(realPath)
		return &bottleError{op: "save config", msg: err.Error()}
	}

	// LUKS format with FIDO2 secret
	if err := FormatBottleWithFIDO2(realPath, fido2Secret); err != nil {
		os.Remove(realPath)
		os.Remove(configPath)
		return err
	}

	// Setup loop device
	loopOut, err := privCmd("losetup", "--find", "--show", "--", realPath).Output()
	if err != nil {
		os.Remove(realPath)
		os.Remove(configPath)
		return &bottleError{op: "loop setup", msg: err.Error()}
	}
	loopDev := strings.TrimSpace(string(loopOut))

	// Open LUKS with FIDO2 secret
	if err := OpenLUKSWithFIDO2(loopDev, mapperName, fido2Secret); err != nil {
		privCmd("losetup", "-d", loopDev).Run()
		os.Remove(realPath)
		os.Remove(configPath)
		return err
	}

	// Create filesystem with label for consistent mount point naming
	if out, err := privCmd("mkfs.ext4", "-q", "-L", getFSLabel(realPath), "/dev/mapper/"+mapperName).CombinedOutput(); err != nil {
		cryptsetupCmd("close", mapperName).Run()
		privCmd("losetup", "-d", loopDev).Run()
		os.Remove(realPath)
		os.Remove(configPath)
		return &bottleError{op: "mkfs", msg: string(out)}
	}

	// Cleanup
	cryptsetupCmd("close", mapperName).Run()
	privCmd("losetup", "-d", loopDev).Run()

	return nil
}
