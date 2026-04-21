package tui

import (
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
)

var (
	mdRenderer      *glamour.TermRenderer
	mdRendererWidth int
	mdRendererMu    sync.Mutex
)

// RenderMarkdown renders markdown text to styled terminal output.
// Width controls the maximum line length; set to 0 for a default of 100.
func RenderMarkdown(text string, width int) string {
	if width < 20 {
		width = 100
	}

	mdRendererMu.Lock()
	defer mdRendererMu.Unlock()

	if mdRenderer == nil || mdRendererWidth != width {
		r, err := glamour.NewTermRenderer(
			glamour.WithEnvironmentConfig(),
			glamour.WithWordWrap(width),
		)
		if err != nil {
			return text
		}
		mdRenderer = r
		mdRendererWidth = width
	}

	out, err := mdRenderer.Render(text)
	if err != nil {
		return text
	}

	return strings.TrimRight(out, "\n")
}
