package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// providerPickerItem represents one selectable provider in the picker.
type providerPickerItem struct {
	ID         string
	Name       string
	ModelCount int
}

// ProviderPicker is an overlay for selecting an AI provider.
type ProviderPicker struct {
	theme       Theme
	visible     bool
	allItems    []providerPickerItem
	items       []providerPickerItem
	selected    int
	width       int
	height      int
	searchQuery string
	keyGetter   func(string) string
}

// NewProviderPicker creates a new provider picker.
func NewProviderPicker(theme Theme) ProviderPicker {
	return ProviderPicker{
		theme:    theme,
		width:    80,
		height:   24,
		selected: 0,
	}
}

// SetKeyGetter sets the function used to retrieve API keys for displaying configured indicators.
func (pp *ProviderPicker) SetKeyGetter(fn func(string) string) {
	pp.keyGetter = fn
}

// SetSize updates the picker's dimensions.
func (pp *ProviderPicker) SetSize(width, height int) {
	pp.width = width
	pp.height = height
}

// Open populates the picker with all available providers from OpenRouter.
func (pp *ProviderPicker) Open() {
	pp.allItems = nil
	providers := Providers()
	if providers != nil {
		for _, prov := range providers {
			pp.allItems = append(pp.allItems, providerPickerItem{
				ID:         prov.ID,
				Name:       prov.Name,
				ModelCount: len(prov.Models),
			})
		}
	}
	pp.items = pp.allItems
	pp.searchQuery = ""
	pp.selected = 0
	pp.visible = true
}

// Close hides the picker.
func (pp *ProviderPicker) Close() {
	pp.visible = false
}

// Visible returns whether the picker is currently open.
func (pp *ProviderPicker) Visible() bool {
	return pp.visible
}

// Up moves the selection up.
func (pp *ProviderPicker) Up() {
	if pp.selected > 0 {
		pp.selected--
	}
}

// Down moves the selection down.
func (pp *ProviderPicker) Down() {
	if pp.selected < len(pp.items)-1 {
		pp.selected++
	}
}

// Selected returns the currently selected item.
func (pp *ProviderPicker) Selected() (providerPickerItem, bool) {
	if pp.selected < 0 || pp.selected >= len(pp.items) {
		return providerPickerItem{}, false
	}
	return pp.items[pp.selected], true
}

// TypeRune adds a character to the search query and re-filters.
func (pp *ProviderPicker) TypeRune(r rune) {
	pp.searchQuery += string(r)
	pp.filter()
}

// Backspace removes the last character from the search query and re-filters.
func (pp *ProviderPicker) Backspace() {
	if len(pp.searchQuery) > 0 {
		runes := []rune(pp.searchQuery)
		pp.searchQuery = string(runes[:len(runes)-1])
		pp.filter()
	}
}

// ClearSearch clears the search query and shows all providers.
func (pp *ProviderPicker) ClearSearch() {
	pp.searchQuery = ""
	pp.filter()
}

// filter updates the items slice to only include providers matching the search query.
func (pp *ProviderPicker) filter() {
	if pp.searchQuery == "" {
		pp.items = pp.allItems
	} else {
		query := strings.ToLower(pp.searchQuery)
		pp.items = nil
		for _, item := range pp.allItems {
			if strings.Contains(strings.ToLower(item.Name), query) ||
				strings.Contains(strings.ToLower(item.ID), query) {
				pp.items = append(pp.items, item)
			}
		}
	}
	pp.selected = 0
}

// View renders the picker as a modal overlay.
func (pp *ProviderPicker) View() string {
	if !pp.visible {
		return ""
	}

	boxWidth := min(pp.width-8, 60)
	innerWidth := boxWidth - 4

	var b strings.Builder
	b.WriteString(pp.theme.Bold.Render("  Select Provider") + "\n")

	// Show active search query
	if pp.searchQuery != "" {
		b.WriteString(pp.theme.Text.Render("  /"+pp.searchQuery) + "\n")
	}

	b.WriteString(pp.theme.Separator.Render(strings.Repeat("─", innerWidth)) + "\n")

	if len(pp.items) == 0 {
		if Providers() == nil {
			b.WriteString(pp.theme.Dim.Render("  Loading providers...") + "\n")
		} else if pp.searchQuery != "" {
			b.WriteString(pp.theme.Dim.Render("  No providers match your search") + "\n")
		} else {
			b.WriteString(pp.theme.Dim.Render("  No providers available") + "\n")
		}
	} else {
		for i, item := range pp.items {
			prefix := "   "
			nameStyle := pp.theme.Text

			if i == pp.selected {
				prefix = " ▸ "
				nameStyle = pp.theme.Suggestion
			}

			// Build key indicator (e.g. "sk**") if a key is configured
			keyIndicator := ""
			if pp.keyGetter != nil {
				if key := pp.keyGetter(item.ID); key != "" {
					if len(key) >= 2 {
						keyIndicator = key[:2] + "**"
					} else {
						keyIndicator = "**"
					}
				}
			}

			name := fmt.Sprintf("%s%s", prefix, item.Name)
			nameLine := nameStyle.Render(name)

			countStr := fmt.Sprintf("%d models", item.ModelCount)
			extraWidth := len(countStr)
			if keyIndicator != "" {
				extraWidth += len(keyIndicator) + 2
			}

			nameWidth := lipgloss.Width(nameLine)
			maxNameWidth := innerWidth - extraWidth - 2
			if nameWidth > maxNameWidth && maxNameWidth > 3 {
				rawName := fmt.Sprintf("%s%s", prefix, item.Name)
				if len(rawName) > maxNameWidth-3 {
					rawName = rawName[:maxNameWidth-3] + "..."
				}
				nameLine = nameStyle.Render(rawName)
				nameWidth = lipgloss.Width(nameLine)
			}

			// Assemble: name + padding + [key indicator] + padding + count
			remaining := innerWidth - nameWidth
			middlePad := 2
			if keyIndicator != "" {
				middlePad = 1
			}

			line := nameLine + strings.Repeat(" ", middlePad)
			remaining -= middlePad

			if keyIndicator != "" {
				indicatorStyled := pp.theme.Dim.Render(keyIndicator)
				line += indicatorStyled
				remaining -= len(keyIndicator)
			}

			endPad := remaining - len(countStr)
			if endPad < 1 {
				endPad = 1
			}
			line += strings.Repeat(" ", endPad) + pp.theme.Dim.Render(countStr)
			b.WriteString(line + "\n")
		}
	}

	b.WriteString(pp.theme.Separator.Render(strings.Repeat("─", innerWidth)) + "\n")
	b.WriteString(pp.theme.Dim.Render("  ↑/↓ navigate · type to filter · enter select · esc cancel") + "\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#555555")).
		Padding(0, 1).
		Width(boxWidth).
		Render(b.String())

	return box
}
