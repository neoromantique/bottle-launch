// Mount operations using udisks2 for LUKS unlock/lock and filesystem mount/unmount.
package main

import (
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// MountInfo holds the state of a mounted bottle
type MountInfo struct {
	LoopDevice      string
	CleartextDevice string
	MountPoint      string
	BottlePath      string
}

// udisksMountBottle mounts a bottle using udisks2
func udisksMountBottle(bottle, password string) (*MountInfo, error) {
	realPath, err := filepath.Abs(bottle)
	if err != nil {
		return nil, err
	}

	info := &MountInfo{BottlePath: realPath}

	// Check if already mounted
	info.LoopDevice = findLoopForFile(realPath)
	if info.LoopDevice != "" {
		info.CleartextDevice = findCleartextForLoop(info.LoopDevice)
		if info.CleartextDevice != "" {
			info.MountPoint = findMountForDevice(info.CleartextDevice)
			if info.MountPoint != "" {
				// Already fully mounted
				return info, nil
			}
		}
	}

	// Setup loop device if needed
	if info.LoopDevice == "" {
		out, err := exec.Command("udisksctl", "loop-setup", "-f", realPath).CombinedOutput()
		if err != nil {
			return nil, &mountError{op: "loop-setup", msg: string(out)}
		}
		// Parse: Mapped file ... as /dev/loop0.
		re := regexp.MustCompile(`/dev/loop\d+`)
		match := re.FindString(string(out))
		if match == "" {
			return nil, &mountError{op: "loop-setup", msg: "could not parse loop device"}
		}
		info.LoopDevice = match
	}

	// Unlock if needed
	if info.CleartextDevice == "" {
		var unlockCmd *exec.Cmd
		if password != "" {
			unlockCmd = exec.Command("udisksctl", "unlock", "-b", info.LoopDevice, "--key-file", "/dev/stdin")
			unlockCmd.Stdin = strings.NewReader(password)
		} else {
			unlockCmd = exec.Command("udisksctl", "unlock", "-b", info.LoopDevice)
		}

		out, err := unlockCmd.CombinedOutput()
		if err != nil {
			outStr := string(out)
			// Check for wrong password
			if strings.Contains(outStr, "Failed to activate device") ||
				strings.Contains(outStr, "No key available") ||
				strings.Contains(outStr, "passphrase") {
				return nil, errWrongPassword
			}
			return nil, &mountError{op: "unlock", msg: outStr}
		}

		// Parse: Unlocked /dev/loop0 as /dev/dm-0.
		re := regexp.MustCompile(`/dev/dm-\d+`)
		match := re.FindString(string(out))
		if match == "" {
			return nil, &mountError{op: "unlock", msg: "could not parse cleartext device"}
		}
		info.CleartextDevice = match
	}

	// Mount if needed
	if info.MountPoint == "" {
		out, err := exec.Command("udisksctl", "mount", "-b", info.CleartextDevice,
			"--options", "nodev,nosuid,noexec").CombinedOutput()
		if err != nil {
			outStr := string(out)
			if strings.Contains(outStr, "Error looking up object for device") && info.LoopDevice != "" {
				// Stale dm device; relock + unlock to refresh udisks state, then retry mount.
				_, _ = exec.Command("udisksctl", "lock", "-b", info.LoopDevice).CombinedOutput()
				var unlockCmd *exec.Cmd
				if password != "" {
					unlockCmd = exec.Command("udisksctl", "unlock", "-b", info.LoopDevice, "--key-file", "/dev/stdin")
					unlockCmd.Stdin = strings.NewReader(password)
				} else {
					unlockCmd = exec.Command("udisksctl", "unlock", "-b", info.LoopDevice)
				}
				out2, err2 := unlockCmd.CombinedOutput()
				if err2 != nil {
					return nil, &mountError{op: "unlock", msg: string(out2)}
				}
				re := regexp.MustCompile(`/dev/dm-\d+`)
				match := re.FindString(string(out2))
				if match == "" {
					return nil, &mountError{op: "unlock", msg: "could not parse cleartext device"}
				}
				info.CleartextDevice = match

				out3, err3 := exec.Command("udisksctl", "mount", "-b", info.CleartextDevice,
					"--options", "nodev,nosuid,noexec").CombinedOutput()
				if err3 != nil {
					return nil, &mountError{op: "mount", msg: string(out3)}
				}
				out = out3
			} else {
				return nil, &mountError{op: "mount", msg: outStr}
			}
		}

		// Parse: Mounted /dev/dm-0 at /run/media/user/...
		re := regexp.MustCompile(`at (/\S+)`)
		match := re.FindStringSubmatch(string(out))
		if len(match) < 2 {
			return nil, &mountError{op: "mount", msg: "could not parse mount point"}
		}
		info.MountPoint = strings.TrimSuffix(match[1], ".")
	}

	return info, nil
}

// udisksUnmountBottle unmounts and locks a bottle
func udisksUnmountBottle(info *MountInfo) error {
	if info == nil {
		return nil
	}

	// Sync filesystem - critical for data persistence
	if info.MountPoint != "" {
		if err := exec.Command("sync", "-f", info.MountPoint).Run(); err != nil {
			// Log but continue - sync failure is concerning but we should still try to unmount
		}
	}

	// Unmount with retry and force fallback
	if info.CleartextDevice != "" {
		out, err := exec.Command("udisksctl", "unmount", "-b", info.CleartextDevice).CombinedOutput()
		if err != nil {
			// Try lazy unmount as fallback (handles busy mounts with open file handles)
			out2, err2 := exec.Command("udisksctl", "unmount", "-b", info.CleartextDevice,
				"--force").CombinedOutput()
			if err2 != nil {
				return &mountError{op: "unmount", msg: string(out) + "; force: " + string(out2)}
			}
		}
	}

	// Lock with retry (kernel may need time to release dm device after unmount)
	if info.LoopDevice != "" {
		var lastErr error
		var lastOut []byte
		for i := 0; i < UnmountRetryCount; i++ {
			if i > 0 {
				time.Sleep(UnmountRetryDelay)
			}
			lastOut, lastErr = exec.Command("udisksctl", "lock", "-b", info.LoopDevice).CombinedOutput()
			if lastErr == nil {
				break
			}
		}
		if lastErr != nil {
			return &mountError{op: "lock", msg: string(lastOut)}
		}
	}

	// Remove loop
	if info.LoopDevice != "" {
		if out, err := exec.Command("udisksctl", "loop-delete", "-b", info.LoopDevice).CombinedOutput(); err != nil {
			return &mountError{op: "loop-delete", msg: string(out)}
		}
	}

	return nil
}

// Errors
type mountError struct {
	op  string
	msg string
}

func (e *mountError) Error() string {
	return e.op + ": " + e.msg
}

var errWrongPassword = &mountError{op: "unlock", msg: "wrong password"}

// udisksMountBottleFIDO2 mounts a bottle using a FIDO2-derived secret
func udisksMountBottleFIDO2(bottle string, fido2Secret []byte) (*MountInfo, error) {
	realPath, err := filepath.Abs(bottle)
	if err != nil {
		return nil, err
	}

	info := &MountInfo{BottlePath: realPath}

	// Check if already mounted
	info.LoopDevice = findLoopForFile(realPath)
	if info.LoopDevice != "" {
		info.CleartextDevice = findCleartextForLoop(info.LoopDevice)
		if info.CleartextDevice != "" {
			info.MountPoint = findMountForDevice(info.CleartextDevice)
			if info.MountPoint != "" {
				// Already fully mounted
				return info, nil
			}
		}
	}

	// Setup loop device if needed
	if info.LoopDevice == "" {
		out, err := exec.Command("udisksctl", "loop-setup", "-f", realPath).CombinedOutput()
		if err != nil {
			return nil, &mountError{op: "loop-setup", msg: string(out)}
		}
		// Parse: Mapped file ... as /dev/loop0.
		re := regexp.MustCompile(`/dev/loop\d+`)
		match := re.FindString(string(out))
		if match == "" {
			return nil, &mountError{op: "loop-setup", msg: "could not parse loop device"}
		}
		info.LoopDevice = match
	}

	// Unlock with FIDO2 secret using key file
	if info.CleartextDevice == "" {
		// Write secret to temp file
		keyPath, cleanup, err := writeSecretToTempFile(fido2Secret, "fido2-unlock-")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		unlockCmd := exec.Command("udisksctl", "unlock", "-b", info.LoopDevice, "--key-file", keyPath)
		out, err := unlockCmd.CombinedOutput()
		if err != nil {
			outStr := string(out)
			// Check for wrong key
			if strings.Contains(outStr, "Failed to activate device") ||
				strings.Contains(outStr, "No key available") ||
				strings.Contains(outStr, "passphrase") {
				return nil, &mountError{op: "unlock", msg: "wrong YubiKey - use the key that created this bottle"}
			}
			return nil, &mountError{op: "unlock", msg: outStr}
		}

		// Parse: Unlocked /dev/loop0 as /dev/dm-0.
		re := regexp.MustCompile(`/dev/dm-\d+`)
		match := re.FindString(string(out))
		if match == "" {
			return nil, &mountError{op: "unlock", msg: "could not parse cleartext device"}
		}
		info.CleartextDevice = match
	}

	// Mount if needed
	if info.MountPoint == "" {
		out, err := exec.Command("udisksctl", "mount", "-b", info.CleartextDevice,
			"--options", "nodev,nosuid,noexec").CombinedOutput()
		if err != nil {
			outStr := string(out)
			if strings.Contains(outStr, "Error looking up object for device") && info.LoopDevice != "" {
				// Stale dm device; relock + unlock to refresh udisks state, then retry mount.
				_, _ = exec.Command("udisksctl", "lock", "-b", info.LoopDevice).CombinedOutput()
				keyPath, cleanup, errKey := writeSecretToTempFile(fido2Secret, "fido2-unlock-")
				if errKey != nil {
					return nil, errKey
				}
				defer cleanup()
				unlockCmd := exec.Command("udisksctl", "unlock", "-b", info.LoopDevice, "--key-file", keyPath)
				out2, err2 := unlockCmd.CombinedOutput()
				if err2 != nil {
					return nil, &mountError{op: "unlock", msg: string(out2)}
				}
				re := regexp.MustCompile(`/dev/dm-\d+`)
				match := re.FindString(string(out2))
				if match == "" {
					return nil, &mountError{op: "unlock", msg: "could not parse cleartext device"}
				}
				info.CleartextDevice = match

				out3, err3 := exec.Command("udisksctl", "mount", "-b", info.CleartextDevice,
					"--options", "nodev,nosuid,noexec").CombinedOutput()
				if err3 != nil {
					return nil, &mountError{op: "mount", msg: string(out3)}
				}
				out = out3
			} else {
				return nil, &mountError{op: "mount", msg: outStr}
			}
		}

		// Parse: Mounted /dev/dm-0 at /run/media/user/...
		re := regexp.MustCompile(`at (/\S+)`)
		match := re.FindStringSubmatch(string(out))
		if len(match) < 2 {
			return nil, &mountError{op: "mount", msg: "could not parse mount point"}
		}
		info.MountPoint = strings.TrimSuffix(match[1], ".")
	}

	return info, nil
}
