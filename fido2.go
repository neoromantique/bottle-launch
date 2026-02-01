// FIDO2/YubiKey support: device enumeration, credential creation, and hmac-secret retrieval.
package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Use constants from constants.go
const (
	fido2RPID     = DefaultFIDO2RPID
	fido2UserName = DefaultFIDO2User
)

// FIDO2Device represents an available authenticator
type FIDO2Device struct {
	Path        string // e.g., "/dev/hidraw3"
	Description string // e.g., "Yubico YubiKey"
}

// CheckFIDO2Available verifies libfido2 tools are installed
func CheckFIDO2Available() error {
	for _, tool := range []string{"fido2-token", "fido2-cred", "fido2-assert"} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("%s not found - install libfido2", tool)
		}
	}
	return nil
}

// CheckUdisksAvailable verifies udisksctl is installed
func CheckUdisksAvailable() error {
	if _, err := exec.LookPath("udisksctl"); err != nil {
		return fmt.Errorf("udisksctl not found - install udisks2")
	}
	return nil
}

// CheckPrivilegeEscalation verifies pkexec or sudo is available
func CheckPrivilegeEscalation() error {
	if _, err := exec.LookPath("pkexec"); err == nil {
		return nil
	}
	if _, err := exec.LookPath("sudo"); err == nil {
		return nil
	}
	return fmt.Errorf("neither pkexec nor sudo found - cannot create LUKS volume")
}

// EnumerateFIDO2Devices lists connected FIDO2 authenticators
func EnumerateFIDO2Devices() ([]FIDO2Device, error) {
	out, err := exec.Command("fido2-token", "-L").Output()
	if err != nil {
		return nil, fmt.Errorf("fido2-token -L failed: %w", err)
	}

	var devices []FIDO2Device
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: /dev/hidraw3: vendor=0x1050, product=0x0407 (Description)
		parts := strings.SplitN(line, ":", 2)
		if len(parts) >= 1 {
			dev := FIDO2Device{Path: strings.TrimSpace(parts[0])}
			if len(parts) >= 2 {
				dev.Description = strings.TrimSpace(parts[1])
			}
			devices = append(devices, dev)
		}
	}
	return devices, nil
}

// generateBottleID creates a random 32-byte ID for a new bottle (base64 encoded)
// This is stored in config and used as clientDataHash for FIDO2 operations
func generateBottleID() (string, error) {
	id := make([]byte, 32)
	if _, err := rand.Read(id); err != nil {
		return "", fmt.Errorf("failed to generate bottle ID: %w", err)
	}
	return base64.StdEncoding.EncodeToString(id), nil
}

// CreateFIDO2Credential creates a credential and returns (credentialID, salt)
// bottleID should be generated fresh via generateBottleID() and saved to config
func CreateFIDO2Credential(device, bottleID string) (credID, salt string, err error) {
	clientData := bottleID // bottleID is already base64-encoded 32 bytes

	// Generate random 32-byte salt
	saltBytes := make([]byte, 32)
	if _, err := rand.Read(saltBytes); err != nil {
		return "", "", fmt.Errorf("generate salt: %w", err)
	}
	salt = base64.StdEncoding.EncodeToString(saltBytes)

	// Create temp input file with restricted permissions
	inputFile, err := os.CreateTemp("", "fido2-cred-input-")
	if err != nil {
		return "", "", err
	}
	defer os.Remove(inputFile.Name())
	os.Chmod(inputFile.Name(), 0600)

	// Write input: cdh, rpid, user_name, user_id
	fmt.Fprintf(inputFile, "%s\n%s\n%s\n%s\n",
		clientData, fido2RPID, fido2UserName, clientData)
	inputFile.Close()

	// Run fido2-cred with input file
	input, err := os.Open(inputFile.Name())
	if err != nil {
		return "", "", err
	}
	defer input.Close()

	var stdout, stderr bytes.Buffer
	cmd := exec.Command("fido2-cred", "-M", "-h", device, "es256")
	cmd.Stdin = input
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("fido2-cred failed: %s", stderr.String())
	}

	// Parse output - credential_id is line 5 (0-indexed: 4)
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) < 5 {
		return "", "", fmt.Errorf("unexpected fido2-cred output format: expected at least 5 lines, got %d", len(lines))
	}
	credID = strings.TrimSpace(lines[4])

	return credID, salt, nil
}

