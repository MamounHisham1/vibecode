package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/vibecode/vibecode/internal/commands"
)

const maxVisibleSuggestions = 8

type suggestion struct {
	name        string
	description string
	aliases     []string
	cmd         *commands.Command
}

type AutocompleteModel struct {
	theme        Theme
	suggestions  []suggestion
	selected     int
	visible      bool
	scrollOffset int
	width        int
}

func NewAutocompleteModel(theme Theme) AutocompleteModel {
	return AutocompleteModel{
		theme: theme,
		width: 80,
	}
}

func (a *AutocompleteModel) SetWidth(w int) {
	if w > 0 {
		a.width = w
	}
}

func (a *AutocompleteModel) Update(input string, registry *commands.Registry) {
	raw := strings.TrimSpace(input)
	if !strings.HasPrefix(raw, "/") || strings.Contains(raw, " ") {
		a.visible = false
		a.suggestions = nil
		a.selected = 0
		a.scrollOffset = 0
		return
	}

	prefix := raw[1:]
	all := registry.All()

	var cmds []*commands.Command
	for _, cmd := range all {
		if strings.HasPrefix(cmd.Name, prefix) {
			cmds = append(cmds, cmd)
		} else {
			for _, alias := range cmd.Aliases {
				if strings.HasPrefix(alias, prefix) {
					cmds = append(cmds, cmd)
					break
				}
			}
		}
	}

	if len(cmds) == 0 {
		a.visible = false
		a.suggestions = nil
		a.selected = 0
		return
	}

	a.suggestions = make([]suggestion, 0, len(cmds))
	for _, cmd := range cmds {
		a.suggestions = append(a.suggestions, suggestion{
			name:        cmd.Name,
			description: cmd.Description,
			aliases:     cmd.Aliases,
			cmd:         cmd,
		})
	}

	sort.Slice(a.suggestions, func(i, j int) bool {
		return a.suggestions[i].name < a.suggestions[j].name
	})

	if a.selected >= len(a.suggestions) {
		a.selected = len(a.suggestions) - 1
	}
	if a.selected < 0 {
		a.selected = 0
	}
	a.visible = true
	a.adjustScroll()
}

func (a *AutocompleteModel) Up() {
	if !a.visible || len(a.suggestions) == 0 {
		return
	}
	if a.selected > 0 {
		a.selected--
	}
	a.adjustScroll()
}

func (a *AutocompleteModel) Down() {
	if !a.visible || len(a.suggestions) == 0 {
		return
	}
	if a.selected < len(a.suggestions)-1 {
		a.selected++
	}
	a.adjustScroll()
}

func (a *AutocompleteModel) SelectedCommand() *commands.Command {
	if !a.visible || a.selected >= len(a.suggestions) {
		return nil
	}
	return a.suggestions[a.selected].cmd
}

func (a *AutocompleteModel) SelectedName() string {
	if !a.visible || a.selected >= len(a.suggestions) {
		return ""
	}
	return a.suggestions[a.selected].name
}

func (a *AutocompleteModel) Dismiss() {
	a.visible = false
	a.selected = 0
	a.scrollOffset = 0
}

func (a *AutocompleteModel) Visible() bool {
	return a.visible && len(a.suggestions) > 0
}

func (a *AutocompleteModel) adjustScroll() {
	if a.selected < a.scrollOffset {
		a.scrollOffset = a.selected
	}
	if a.selected >= a.scrollOffset+maxVisibleSuggestions {
		a.scrollOffset = a.selected - maxVisibleSuggestions + 1
	}
}

func (a AutocompleteModel) View() string {
	if !a.visible || len(a.suggestions) == 0 {
		return ""
	}

	t := a.theme

	end := a.scrollOffset + maxVisibleSuggestions
	if end > len(a.suggestions) {
		end = len(a.suggestions)
	}
	visible := a.suggestions[a.scrollOffset:end]

	var b strings.Builder
	b.WriteString(t.Dim.Render("  Commands:") + "\n")

	nameWidth := 14
	maxWidth := a.width - 4
	if maxWidth < 40 {
		maxWidth = 40
	}

	for i, s := range visible {
		name := "/" + s.name
		if len(name) > nameWidth {
			name = name[:nameWidth]
		}

		desc := s.description
		aliasStr := ""
		if len(s.aliases) > 0 {
			aliasStr = fmt.Sprintf(" (%s)", strings.Join(s.aliases, ","))
		}

		availWidth := maxWidth - nameWidth - len(aliasStr) - 3
		if availWidth < 10 {
			availWidth = 10
		}
		if len(desc) > availWidth {
			desc = desc[:availWidth-1] + "…"
		}

		line := fmt.Sprintf("  %-"+fmt.Sprintf("%d", nameWidth)+"s %s%s", name, desc, aliasStr)

		if i+a.scrollOffset == a.selected {
			if len(line) > maxWidth {
				line = line[:maxWidth]
			}
			line = t.InverseCursor.Render(line)
		} else {
			line = t.Dim.Render(line)
		}

		b.WriteString(line + "\n")
	}

	if len(a.suggestions) > maxVisibleSuggestions {
		remaining := len(a.suggestions) - end
		if remaining > 0 {
			b.WriteString(t.Dim.Render(fmt.Sprintf("  … and %d more", remaining)) + "\n")
		}
	}

	b.WriteString(t.Dim.Render("  tab complete · enter run · esc dismiss"))

	return b.String()
}
