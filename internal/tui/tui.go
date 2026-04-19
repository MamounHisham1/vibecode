package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/reflow/wordwrap"
)

const (
	assistantDot = "◉"
	userPointer  = "▸"
	toolPointer  = "↳"
	tickInterval = 80 * time.Millisecond
	fadeFrames   = 12
)

var spinnerVerbs = []string{
	"Thinking",
	"Analyzing",
	"Planning",
	"Reasoning",
	"Working",
	"Computing",
	"Building",
	"Processing",
	"Deliberating",
	"Synthesizing",
}

var spinnerFrames = []string{
	"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏",
}

var readLinesShownRE = regexp.MustCompile(`\((\d+) of (\d+) lines shown\)\s*$`)

// ─── Types ──────────────────────────────────────────────────────

type transcriptKind int

const (
	transcriptUser transcriptKind = iota
	transcriptAssistant
	transcriptTool
	transcriptSystem
)

type toolState int

const (
	toolRunning toolState = iota
	toolSucceeded
	toolFailed
)

type Model struct {
	theme Theme
	input InputModel

	entries      []transcriptItem
	streamBuf    *strings.Builder
	toolIndex    map[string]int
	status       string
	modelName    string
	dir          string
	quitting     bool
	waiting      bool
	welcome      bool
	width        int
	height       int
	blinkOn      bool
	scrollOffset int // lines scrolled up from bottom (0 = pinned)
	verbIndex    int
	frameIndex   int
	inputChan    chan<- string
	expanded     map[string]bool
	ctrlOPress   bool

	// Token/cost tracking
	totalTokens int
	lastTokens  int

	// Session stats
	sessionStart time.Time
	turnCount    int

	// Cancellation
	cancelFunc  context.CancelFunc
	interruptCh chan struct{}
}

type transcriptItem struct {
	kind    transcriptKind
	text    string
	tool    toolEntry
	isError bool
}

type toolEntry struct {
	ID       string
	Name     string
	Label    string
	Args     string
	Input    json.RawMessage
	State    toolState
	Started  time.Time
	Finished time.Time
	Summary  string
	Preview  []string
	ErrText  string
	IsDiff   bool // true when Preview contains styled diff lines
}

type responseMsg struct{ chunk string }

type toolStartMsg struct {
	name  string
	id    string
	input json.RawMessage
}

type toolDoneMsg struct {
	name    string
	id      string
	output  string
	err     error
	started time.Time
}

type interruptMsg struct{}
type doneMsg struct{}
type errMsg struct{ err error }
type tickMsg struct{}
type compactMsg struct{}
type usageMsg struct {
	inputTokens  int
	outputTokens int
}

// ─── Constructor ────────────────────────────────────────────────

func New(inputChan chan<- string) *Model {
	theme := DefaultTheme()
	input := NewInputModel(theme)
	input.SetWidth(80)

	return &Model{
		theme:        theme,
		input:        input,
		entries:      nil,
		streamBuf:    &strings.Builder{},
		toolIndex:    make(map[string]int),
		status:       "",
		modelName:    "",
		quitting:     false,
		waiting:      false,
		welcome:      true,
		width:        80,
		height:       24,
		blinkOn:      true,
		verbIndex:    0,
		frameIndex:   0,
		inputChan:    inputChan,
		expanded:     make(map[string]bool),
		sessionStart: time.Now(),
		interruptCh:  make(chan struct{}),
	}
}

func (m *Model) Init() tea.Cmd {
	return tickCmd
}

func tickCmd() tea.Msg {
	time.Sleep(tickInterval)
	return tickMsg{}
}

