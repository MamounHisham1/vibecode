package tui

import "github.com/charmbracelet/lipgloss"

// Theme defines the color palette for the TUI.
type Theme struct {
	Accent    lipgloss.Style
	Dim       lipgloss.Style
	Bold      lipgloss.Style
	Success   lipgloss.Style
	Error     lipgloss.Style
	Tool      lipgloss.Style
	Separator lipgloss.Style
	Prompt    lipgloss.Style
}

func DefaultTheme() Theme {
	return Theme{
		Accent:    lipgloss.NewStyle().Foreground(lipgloss.Color("12")),  // bright cyan
		Dim:       lipgloss.NewStyle().Foreground(lipgloss.Color("242")), // gray
		Bold:      lipgloss.NewStyle().Bold(true),
		Success:   lipgloss.NewStyle().Foreground(lipgloss.Color("2")),  // green
		Error:     lipgloss.NewStyle().Foreground(lipgloss.Color("1")),  // red
		Tool:      lipgloss.NewStyle().Foreground(lipgloss.Color("12")), // cyan
		Separator: lipgloss.NewStyle().Foreground(lipgloss.Color("236")), // dark gray
		Prompt:    lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true),
	}
}
