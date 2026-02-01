// Package main provides bottle-launch, a TUI/CLI application for launching
// Flatpak applications with data stored in encrypted LUKS2 containers.
package main

import "time"

const (
	// ExitSIGINT is the exit code when terminated by SIGINT (128 + signal number per POSIX).
	ExitSIGINT = 130

	// UnmountRetryCount is the number of retry attempts for unmount/lock operations.
	UnmountRetryCount = 3

	// UnmountRetryDelay is the delay between unmount/lock retry attempts.
	UnmountRetryDelay = 500 * time.Millisecond

	// DefaultFIDO2RPID is the relying party ID for FIDO2 credential creation.
	DefaultFIDO2RPID = "bottle-launch"

	// DefaultFIDO2User is the user name for FIDO2 credential creation.
	DefaultFIDO2User = "bottle-user"
)
