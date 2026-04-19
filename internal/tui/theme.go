package tui

import (
	"github.com/charmbracelet/lipgloss"
)

type Theme struct {
	// Brand
	Brand      lipgloss.Style
	BrandLight lipgloss.Style
	BrandDim   lipgloss.Style
	BrandGlow  lipgloss.Style

	// Text
	Text   lipgloss.Style
	Dim    lipgloss.Style
	Bold   lipgloss.Style
	Subtle lipgloss.Style
	Italic lipgloss.Style

	// Assistant
	AssistantDot  lipgloss.Style
	AssistantIcon lipgloss.Style

	// Tool entries
	ToolIcon     lipgloss.Style
	ToolName     lipgloss.Style
	ToolArgs     lipgloss.Style
	ToolDot      lipgloss.Style
	ToolSuccess  lipgloss.Style
	ToolError    lipgloss.Style
	ToolRunning  lipgloss.Style
	ToolDuration lipgloss.Style
	ToolBorder   lipgloss.Style

	// User
	UserPointer lipgloss.Style
	UserText    lipgloss.Style
	UserBg      lipgloss.Style

	// Status colors
	Success lipgloss.Style
	Error   lipgloss.Style
	Warning lipgloss.Style
	Info    lipgloss.Style

	// Diff colors
	DiffAdd        lipgloss.Style // added line foreground
	DiffRemove     lipgloss.Style // removed line foreground
	DiffAddBg      lipgloss.Style // added line background
	DiffRemoveBg   lipgloss.Style // removed line background
	DiffAddWord    lipgloss.Style // word-level addition highlight
	DiffRemoveWord lipgloss.Style // word-level removal highlight
	DiffLineNum    lipgloss.Style // line numbers
	DiffMarker     lipgloss.Style // +/- prefix

	// Prompt / Input
	PromptChar     lipgloss.Style
	PromptBorder   lipgloss.Style
	PromptActive   lipgloss.Style
	InverseCursor  lipgloss.Style
	InputHint      lipgloss.Style
	InputLabel     lipgloss.Style
	InputBorderDim lipgloss.Style
	Separator      lipgloss.Style

	// Suggestions
	Suggestion     lipgloss.Style
	SuggestionKey  lipgloss.Style
	SuggestionDesc lipgloss.Style

	// Status bar
	StatusBar      lipgloss.Style
	StatusBarBrand lipgloss.Style
	StatusBarInfo  lipgloss.Style
	StatusBarDim   lipgloss.Style

	// Welcome screen
	WelcomeTitle    lipgloss.Style
	WelcomeSubtitle lipgloss.Style
	WelcomeBorder   lipgloss.Style
	WelcomeTip      lipgloss.Style
	WelcomeKey      lipgloss.Style
	WelcomeDesc     lipgloss.Style
}

