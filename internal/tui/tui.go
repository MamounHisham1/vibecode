package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	streamBuf   *strings.Builder

	inputChan chan<- string
}

type activeTool struct {
	name    string
	id      string
	started time.Time
}

type responseMsg struct{ chunk string }
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
		FPS:    80,
	}

	return Model{
		theme:      DefaultTheme(),
		textarea:   ta,
		spinner:    s,
		output:     &strings.Builder{},
		streamBuf:  &strings.Builder{},
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

// finalizeStream flushes stream buffer to output as rendered markdown.
func (m *Model) finalizeStream() {
	text := m.streamBuf.String()
	if text == "" {
		return
	}
	// Render each line with the ⎿ prefix like Claude Code's MessageResponse
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for _, line := range lines {
		m.output.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render("  ⎿  ") + line + "\n")
	}
	m.streamBuf.Reset()
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
				m.finalizeStream()
				m.output.WriteString("\n")
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
				// User message: no prefix, just the text
				m.output.WriteString(input + "\n\n")
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

	case responseMsg:
		m.streamBuf.WriteString(msg.chunk)
		return m, nil

	case toolStartMsg:
		m.finalizeStream()
		m.activeTools = append(m.activeTools, activeTool{
			name: msg.name, id: msg.id, started: time.Now(),
		})
		// Claude Code: ● ToolName — no extra formatting
		m.output.WriteString(fmt.Sprintf("● %s\n", m.theme.ToolName.Render(msg.name)))
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

		if msg.err != nil {
			m.output.WriteString(fmt.Sprintf("● %s (%s) %s\n",
				m.theme.ToolError.Render(msg.name),
				m.theme.ToolError.Render(truncate(msg.err.Error(), 60)),
				m.theme.Dim.Render(durStr),
			))
		} else {
			summary := truncate(strings.TrimSpace(msg.output), 80)
			m.output.WriteString(fmt.Sprintf("● %s (%s) %s\n",
				m.theme.ToolSuccess.Render(msg.name),
				m.theme.Dim.Render(summary),
				m.theme.Dim.Render(durStr),
			))
		}
		return m, nil

	case doneMsg:
		m.finalizeStream()
		m.output.WriteString("\n")
		m.waiting = false
		m.activeTools = nil
		return m, nil

	case errMsg:
		m.finalizeStream()
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

	// Status bar
	b.WriteString(m.renderStatus() + "\n\n")

	// Finished output
	b.WriteString(m.output.String())

	// Live streaming text — render with ⎿ prefix
	if m.streamBuf.Len() > 0 {
		stream := m.streamBuf.String()
		lines := strings.Split(strings.TrimRight(stream, "\n"), "\n")
		prefix := lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render("  ⎿  ")
		for _, line := range lines {
			b.WriteString(prefix + line + "\n")
		}
	}

	// Thin separator
	sepWidth := min(m.width-1, 79)
	if sepWidth < 10 {
		sepWidth = 40
	}
	b.WriteString(m.theme.Subtle.Render(strings.Repeat("─", sepWidth)) + "\n")

	// Input
	if m.waiting {
		b.WriteString(m.theme.Brand.Render("  " + m.spinner.View()))
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
	c.program.Send(responseMsg{chunk: text})
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
