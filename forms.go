// Form definitions using the huh library for bottle creation wizards.
package main

import (
	"github.com/charmbracelet/huh"
)

// createBottleForm creates a huh form for creating a new bottle
func createBottleForm() *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Key("name").
				Title("Bottle Name").
				Placeholder("my-bottle").
				Validate(func(s string) error {
					if s == "" {
						return errBottlePathRequired
					}
					return nil
				}),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Key("size").
				Title("Bottle Size").
				Options(
					huh.NewOption("500 MB", "500M"),
					huh.NewOption("1 GB", "1G"),
					huh.NewOption("2 GB", "2G"),
					huh.NewOption("5 GB", "5G"),
					huh.NewOption("10 GB", "10G"),
				).
				Value(new(string)),
		),
		huh.NewGroup(
			huh.NewInput().
				Key("password").
				Title("Encryption Password").
				EchoMode(huh.EchoModePassword).
				Validate(func(s string) error {
					if s == "" {
						return &bottleError{op: "password", msg: "required"}
					}
					return nil
				}),
		),
		huh.NewGroup(
			huh.NewInput().
				Key("confirm").
				Title("Confirm Password").
				EchoMode(huh.EchoModePassword).
				Validate(func(s string) error {
					return nil // Will validate against password after form completion
				}),
		),
	).WithShowHelp(true).WithShowErrors(true)
}

// createBottleFormYubiKey creates a huh form for creating a YubiKey-protected bottle
// This form only asks for name and size - no password (YubiKey provides the key)
func createBottleFormYubiKey() *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Key("name").
				Title("Bottle Name").
				Placeholder("my-secure-bottle").
				Validate(func(s string) error {
					if s == "" {
						return errBottlePathRequired
					}
					return nil
				}),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Key("size").
				Title("Bottle Size").
				Options(
					huh.NewOption("500 MB", "500M"),
					huh.NewOption("1 GB", "1G"),
					huh.NewOption("2 GB", "2G"),
					huh.NewOption("5 GB", "5G"),
					huh.NewOption("10 GB", "10G"),
				).
				Value(new(string)),
		),
	).WithShowHelp(true).WithShowErrors(true)
}
