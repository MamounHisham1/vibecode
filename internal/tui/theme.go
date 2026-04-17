package tui

import "github.com/charmbracelet/lipgloss"

type Theme struct {
	UserLabel  lipgloss.Style
	ToolName   lipgloss.Style
	ToolBorder lipgloss.Style
	Dim        lipgloss.Style
	Bold       lipgloss.Style
	Success    lipgloss.Style
	Error      lipgloss.Style
	Warning    lipgloss.Style
	Separator  lipgloss.Style
	Prompt     lipgloss.Style
	Assistant  lipgloss.Style
	Path       lipgloss.Style
}

func DefaultTheme() Theme {
	return Theme{
		UserLabel:  lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true),
		ToolName:   lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true),
		ToolBorder: lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		Dim:        lipgloss.NewStyle().Foreground(lipgloss.Color("243")),
		Bold:       lipgloss.NewStyle().Bold(true),
		Success:    lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
		Error:      lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
		Warning:    lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		Separator:  lipgloss.NewStyle().Foreground(lipgloss.Color("238")),
		Prompt:     lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true),
		Assistant:  lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		Path:       lipgloss.NewStyle().Foreground(lipgloss.Color("110")),
	}
}