func DefaultTheme() Theme {
	brand := lipgloss.Color("#D77757")
	brandLight := lipgloss.Color("#EB9F7F")
	brandDim := lipgloss.Color("#9E5A3C")
	brandGlow := lipgloss.Color("#FFB088")

	return Theme{
		// Brand
		Brand:      lipgloss.NewStyle().Foreground(brand).Bold(true),
		BrandLight: lipgloss.NewStyle().Foreground(brandLight),
		BrandDim:   lipgloss.NewStyle().Foreground(brandDim),
		BrandGlow:  lipgloss.NewStyle().Foreground(brandGlow),

		// Text
		Text:   lipgloss.NewStyle().Foreground(lipgloss.Color("#E0E0E0")),
		Dim:    lipgloss.NewStyle().Foreground(lipgloss.Color("#6B6B6B")),
		Bold:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")),
		Subtle: lipgloss.NewStyle().Foreground(lipgloss.Color("#404040")),
		Italic: lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("#A0A0A0")),

		// Assistant
		AssistantDot:  lipgloss.NewStyle().Foreground(brand),
		AssistantIcon: lipgloss.NewStyle().Foreground(brandLight),

		// Tool entries
		ToolIcon:     lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")),
		ToolName:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E0E0E0")),
		ToolArgs:     lipgloss.NewStyle().Foreground(lipgloss.Color("#808080")),
		ToolDot:      lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")),
		ToolSuccess:  lipgloss.NewStyle().Foreground(lipgloss.Color("#4EBA65")),
		ToolError:    lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B80")),
		ToolRunning:  lipgloss.NewStyle().Foreground(lipgloss.Color("#D4A843")),
		ToolDuration: lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")),
		ToolBorder:   lipgloss.NewStyle().Foreground(lipgloss.Color("#3A3A3A")),

		// User
		UserPointer: lipgloss.NewStyle().Foreground(brand).Bold(true),
		UserText:    lipgloss.NewStyle().Foreground(lipgloss.Color("#E8E8E8")),
		UserBg:      lipgloss.NewStyle().Background(lipgloss.Color("#2A2A2A")),

		// Status colors
		Success: lipgloss.NewStyle().Foreground(lipgloss.Color("#4EBA65")),
		Error:   lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B80")),
		Warning: lipgloss.NewStyle().Foreground(lipgloss.Color("#FFC107")),
		Info:    lipgloss.NewStyle().Foreground(lipgloss.Color("#6CB4EE")),

		// Diff colors (from Claude Code dark theme palette)
		DiffAdd:        lipgloss.NewStyle().Foreground(lipgloss.Color("#4EBA65")),
		DiffRemove:     lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B80")),
		DiffAddBg:      lipgloss.NewStyle().Foreground(lipgloss.Color("#56D364")).Background(lipgloss.Color("#1A2E1A")),
		DiffRemoveBg:   lipgloss.NewStyle().Foreground(lipgloss.Color("#FF7B72")).Background(lipgloss.Color("#2E1A1A")),
		DiffAddWord:    lipgloss.NewStyle().Foreground(lipgloss.Color("#38A660")),
		DiffRemoveWord: lipgloss.NewStyle().Foreground(lipgloss.Color("#B3596B")),
		DiffLineNum:    lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")),
		DiffMarker:     lipgloss.NewStyle().Bold(true),

		// Prompt / Input
		PromptChar:     lipgloss.NewStyle().Foreground(brand).Bold(true),
		PromptBorder:   lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")),
		PromptActive:   lipgloss.NewStyle().Foreground(brand),
		InverseCursor:  lipgloss.NewStyle().Background(lipgloss.Color("#FFFFFF")).Foreground(lipgloss.Color("#000000")),
		InputHint:      lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")),
		InputLabel:     lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")),
		InputBorderDim: lipgloss.NewStyle().Foreground(lipgloss.Color("#333333")),
		Separator:      lipgloss.NewStyle().Foreground(lipgloss.Color("#2E2E2E")),

		// Suggestions
		Suggestion:     lipgloss.NewStyle().Foreground(lipgloss.Color("#B1B9F9")),
		SuggestionKey:  lipgloss.NewStyle().Foreground(brandLight),
		SuggestionDesc: lipgloss.NewStyle().Foreground(lipgloss.Color("#6B6B6B")),

		// Status bar
		StatusBar:      lipgloss.NewStyle().Background(lipgloss.Color("#1A1A1A")).Foreground(lipgloss.Color("#888888")),
		StatusBarBrand: lipgloss.NewStyle().Background(lipgloss.Color("#1A1A1A")).Foreground(brand).Bold(true),
		StatusBarInfo:  lipgloss.NewStyle().Background(lipgloss.Color("#1A1A1A")).Foreground(lipgloss.Color("#666666")),
		StatusBarDim:   lipgloss.NewStyle().Background(lipgloss.Color("#1A1A1A")).Foreground(lipgloss.Color("#444444")),

		// Welcome screen
		WelcomeTitle:    lipgloss.NewStyle().Foreground(brand).Bold(true),
		WelcomeSubtitle: lipgloss.NewStyle().Foreground(lipgloss.Color("#808080")).Italic(true),
		WelcomeBorder:   lipgloss.NewStyle().Foreground(brandDim),
		WelcomeTip:      lipgloss.NewStyle().Foreground(lipgloss.Color("#5A5A5A")),
		WelcomeKey:      lipgloss.NewStyle().Foreground(brandLight).Bold(true),
		WelcomeDesc:     lipgloss.NewStyle().Foreground(lipgloss.Color("#6B6B6B")),
	}
}
