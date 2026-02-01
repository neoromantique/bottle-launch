// Lipgloss styles for TUI colors and formatting.
package main

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	primaryColor   = lipgloss.Color("212") // Pink
	secondaryColor = lipgloss.Color("86")  // Cyan
	errorColor     = lipgloss.Color("196") // Red
	warningColor   = lipgloss.Color("214") // Orange
	dimColor       = lipgloss.Color("240") // Gray

	// Header/Footer
	headerStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true)

	footerStyle = lipgloss.NewStyle().
			Foreground(dimColor)

	// Titles
	titleStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true).
			MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Bold(true)

	// Items
	itemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	selectedItemStyle = lipgloss.NewStyle().
				Foreground(primaryColor).
				Bold(true)

	cursorStyle = lipgloss.NewStyle().
			Foreground(primaryColor)

	selectedStyle = lipgloss.NewStyle().
			Foreground(secondaryColor)

	// Status
	dimStyle = lipgloss.NewStyle().
			Foreground(dimColor)

	hintStyle = lipgloss.NewStyle().
			Foreground(dimColor).
			Italic(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(errorColor).
			Bold(true)

	warningStyle = lipgloss.NewStyle().
			Foreground(warningColor).
			Bold(true)

	// Spinner
	spinnerStyle = lipgloss.NewStyle().
			Foreground(primaryColor)
)