// GetFIDO2Secret retrieves the hmac-secret (requires touch)
// bottleID comes from config.FIDO2BottleID
// Returns raw 32-byte secret
func GetFIDO2Secret(device, bottleID, credID, salt string) ([]byte, error) {
	clientData := bottleID // bottleID is already base64-encoded 32 bytes

	// Create temp input file
	inputFile, err := os.CreateTemp("", "fido2-assert-input-")
	if err != nil {
		return nil, err
	}
	defer os.Remove(inputFile.Name())
	os.Chmod(inputFile.Name(), 0600)

	// Write input: cdh, rpid, cred_id, hmac_salt
	fmt.Fprintf(inputFile, "%s\n%s\n%s\n%s\n",
		clientData, fido2RPID, credID, salt)
	inputFile.Close()

	// Run fido2-assert
	input, err := os.Open(inputFile.Name())
	if err != nil {
		return nil, err
	}
	defer input.Close()

	var stdout, stderr bytes.Buffer
	cmd := exec.Command("fido2-assert", "-G", "-h", device, "es256")
	cmd.Stdin = input
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("fido2-assert failed: %s", stderr.String())
	}

	// Parse output - hmac_secret is last line (may be line 4 or 5 depending on flags)
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) < 5 {
		return nil, fmt.Errorf("unexpected fido2-assert output: expected at least 5 lines, got %d", len(lines))
	}
	hmacSecretB64 := strings.TrimSpace(lines[len(lines)-1])

	// Decode base64 to raw bytes
	secret, err := base64.StdEncoding.DecodeString(hmacSecretB64)
	if err != nil {
		return nil, fmt.Errorf("decode hmac-secret: %w", err)
	}
	if len(secret) != 32 {
		return nil, fmt.Errorf("unexpected hmac-secret length: %d", len(secret))
	}

	return secret, nil
}

// privCmd creates a command with appropriate privilege escalation
// Tries pkexec first (graphical polkit prompt), falls back to sudo
func privCmd(name string, args ...string) *exec.Cmd {
	if _, err := exec.LookPath("pkexec"); err == nil {
		return exec.Command("pkexec", append([]string{name}, args...)...)
	}
	return exec.Command("sudo", append([]string{name}, args...)...)
}

// cryptsetupCmd creates a command with appropriate privilege escalation
// Tries pkexec first (graphical polkit prompt), falls back to sudo
func cryptsetupCmd(args ...string) *exec.Cmd {
	return privCmd("cryptsetup", args...)
}

// writeSecretToTempFile writes binary secret to a temp file with mode 0600
// Returns path and cleanup function
func writeSecretToTempFile(secret []byte, prefix string) (string, func(), error) {
	f, err := os.CreateTemp("", prefix)
	if err != nil {
		return "", nil, err
	}
	path := f.Name()
	os.Chmod(path, 0600)
	f.Write(secret)
	f.Close()
	cleanup := func() { os.Remove(path) }
	return path, cleanup, nil
}

// FormatBottleWithFIDO2 creates a LUKS-encrypted bottle using FIDO2-derived secret
func FormatBottleWithFIDO2(bottlePath string, fido2Secret []byte) error {
	keyPath, cleanup, err := writeSecretToTempFile(fido2Secret, "fido2-luks-key-")
	if err != nil {
		return err
	}
	defer cleanup()

	cmd := cryptsetupCmd("luksFormat",
		"--type", "luks2",
		"--batch-mode",
		"--key-file", keyPath,
		bottlePath)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("luksFormat: %s", stderr.String())
	}
	return nil
}

// OpenLUKSWithFIDO2 opens a LUKS device using FIDO2-derived secret
func OpenLUKSWithFIDO2(loopDev, mapperName string, fido2Secret []byte) error {
	keyPath, cleanup, err := writeSecretToTempFile(fido2Secret, "fido2-luks-open-")
	if err != nil {
		return err
	}
	defer cleanup()

	cmd := cryptsetupCmd("open",
		"--key-file", keyPath,
		loopDev, mapperName)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cryptsetup open: %s", stderr.String())
	}
	return nil
}

// IsFIDO2Bottle checks if a bottle is configured to use FIDO2
// Returns true if all FIDO2 fields are present, false if none are present
// Returns error if partially configured (corrupted state)
func IsFIDO2Bottle(perms *Permissions) (bool, error) {
	hasBottleID := perms.FIDO2BottleID != ""
	hasCredID := perms.FIDO2CredentialID != ""
	hasSalt := perms.FIDO2Salt != ""

	// All present = FIDO2 bottle
	if hasBottleID && hasCredID && hasSalt {
		return true, nil
	}

	// None present = password bottle
	if !hasBottleID && !hasCredID && !hasSalt {
		return false, nil
	}

	// Partial = corrupted config
	return false, fmt.Errorf("config corrupted: FIDO2 data incomplete (bottle_id=%v, cred_id=%v, salt=%v)",
		hasBottleID, hasCredID, hasSalt)
}