// ─── Update ─────────────────────────────────────────────────────

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.SetWidth(m.inputTextWidth())
		m.input.SetMaxLines(m.height / 2)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "shift+up", "pgup":
			m.scrollOffset += 10
			return m, nil
		case "shift+down", "pgdown":
			m.scrollOffset = 0
			return m, nil
		case "ctrl+c":
			if m.waiting {
				m.interruptAgent()
				return m, nil
			}
			if m.input.IsEmpty() {
				m.quitting = true
				return m, tea.Quit
			}
			// Non-empty input: clear the input text on first Ctrl+C
			m.input.SetValue("")
			return m, nil

		case "ctrl+o":
			m.toggleExpandAll()
			return m, nil

		case "enter":
			if m.waiting {
				return m, nil
			}
			raw := m.input.Value()
			if strings.TrimSpace(raw) == "" {
				return m, nil
			}
			// Backslash+enter inserts newline
			if strings.HasSuffix(raw, "\\") {
				m.input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'\n'}})
				return m, nil
			}
			m.appendUserMessage(raw)
			m.input.Reset()
			m.scrollOffset = 0
			m.waiting = true
			m.input.SetWaiting(true)
			m.welcome = false
			m.turnCount++
			m.advanceSpinnerVerb()
			m.inputChan <- raw
			return m, nil

		default:
			m.input.Update(msg)
			return m, nil
		}

	case tickMsg:
		m.frameIndex = (m.frameIndex + 1) % len(spinnerFrames)
		m.blinkOn = true // cursor always visible
		m.input.SetBlink(true)
		if m.waiting || len(m.toolIndex) > 0 {
			m.input.SetSpinnerFrame(spinnerFrames[m.frameIndex])
			return m, tickCmd
		}
		return m, tickCmd // keep ticking so cursor never disappears

	case tea.MouseMsg:
		if msg.Type == tea.MouseWheelUp {
			m.scrollOffset += 3
		} else if msg.Type == tea.MouseWheelDown {
			if m.scrollOffset > 3 {
				m.scrollOffset -= 3
			} else {
				m.scrollOffset = 0
			}
		}
		return m, nil

	case interruptMsg:
		m.finalizeStream()
		m.appendSystemMessage("Interrupted by user", false)
		m.waiting = false
		m.input.SetWaiting(false)
		m.toolIndex = make(map[string]int)
		return m, nil

	case responseMsg:
		m.streamBuf.WriteString(msg.chunk)
		return m, nil

	case toolStartMsg:
		m.finalizeStream()
		m.addToolEntry(msg.name, msg.id, msg.input)
		return m, nil

	case toolDoneMsg:
		m.finalizeStream()
		m.completeToolEntry(msg.id, msg.output, msg.err, msg.started)
		return m, nil

	case doneMsg:
		m.finalizeStream()
		m.waiting = false
		m.input.SetWaiting(false)
		m.toolIndex = make(map[string]int)
		return m, nil

	case errMsg:
		m.finalizeStream()
		// Suppress duplicate errors from context cancellation —
		// interruptAgent() already shows "Interrupted by user".
		if m.waiting {
			errText := msg.err.Error()
			if strings.Contains(errText, "context canceled") || strings.Contains(errText, "context.Cancel") {
				// Already handled by interruptAgent
				return m, nil
			}
			m.appendSystemMessage(fmt.Sprintf("Error: %s", msg.err), true)
		}
		m.waiting = false
		m.input.SetWaiting(false)
		m.toolIndex = make(map[string]int)
		return m, nil

	case compactMsg:
		m.finalizeStream()
		m.appendSystemMessage("Conversation compacted to free context window space", false)
		return m, nil

	case usageMsg:
		m.totalTokens += msg.inputTokens + msg.outputTokens
		m.lastTokens = msg.inputTokens + msg.outputTokens
		return m, nil
	}

	return m, nil
}

func (m *Model) toggleExpandAll() {
	anyCollapsed := false
	for _, entry := range m.entries {
		if entry.kind == transcriptTool && entry.tool.State != toolRunning {
			if !m.expanded[entry.tool.ID] {
				anyCollapsed = true
				break
			}
		}
	}

	for _, entry := range m.entries {
		if entry.kind == transcriptTool && entry.tool.State != toolRunning {
			if anyCollapsed {
				m.expanded[entry.tool.ID] = true
			} else {
				delete(m.expanded, entry.tool.ID)
			}
		}
	}
}

// ─── View ───────────────────────────────────────────────────────

