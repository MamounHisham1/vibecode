package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const promptChar = "❯"
const promptPad = 3 // "❯ " = 2 display columns + 1 space

type undoEntry struct {
	value  []rune
	cursor int
}

// InputModel is a full-featured text input with cursor movement, kill ring,
// undo, history, multi-line support, viewport windowing, and Claude Code-style rendering.
type InputModel struct {
	value        []rune
	cursor       int
	width        int
	maxLines     int
	blinkOn      bool
	focused      bool
	waiting      bool
	spinnerFrame string

	history   []string
	histIdx   int
	histDraft string

	killRing []string

	undoStack []undoEntry

	lastEscape time.Time

	theme       Theme
	placeholder string
}

func NewInputModel(theme Theme) InputModel {
	return InputModel{
		value:       make([]rune, 0),
		cursor:      0,
		width:       80,
		maxLines:    15,
		blinkOn:     true,
		focused:     true,
		history:     make([]string, 0),
		histIdx:     -1,
		killRing:    make([]string, 0),
		undoStack:   make([]undoEntry, 0),
		placeholder: "Ask me anything...",
		theme:       theme,
	}
}

// --- Public API ---

func (m *InputModel) Value() string {
	return string(m.value)
}

func (m *InputModel) SetValue(s string) {
	m.value = []rune(s)
	m.cursor = len(m.value)
	m.exitHistory()
}

func (m *InputModel) Reset() {
	if len(m.value) > 0 {
		m.history = append(m.history, string(m.value))
		if len(m.history) > 1000 {
			m.history = m.history[1:]
		}
	}
	m.value = make([]rune, 0)
	m.cursor = 0
	m.histIdx = -1
	m.undoStack = m.undoStack[:0]
}

func (m *InputModel) SetWidth(w int) {
	if w > 0 {
		m.width = w
	}
}

func (m *InputModel) SetMaxLines(n int) {
	if n >= 2 {
		m.maxLines = n
	}
}

func (m *InputModel) SetWaiting(w bool) {
	m.waiting = w
}

func (m *InputModel) SetBlink(on bool) {
	m.blinkOn = on
}

func (m *InputModel) SetSpinnerFrame(frame string) {
	m.spinnerFrame = frame
}

func (m *InputModel) IsEmpty() bool {
	return len(m.value) == 0
}

// --- Update / Key Handling ---

func (m *InputModel) Update(msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}

	if m.waiting {
		return nil
	}

	// Shift+enter inserts newline
	if key.String() == "shift+enter" {
		m.saveUndo()
		m.insertRune('\n')
		m.exitHistory()
		return nil
	}

	// Space key
	if key.Type == tea.KeySpace {
		m.saveUndo()
		m.insertRune(' ')
		m.exitHistory()
		return nil
	}

	// Regular rune input
	if key.Type == tea.KeyRunes {
		for _, r := range key.Runes {
			if r == '\n' {
				continue
			}
			m.saveUndo()
			m.insertRune(r)
		}
		m.exitHistory()
		return nil
	}

	switch key.String() {
	case "left":
		m.moveLeft()
	case "right":
		m.moveRight()
	case "home", "ctrl+a":
		m.cursor = m.startOfCurrentLine()
	case "end", "ctrl+e":
		m.cursor = m.endOfCurrentLine()
	case "ctrl+left", "alt+b":
		m.moveWordLeft()
	case "ctrl+right", "alt+f":
		m.moveWordRight()
	case "backspace":
		m.saveUndo()
		m.deleteBackward()
		m.exitHistory()
	case "delete", "ctrl+d":
		if m.cursor >= len(m.value) && len(m.value) == 0 {
			return nil
		}
		m.saveUndo()
		m.deleteForward()
		m.exitHistory()
	case "ctrl+k":
		m.killToEnd()
	case "ctrl+u":
		m.killToStart()
	case "ctrl+w":
		m.killWordBackward()
	case "ctrl+y":
		m.yank()
	case "ctrl+z":
		m.undo()
	case "up", "ctrl+p":
		m.historyUp()
	case "down", "ctrl+n":
		m.historyDown()
	case "esc":
		m.handleEscape()
	}

	return nil
}

// --- Cursor Movement ---

func (m *InputModel) moveLeft() {
	if m.cursor > 0 {
		m.cursor--
	}
}

func (m *InputModel) moveRight() {
	if m.cursor < len(m.value) {
		m.cursor++
	}
}

func (m *InputModel) moveWordLeft() {
	pos := m.cursor - 1
	for pos >= 0 && isWordBreak(m.value[pos]) {
		pos--
	}
	for pos >= 0 && !isWordBreak(m.value[pos]) {
		pos--
	}
	m.cursor = pos + 1
}

