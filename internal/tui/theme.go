package tui

import "github.com/charmbracelet/lipgloss"

// Theme uses Claude Code's dark mode palette.
type Theme struct {
	// Brand identity
	Brand lipgloss.Style // Orange for logo only

	// Tool display — mimics Claude Code's tool rows
	ToolName    lipgloss.Style // Bold white for tool names
	ToolSuccess lipgloss.Style // Green ● for completed
	ToolError   lipgloss.Style // Red ● for errors

	// Text hierarchy
	Dim    lipgloss.Style
	Bold   lipgloss.Style
	Subtle lipgloss.Style

	// Semantic
	Success lipgloss.Style
	Error   lipgloss.Style
	Warning lipgloss.Style

	// UI chrome
	Prompt    lipgloss.Style
	Separator lipgloss.Style
}

func DefaultTheme() Theme {
	return Theme{
		Brand: lipgloss.NewStyle().Foreground(lipgloss.Color("#D77757")),

		// Tools — bold white names, colored status
		ToolName:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")),
		ToolSuccess: lipgloss.NewStyle().Foreground(lipgloss.Color("#4EBA65")),
		ToolError:   lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B80")),

		// Text
		Dim:    lipgloss.NewStyle().Foreground(lipgloss.Color("#999999")),
		Bold:   lipgloss.NewStyle().Bold(true),
		Subtle: lipgloss.NewStyle().Foreground(lipgloss.Color("#505050")),

		// Semantic
		Success: lipgloss.NewStyle().Foreground(lipgloss.Color("#4EBA65")),
		Error:   lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B80")),
		Warning: lipgloss.NewStyle().Foreground(lipgloss.Color("#FFC107")),

		// Chrome
		Prompt:    lipgloss.NewStyle().Foreground(lipgloss.Color("#999999")),
		Separator: lipgloss.NewStyle().Foreground(lipgloss.Color("#505050")),
	}
}
