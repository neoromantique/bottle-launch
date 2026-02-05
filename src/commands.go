// Bubbletea commands: async operations for mounting, app execution, and FIDO2 workflows.
package main

import (
	"os/exec"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
)

// Message types

type errMsg struct {
	err error
}

type bottlesLoadedMsg struct {
	bottles []string
}

type appsLoadedMsg struct {
	apps []FlatpakApp
}

type mountSuccessMsg struct {
	info *MountInfo
}

type mountFailedMsg struct {
	err           error
	wrongPassword bool
}

type appFinishedMsg struct {
	err error
}

type bottleCreatedMsg struct {
	path string
}

type bottleDeletedMsg struct {
	path string
}

// FIDO2 message types

type fido2DevicesMsg struct {
	devices []FIDO2Device
	err     error
}

type fido2CredentialCreatedMsg struct {
	credID string
	salt   string
	err    error
}

type fido2SecretReadyMsg struct {
	secret []byte
	err    error
}

type fido2BottleCreatedMsg struct {
	path string
	err  error
}

type fido2UnlockSuccessMsg struct {
	info *MountInfo
}

type fido2UnlockFailedMsg struct {
	err error
}

// Commands

func loadBottlesCmd() tea.Cmd {
	return func() tea.Msg {
		bottles := listBottles()
		return bottlesLoadedMsg{bottles: bottles}
	}
}

func loadAppsCmd() tea.Cmd {
	return func() tea.Msg {
		apps := listFlatpakApps()
		return appsLoadedMsg{apps: apps}
	}
}

func mountBottleCmd(bottle, password string) tea.Cmd {
	return func() tea.Msg {
		info, err := udisksMountBottle(bottle, password)
		if err != nil {
			if err == errWrongPassword {
				return mountFailedMsg{err: err, wrongPassword: true}
			}
			return mountFailedMsg{err: err}
		}
		return mountSuccessMsg{info: info}
	}
}

func startFlatpakCmd(appID, mountPoint string, perms *Permissions, extraArgs []string) (tea.Cmd, *exec.Cmd) {
	c := buildFlatpakCommand(appID, mountPoint, perms, extraArgs)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return appFinishedMsg{err: err}
	}), c
}

func createBottleCmd(name, size, password string) tea.Cmd {
	return func() tea.Msg {
		// Ensure .bottle extension
		if filepath.Ext(name) != ".bottle" {
			name += ".bottle"
		}

		bottlePath := filepath.Join(bottleDir, name)

		err := createBottleBase(bottlePath, size, password, false)
		if err != nil {
			return errMsg{err: err}
		}
		return bottleCreatedMsg{path: bottlePath}
	}
}

func deleteBottleCmd(bottle string) tea.Cmd {
	return func() tea.Msg {
		err := deleteBottle(bottle)
		if err != nil {
			return errMsg{err: err}
		}
		return bottleDeletedMsg{path: bottle}
	}
}

// FIDO2 commands

func enumerateFIDO2DevicesCmd() tea.Cmd {
	return func() tea.Msg {
		devices, err := EnumerateFIDO2Devices()
		return fido2DevicesMsg{devices: devices, err: err}
	}
}

func createFIDO2CredentialCmd(device, bottleID string) tea.Cmd {
	return func() tea.Msg {
		credID, salt, err := CreateFIDO2Credential(device, bottleID)
		return fido2CredentialCreatedMsg{credID: credID, salt: salt, err: err}
	}
}

func getFIDO2SecretCmd(device, bottleID, credID, salt string) tea.Cmd {
	return func() tea.Msg {
		secret, err := GetFIDO2Secret(device, bottleID, credID, salt)
		return fido2SecretReadyMsg{secret: secret, err: err}
	}
}

func createBottleYubiKeyCmd(name, size string, secret []byte, bottleID, credID, salt, device string) tea.Cmd {
	return func() tea.Msg {
		// Ensure .bottle extension
		if filepath.Ext(name) != ".bottle" {
			name += ".bottle"
		}

		bottlePath := filepath.Join(bottleDir, name)

		err := CreateBottleWithYubiKey(bottlePath, size, secret, bottleID, credID, salt, device)
		if err != nil {
			return fido2BottleCreatedMsg{err: err}
		}
		return fido2BottleCreatedMsg{path: bottlePath}
	}
}

func mountBottleFIDO2Cmd(bottle, device, bottleID, credID, salt string) tea.Cmd {
	return func() tea.Msg {
		// Get FIDO2 secret (requires touch)
		secret, err := GetFIDO2Secret(device, bottleID, credID, salt)
		if err != nil {
			return fido2UnlockFailedMsg{err: err}
		}

		// Mount using the secret
		info, err := udisksMountBottleFIDO2(bottle, secret)
		if err != nil {
			return fido2UnlockFailedMsg{err: err}
		}
		return fido2UnlockSuccessMsg{info: info}
	}
}
