package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// modelPickerItem represents one selectable model in the picker.
type modelPickerItem struct {
	ProviderID   string
	ProviderName string
	ModelID      string
	ModelName    string
	Description  string
}

// ModelPicker is an overlay for selecting a new LLM model.
type ModelPicker struct {
	theme       Theme
	visible     bool
	allItems    []modelPickerItem // full unfiltered list
	items       []modelPickerItem // filtered list
	selected    int
	width       int
	height      int
	searchQuery string
}

// NewModelPicker creates a new model picker.
func NewModelPicker(theme Theme) ModelPicker {
	return ModelPicker{
		theme:    theme,
		width:    80,
		height:   24,
		selected: 0,
	}
}

// SetSize updates the picker's dimensions.
func (mp *ModelPicker) SetSize(width, height int) {
	mp.width = width
	mp.height = height
}

// Open populates the picker with available models and makes it visible.
// currentProvider and currentModel are used to pre-select the active model.
func (mp *ModelPicker) Open(currentProvider, currentModel string) {
	mp.allItems = nil
	providers := Providers()
	if providers == nil {
		// Cache not populated yet — show empty picker
		mp.items = nil
		mp.searchQuery = ""
		mp.selected = 0
		mp.visible = true
		return
	}
	for _, prov := range providers {
		for _, m := range prov.Models {
			mp.allItems = append(mp.allItems, modelPickerItem{
				ProviderID:   prov.ID,
				ProviderName: prov.Name,
				ModelID:      m.ID,
				ModelName:    m.Name,
				Description:  m.Description,
			})
		}
	}
	mp.items = mp.allItems
	mp.searchQuery = ""

	// Pre-select the current model
	mp.selected = 0
	for i, item := range mp.items {
		if item.ProviderID == currentProvider && item.ModelID == currentModel {
			mp.selected = i
			break
		}
	}

	mp.visible = true
}

// Close hides the picker.
func (mp *ModelPicker) Close() {
	mp.visible = false
}

// Visible returns whether the picker is currently open.
func (mp *ModelPicker) Visible() bool {
	return mp.visible
}

// Up moves the selection up.
func (mp *ModelPicker) Up() {
	if mp.selected > 0 {
		mp.selected--
	}
}

// Down moves the selection down.
func (mp *ModelPicker) Down() {
	if mp.selected < len(mp.items)-1 {
		mp.selected++
	}
}

// Selected returns the currently selected item.
func (mp *ModelPicker) Selected() (modelPickerItem, bool) {
	if mp.selected < 0 || mp.selected >= len(mp.items) {
		return modelPickerItem{}, false
	}
	return mp.items[mp.selected], true
}

// TypeRune adds a character to the search query and re-filters.
func (mp *ModelPicker) TypeRune(r rune) {
	mp.searchQuery += string(r)
	mp.filter()
}

// Backspace removes the last character from the search query and re-filters.
func (mp *ModelPicker) Backspace() {
	if len(mp.searchQuery) > 0 {
		runes := []rune(mp.searchQuery)
		mp.searchQuery = string(runes[:len(runes)-1])
		mp.filter()
	}
}

// ClearSearch clears the search query and shows all models.
func (mp *ModelPicker) ClearSearch() {
	mp.searchQuery = ""
	mp.filter()
}

// filter updates the items slice to only include models matching the search query.
func (mp *ModelPicker) filter() {
	if mp.searchQuery == "" {
		mp.items = mp.allItems
	} else {
		query := strings.ToLower(mp.searchQuery)
		mp.items = nil
		for _, item := range mp.allItems {
			if strings.Contains(strings.ToLower(item.ModelName), query) ||
				strings.Contains(strings.ToLower(item.ModelID), query) ||
				strings.Contains(strings.ToLower(item.ProviderName), query) ||
				strings.Contains(strings.ToLower(item.Description), query) {
				mp.items = append(mp.items, item)
			}
		}
	}
	mp.selected = 0
}

// View renders the picker as a modal overlay.
func (mp *ModelPicker) View() string {
	if !mp.visible {
		return ""
	}

	boxWidth := min(mp.width-8, 80)
	innerWidth := boxWidth - 4 // padding inside border

	var b strings.Builder
	b.WriteString(mp.theme.Bold.Render("  Select Model") + "\n")

	// Show active search query
	if mp.searchQuery != "" {
		b.WriteString(mp.theme.Text.Render("  /"+mp.searchQuery) + "\n")
	}

	b.WriteString(mp.theme.Separator.Render(strings.Repeat("─", innerWidth)) + "\n")

	if len(mp.items) == 0 {
		if mp.searchQuery == "" {
			b.WriteString(mp.theme.Dim.Render("  Loading providers...") + "\n")
		} else {
			b.WriteString(mp.theme.Dim.Render("  No models match your search") + "\n")
		}
	} else {
		for i, item := range mp.items {
			prefix := "   "
			nameStyle := mp.theme.Text
			descStyle := mp.theme.Dim

			if i == mp.selected {
				prefix = " ▸ "
				nameStyle = mp.theme.Suggestion
			}

			name := fmt.Sprintf("%s%s", prefix, item.ModelName)
			nameLine := nameStyle.Render(name)

			// Pad or truncate name to align descriptions
			nameWidth := lipgloss.Width(nameLine)
			maxNameWidth := innerWidth / 2
			if nameWidth > maxNameWidth {
				// Truncate the raw name, not the styled one
				rawName := fmt.Sprintf("%s%s", prefix, item.ModelName)
				if len(rawName) > maxNameWidth-3 {
					rawName = rawName[:maxNameWidth-3] + "..."
				}
				nameLine = nameStyle.Render(rawName)
				nameWidth = lipgloss.Width(nameLine)
			}

			descPad := maxNameWidth - nameWidth
			if descPad < 1 {
				descPad = 1
			}

			desc := item.Description
			maxDescWidth := innerWidth - maxNameWidth - 2
			if len(desc) > maxDescWidth && maxDescWidth > 3 {
				desc = desc[:maxDescWidth-3] + "..."
			}

			line := nameLine + strings.Repeat(" ", descPad) + descStyle.Render(desc)
			b.WriteString(line + "\n")
		}
	}

	b.WriteString(mp.theme.Separator.Render(strings.Repeat("─", innerWidth)) + "\n")
	b.WriteString(mp.theme.Dim.Render("  ↑/↓ navigate · type to filter · enter select · esc cancel") + "\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#555555")).
		Padding(0, 1).
		Width(boxWidth).
		Render(b.String())

	return box
}
