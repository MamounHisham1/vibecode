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

	activeTools []activeTool
	textBuf     *strings.Builder

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
		Frames: []string{"●", "●"}, // Claude Code uses blinking ●
		FPS:    500,
	}

	return Model{
		theme:      DefaultTheme(),
		textarea:   ta,
		spinner:    s,
		output:     &strings.Builder{},
		textBuf:    &strings.Builder{},
		inputChan:  inputChan,
		width:      80,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, tickCmd)
}

func tickCmd() tea.Msg {
	time.Sleep(100 * time.Millisecond)
	return tickMsg{}
}

func (m *Model) flushTextBuf() {
	text := m.textBuf.String()
	if text == "" {
		return
	}
	rendered := RenderMarkdown(text)
	m.output.WriteString(rendered + "\n")
	m.textBuf.Reset()
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
				m.flushTextBuf()
				m.output.WriteString(m.theme.Dim.Render("(cancelled)\n\n"))
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
				m.output.WriteString(m.theme.Prompt.Render("> ") + input + "\n\n")
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
		m.flushTextBuf()
		m.activeTools = append(m.activeTools, activeTool{
			name: msg.name, id: msg.id, started: time.Now(),
		})
		// Claude Code style: ● ToolName
		m.output.WriteString(fmt.Sprintf(" %s %s\n",
			m.theme.Brand.Render("●"),
			m.theme.ToolName.Render(msg.name),
		))
		return m, tickCmd

	case toolDoneMsg:
		for i, t := range m.activeTools {
			if t.id == msg.id {
				m.activeTools = append(m.activeTools[:i], m.activeTools[i+1:]...)
				break
			}
		}

		duration := time.Since(msg.started).Round(time.Millisecond)
		durStr := formatDuration(duration)

		// Claude Code style: ● ToolName (result) duration
		if msg.err != nil {
			m.output.WriteString(fmt.Sprintf(" %s %s (%s) %s\n",
				m.theme.ToolError.Render("●"),
				m.theme.ToolName.Render(msg.name),
				m.theme.ToolError.Render(truncate(msg.err.Error(), 60)),
				m.theme.Dim.Render(durStr),
			))
		} else {
			summary := truncate(strings.TrimSpace(msg.output), 80)
			m.output.WriteString(fmt.Sprintf(" %s %s (%s) %s\n",
				m.theme.ToolSuccess.Render("●"),
				m.theme.ToolName.Render(msg.name),
				m.theme.Dim.Render(summary),
				m.theme.Dim.Render(durStr),
			))
		}
		return m, nil

	case responseMsg:
		m.textBuf.WriteString(msg.text)
		return m, nil

	case doneMsg:
		m.flushTextBuf()
		m.output.WriteString("\n")
		m.waiting = false
		m.activeTools = nil
		return m, nil

	case errMsg:
		m.flushTextBuf()
		m.output.WriteString(m.theme.Error.Render(fmt.Sprintf("Error: %s\n\n", msg.err)))
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

	// Claude Code style: status line with brand name
	b.WriteString(m.renderStatus() + "\n")

	// Output area
	b.WriteString(m.output.String())

	// Live text buffer
	if m.textBuf.Len() > 0 {
		b.WriteString(m.textBuf.String())
	}

	// Separator — thin, subtle
	sepWidth := min(m.width-1, 79)
	if sepWidth < 10 {
		sepWidth = 40
	}
	b.WriteString(m.theme.Subtle.Render(strings.Repeat("─", sepWidth)) + "\n")

	// Input
	if m.waiting {
		b.WriteString(m.theme.Dim.Render("  ⋮ "))
	} else {
		b.WriteString(m.theme.Prompt.Render("> "))
		b.WriteString(m.textarea.View())
	}

	return b.String()
}

func (m Model) renderStatus() string {
	return fmt.Sprintf(" %s %s",
		m.theme.Brand.Bold(true).Render("vibe code"),
		m.theme.Dim.Render(m.status),
	)
}

func (m *Model) SetStatus(model, dir string) {
	m.status = fmt.Sprintf("· %s · %s", model, shortenPath(dir))
}

// Callback for the agent loop.
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

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return d.Round(time.Millisecond).String()
}

func truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func shortenPath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if strings.HasPrefix(p, home) {
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
