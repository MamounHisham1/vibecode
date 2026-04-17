package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

type Model struct {
	theme     Theme
	textarea  textarea.Model
	spinner   spinner.Model
	output    *strings.Builder
	status    string
	quitting  bool
	waiting   bool
	width     int

	// Track active tools for inline spinner updates
	activeTools []activeTool

	inputChan chan<- string
}

type activeTool struct {
	name    string
	id      string
	started time.Time
}

type userMsg struct{ text string }
type responseMsg struct{ text string }
type toolStartMsg struct {
	name string
	id   string
}
type toolDoneMsg struct {
	name    string
	id      string
	output  string
	err     error
	started time.Time
}
type doneMsg struct{}
type errMsg struct{ err error }
type tickMsg struct{}

func New(inputChan chan<- string) Model {
	ta := textarea.New()
	ta.Placeholder = ""
	ta.Focus()
	ta.CharLimit = 0
	ta.SetWidth(80)
	ta.SetHeight(1)
	ta.ShowLineNumbers = false

	s := spinner.New()
	s.Spinner = spinner.Spinner{
		Frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		FPS:    100,
	}

	return Model{
		theme:      DefaultTheme(),
		textarea:   ta,
		spinner:    s,
		output:     &strings.Builder{},
		inputChan:  inputChan,
		width:      80,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, textarea.Blink, tickCmd)
}

func tickCmd() tea.Msg {
	time.Sleep(100 * time.Millisecond)
	return tickMsg{}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.textarea.SetWidth(min(msg.Width-4, 120))
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			if m.waiting {
				m.output.WriteString(m.theme.Warning.Render("  (cancelled)\n\n"))
				m.waiting = false
				return m, nil
			}
			m.quitting = true
			return m, tea.Quit
		case tea.KeyEnter:
			if !m.waiting {
				input := m.textarea.Value()
				if strings.TrimSpace(input) == "" {
					return m, nil
				}
				m.output.WriteString(m.theme.UserLabel.Render("> ") + input + "\n\n")
				m.textarea.Reset()
				m.waiting = true
				m.inputChan <- input
				return m, nil
			}
		}

	case tickMsg:
		if m.waiting || len(m.activeTools) > 0 {
			return m, tickCmd
		}
		return m, nil

	case userMsg:
		m.output.WriteString(msg.text)
		return m, nil

	case toolStartMsg:
		m.activeTools = append(m.activeTools, activeTool{
			name: msg.name, id: msg.id, started: time.Now(),
		})
		m.output.WriteString(fmt.Sprintf("  %s %s\n", m.spinner.View(), m.theme.ToolName.Render(msg.name)))
		return m, m.spinner.Tick

	case toolDoneMsg:
		// Remove from active tools
		for i, t := range m.activeTools {
			if t.id == msg.id {
				m.activeTools = append(m.activeTools[:i], m.activeTools[i+1:]...)
				break
			}
		}

		duration := time.Since(msg.started).Round(time.Millisecond)
		durStr := formatDuration(duration)

		if msg.err != nil {
			m.output.WriteString(fmt.Sprintf("  %s %s %s (%s)\n",
				m.theme.Error.Render("✗"),
				m.theme.ToolName.Render(msg.name),
				m.theme.Error.Render(truncate(msg.err.Error(), 80)),
				m.theme.Dim.Render(durStr),
			))
		} else {
			summary := truncate(strings.TrimSpace(msg.output), 100)
			m.output.WriteString(fmt.Sprintf("  %s %s %s (%s)\n",
				m.theme.Success.Render("✓"),
				m.theme.ToolName.Render(msg.name),
				m.theme.Dim.Render(summary),
				m.theme.Dim.Render(durStr),
			))
		}
		return m, nil

	case responseMsg:
		m.output.WriteString(msg.text)
		return m, nil

	case doneMsg:
		m.output.WriteString("\n")
		m.waiting = false
		m.activeTools = nil
		return m, nil

	case errMsg:
		m.output.WriteString(m.theme.Error.Render(fmt.Sprintf("\n  Error: %s\n\n", msg.err)))
		m.waiting = false
		m.activeTools = nil
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
	b.WriteString(m.renderStatus() + "\n")

	// Output area
	b.WriteString(m.output.String())

	// Separator + input
	b.WriteString(m.theme.Separator.Render("─"+strings.Repeat("─", min(m.width-1, 79))) + "\n")

	if m.waiting {
		b.WriteString(m.theme.Dim.Render("  ⋮ "))
	} else {
		b.WriteString(m.theme.Prompt.Render("> "))
		b.WriteString(m.textarea.View())
	}

	return b.String()
}

func (m Model) renderStatus() string {
	parts := []string{m.theme.Bold.Render("vibe code")}

	if m.status != "" {
		parts = append(parts, m.theme.Dim.Render("·"), m.theme.Dim.Render(m.status))
	}

	return " " + strings.Join(parts, " ")
}

func (m *Model) SetStatus(model, dir string) {
	m.status = fmt.Sprintf("%s · %s", model, shortenPath(dir))
}

// Callback implementation for the agent loop.
type TUICallback struct {
	program    *tea.Program
	startTimes map[string]time.Time
}

func NewCallback(p *tea.Program) *TUICallback {
	return &TUICallback{program: p, startTimes: make(map[string]time.Time)}
}

func (c *TUICallback) OnText(text string) {
	c.program.Send(responseMsg{text: text})
}

func (c *TUICallback) OnToolStart(name, id string) {
	c.startTimes[id] = time.Now()
	c.program.Send(toolStartMsg{name: name, id: id})
}

func (c *TUICallback) OnToolOutput(name, id, output string, err error) {
	started := c.startTimes[id]
	if started.IsZero() {
		started = time.Now()
	}
	delete(c.startTimes, id)
	c.program.Send(toolDoneMsg{name: name, id: id, output: output, err: err, started: started})
}

func (c *TUICallback) OnDone() {
	c.program.Send(doneMsg{})
}

func (c *TUICallback) OnError(err error) {
	c.program.Send(errMsg{err: err})
}

// Helpers

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return d.Round(time.Millisecond).String()
}

func truncate(s string, max int) string {
	// Replace newlines for single-line display
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func shortenPath(p string) string {
	home := ""
	if h, err := homeDir(); err == nil {
		home = h
	}
	if home != "" && strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func homeDir() (string, error) {
	return os.UserHomeDir()
}