func (m *InputModel) moveWordRight() {
	pos := m.cursor
	for pos < len(m.value) && !isWordBreak(m.value[pos]) {
		pos++
	}
	for pos < len(m.value) && isWordBreak(m.value[pos]) {
		pos++
	}
	m.cursor = pos
}

// startOfCurrentLine finds the start of the logical line the cursor is on.
func (m *InputModel) startOfCurrentLine() int {
	i := m.cursor - 1
	for i >= 0 && m.value[i] != '\n' {
		i--
	}
	return i + 1
}

// endOfCurrentLine finds the end of the logical line the cursor is on.
func (m *InputModel) endOfCurrentLine() int {
	i := m.cursor
	for i < len(m.value) && m.value[i] != '\n' {
		i++
	}
	return i
}

func isWordBreak(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n'
}

// --- Editing ---

func (m *InputModel) insertRune(r rune) {
	m.value = append(m.value, 0)
	copy(m.value[m.cursor+1:], m.value[m.cursor:])
	m.value[m.cursor] = r
	m.cursor++
}

func (m *InputModel) deleteBackward() {
	if m.cursor == 0 {
		return
	}
	m.value = append(m.value[:m.cursor-1], m.value[m.cursor:]...)
	m.cursor--
}

func (m *InputModel) deleteForward() {
	if m.cursor >= len(m.value) {
		return
	}
	m.value = append(m.value[:m.cursor], m.value[m.cursor+1:]...)
}

// --- Kill Ring ---

func (m *InputModel) killToEnd() {
	if m.cursor >= len(m.value) {
		return
	}
	m.saveUndo()
	killed := string(m.value[m.cursor:])
	m.pushKill(killed)
	m.value = m.value[:m.cursor]
}

func (m *InputModel) killToStart() {
	if m.cursor == 0 {
		return
	}
	m.saveUndo()
	killed := string(m.value[:m.cursor])
	m.pushKill(killed)
	m.value = m.value[m.cursor:]
	m.cursor = 0
}

func (m *InputModel) killWordBackward() {
	start := m.cursor
	pos := m.cursor - 1
	// Skip word characters backward
	for pos >= 0 && !isWordBreak(m.value[pos]) {
		pos--
	}
	// Skip whitespace backward (include the space before the word)
	for pos >= 0 && isWordBreak(m.value[pos]) {
		pos--
	}
	end := pos + 1
	if end < start {
		m.saveUndo()
		killed := string(m.value[end:start])
		m.pushKill(killed)
		m.value = append(m.value[:end], m.value[start:]...)
		m.cursor = end
	}
}

func (m *InputModel) yank() {
	if len(m.killRing) == 0 {
		return
	}
	m.saveUndo()
	text := m.killRing[len(m.killRing)-1]
	for _, r := range text {
		m.insertRune(r)
	}
}

func (m *InputModel) pushKill(s string) {
	m.killRing = append(m.killRing, s)
	if len(m.killRing) > 32 {
		m.killRing = m.killRing[1:]
	}
}

// --- Undo ---

func (m *InputModel) saveUndo() {
	entry := undoEntry{
		value:  append([]rune(nil), m.value...),
		cursor: m.cursor,
	}
	m.undoStack = append(m.undoStack, entry)
	if len(m.undoStack) > 100 {
		m.undoStack = m.undoStack[1:]
	}
}

func (m *InputModel) undo() {
	if len(m.undoStack) == 0 {
		return
	}
	entry := m.undoStack[len(m.undoStack)-1]
	m.undoStack = m.undoStack[:len(m.undoStack)-1]
	m.value = entry.value
	m.cursor = entry.cursor
}

// --- History ---

func (m *InputModel) historyUp() {
	if len(m.history) == 0 {
		return
	}
	if m.histIdx == -1 {
		m.histDraft = string(m.value)
		m.histIdx = len(m.history) - 1
	} else if m.histIdx > 0 {
		m.histIdx--
	}
	m.value = []rune(m.history[m.histIdx])
	m.cursor = len(m.value)
}

func (m *InputModel) historyDown() {
	if m.histIdx == -1 {
		return
	}
	if m.histIdx < len(m.history)-1 {
		m.histIdx++
		m.value = []rune(m.history[m.histIdx])
		m.cursor = len(m.value)
	} else {
		m.histIdx = -1
		m.value = []rune(m.histDraft)
		m.cursor = len(m.value)
	}
}

func (m *InputModel) exitHistory() {
	m.histIdx = -1
}

// --- Escape ---