func (m *Model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	// Status bar
	b.WriteString(m.renderStatusBar())

	// Welcome screen
	if m.welcome && len(m.entries) == 0 && m.streamBuf.Len() == 0 && len(m.toolIndex) == 0 {
		b.WriteString("\n")
		b.WriteString(m.renderWelcome())
	}

	// Transcript
	content := m.renderTranscript()
	if content != "" {
		b.WriteString("\n" + content)
	}

	// Input area
	b.WriteString("\n\n")
	b.WriteString(m.renderInputArea())

	fullView := b.String()

	// Viewport: fit into terminal height.
	// scrollOffset lets the user scroll up to see earlier messages.
	viewportHeight := m.height - 1
	if viewportHeight < 3 {
		viewportHeight = 3
	}

	totalLines := countWrappedLines(fullView, m.width)
	if totalLines <= viewportHeight {
		return fullView
	}

	// Clamp scroll offset
	maxScroll := totalLines - viewportHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scrollOffset > maxScroll {
		m.scrollOffset = maxScroll
	}

	rawLines := strings.Split(fullView, "\n")
	// Clamp scrollOffset to raw lines count (not wrapped visual lines)
	maxRawScroll := len(rawLines) - viewportHeight
	if maxRawScroll < 0 {
		maxRawScroll = 0
	}
	if m.scrollOffset > maxRawScroll {
		m.scrollOffset = maxRawScroll
	}

	endIdx := len(rawLines) - m.scrollOffset
	startIdx := endIdx - viewportHeight
	if startIdx < 0 {
		startIdx = 0
	}
	if endIdx > len(rawLines) {
		endIdx = len(rawLines)
	}
	if endIdx < startIdx {
		endIdx = startIdx
	}

	return strings.Join(rawLines[startIdx:endIdx], "\n")
}

// countWrappedLines counts the visual lines a string will occupy,
// accounting for line breaks and word wrapping at the given width.
func countWrappedLines(s string, width int) int {
	if width <= 0 {
		width = 80
	}
	lines := strings.Split(s, "\n")
	count := 0
	for _, line := range lines {
		if line == "" {
			count++
			continue
		}
		// Measure the visual width (handles ANSI escape codes correctly)
		w := ansi.StringWidth(line)
		if w <= 0 {
			w = len(line)
		}
		if w > width {
			count += (w + width - 1) / width
		} else {
			count++
		}
	}
	return count
}

// unused
var _ = countWrappedLines

func (m *Model) renderStatusBar() string {
	t := m.theme
	width := m.width

	// Left side: brand + model + directory
	left := " " + t.StatusBarBrand.Render("vibe code")

	if m.status != "" {
		left += t.StatusBarInfo.Render(" " + m.status)
	}

	// Right side: token count + turn count + elapsed + hints
	elapsed := time.Since(m.sessionStart).Round(time.Second)
	right := ""
	if m.totalTokens > 0 {
		right += t.StatusBarDim.Render(formatTokenCount(m.totalTokens))
		right += t.StatusBarDim.Render(" · ")
	}
	if m.turnCount > 0 {
		right += t.StatusBarDim.Render(fmt.Sprintf("turn %d", m.turnCount))
		right += t.StatusBarDim.Render(" · ")
	}
	right += t.StatusBarDim.Render(elapsed.String())

	if m.waiting {
		right += t.StatusBarDim.Render(" · ")
		right += t.StatusBar.Render("ctrl+c stop")
	}

	// Pad to full width
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	pad := width - leftW - rightW
	if pad < 1 {
		pad = 1
	}

	bar := left + strings.Repeat(" ", pad) + right
	// Fill to width with background
	bar = t.StatusBar.Render(bar)
	// Ensure it fills the width
	if lipgloss.Width(bar) < width {
		bar += t.StatusBar.Render(strings.Repeat(" ", width-lipgloss.Width(bar)))
	}

	return bar
}

func (m *Model) renderTranscript() string {
	var b strings.Builder

	for i, entry := range m.entries {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(m.renderEntry(entry))
	}

	// Live streaming text
	if m.streamBuf.Len() > 0 {
		if len(m.entries) > 0 {
			b.WriteString("\n")
		}
		b.WriteString(m.renderAssistantEntry(m.streamBuf.String()))
	}

	// Thinking spinner
	if m.waiting && len(m.toolIndex) == 0 && m.streamBuf.Len() == 0 {
		if len(m.entries) > 0 {
			b.WriteString("\n")
		}
		b.WriteString(m.renderThinkingState())
	}

	return b.String()
}

func (m *Model) renderInputArea() string {
	return m.input.View()
}

// ─── Entry Renderers ────────────────────────────────────────────

func (m *Model) renderEntry(entry transcriptItem) string {
	switch entry.kind {
	case transcriptUser:
		return m.renderUserEntry(entry.text)
	case transcriptAssistant:
		return m.renderAssistantEntry(entry.text)
	case transcriptTool:
		return m.renderToolEntry(entry.tool)
	case transcriptSystem:
		return m.renderSystemEntry(entry.text, entry.isError)
	default:
		return ""
	}
}

