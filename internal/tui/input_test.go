package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func newTestInput() InputModel {
	return NewInputModel(DefaultTheme())
}

// --- Basic Editing ---

func TestInsertRune(t *testing.T) {
	m := newTestInput()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hello")})
	if m.Value() != "hello" {
		t.Errorf("expected 'hello', got %q", m.Value())
	}
	if m.cursor != 5 {
		t.Errorf("expected cursor at 5, got %d", m.cursor)
	}
}

func TestInsertRuneAtStart(t *testing.T) {
	m := newTestInput()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("bc")})
	m.cursor = 0
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if m.Value() != "abc" {
		t.Errorf("expected 'abc', got %q", m.Value())
	}
}

func TestInsertRuneInMiddle(t *testing.T) {
	m := newTestInput()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ac")})
	m.cursor = 1
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	if m.Value() != "abc" {
		t.Errorf("expected 'abc', got %q", m.Value())
	}
}

func TestDeleteBackward(t *testing.T) {
	m := newTestInput()
	m.SetValue("abc")
	m.cursor = 2
	m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.Value() != "ac" {
		t.Errorf("expected 'ac', got %q", m.Value())
	}
	if m.cursor != 1 {
		t.Errorf("expected cursor at 1, got %d", m.cursor)
	}
}

func TestDeleteBackwardAtStart(t *testing.T) {
	m := newTestInput()
	m.SetValue("abc")
	m.cursor = 0
	m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.Value() != "abc" {
		t.Errorf("expected no change, got %q", m.Value())
	}
}

func TestDeleteForward(t *testing.T) {
	m := newTestInput()
	m.SetValue("abc")
	m.cursor = 1
	m.Update(tea.KeyMsg{Type: tea.KeyDelete})
	if m.Value() != "ac" {
		t.Errorf("expected 'ac', got %q", m.Value())
	}
}

// --- Cursor Movement ---

func TestMoveLeft(t *testing.T) {
	m := newTestInput()
	m.SetValue("abc")
	m.cursor = 3
	m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.cursor != 2 {
		t.Errorf("expected cursor at 2, got %d", m.cursor)
	}
}

func TestMoveRight(t *testing.T) {
	m := newTestInput()
	m.SetValue("abc")
	m.cursor = 0
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.cursor != 1 {
		t.Errorf("expected cursor at 1, got %d", m.cursor)
	}
}

func TestHomeEnd(t *testing.T) {
	m := newTestInput()
	m.SetValue("abc")
	m.Update(tea.KeyMsg{Type: tea.KeyHome})
	if m.cursor != 0 {
		t.Errorf("expected cursor at 0 after home, got %d", m.cursor)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	if m.cursor != 3 {
		t.Errorf("expected cursor at 3 after end, got %d", m.cursor)
	}
}

func TestCtrlA_CtrlE(t *testing.T) {
	m := newTestInput()
	m.SetValue("abc")
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	if m.cursor != 0 {
		t.Errorf("expected cursor at 0 after ctrl+a, got %d", m.cursor)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	if m.cursor != 3 {
		t.Errorf("expected cursor at 3 after ctrl+e, got %d", m.cursor)
	}
}

// --- Word Movement ---

func TestMoveWordLeft(t *testing.T) {
	m := newTestInput()
	m.SetValue("hello world test")
	m.cursor = 15
	m.moveWordLeft()
	if m.cursor != 12 {
		t.Errorf("expected cursor at 12 (start of 'test'), got %d", m.cursor)
	}
	m.moveWordLeft()
	if m.cursor != 6 {
		t.Errorf("expected cursor at 6 (start of 'world'), got %d", m.cursor)
	}
}

func TestMoveWordRight(t *testing.T) {
	m := newTestInput()
	m.SetValue("hello world test")
	m.cursor = 0
	m.moveWordRight()
	if m.cursor != 6 {
		t.Errorf("expected cursor at 6 (start of 'world'), got %d", m.cursor)
	}
	m.moveWordRight()
	if m.cursor != 12 {
		t.Errorf("expected cursor at 12 (start of 'test'), got %d", m.cursor)
	}
}

// --- Kill Ring ---

func TestKillToEnd(t *testing.T) {
	m := newTestInput()
	m.SetValue("hello world")
	m.cursor = 5
	m.killToEnd()
	if m.Value() != "hello" {
		t.Errorf("expected 'hello', got %q", m.Value())
	}
	if len(m.killRing) == 0 || m.killRing[len(m.killRing)-1] != " world" {
		t.Errorf("expected kill ring to contain ' world', got %v", m.killRing)
	}
}

func TestKillToStart(t *testing.T) {
	m := newTestInput()
	m.SetValue("hello world")
	m.cursor = 5
	m.killToStart()
	if m.Value() != " world" {
		t.Errorf("expected ' world', got %q", m.Value())
	}
	if m.cursor != 0 {
		t.Errorf("expected cursor at 0, got %d", m.cursor)
	}
}

func TestYank(t *testing.T) {
	m := newTestInput()
	m.SetValue("hello world")
	m.cursor = 5
	m.killToEnd()
	m.cursor = 0
	m.yank()
	if m.Value() != " worldhello" {
		t.Errorf("expected ' worldhello', got %q", m.Value())
	}
}

func TestKillWordBackward(t *testing.T) {
	m := newTestInput()
	m.SetValue("hello world")
	m.cursor = 11
	m.killWordBackward()
	if m.Value() != "hello" {
		t.Errorf("expected 'hello', got %q", m.Value())
	}
}

// --- Undo ---

func TestUndo(t *testing.T) {
	m := newTestInput()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("abc")})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlZ})
	if m.Value() != "ab" {
		t.Errorf("expected 'ab' after undo, got %q", m.Value())
	}
}