func (m *InputModel) handleEscape() {
	now := time.Now()
	if now.Sub(m.lastEscape) < 500*time.Millisecond {
		m.saveUndo()
		m.value = make([]rune, 0)
		m.cursor = 0
		m.exitHistory()
	}
	m.lastEscape = now
}

// --- View / Rendering ---

// visualLine tracks a wrapped line and which runes from the buffer it contains.
type visualLine struct {
	text      string
	startRune int
	runeCount int
}

func (m InputModel) View() string {
	if m.waiting {
		return m.renderWaiting()
	}

	contentWidth := m.width - promptPad - 2 // -2 for border sides
	if contentWidth < 20 {
		contentWidth = 20
	}

	if len(m.value) == 0 {
		return m.renderEmpty(contentWidth)
	}

	// Build visual lines from the rune buffer
	vlines := m.buildVisualLines(contentWidth)

	// Find cursor position in visual grid
	cursorLine, cursorCol := m.findCursorInVisual(vlines)

	// Viewport windowing
	visible, scrollStart := m.viewportWindow(vlines, cursorLine)
	visCursorLine := cursorLine - scrollStart

	// Render visible lines
	var b strings.Builder
	for i, vl := range visible {
		if i > 0 {
			b.WriteString("\n")
		}
		if i == 0 && scrollStart == 0 {
			b.WriteString(m.theme.PromptChar.Render(promptChar) + " ")
		} else {
			b.WriteString(strings.Repeat(" ", promptPad))
		}

		if i == visCursorLine && m.blinkOn {
			b.WriteString(renderLineWithCursor(vl.text, cursorCol, m.theme))
		} else if i == visCursorLine && !m.blinkOn {
			b.WriteString(vl.text)
		} else {
			b.WriteString(vl.text)
		}
	}

	b.WriteString("\n")
	b.WriteString(m.renderBottomBorder())
	return b.String()
}

func (m InputModel) renderEmpty(contentWidth int) string {
	var b strings.Builder
	b.WriteString(m.theme.PromptChar.Render(promptChar) + " ")

	ph := m.placeholder
	if len(ph) == 0 {
		ph = " "
	}
	phRunes := []rune(ph)
	// Truncate placeholder to content width
	if len(phRunes) > contentWidth {
		phRunes = phRunes[:contentWidth]
	}

	if m.blinkOn {
		// Invert first char (cursor on placeholder)
		first := m.theme.InverseCursor.Render(string(phRunes[0]))
		rest := m.theme.InputHint.Render(string(phRunes[1:]))
		b.WriteString(first + rest)
	} else {
		b.WriteString(m.theme.InputHint.Render(string(phRunes)))
	}

	b.WriteString("\n")
	b.WriteString(m.renderBottomBorder())
	return b.String()
}

func (m InputModel) renderWaiting() string {
	t := m.theme
	var b strings.Builder
	b.WriteString(t.BrandDim.Render(promptChar+" ") + " ")
	frame := m.spinnerFrame
	if frame == "" {
		frame = "⠋"
	}
	b.WriteString(t.AssistantDot.Render(frame+" ") + t.Dim.Render("waiting..."))
	b.WriteString("\n")
	b.WriteString(m.renderBottomBorder())
	return b.String()
}

func (m InputModel) renderBottomBorder() string {
	t := m.theme
	borderStyle := t.PromptBorder
	if m.waiting {
		borderStyle = t.InputBorderDim
	}
	borderWidth := m.width
	if borderWidth < 4 {
		borderWidth = 4
	}
	inner := strings.Repeat("─", borderWidth-2)
	return borderStyle.Render("╰" + inner + "╯")
}

