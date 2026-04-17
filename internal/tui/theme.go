package tui

import "github.com/charmbracelet/lipgloss"

// Colors matching Claude Code's dark theme
type Theme struct {
	// Brand
	Brand lipgloss.Style // Claude orange: rgb(215,119,87)

	// Tool display
	ToolName    lipgloss.Style
	ToolSuccess lipgloss.Style
	ToolError   lipgloss.Style
	ToolActive  lipgloss.Style

	// Text hierarchy
	Text    lipgloss.Style
	Dim     lipgloss.Style
	Bold    lipgloss.Style
	Subtle  lipgloss.Style

	// Semantic
	Success lipgloss.Style
	Error   lipgloss.Style
	Warning lipgloss.Style

	// UI chrome
	Prompt    lipgloss.Style
	Separator lipgloss.Style

	// Diff
	DiffAdded   lipgloss.Style
	DiffRemoved lipgloss.Style
}

func DefaultTheme() Theme {
	return Theme{
		// Brand — Claude orange
		Brand: lipgloss.NewStyle().Foreground(lipgloss.Color("#D77757")),

		// Tool name — bold white, like Claude Code's userFacingToolName
		ToolName: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")),
		// Tool status indicators
		ToolSuccess: lipgloss.NewStyle().Foreground(lipgloss.Color("#4EBA65")), // rgb(78,186,101)
		ToolError:   lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B80")), // rgb(255,107,128)
		ToolActive:  lipgloss.NewStyle().Foreground(lipgloss.Color("#D77757")), // pulsing orange

		// Text hierarchy
		Text:   lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")),
		Dim:    lipgloss.NewStyle().Foreground(lipgloss.Color("#999999")),
		Bold:   lipgloss.NewStyle().Bold(true),
		Subtle: lipgloss.NewStyle().Foreground(lipgloss.Color("#505050")),

		// Semantic
		Success: lipgloss.NewStyle().Foreground(lipgloss.Color("#4EBA65")),
		Error:   lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B80")),
		Warning: lipgloss.NewStyle().Foreground(lipgloss.Color("#FFC107")),

		// UI chrome
		Prompt:    lipgloss.NewStyle().Foreground(lipgloss.Color("#D77757")).Bold(true),
		Separator: lipgloss.NewStyle().Foreground(lipgloss.Color("#505050")),

		// Diff
		DiffAdded:   lipgloss.NewStyle().Foreground(lipgloss.Color("#38A660")),
		DiffRemoved: lipgloss.NewStyle().Foreground(lipgloss.Color("#B3596B")),
	}
}