func (m *Model) renderUserEntry(text string) string {
	t := m.theme
	w := max(20, m.transcriptWidth()-4)
	wrapped := wordwrap.String(strings.TrimRight(text, "\n"), w)

	prefix := t.UserPointer.Render(userPointer + " ")
	return prefix + t.UserText.Render(wrapped)
}

func (m *Model) renderAssistantEntry(text string) string {
	t := m.theme
	dot := t.AssistantDot.Render(assistantDot + " ")
	return gutterBlock(
		dot,
		"  ",
		RenderMarkdown(text),
	)
}

func (m *Model) renderToolEntry(tool toolEntry) string {
	var b strings.Builder
	b.WriteString(m.renderToolHeadline(tool))

	if tool.State == toolRunning {
		frame := spinnerFrames[m.frameIndex%len(spinnerFrames)]
		bodyLines := []string{m.theme.ToolRunning.Render(frame + " Running…")}
		b.WriteString("\n")
		b.WriteString(renderNestedBlock(bodyLines, m.theme.ToolBorder.Render("  "+toolPointer+"  ")))
		return strings.TrimRight(b.String(), "\n")
	}

	// Collapsed by default
	if !m.expanded[tool.ID] {
		summary := tool.Summary
		if summary == "" && tool.State == toolFailed {
			summary = tool.ErrText
		}
		if summary != "" {
			style := m.theme.Text
			if tool.State == toolFailed {
				style = m.theme.Error
			}
			b.WriteString("\n")
			b.WriteString(renderNestedBlock([]string{style.Render(summary)}, m.theme.ToolBorder.Render("  "+toolPointer+"  ")))
		}
		if len(tool.Preview) > 0 {
			b.WriteString("\n")
			if tool.IsDiff {
				// Show first few diff lines inline in collapsed view
				maxShow := 8
				showLines := tool.Preview
				if len(showLines) > maxShow {
					showLines = showLines[:maxShow]
				}
				b.WriteString(renderNestedBlock(showLines, m.theme.Subtle.Render("  "+toolPointer+"  ")))
				if len(tool.Preview) > maxShow {
					b.WriteString(renderNestedBlock(
						[]string{m.theme.Dim.Render(fmt.Sprintf("ctrl+o to expand (%d more lines)", len(tool.Preview)-maxShow))},
						m.theme.Subtle.Render("  "+toolPointer+"  "),
					))
				}
			} else {
				b.WriteString(renderNestedBlock(
					[]string{m.theme.Dim.Render("ctrl+o to expand")},
					m.theme.Subtle.Render("  "+toolPointer+"  "),
				))
			}
		}
		return strings.TrimRight(b.String(), "\n")
	}

	// Expanded
	bodyLines := make([]string, 0, len(tool.Preview)+1)

	for _, line := range tool.Preview {
		if tool.IsDiff {
			bodyLines = append(bodyLines, line) // already styled by formatDiffPreview
		} else {
			bodyLines = append(bodyLines, m.theme.Dim.Render(line))
		}
	}

	if tool.State == toolFailed {
		summary := tool.Summary
		if summary == "" {
			summary = tool.ErrText
		}
		if summary != "" {
			bodyLines = append(bodyLines, m.theme.Error.Render(summary))
		}
	} else if tool.Summary != "" {
		style := m.theme.Text
		if len(tool.Preview) > 0 {
			style = m.theme.Dim
		}
		bodyLines = append(bodyLines, style.Render(tool.Summary))
	}

	if len(bodyLines) > 0 {
		b.WriteString("\n")
		b.WriteString(renderNestedBlock(bodyLines, m.theme.ToolBorder.Render("  "+toolPointer+"  ")))
	}

	return strings.TrimRight(b.String(), "\n")
}

func (m *Model) renderToolHeadline(tool toolEntry) string {
	t := m.theme

	// Icon based on tool type
	icon := "⚙"
	dotStyle := t.ToolDot

	switch tool.State {
	case toolRunning:
		icon = "◎"
		dotStyle = t.ToolRunning
	case toolSucceeded:
		icon = "✓"
		dotStyle = t.ToolSuccess
	case toolFailed:
		icon = "✗"
		dotStyle = t.ToolError
	}

	line := dotStyle.Render(icon) + " " + t.ToolName.Render(tool.Label)
	if tool.Args != "" {
		line += " " + t.ToolArgs.Render(tool.Args)
	}
	if !tool.Finished.IsZero() && !tool.Started.IsZero() {
		dur := formatDuration(tool.Finished.Sub(tool.Started))
		if dur != "" {
			line += " " + t.ToolDuration.Render(dur)
		}
	}
	return line
}