// buildVisualLines wraps the rune buffer into visual lines, breaking at
// word boundaries when possible. If a single word is longer than
// contentWidth it is still broken (hard-wrap) so the cursor remains
// well-behaved, but normal text flows at spaces and punctuation.
func (m InputModel) buildVisualLines(contentWidth int) []visualLine {
	if contentWidth < 1 {
		contentWidth = 1
	}

	var vlines []visualLine
	runeIdx := 0

	// wordBreakRunes lists characters that are valid line-break points.
	isBreak := func(r rune) bool {
		return r == ' ' || r == '\t' || r == '-' || r == '_' || r == '.' || r == ',' || r == ';' || r == ':' || r == '/' || r == '\\' || r == '|' || r == '(' || r == ')' || r == '[' || r == ']' || r == '{' || r == '}' || r == '=' || r == '+' || r == '*' || r == '&' || r == '<' || r == '>'
	}

	// Split by newlines into logical lines, then wrap each
	for runeIdx <= len(m.value) {
		if runeIdx == len(m.value) {
			break
		}

		// Find end of logical line (next newline or end)
		lineEnd := runeIdx
		for lineEnd < len(m.value) && m.value[lineEnd] != '\n' {
			lineEnd++
		}
		lineRunes := m.value[runeIdx:lineEnd]

		if len(lineRunes) == 0 {
			vlines = append(vlines, visualLine{
				text:      "",
				startRune: runeIdx,
				runeCount: 0,
			})
			runeIdx = lineEnd + 1 // skip the newline
			continue
		}

		// Wrap this logical line at contentWidth, respecting word boundaries
		start := 0
		for start < len(lineRunes) {
			remaining := len(lineRunes) - start
			if remaining <= contentWidth {
				// Rest fits on one line
				chunk := lineRunes[start:]
				vlines = append(vlines, visualLine{
					text:      string(chunk),
					startRune: runeIdx + start,
					runeCount: len(chunk),
				})
				start = len(lineRunes)
				continue
			}

			// Look for a word-break character in the range [start, start+contentWidth)
			breakAt := -1
			limit := start + contentWidth
			if limit > len(lineRunes) {
				limit = len(lineRunes)
			}
			for i := start; i < limit; i++ {
				if isBreak(lineRunes[i]) {
					breakAt = i
					break
				}
			}

			if breakAt >= 0 {
				// Found a word boundary — wrap after the break character
				chunk := lineRunes[start : breakAt+1]
				vlines = append(vlines, visualLine{
					text:      string(chunk),
					startRune: runeIdx + start,
					runeCount: len(chunk),
				})
				start = breakAt + 1
			} else {
				// No word boundary found — hard wrap at contentWidth
				end := start + contentWidth
				if end > len(lineRunes) {
					end = len(lineRunes)
				}
				chunk := lineRunes[start:end]
				vlines = append(vlines, visualLine{
					text:      string(chunk),
					startRune: runeIdx + start,
					runeCount: len(chunk),
				})
				start = end
			}
		}

		runeIdx = lineEnd
		if runeIdx < len(m.value) && m.value[runeIdx] == '\n' {
			runeIdx++
		}
	}

	// Handle case where buffer ends with newline (trailing empty line)
	if len(m.value) > 0 && m.value[len(m.value)-1] == '\n' {
		vlines = append(vlines, visualLine{
			text:      "",
			startRune: len(m.value),
			runeCount: 0,
		})
	}

	if len(vlines) == 0 {
		vlines = append(vlines, visualLine{
			text:      "",
			startRune: 0,
			runeCount: 0,
		})
	}

	return vlines
}

// findCursorInVisual returns (visual line index, column within that line) for the cursor.
func (m InputModel) findCursorInVisual(vlines []visualLine) (int, int) {
	for i, vl := range vlines {
		if m.cursor >= vl.startRune && m.cursor < vl.startRune+vl.runeCount {
			col := m.cursor - vl.startRune
			return i, col
		}
		// Cursor at the end of a line that is followed by a newline
		if m.cursor == vl.startRune+vl.runeCount {
			// Check if next line starts after a newline (meaning cursor is after the last char of this logical line)
			nextStart := vl.startRune + vl.runeCount
			if nextStart < len(m.value) && m.value[nextStart] == '\n' {
				return i, vl.runeCount
			}
		}
	}

	// Cursor at end of text
	lastIdx := len(vlines) - 1
	if lastIdx >= 0 {
		vl := vlines[lastIdx]
		col := m.cursor - vl.startRune
		if col < 0 {
			col = 0
		}
		return lastIdx, col
	}
	return 0, 0
}

// viewportWindow returns visible lines and the scroll offset.
func (m InputModel) viewportWindow(vlines []visualLine, cursorLine int) ([]visualLine, int) {
	max := m.maxLines
	if max < 2 {
		max = 2
	}
	if len(vlines) <= max {
		return vlines, 0
	}

	half := max / 2
	start := cursorLine - half
	if start < 0 {
		start = 0
	}
	if start+max > len(vlines) {
		start = len(vlines) - max
	}

	return vlines[start : start+max], start
}

// renderLineWithCursor renders a line with an inverted cursor block at col.
func renderLineWithCursor(line string, col int, t Theme) string {
	runes := []rune(line)

	if col >= len(runes) {
		// Cursor past end: inverted space
		return line + t.InverseCursor.Render(" ")
	}

	before := string(runes[:col])
	cursorChar := string(runes[col])
	after := string(runes[col+1:])

	return before + t.InverseCursor.Render(cursorChar) + after
}

// unused but kept for potential future use
var _ = ansi.StringWidth
var _ = lipgloss.NewStyle