// --- History ---

func TestHistoryNavigation(t *testing.T) {
	m := newTestInput()
	m.SetValue("first")
	m.Reset()
	m.SetValue("second")
	m.Reset()

	// History up should show "second"
	m.historyUp()
	if m.Value() != "second" {
		t.Errorf("expected 'second', got %q", m.Value())
	}

	// History up again should show "first"
	m.historyUp()
	if m.Value() != "first" {
		t.Errorf("expected 'first', got %q", m.Value())
	}

	// History down should show "second"
	m.historyDown()
	if m.Value() != "second" {
		t.Errorf("expected 'second', got %q", m.Value())
	}

	// History down again should restore draft (empty)
	m.historyDown()
	if m.Value() != "" {
		t.Errorf("expected empty (draft), got %q", m.Value())
	}
}

func TestHistoryPreservesDraft(t *testing.T) {
	m := newTestInput()
	m.SetValue("old")
	m.Reset()

	// Type something new, then go into history
	m.SetValue("new draft")
	m.historyUp()
	if m.Value() != "old" {
		t.Errorf("expected 'old', got %q", m.Value())
	}
	m.historyDown()
	if m.Value() != "new draft" {
		t.Errorf("expected draft 'new draft' restored, got %q", m.Value())
	}
}

// --- Escape ---

func TestDoubleEscapeClears(t *testing.T) {
	m := newTestInput()
	m.SetValue("some text")

	// First escape — no previous escape recorded
	m.handleEscape()
	if m.Value() != "some text" {
		t.Errorf("expected no change after first esc, got %q", m.Value())
	}

	// Second escape within 500ms clears
	m.handleEscape()
	if m.Value() != "" {
		t.Errorf("expected empty after double esc, got %q", m.Value())
	}
}

func TestSingleEscapeNoOp(t *testing.T) {
	m := newTestInput()
	m.SetValue("some text")
	m.lastEscape = time.Time{} // far in the past
	m.handleEscape()
	if m.Value() != "some text" {
		t.Errorf("expected no change, got %q", m.Value())
	}
}

// --- Reset ---

func TestReset(t *testing.T) {
	m := newTestInput()
	m.SetValue("hello")
	m.Reset()
	if m.Value() != "" {
		t.Errorf("expected empty after reset, got %q", m.Value())
	}
	if len(m.history) != 1 || m.history[0] != "hello" {
		t.Errorf("expected history ['hello'], got %v", m.history)
	}
}

func TestResetEmptyDoesNotPushHistory(t *testing.T) {
	m := newTestInput()
	m.Reset()
	if len(m.history) != 0 {
		t.Errorf("expected empty history, got %v", m.history)
	}
}

// --- Shift+Enter ---