func (m *Model) renderSystemEntry(text string, isError bool) string {
	style := m.theme.Dim
	if isError {
		style = m.theme.Error
	}
	icon := "ℹ"
	if isError {
		icon = "⚠"
	}
	return renderNestedBlock(
		[]string{style.Render(icon + " " + text)},
		m.theme.Subtle.Render("  "),
	)
}

func (m *Model) renderThinkingState() string {
	t := m.theme
	frame := spinnerFrames[m.frameIndex%len(spinnerFrames)]
	verb := spinnerVerbs[m.verbIndex%len(spinnerVerbs)]

	spinner := t.AssistantDot.Render(frame)
	label := t.BrandLight.Render(verb)
	dots := t.Dim.Render("...")

	return spinner + " " + label + dots
}

func (m *Model) renderWelcome() string {
	t := m.theme
	var b strings.Builder

	b.WriteString("\n")

	// ASCII Art Title
	titleLines := []string{
		"██╗   ██╗██╗██████╗ ███████╗ ██████╗    ██████╗ ███████╗ ██████╗ ███████╗",
		"██║   ██║██║██╔══██╗██╔════╝██╔═══██╗   ██╔══██╗██╔════╝██╔═══██╗██╔════╝",
		"██║   ██║██║██║  ██║█████╗  ██║   ██║   ██████╔╝█████╗  ██║   ██║███████╗",
		"╚██╗ ██╔╝██║██║  ██║██╔══╝  ██║   ██║   ██╔══██╗██╔══╝  ██║   ██║╚════██║",
		" ╚████╔╝ ██║██████╔╝███████╗╚██████╔╝   ██║  ██║███████╗╚██████╔╝███████║",
		"  ╚═══╝  ╚═╝╚═════╝ ╚══════╝ ╚═════╝    ╚═╝  ╚═╝╚══════╝ ╚═════╝ ╚══════╝",
	}

	maxW := m.transcriptWidth()
	for _, line := range titleLines {
		if len(line) > maxW {
			continue
		}
		b.WriteString("  " + t.WelcomeBorder.Render(line) + "\n")
	}

	b.WriteString("\n")

	// Subtitle
	subtitle := "AI-powered coding agent for your terminal"
	b.WriteString("  " + t.WelcomeSubtitle.Render(subtitle) + "\n")
	b.WriteString("\n")

	// Model info
	if m.status != "" {
		b.WriteString("  " + t.Dim.Render("Model: ") + t.Text.Render(m.status) + "\n")
		b.WriteString("\n")
	}

	// Quick start tips with styled keys
	b.WriteString("  " + t.Bold.Render("Getting started") + "\n")
	b.WriteString("\n")

	tips := []struct {
		key  string
		desc string
	}{
		{"enter", "Send a message"},
		{"shift+enter", "New line (multi-line input)"},
		{"ctrl+o", "Expand/collapse tool output"},
		{"ctrl+c", "Stop generation or exit"},
	}

	for _, tip := range tips {
		b.WriteString("  " + t.WelcomeKey.Render("  "+tip.key) + "  " + t.WelcomeDesc.Render(tip.desc) + "\n")
	}

	b.WriteString("\n")
	b.WriteString("  " + t.Dim.Render("Type a message below to get started →") + "\n")

	return b.String()
}

// ─── Data Methods ───────────────────────────────────────────────

func (m *Model) finalizeStream() {
	text := strings.TrimSpace(m.streamBuf.String())
	if text == "" {
		m.streamBuf.Reset()
		return
	}
	m.entries = append(m.entries, transcriptItem{
		kind: transcriptAssistant,
		text: text,
	})
	m.streamBuf.Reset()
}

func (m *Model) appendUserMessage(text string) {
	m.entries = append(m.entries, transcriptItem{
		kind: transcriptUser,
		text: strings.TrimRight(text, "\n"),
	})
}

func (m *Model) appendSystemMessage(text string, isError bool) {
	m.entries = append(m.entries, transcriptItem{
		kind:    transcriptSystem,
		text:    text,
		isError: isError,
	})
}

