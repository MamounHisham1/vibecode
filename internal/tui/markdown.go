package tui

import (
	"strings"

	"github.com/charmbracelet/glamour"
)

var mdRenderer *glamour.TermRenderer

func init() {
	r, err := glamour.NewTermRenderer(
		glamour.WithEnvironmentConfig(),
		glamour.WithWordWrap(100),
	)
	if err != nil {
		// Fallback: no formatting
		mdRenderer, _ = glamour.NewTermRenderer()
		return
	}
	mdRenderer = r
}

// RenderMarkdown renders markdown text to styled terminal output.
func RenderMarkdown(text string) string {
	if mdRenderer == nil {
		return text
	}

	out, err := mdRenderer.Render(text)
	if err != nil {
		return text
	}

	return strings.TrimRight(out, "\n")
}
