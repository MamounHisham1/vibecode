package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

type Model struct {
	theme     Theme
	textarea  textarea.Model
	spinner   spinner.Model
	output    strings.Builder
	status    string
	quitting  bool
	waiting   bool // waiting for LLM response

	// Channel to send user input to the agent loop
	inputChan chan<- string
}

type userMsg struct{ text string }
type responseMsg struct{ text string }
type toolStartMsg struct{ name string }
type toolDoneMsg struct {
	name   string
	output string
	err    error
}
type doneMsg struct{}
type errMsg struct{ err error }

func New(inputChan chan<- string) Model {
	ta := textarea.New()
	ta.Placeholder = "Ask anything..."
	ta.Focus()
	ta.CharLimit = 0
	ta.SetWidth(80)
	ta.SetHeight(1)

	s := spinner.New()
	s.Spinner = spinner.Spinner{
		Frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		FPS:    80,
	}

	return Model{
		theme:     DefaultTheme(),
		textarea:  ta,
		spinner:   s,
		inputChan: inputChan,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, textarea.Blink)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.quitting = true
			return m, tea.Quit
		case tea.KeyEnter:
			if !m.waiting {
				input := m.textarea.Value()
				if strings.TrimSpace(input) == "" {
					return m, nil
				}
				m.output.WriteString(m.theme.Bold.Render("> ") + input + "\n\n")
				m.textarea.Reset()
				m.waiting = true
				m.inputChan <- input
				return m, nil
			}
		}

	case userMsg:
		m.output.WriteString(msg.text)
		return m, nil

	case toolStartMsg:
		m.output.WriteString(m.theme.Tool.Render(fmt.Sprintf(" ◐ %s", msg.name)) + "\n")
		return m, m.spinner.Tick

	case toolDoneMsg:
		icon := m.theme.Success.Render(" ✓")
		if msg.err != nil {
			icon = m.theme.Error.Render(" ✗")
		}
		summary := msg.output
		if len(summary) > 120 {
			summary = summary[:120] + "..."
		}
		m.output.WriteString(fmt.Sprintf("%s %s: %s\n", icon, msg.name, summary))
		return m, nil

	case responseMsg:
		m.output.WriteString(msg.text)
		return m, nil

	case doneMsg:
		m.output.WriteString("\n")
		m.waiting = false
		return m, nil

	case errMsg:
		m.output.WriteString(m.theme.Error.Render(fmt.Sprintf("Error: %s\n", msg.err)))
		m.waiting = false
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	// Status bar
	status := m.status
	if m.waiting {
		status = m.spinner.View() + " " + status
	}
	b.WriteString(m.theme.Dim.Render(status) + "\n\n")

	// Output area
	b.WriteString(m.output.String())

	// Separator
	b.WriteString(m.theme.Separator.Render(strings.Repeat("─", 60)) + "\n")

	// Input
	if m.waiting {
		b.WriteString(m.theme.Dim.Render("  waiting..."))
	} else {
		b.WriteString(m.theme.Prompt.Render("> ") + m.textarea.View())
	}

	return b.String()
}

// SetStatus updates the status bar text.
func (m *Model) SetStatus(provider, model, dir string) {
	m.status = fmt.Sprintf(" vibe code · %s · %s", model, dir)
}

// Callback implementation for the agent loop.
type TUICallback struct {
	program *tea.Program
}

func NewCallback(p *tea.Program) *TUICallback {
	return &TUICallback{program: p}
}

func (c *TUICallback) OnText(text string) {
	c.program.Send(responseMsg{text: text})
}

func (c *TUICallback) OnToolStart(name, id string) {
	c.program.Send(toolStartMsg{name: name})
}

func (c *TUICallback) OnToolOutput(name, id, output string, err error) {
	c.program.Send(toolDoneMsg{name: name, output: output, err: err})
}

func (c *TUICallback) OnDone() {
	c.program.Send(doneMsg{})
}

func (c *TUICallback) OnError(err error) {
	c.program.Send(errMsg{err: err})
}