func (m *Model) addToolEntry(name, id string, input json.RawMessage) {
	label, args := summarizeToolCall(name, input)
	m.entries = append(m.entries, transcriptItem{
		kind: transcriptTool,
		tool: toolEntry{
			ID:      id,
			Name:    name,
			Label:   label,
			Args:    args,
			Input:   input,
			State:   toolRunning,
			Started: time.Now(),
		},
	})
	m.toolIndex[id] = len(m.entries) - 1
}

func (m *Model) completeToolEntry(id, output string, err error, started time.Time) {
	idx, ok := m.toolIndex[id]
	if !ok {
		label, args := summarizeToolCall("", nil)
		m.entries = append(m.entries, transcriptItem{
			kind: transcriptTool,
			tool: toolEntry{
				ID:      id,
				Label:   label,
				Args:    args,
				State:   toolFailed,
				Started: started,
			},
		})
		idx = len(m.entries) - 1
	}

	entry := m.entries[idx]
	entry.tool.Started = started
	entry.tool.Finished = time.Now()
	entry.tool.Summary, entry.tool.Preview = summarizeToolResult(entry.tool.Name, output, err)

	// Compute diff preview for file-modifying tools
	if err == nil {
		switch entry.tool.Name {
		case "edit_file":
			// Diff from tool INPUT (has old_string / new_string)
			if len(entry.tool.Input) > 0 {
				var editIn struct {
					Path      string `json:"path"`
					OldString string `json:"old_string"`
					NewString string `json:"new_string"`
				}
				if json.Unmarshal(entry.tool.Input, &editIn) == nil && editIn.OldString != editIn.NewString {
					summary, preview := formatDiffPreview(editIn.OldString, editIn.NewString, m.theme, m.transcriptWidth(), editIn.Path)
					entry.tool.Summary = summary
					entry.tool.Preview = preview
					entry.tool.IsDiff = len(preview) > 0
				}
			}

		case "write_file":
			// Diff from tool OUTPUT (has old_content / new_content)
			var writeOut struct {
				Path       string `json:"path"`
				OldContent string `json:"old_content"`
				NewContent string `json:"new_content"`
			}
			if json.Unmarshal([]byte(output), &writeOut) == nil && writeOut.OldContent != writeOut.NewContent {
				summary, preview := formatDiffPreview(writeOut.OldContent, writeOut.NewContent, m.theme, m.transcriptWidth(), writeOut.Path)
				entry.tool.Summary = summary
				entry.tool.Preview = preview
				entry.tool.IsDiff = len(preview) > 0
			}
		}
	}

	if err != nil {
		entry.tool.State = toolFailed
		entry.tool.ErrText = err.Error()
		if entry.tool.Summary == "" {
			entry.tool.Summary = err.Error()
		}
	} else {
		entry.tool.State = toolSucceeded
	}
	m.entries[idx] = entry
	delete(m.toolIndex, id)
}

func (m *Model) SetStatus(model, dir string) {
	m.modelName = model
	m.dir = dir
	m.status = fmt.Sprintf("%s · %s", model, shortenPath(dir))
}

func (m *Model) advanceSpinnerVerb() {
	m.verbIndex = (m.verbIndex + 1) % len(spinnerVerbs)
}

// interruptAgent cancels the running agent turn so the goroutine can proceed
// to wait for the next message.  It does NOT close inputChan, so the session
// stays alive and the user can continue chatting.
func (m *Model) interruptAgent() {
	// Cancel the per-turn context — this stops the LLM stream and tool execution.
	if m.cancelFunc != nil {
		m.cancelFunc()
	}

	// Give the agent goroutine a moment to react (it will receive the context
	// cancellation and return from Run).  We use a short timeout to avoid
	// blocking the TUI event loop.
	select {
	case <-m.interruptCh:
		// Agent acknowledged interruption
	case <-time.After(2 * time.Second):
		// Timeout — force the UI state anyway
	}

	// Update UI state directly since the agent goroutine may not send the
	// proper done/error messages if it was forcefully cancelled.
	m.finalizeStream()
	m.appendSystemMessage("Interrupted by user", false)
	m.waiting = false
	m.input.SetWaiting(false)
	m.toolIndex = make(map[string]int)
}

// SetCancelFunc sets the context cancel function used to interrupt the agent.
func (m *Model) SetCancelFunc(fn context.CancelFunc) {
	m.cancelFunc = fn
}