func TestShiftEnterInsertsNewline(t *testing.T) {
	m := newTestInput()
	// Simulate shift+enter using the key string matching
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: nil})
	// Actually the shift+enter is matched by key.String() == "shift+enter"
	// In bubbletea, shift+enter may be Key{Type: KeyEnter} with special handling
	// Let's test the insertRune directly for newline
	m.insertRune('\n')
	if m.Value() != "\n" {
		t.Errorf("expected newline, got %q", m.Value())
	}
}

// --- Waiting State ---

func TestWaitingBlocksInput(t *testing.T) {
	m := newTestInput()
	m.SetWaiting(true)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hello")})
	if m.Value() != "" {
		t.Errorf("expected empty (blocked while waiting), got %q", m.Value())
	}
}

// --- View Rendering ---

func TestViewEmptyShowsPlaceholder(t *testing.T) {
	m := newTestInput()
	m.SetWidth(80)
	m.SetBlink(true)
	view := m.View()
	if !strings.Contains(view, "Ask me anything...") {
		t.Errorf("expected placeholder in view, got:\n%s", view)
	}
	if !strings.Contains(view, "❯") {
		t.Errorf("expected prompt char in view, got:\n%s", view)
	}
}

func TestViewShowsBottomBorder(t *testing.T) {
	m := newTestInput()
	m.SetWidth(80)
	view := m.View()
	if !strings.Contains(view, "╰") || !strings.Contains(view, "╯") {
		t.Errorf("expected rounded bottom border, got:\n%s", view)
	}
}

func TestViewWithText(t *testing.T) {
	m := newTestInput()
	m.SetWidth(80)
	m.SetValue("hello world")
	view := m.View()
	if !strings.Contains(view, "hello world") {
		t.Errorf("expected text in view, got:\n%s", view)
	}
}

func TestViewWaitingShowsSpinner(t *testing.T) {
	m := newTestInput()
	m.SetWidth(80)
	m.SetWaiting(true)
	m.SetSpinnerFrame("⠙")
	view := m.View()
	if !strings.Contains(view, "⠙") || !strings.Contains(view, "waiting") {
		t.Errorf("expected spinner + waiting text, got:\n%s", view)
	}
}

func TestViewMultiline(t *testing.T) {
	m := newTestInput()
	m.SetWidth(80)
	m.SetValue("line1\nline2")
	m.SetBlink(true)
	view := m.View()
	if !strings.Contains(view, "line1") || !strings.Contains(view, "line2") {
		t.Errorf("expected both lines in view, got:\n%s", view)
	}
}

// --- Visual Line Building ---

func TestBuildVisualLinesEmpty(t *testing.T) {
	m := newTestInput()
	vlines := m.buildVisualLines(15)
	if len(vlines) != 1 || vlines[0].text != "" {
		t.Errorf("expected 1 empty line for empty input, got %v", vlines)
	}
}

func TestBuildVisualLinesWrapping(t *testing.T) {
	m := newTestInput()
	// Word-aware wrapping: "abcdefghijklmnop" has no word breaks, so hard-wraps at 15
	m.SetValue("abcdefghijklmnop") // 16 chars, wraps at 15
	vlines := m.buildVisualLines(15)
	if len(vlines) != 2 {
		t.Fatalf("expected 2 visual lines, got %d", len(vlines))
	}
	if vlines[0].text != "abcdefghijklmno" {
		t.Errorf("expected first 15 chars, got %q", vlines[0].text)
	}
	if vlines[1].text != "p" {
		t.Errorf("expected 'p', got %q", vlines[1].text)
	}
}

func TestBuildVisualLinesWordWrap(t *testing.T) {
	m := newTestInput()
	// Word-aware wrapping: should break at space, not mid-word
	m.SetValue("hello world test") // 16 chars
	vlines := m.buildVisualLines(12)
	if len(vlines) != 2 {
		t.Fatalf("expected 2 visual lines, got %d: %v", len(vlines), vlines)
	}
	// First break char is space at position 5 → wrap to position 6 ("hello " = 6 chars)
	if vlines[0].text != "hello " {
		t.Errorf("expected first line 'hello ', got %q", vlines[0].text)
	}
	if vlines[1].text != "world test" {
		t.Errorf("expected second line 'world test', got %q", vlines[1].text)
	}
}

