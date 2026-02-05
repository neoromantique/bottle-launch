// View rendering: all TUI screens and list item delegates.
package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m model) renderHeader() string {
	return headerStyle.Render("BOTTLE LAUNCHER")
}

func (m model) renderFooter() string {
	return footerStyle.Render(m.help.View(m.keys))
}

func (m model) renderLoading() string {
	content := lipgloss.JoinVertical(lipgloss.Left,
		m.renderHeader(),
		"",
		m.spinner.View()+" "+m.loadingMsg,
		"",
		m.renderFooter(),
	)
	return content
}

func (m model) renderBottleList() string {
	var sb strings.Builder

	sb.WriteString(m.renderHeader())
	sb.WriteString("\n\n")

	if len(m.bottles) == 0 {
		sb.WriteString(dimStyle.Render("No bottles found. Press 'n' to create one."))
	} else {
		sb.WriteString(m.bottleList.View())
	}

	sb.WriteString("\n\n")
	sb.WriteString(hintStyle.Render("[n] New bottle (password)  [y] New bottle (YubiKey)"))
	sb.WriteString("\n\n")
	sb.WriteString(m.renderFooter())

	return sb.String()
}

func (m model) renderBottleActions() string {
	var sb strings.Builder

	sb.WriteString(m.renderHeader())
	sb.WriteString("\n\n")

	// Show bottle name with auth type indicator
	bottleTitle := "Bottle: " + bottleName(m.selectedBottle)
	isFIDO2, _ := IsFIDO2Bottle(m.permissions)
	if isFIDO2 {
		bottleTitle += " (YubiKey)"
	}
	sb.WriteString(subtitleStyle.Render(bottleTitle))
	sb.WriteString("\n\n")

	options := []string{
		"[l] Launch app",
		"[p] Edit permissions",
		"[d] Delete bottle",
	}

	for i, opt := range options {
		if i == m.cursor {
			sb.WriteString(cursorStyle.Render("> ") + selectedItemStyle.Render(opt) + "\n")
		} else {
			sb.WriteString("  " + opt + "\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("Press Esc to go back"))
	sb.WriteString("\n\n")
	sb.WriteString(m.renderFooter())

	return sb.String()
}

func (m model) renderPermissions() string {
	var sb strings.Builder

	sb.WriteString(m.renderHeader())
	sb.WriteString("\n\n")
	sb.WriteString(subtitleStyle.Render("Permissions"))
	sb.WriteString("\n\n")

	for i, def := range permissionDefs {
		var checkbox string
		enabled := m.permissions.IsEnabled(i)
		if enabled {
			checkbox = selectedStyle.Render("[x]")
		} else {
			checkbox = dimStyle.Render("[ ]")
		}

		line := fmt.Sprintf("%s [%s] %s", checkbox, def.Key, def.Label)

		if i == m.cursor {
			line = cursorStyle.Render("> ") + line
		} else {
			line = "  " + line
		}

		sb.WriteString(line + "\n")
	}

	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("Space to toggle, or press shortcut key (n/a/g/w/x/c/p)"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("Enter/Esc to save and return"))
	sb.WriteString("\n\n")
	sb.WriteString(m.renderFooter())

	return sb.String()
}

func (m model) renderAppSelect() string {
	var sb strings.Builder

	sb.WriteString(m.renderHeader())
	sb.WriteString("\n\n")

	if len(m.apps) == 0 {
		sb.WriteString(errorStyle.Render("No Flatpak apps installed!"))
	} else {
		sb.WriteString(m.appList.View())
	}

	sb.WriteString("\n")
	sb.WriteString(m.renderFooter())

	return sb.String()
}

func (m model) renderLaunchConfirm() string {
	var sb strings.Builder

	sb.WriteString(m.renderHeader())
	sb.WriteString("\n\n")
	sb.WriteString(subtitleStyle.Render("Ready to launch"))
	sb.WriteString("\n\n")

	sb.WriteString("  App:    " + m.selectedApp.Name + "\n")
	sb.WriteString("  ID:     " + dimStyle.Render(m.selectedApp.ID) + "\n")
	sb.WriteString("  Bottle: " + bottleName(m.selectedBottle) + "\n")
	sb.WriteString("\n")

	sb.WriteString("  Permissions: " + dimStyle.Render(m.permissions.Summary()) + "\n")
	sb.WriteString("\n")

	options := []string{
		"[l] Launch now",
		"[p] Edit permissions first",
	}

	for _, opt := range options {
		sb.WriteString("  " + opt + "\n")
	}

	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("Esc to go back"))
	sb.WriteString("\n\n")
	sb.WriteString(m.renderFooter())

	return sb.String()
}

func (m model) renderPasswordInput() string {
	var sb strings.Builder

	sb.WriteString(m.renderHeader())
	sb.WriteString("\n\n")
	sb.WriteString(subtitleStyle.Render("Enter bottle password"))
	sb.WriteString("\n\n")

	if m.errMsg != "" && m.state == viewPasswordInput {
		sb.WriteString(errorStyle.Render(m.errMsg))
		sb.WriteString("\n\n")
	}

	sb.WriteString("  " + m.passwordInput.View())
	sb.WriteString("\n\n")
	sb.WriteString(dimStyle.Render("Enter to unlock, Esc to cancel"))
	sb.WriteString("\n\n")
	sb.WriteString(m.renderFooter())

	return sb.String()
}

func (m model) renderCreateBottle() string {
	var sb strings.Builder

	sb.WriteString(m.renderHeader())
	sb.WriteString("\n\n")
	sb.WriteString(subtitleStyle.Render("Create New Bottle"))
	sb.WriteString("\n\n")

	if m.createForm != nil {
		sb.WriteString(m.createForm.View())
	}

	sb.WriteString("\n")
	sb.WriteString(m.renderFooter())

	return sb.String()
}

func (m model) renderDeleteConfirm() string {
	var sb strings.Builder

	sb.WriteString(m.renderHeader())
	sb.WriteString("\n\n")
	sb.WriteString(warningStyle.Render("Delete bottle?"))
	sb.WriteString("\n\n")

	sb.WriteString("  " + bottleName(m.selectedBottle) + "\n\n")
	sb.WriteString(errorStyle.Render("  This cannot be undone!"))
	sb.WriteString("\n\n")

	sb.WriteString("  [y] Yes, delete\n")
	sb.WriteString("  [n] No, cancel\n")

	sb.WriteString("\n")
	sb.WriteString(m.renderFooter())

	return sb.String()
}

func (m model) renderRunning() string {
	var sb strings.Builder

	sb.WriteString(m.renderHeader())
	sb.WriteString("\n\n")
	sb.WriteString(m.spinner.View() + " Running " + m.selectedApp.Name + "...")
	sb.WriteString("\n\n")
	sb.WriteString(dimStyle.Render("The application is running. Close it to return here."))
	sb.WriteString("\n\n")
	sb.WriteString(m.renderFooter())

	return sb.String()
}

func (m model) renderError() string {
	var sb strings.Builder

	sb.WriteString(m.renderHeader())
	sb.WriteString("\n\n")
	sb.WriteString(errorStyle.Render("Error"))
	sb.WriteString("\n\n")

	sb.WriteString("  " + m.errMsg)
	sb.WriteString("\n\n")
	sb.WriteString(dimStyle.Render("Press Enter or Esc to continue"))
	sb.WriteString("\n\n")
	sb.WriteString(m.renderFooter())

	return sb.String()
}

// List item types for bubbles/list

type bottleItem struct {
	path      string
	name      string
	isYubiKey bool
}

func (i bottleItem) Title() string {
	if i.isYubiKey {
		return i.name + " (YubiKey)"
	}
	return i.name
}
func (i bottleItem) Description() string { return i.path }
func (i bottleItem) FilterValue() string { return i.name }

type appItem struct {
	app FlatpakApp
}

func (i appItem) Title() string       { return i.app.Name }
func (i appItem) Description() string { return i.app.ID }
func (i appItem) FilterValue() string { return i.app.Name + " " + i.app.ID }

// Custom delegates for list items

type bottleItemDelegate struct{}

func (d bottleItemDelegate) Height() int                             { return 1 }
func (d bottleItemDelegate) Spacing() int                            { return 0 }
func (d bottleItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d bottleItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i, ok := item.(bottleItem)
	if !ok {
		return
	}

	str := i.name
	if index == m.Index() {
		str = cursorStyle.Render("> ") + selectedItemStyle.Render(str)
	} else {
		str = "  " + itemStyle.Render(str)
	}

	fmt.Fprint(w, str)
}

type appItemDelegate struct{}

func (d appItemDelegate) Height() int                             { return 2 }
func (d appItemDelegate) Spacing() int                            { return 0 }
func (d appItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d appItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i, ok := item.(appItem)
	if !ok {
		return
	}

	var str string
	if index == m.Index() {
		str = cursorStyle.Render("> ") + selectedItemStyle.Render(i.app.Name) + "\n    " + dimStyle.Render(i.app.ID)
	} else {
		str = "  " + itemStyle.Render(i.app.Name) + "\n    " + dimStyle.Render(i.app.ID)
	}

	fmt.Fprint(w, str)
}

// FIDO2 views

func (m model) renderCreateBottleYubiKey() string {
	var sb strings.Builder

	sb.WriteString(m.renderHeader())
	sb.WriteString("\n\n")
	sb.WriteString(subtitleStyle.Render("Create YubiKey Bottle"))
	sb.WriteString("\n\n")

	switch m.fido2Step {
	case -1:
		// Error step
		sb.WriteString(errorStyle.Render("Error: " + m.fido2Error))
		sb.WriteString("\n\n")
		sb.WriteString(dimStyle.Render("Press Esc to go back"))

	case 0:
		// Form step (name and size)
		if m.createForm != nil {
			sb.WriteString(m.createForm.View())
		}

	case 1:
		// Device selection step
		if len(m.fido2Devices) == 0 {
			sb.WriteString(warningStyle.Render("No FIDO2 device found."))
			sb.WriteString("\n\n")
			sb.WriteString("  Insert YubiKey and press Enter.\n")
			sb.WriteString("\n")
			sb.WriteString(dimStyle.Render("[r] Retry  [Esc] Cancel"))
		} else if len(m.fido2Devices) == 1 {
			sb.WriteString("  Found: ")
			sb.WriteString(selectedStyle.Render(m.fido2Devices[0].Path))
			if m.fido2Devices[0].Description != "" {
				sb.WriteString("\n  " + dimStyle.Render(m.fido2Devices[0].Description))
			}
			sb.WriteString("\n\n")
			sb.WriteString(dimStyle.Render("[Enter] Continue  [Esc] Cancel"))
		} else {
			sb.WriteString("  Select YubiKey:\n\n")
			for i, dev := range m.fido2Devices {
				if i == m.fido2DeviceSel {
					sb.WriteString(cursorStyle.Render("> ") + selectedItemStyle.Render(dev.Path))
				} else {
					sb.WriteString("  " + itemStyle.Render(dev.Path))
				}
				if dev.Description != "" {
					sb.WriteString(" " + dimStyle.Render(dev.Description))
				}
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
			sb.WriteString(dimStyle.Render("[Enter] Select  [Esc] Cancel"))
		}

		if m.fido2Error != "" {
			sb.WriteString("\n\n")
			sb.WriteString(errorStyle.Render("Error: " + m.fido2Error))
		}

	case 2:
		// Credential created, prompt for secret
		sb.WriteString("  Credential created.\n\n")
		sb.WriteString("  Press Enter to generate encryption key.\n")
		sb.WriteString("  You will need to touch YubiKey again.\n")
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render("[Enter] Continue  [Esc] Cancel"))

		if m.fido2Error != "" {
			sb.WriteString("\n\n")
			sb.WriteString(errorStyle.Render("Error: " + m.fido2Error))
		}

	case 3:
		// Secret ready, prompt for bottle creation
		sb.WriteString("  Encryption key generated.\n\n")
		sb.WriteString("  Press Enter to create the encrypted bottle.\n")
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render("[Enter] Create bottle  [Esc] Cancel"))

		if m.fido2Error != "" {
			sb.WriteString("\n\n")
			sb.WriteString(errorStyle.Render("Error: " + m.fido2Error))
		}

	case 4:
		// Success
		sb.WriteString(selectedStyle.Render("Bottle created successfully!"))
		sb.WriteString("\n\n")
		sb.WriteString(warningStyle.Render("WARNING: "))
		sb.WriteString("This bottle can ONLY be unlocked with this specific YubiKey.\n")
		sb.WriteString("         If you lose this YubiKey, the data is PERMANENTLY UNRECOVERABLE.\n")
		sb.WriteString("\n")
		sb.WriteString("  Back up your config file:\n")
		sb.WriteString("  " + dimStyle.Render("~/.config/bottle-launch/<hash>.conf"))
		sb.WriteString("\n\n")
		sb.WriteString(dimStyle.Render("[Enter] Done"))
	}

	sb.WriteString("\n\n")
	sb.WriteString(m.renderFooter())

	return sb.String()
}

func (m model) renderFIDO2Unlock() string {
	var sb strings.Builder

	sb.WriteString(m.renderHeader())
	sb.WriteString("\n\n")
	sb.WriteString(subtitleStyle.Render("Unlock with YubiKey"))
	sb.WriteString("\n\n")

	if len(m.fido2Devices) == 0 {
		sb.WriteString(warningStyle.Render("YubiKey not found."))
		sb.WriteString("\n\n")
		sb.WriteString("  Insert your YubiKey and try again.\n")
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render("[r] Retry  [Esc] Cancel"))
	} else if len(m.fido2Devices) == 1 {
		sb.WriteString("  Found: ")
		sb.WriteString(selectedStyle.Render(m.fido2Devices[0].Path))
		sb.WriteString("\n\n")
		sb.WriteString("  Press Enter to unlock (requires touch).\n")
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render("[Enter] Unlock  [Esc] Cancel"))
	} else {
		sb.WriteString("  Select YubiKey:\n\n")
		for i, dev := range m.fido2Devices {
			if i == m.fido2DeviceSel {
				sb.WriteString(cursorStyle.Render("> ") + selectedItemStyle.Render(dev.Path))
			} else {
				sb.WriteString("  " + itemStyle.Render(dev.Path))
			}
			if dev.Description != "" {
				sb.WriteString(" " + dimStyle.Render(dev.Description))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render("[Enter] Unlock  [Esc] Cancel"))
	}

	if m.fido2Error != "" {
		sb.WriteString("\n\n")
		sb.WriteString(errorStyle.Render("Error: " + m.fido2Error))
		sb.WriteString("\n\n")
		sb.WriteString(dimStyle.Render("[r] Retry  [Esc] Cancel"))
	}

	sb.WriteString("\n\n")
	sb.WriteString(m.renderFooter())

	return sb.String()
}