func (m *Model) transcriptWidth() int {
	return min(max(m.width-2, 24), 120)
}

func (m *Model) inputTextWidth() int {
	return min(max(m.width-6, 20), 112)
}

// ─── Callback ───────────────────────────────────────────────────

type TUICallback struct {
	program    *tea.Program
	mu         sync.Mutex
	startTimes map[string]time.Time
}

func NewCallback(p *tea.Program) *TUICallback {
	return &TUICallback{program: p, startTimes: make(map[string]time.Time)}
}

func (c *TUICallback) OnText(text string) {
	c.program.Send(responseMsg{chunk: text})
}

func (c *TUICallback) OnToolStart(name, id string, input json.RawMessage) {
	c.mu.Lock()
	c.startTimes[id] = time.Now()
	c.mu.Unlock()
	c.program.Send(toolStartMsg{name: name, id: id, input: input})
}

func (c *TUICallback) OnToolOutput(name, id, output string, err error) {
	c.mu.Lock()
	started := c.startTimes[id]
	if started.IsZero() {
		started = time.Now()
	}
	delete(c.startTimes, id)
	c.mu.Unlock()
	c.program.Send(toolDoneMsg{name: name, id: id, output: output, err: err, started: started})
}

func (c *TUICallback) OnDone() {
	c.program.Send(doneMsg{})
}

func (c *TUICallback) OnError(err error) {
	c.program.Send(errMsg{err: err})
}

func (c *TUICallback) OnCompact(summary string) {
	c.program.Send(compactMsg{})
}

func (c *TUICallback) OnUsage(inputTokens, outputTokens int) {
	c.program.Send(usageMsg{inputTokens: inputTokens, outputTokens: outputTokens})
}

// ─── Tool Summary ───────────────────────────────────────────────

func summarizeToolCall(name string, input json.RawMessage) (string, string) {
	switch name {
	case "shell":
		var in struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(input, &in) == nil {
			return "Bash", previewCommand(in.Command, 120)
		}
		return "Bash", ""
	case "git":
		var in struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(input, &in) == nil {
			return "Git", previewCommand(in.Command, 120)
		}
		return "Git", ""
	case "read_file":
		var in struct {
			Path   string `json:"path"`
			Offset int    `json:"offset"`
			Limit  int    `json:"limit"`
		}
		if json.Unmarshal(input, &in) == nil {
			args := shortenPath(in.Path)
			if in.Offset > 0 && in.Limit > 0 {
				args += fmt.Sprintf(" · lines %d-%d", in.Offset, in.Offset+in.Limit-1)
			} else if in.Offset > 0 {
				args += fmt.Sprintf(" · from line %d", in.Offset)
			}
			return "Read", args
		}
		return "Read", ""
	case "write_file":
		var in struct {
			Path string `json:"path"`
		}
		if json.Unmarshal(input, &in) == nil {
			return "Write", shortenPath(in.Path)
		}
		return "Write", ""
	case "edit_file":
		var in struct {
			Path string `json:"path"`
		}
		if json.Unmarshal(input, &in) == nil {
			return "Edit", shortenPath(in.Path)
		}
		return "Edit", ""
	case "grep":
		var in struct {
			Pattern string `json:"pattern"`
			Path    string `json:"path"`
		}
		if json.Unmarshal(input, &in) == nil {
			args := in.Pattern
			if in.Path != "" {
				args += " · " + shortenPath(in.Path)
			}
			return "Grep", previewCommand(args, 120)
		}
		return "Grep", ""
	case "glob":
		var in struct {
			Pattern string `json:"pattern"`
		}
		if json.Unmarshal(input, &in) == nil {
			return "Glob", in.Pattern
		}
		return "Glob", ""
	case "web_fetch":
		var in struct {
			URL string `json:"url"`
		}
		if json.Unmarshal(input, &in) == nil {
			return "WebFetch", in.URL
		}
		return "WebFetch", ""
	case "ask_user":
		var in struct {
			Question string `json:"question"`
		}
		if json.Unmarshal(input, &in) == nil {
			return "AskUser", previewCommand(in.Question, 120)
		}
		return "AskUser", ""
	default:
		if name == "" {
			return "Tool", ""
		}
		return humanizeToolName(name), previewCommand(string(input), 120)
	}
}