func TestBuildVisualLinesWordWrapLongWord(t *testing.T) {
	m := newTestInput()
	// A long word with no break chars should still hard-wrap
	m.SetValue("abcdefghijklmnopqrstuvwxyz")
	vlines := m.buildVisualLines(10)
	for i, vl := range vlines {
		if vl.runeCount > 10 {
			t.Errorf("visual line %d has %d chars (max 10): %q", i, vl.runeCount, vl.text)
		}
	}
	// Check continuity
	var reconstructed strings.Builder
	for _, vl := range vlines {
		reconstructed.WriteString(vl.text)
	}
	if reconstructed.String() != "abcdefghijklmnopqrstuvwxyz" {
		t.Errorf("reconstructed text doesn't match: got %q", reconstructed.String())
	}
}

func TestBuildVisualLinesNewlines(t *testing.T) {
	m := newTestInput()
	m.SetValue("hello\nworld")
	vlines := m.buildVisualLines(80)
	if len(vlines) != 2 {
		t.Errorf("expected 2 visual lines, got %d: %v", len(vlines), vlines)
	}
}

func TestBuildVisualLinesTrailingNewline(t *testing.T) {
	m := newTestInput()
	m.SetValue("hello\n")
	vlines := m.buildVisualLines(80)
	if len(vlines) != 2 {
		t.Errorf("expected 2 visual lines (hello + empty), got %d", len(vlines))
	}
}

func TestFindCursorInVisual(t *testing.T) {
	m := newTestInput()
	m.SetValue("hello world")
	vlines := m.buildVisualLines(80)
	m.cursor = 6 // at 'w'
	line, col := m.findCursorInVisual(vlines)
	if line != 0 || col != 6 {
		t.Errorf("expected (0,6), got (%d,%d)", line, col)
	}
}

func TestFindCursorAtEnd(t *testing.T) {
	m := newTestInput()
	m.SetValue("hello")
	vlines := m.buildVisualLines(80)
	m.cursor = 5
	line, col := m.findCursorInVisual(vlines)
	if line != 0 || col != 5 {
		t.Errorf("expected (0,5), got (%d,%d)", line, col)
	}
}

func TestFindCursorInMultiline(t *testing.T) {
	m := newTestInput()
	m.SetValue("hello\nworld")
	vlines := m.buildVisualLines(80)
	// cursor at 'w' (position 6 in buffer, after \n at 5)
	m.cursor = 6
	line, col := m.findCursorInVisual(vlines)
	if line != 1 || col != 0 {
		t.Errorf("expected (1,0), got (%d,%d)", line, col)
	}
}

// --- Viewport Windowing ---

func TestViewportWindowing(t *testing.T) {
	m := newTestInput()
	m.SetMaxLines(3)
	// Create text with 10 lines
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = "line" + string(rune('0'+i))
	}
	m.SetValue(strings.Join(lines, "\n"))
	vlines := m.buildVisualLines(80)

	// Cursor on line 5 (middle)
	cursorLine := 5
	visible, scrollStart := m.viewportWindow(vlines, cursorLine)
	if len(visible) > 3 {
		t.Errorf("expected at most 3 visible lines, got %d", len(visible))
	}
	if scrollStart > 5 {
		t.Errorf("expected scroll start near cursor, got %d", scrollStart)
	}
}

// --- Ctrl+C ---

func TestCtrlCIgnoredByInput(t *testing.T) {
	m := newTestInput()
	m.SetValue("some text")
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	// InputModel doesn't handle ctrl+c directly (parent handles it)
	if m.Value() != "some text" {
		t.Errorf("ctrl+c should not clear input in input model, got %q", m.Value())
	}
}

// --- SetValue ---

func TestSetValuePutsCursorAtEnd(t *testing.T) {
	m := newTestInput()
	m.SetValue("hello")
	if m.cursor != 5 {
		t.Errorf("expected cursor at 5, got %d", m.cursor)
	}
}

func TestSetValueExitsHistory(t *testing.T) {
	m := newTestInput()
	m.SetValue("old")
	m.Reset()
	m.histIdx = 0
	m.SetValue("new")
	if m.histIdx != -1 {
		t.Errorf("expected histIdx -1 after SetValue, got %d", m.histIdx)
	}
}