func summarizeToolResult(name, raw string, err error) (string, []string) {
	if err != nil {
		if strings.TrimSpace(raw) != "" {
			return truncateOneLine(raw, 140), nil
		}
		return err.Error(), nil
	}

	switch name {
	case "shell":
		var out struct {
			Output  string `json:"output"`
			Success bool   `json:"success"`
			Error   string `json:"error"`
		}
		if json.Unmarshal([]byte(raw), &out) == nil {
			lines := cleanLines(out.Output)
			summary := ""
			if len(lines) > 0 {
				summary = fmt.Sprintf("%d lines", len(lines))
			} else if out.Success {
				summary = "Completed"
			}
			if out.Error != "" {
				summary = out.Error
			}
			return summary, tailLines(lines, 5)
		}

	case "read_file":
		if text, ok := decodeJSONString(raw); ok {
			if match := readLinesShownRE.FindStringSubmatch(text); len(match) == 3 {
				return fmt.Sprintf("Read %s of %s lines", match[1], match[2]), nil
			}
			lines := cleanLines(text)
			if len(lines) > 0 {
				return fmt.Sprintf("Read %d lines", len(lines)), nil
			}
			return "Read complete", nil
		}

	case "web_fetch":
		var out struct {
			URL        string `json:"url"`
			Status     int    `json:"status"`
			Content    string `json:"content"`
			ContentLen int    `json:"content_len"`
		}
		if json.Unmarshal([]byte(raw), &out) == nil {
			summary := fmt.Sprintf("Fetched %s", out.URL)
			if out.ContentLen > 0 {
				summary += fmt.Sprintf(" · %d chars", out.ContentLen)
			}
			return summary, headLines(cleanLines(out.Content), 3)
		}
	}

	if text, ok := decodeJSONString(raw); ok {
		return summarizeTextResult(text)
	}

	var generic map[string]any
	if json.Unmarshal([]byte(raw), &generic) == nil {
		if output, ok := generic["output"].(string); ok {
			return summarizeTextResult(output)
		}
		if content, ok := generic["content"].(string); ok {
			return summarizeTextResult(content)
		}
	}

	return summarizeTextResult(raw)
}

func summarizeTextResult(text string) (string, []string) {
	clean := strings.TrimSpace(text)
	if clean == "" {
		return "Completed", nil
	}

	lines := cleanLines(clean)
	if len(lines) == 0 {
		return "Completed", nil
	}
	if len(lines) == 1 && lipgloss.Width(lines[0]) <= 120 {
		return lines[0], nil
	}

	return fmt.Sprintf("%d lines", len(lines)), headLines(lines, 4)
}

// ─── Helpers ────────────────────────────────────────────────────

func previewCommand(s string, maxWidth int) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.Join(strings.Fields(s), " ")
	return truncateOneLine(s, maxWidth)
}

func decodeJSONString(raw string) (string, bool) {
	var out string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return "", false
	}
	return out, true
}

func cleanLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	rawLines := strings.Split(text, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		line = strings.TrimRight(line, " \t")
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func headLines(lines []string, n int) []string {
	if len(lines) <= n {
		return lines
	}
	return lines[:n]
}

func tailLines(lines []string, n int) []string {
	if len(lines) <= n {
		return lines
	}
	return lines[len(lines)-n:]
}

func gutterBlock(firstPrefix, nextPrefix, content string) string {
	content = strings.TrimRight(content, "\n")
	if content == "" {
		return ""
	}

	lines := strings.Split(content, "\n")
	var b strings.Builder
	for i, line := range lines {
		prefix := nextPrefix
		if i == 0 {
			prefix = firstPrefix
		}
		b.WriteString(prefix)
		b.WriteString(line)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderNestedBlock(lines []string, prefix string) string {
	var b strings.Builder
	for _, line := range lines {
		for _, part := range strings.Split(strings.TrimRight(line, "\n"), "\n") {
			b.WriteString(prefix)
			b.WriteString(part)
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func humanizeToolName(name string) string {
	name = strings.ReplaceAll(name, "_", " ")
	parts := strings.Fields(name)
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func formatTokenCount(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%dk tokens", n/1000)
	}
	return fmt.Sprintf("%d tokens", n)
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return d.Round(time.Millisecond).String()
}

func truncateOneLine(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(strings.Join(strings.Fields(s), " "))
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func shortenPath(p string) string {
	if p == "" {
		return ""
	}

	if abs, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, abs) {
		return "~" + p[len(abs):]
	}
	return p
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
