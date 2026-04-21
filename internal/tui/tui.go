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
	"github.com/vibecode/vibecode/config"
	"github.com/vibecode/vibecode/internal/commands"
	"github.com/vibecode/vibecode/internal/openrouter"
	"github.com/vibecode/vibecode/internal/provider"
	"github.com/vibecode/vibecode/internal/session"
	"github.com/vibecode/vibecode/internal/tool"
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

type providerSetupPhase int

const (
	providerSetupNone providerSetupPhase = iota
	providerSetupPicker
	providerSetupEndpoint
	providerSetupKeyInput
)

type Model struct {
	theme Theme
	input InputModel

	entries      []transcriptItem
	streamBuf    *strings.Builder
	toolIndex    map[string]int
	status       string
	modelName    string
	providerName string
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

	// Session stats
	sessionStart time.Time
	turnCount    int
	tokenUsage   session.SessionUsage

	// Cancellation
	cancelFunc  context.CancelFunc
	interruptCh chan struct{}

	// Command handler
	commandHandler func(cmdName, args string) (string, bool) // returns (output, clearHistory)

	// Model change handler: called when user selects a new model from the picker.
	// Returns an error if the provider could not be built.
	modelChangeHandler func(providerID, modelID, baseURL string) error

	// Autocomplete
	autocomplete AutocompleteModel
	cmdRegistry  *commands.Registry

	// Model picker
	modelPicker ModelPicker

	// Provider picker
	providerPicker ProviderPicker

	// Provider setup flow state
	providerSetup       providerSetupPhase
	setupProvider       ProviderInfo
	setupEndpointSelected int
	setupInput          []rune
	setupInputCur       int
	setupBlinkOn        bool

	// API key filter for model/provider pickers
	hasAPIKey func(string) bool

	// Config access for saving API keys during provider setup
	config *config.Config

	// Ask user question state
	askQuestion string
	askOptions  []string
	askAnswer   chan string

	// Per-tool expand/collapse: maps tool ID to its start line in the rendered view
	toolStartLines map[string]int

	// Plan mode
	planMode bool

	// Version display
	version string
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

type askQuestionMsg struct {
	question string
	options  []string
	answer   chan string
}

type planModeMsg struct {
	active bool
}

type tokenUsageMsg struct {
	usage session.SessionUsage
}

type compactionMsg struct {
	summary string
}

// ─── Constructor ────────────────────────────────────────────────

func New(inputChan chan<- string) *Model {
	theme := DefaultTheme()
	input := NewInputModel(theme)
	input.SetWidth(80)

	return &Model{
		theme:        theme,
		input:        input,
		autocomplete: NewAutocompleteModel(theme),
		modelPicker:      NewModelPicker(theme),
		providerPicker:   NewProviderPicker(theme),
		providerSetup:    providerSetupNone,
		setupInput:       make([]rune, 0),
		setupBlinkOn:     true,
		entries:          nil,
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

// refreshPickerMsg is sent when provider data has been fetched and the picker should refresh.
type refreshPickerMsg struct{}

func fetchProvidersForPickerCmd() tea.Msg {
	client := openrouter.NewClient()
	data, err := openrouter.GlobalCache.FetchOrGet(client)
	if err == nil {
		provider.BuildRegistryFromOpenRouter(data)
	}
	return refreshPickerMsg{}
}

// ─── Update ─────────────────────────────────────────────────────

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.SetWidth(m.inputTextWidth())
		m.input.SetMaxLines(m.height / 2)
		m.autocomplete.SetWidth(m.width)
		m.modelPicker.SetSize(m.width, m.height)
		m.providerPicker.SetSize(m.width, m.height)
		return m, nil

	case refreshPickerMsg:
		if m.providerPicker.Visible() {
			m.providerPicker.Open()
		}
		if m.modelPicker.Visible() {
			m.modelPicker.Open(m.providerName, m.modelName)
		}
		return m, nil

	case tea.KeyMsg:
		// Provider setup endpoint picker phase
		if m.providerSetup == providerSetupEndpoint {
			switch msg.String() {
			case "ctrl+c":
				m.providerSetup = providerSetupNone
				m.setupEndpointSelected = 0
				return m, nil
			case "esc":
				m.providerSetup = providerSetupNone
				m.setupEndpointSelected = 0
				return m, nil
			case "up", "ctrl+p":
				if m.setupEndpointSelected > 0 {
					m.setupEndpointSelected--
				}
				return m, nil
			case "down", "ctrl+n":
				if m.setupEndpointSelected < len(m.setupProvider.Endpoints)-1 {
					m.setupEndpointSelected++
				}
				return m, nil
			case "enter":
				chosen := m.setupProvider.Endpoints[m.setupEndpointSelected]
				m.setupProvider.BaseURL = chosen.BaseURL
				if chosen.APIType != "" {
					m.setupProvider.APIType = chosen.APIType
				}
				m.providerSetup = providerSetupNone
				m.setupEndpointSelected = 0
				// After choosing endpoint, check if key is needed
				if m.setupProvider.APIType == "ollama" || (m.hasAPIKey != nil && m.hasAPIKey(m.setupProvider.ID)) {
					m.modelPicker.OpenForProvider(m.setupProvider.ID, m.modelName)
					if len(Providers()) == 0 {
						return m, fetchProvidersForPickerCmd
					}
				} else {
					m.providerSetup = providerSetupKeyInput
					m.setupInput = m.setupInput[:0]
					m.setupInputCur = 0
				}
				return m, nil
			}
			return m, nil
		}
		// Provider setup key input phase
		if m.providerSetup == providerSetupKeyInput {
			switch msg.String() {
			case "ctrl+c":
				m.providerSetup = providerSetupNone
				m.setupInput = m.setupInput[:0]
				m.setupInputCur = 0
				return m, nil
			case "esc":
				m.providerSetup = providerSetupNone
				m.setupInput = m.setupInput[:0]
				m.setupInputCur = 0
				return m, nil
			case "enter":
				apiKey := string(m.setupInput)
				if apiKey != "" && m.config != nil {
					if m.config.APIKeys == nil {
						m.config.APIKeys = make(map[string]string)
					}
					m.config.APIKeys[m.setupProvider.ID] = apiKey
					_ = m.config.Save()
				}
				m.providerSetup = providerSetupNone
				m.setupInput = m.setupInput[:0]
				m.setupInputCur = 0
				m.modelPicker.OpenForProvider(m.setupProvider.ID, m.modelName)
				if len(Providers()) == 0 {
					return m, fetchProvidersForPickerCmd
				}
				return m, nil
			case "backspace":
				if m.setupInputCur > 0 {
					m.setupInput = append(m.setupInput[:m.setupInputCur-1], m.setupInput[m.setupInputCur:]...)
					m.setupInputCur--
				}
				return m, nil
			case "delete":
				if m.setupInputCur < len(m.setupInput) {
					m.setupInput = append(m.setupInput[:m.setupInputCur], m.setupInput[m.setupInputCur+1:]...)
				}
				return m, nil
			case "left":
				if m.setupInputCur > 0 {
					m.setupInputCur--
				}
				return m, nil
			case "right":
				if m.setupInputCur < len(m.setupInput) {
					m.setupInputCur++
				}
				return m, nil
			default:
				if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
					for _, r := range msg.Runes {
						m.setupInput = append(m.setupInput, 0)
						copy(m.setupInput[m.setupInputCur+1:], m.setupInput[m.setupInputCur:])
						m.setupInput[m.setupInputCur] = r
						m.setupInputCur++
					}
					return m, nil
				}
			}
			return m, nil
		}

		// Provider picker takes precedence when visible
		if m.providerPicker.Visible() {
			switch msg.String() {
			case "up", "ctrl+p":
				m.providerPicker.Up()
				return m, nil
			case "down", "ctrl+n":
				m.providerPicker.Down()
				return m, nil
			case "enter":
				if item, ok := m.providerPicker.Selected(); ok {
					m.providerPicker.Close()
					m.setupProvider = ProviderInfo{ID: item.ID, Name: item.Name}
					// Find full provider info from cached data
					for _, p := range Providers() {
						if p.ID == item.ID {
							m.setupProvider = p
							break
						}
					}
					// If provider has multiple endpoints, let user choose first
					if len(m.setupProvider.Endpoints) > 1 {
						m.providerSetup = providerSetupEndpoint
						m.setupEndpointSelected = 0
						return m, nil
					}
					// Otherwise proceed to key check or model picker
					if m.setupProvider.APIType == "ollama" || (m.hasAPIKey != nil && m.hasAPIKey(item.ID)) {
						m.modelPicker.OpenForProvider(item.ID, m.modelName)
						if len(Providers()) == 0 {
							return m, fetchProvidersForPickerCmd
						}
					} else {
						m.providerSetup = providerSetupKeyInput
						m.setupInput = m.setupInput[:0]
						m.setupInputCur = 0
					}
				}
				return m, nil
			case "esc", "ctrl+c":
				m.providerPicker.Close()
				return m, nil
			case "backspace":
				m.providerPicker.Backspace()
				return m, nil
			case "ctrl+u":
				m.providerPicker.ClearSearch()
				return m, nil
			case " ":
				m.providerPicker.TypeRune(' ')
				return m, nil
			default:
				if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
					for _, r := range msg.Runes {
						m.providerPicker.TypeRune(r)
					}
					return m, nil
				}
			}
			return m, nil
		}

		// Model picker takes precedence when visible
		if m.modelPicker.Visible() {
			switch msg.String() {
			case "up", "ctrl+p":
				m.modelPicker.Up()
				return m, nil
			case "down", "ctrl+n":
				m.modelPicker.Down()
				return m, nil
			case "enter":
				if item, ok := m.modelPicker.Selected(); ok {
					m.modelPicker.Close()
					if m.modelChangeHandler != nil {
						baseURL := ""
						if m.setupProvider.ID == item.ProviderID {
							baseURL = m.setupProvider.BaseURL
						}
						if err := m.modelChangeHandler(item.ProviderID, item.ModelID, baseURL); err != nil {
							m.appendSystemMessage(fmt.Sprintf("Failed to switch model: %s", err), true)
						} else {
							m.appendSystemMessage(fmt.Sprintf("Switched to %s (%s)", item.ModelName, item.ProviderName), false)
							m.SetStatus(item.ModelID, m.dir)
						}
					}
				}
				return m, nil
			case "esc", "ctrl+c":
				m.modelPicker.Close()
				return m, nil
			case "backspace":
				m.modelPicker.Backspace()
				return m, nil
			case "ctrl+u":
				m.modelPicker.ClearSearch()
				return m, nil
			case " ":
				m.modelPicker.TypeRune(' ')
				return m, nil
			default:
				// Route printable characters to the model picker search
				if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
					for _, r := range msg.Runes {
						m.modelPicker.TypeRune(r)
					}
					return m, nil
				}
			}
			return m, nil
		}

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
			m.autocomplete.Dismiss()
			return m, nil

		case "ctrl+o":
			m.toggleExpandAll()
			return m, nil

		case "tab":
			if m.autocomplete.Visible() {
				name := m.autocomplete.SelectedName()
				if name != "" {
					m.input.SetValue("/" + name + " ")
					m.autocomplete.Dismiss()
				}
				return m, nil
			}
			m.input.Update(msg)
			m.updateAutocomplete()
			return m, nil

		case "up", "ctrl+p":
			if m.autocomplete.Visible() {
				m.autocomplete.Up()
				return m, nil
			}
			m.input.Update(msg)
			m.updateAutocomplete()
			return m, nil

		case "down", "ctrl+n":
			if m.autocomplete.Visible() {
				m.autocomplete.Down()
				return m, nil
			}
			m.input.Update(msg)
			m.updateAutocomplete()
			return m, nil

		case "esc":
			if m.autocomplete.Visible() {
				m.autocomplete.Dismiss()
				return m, nil
			}
			m.input.Update(msg)
			m.updateAutocomplete()
			return m, nil

		case "enter":
			if m.waiting {
				return m, nil
			}

			// If autocomplete is visible, accept the selected suggestion
			if m.autocomplete.Visible() {
				name := m.autocomplete.SelectedName()
				if name != "" {
					m.input.SetValue("/" + name)
					m.autocomplete.Dismiss()

					// Special case: /model with no args opens the interactive picker
					if name == "model" {
						m.modelPicker.Open(m.providerName, m.modelName)
						m.input.Reset()
						m.welcome = false
						if len(Providers()) == 0 {
							return m, fetchProvidersForPickerCmd
						}
						return m, nil
					}

					// Special case: /providers opens the interactive provider picker
					if name == "providers" || name == "p" {
						m.providerPicker.Open()
						m.input.Reset()
						m.welcome = false
						if len(Providers()) == 0 {
							return m, fetchProvidersForPickerCmd
						}
						return m, nil
					}

					// Now process as a slash command
					if m.commandHandler != nil {
						output, clearHist := m.commandHandler(name, "")
						m.input.Reset()
						m.welcome = false
						if output != "" {
							m.appendSystemMessage(output, false)
						}
						if clearHist {
							m.entries = nil
						}
					}
				}
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

			// Check for slash commands
			if strings.HasPrefix(strings.TrimSpace(raw), "/") && m.commandHandler != nil {
				parts := strings.SplitN(strings.TrimSpace(raw)[1:], " ", 2)
				cmdName := parts[0]
				cmdArgs := ""
				if len(parts) > 1 {
					cmdArgs = parts[1]
				}

				// Special case: /model with no args opens the interactive picker
				if cmdName == "model" && cmdArgs == "" {
					m.modelPicker.Open(m.providerName, m.modelName)
					m.input.Reset()
					m.welcome = false
					if len(Providers()) == 0 {
						return m, fetchProvidersForPickerCmd
					}
					return m, nil
				}

				// Special case: /providers opens the interactive provider picker
				if (cmdName == "providers" || cmdName == "p") && cmdArgs == "" {
					m.providerPicker.Open()
					m.input.Reset()
					m.welcome = false
					if len(Providers()) == 0 {
						return m, fetchProvidersForPickerCmd
					}
					return m, nil
				}

				output, clearHist := m.commandHandler(cmdName, cmdArgs)
				m.input.Reset()
				m.welcome = false
				if output != "" {
					m.appendSystemMessage(output, false)
				}
				if clearHist {
					m.entries = nil
				}
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
			m.updateAutocomplete()
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
		} else if msg.Type == tea.MouseLeft {
			// Click on a tool entry to toggle expand/collapse
			m.toggleToolAtLine(msg.Y)
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

	case askQuestionMsg:
		m.askQuestion = msg.question
		m.askOptions = msg.options
		m.askAnswer = msg.answer
		m.waiting = false
		m.input.SetWaiting(false)
		return m, nil

	case planModeMsg:
		m.planMode = msg.active
		return m, nil

	case tokenUsageMsg:
		m.tokenUsage = msg.usage
		return m, nil

	case compactionMsg:
		m.appendSystemMessage("Context auto-compacted", false)
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

// toggleToolAtLine toggles expand/collapse for the tool entry at the given screen line.
func (m *Model) toggleToolAtLine(y int) {
	if m.toolStartLines == nil {
		return
	}

	// Find which tool entry this line falls within
	var bestID string
	var bestLine int
	for id, line := range m.toolStartLines {
		if line <= y && line > bestLine {
			bestID = id
			bestLine = line
		}
	}

	if bestID != "" {
		if m.expanded[bestID] {
			delete(m.expanded, bestID)
		} else {
			m.expanded[bestID] = true
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

	// Input area (or picker overlay when open, or key input during provider setup)
	b.WriteString("\n\n")
	if m.providerSetup == providerSetupEndpoint {
		b.WriteString(m.renderProviderEndpointPicker())
	} else if m.providerSetup == providerSetupKeyInput {
		b.WriteString(m.renderProviderKeyInput())
	} else if m.providerPicker.Visible() {
		b.WriteString(m.providerPicker.View())
	} else if m.modelPicker.Visible() {
		b.WriteString(m.modelPicker.View())
	} else {
		b.WriteString(m.renderInputArea())
	}

	// Token info below input
	if !m.modelPicker.Visible() && !m.providerPicker.Visible() && m.providerSetup != providerSetupKeyInput && m.providerSetup != providerSetupEndpoint {
		b.WriteString(m.renderTokenInfo())
	}

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

	if m.planMode {
		left += t.StatusBarBrand.Render(" PLAN MODE")
	}

	if m.status != "" {
		left += t.StatusBarInfo.Render(" " + m.status)
	}

	// Right side: token count + turn count + elapsed + hints
	elapsed := time.Since(m.sessionStart).Round(time.Second)
	right := ""
	if m.tokenUsage.TotalTokens() > 0 {
		right += t.StatusBarDim.Render(session.FormatTokenCount(m.tokenUsage.TotalTokens()) + " tokens")
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
	var b strings.Builder
	b.WriteString(m.input.View())
	if m.autocomplete.Visible() {
		b.WriteString("\n")
		b.WriteString(m.autocomplete.View())
	}
	return b.String()
}

func (m *Model) renderTokenInfo() string {
	if m.tokenUsage.TotalTokens() == 0 {
		return ""
	}
	t := m.theme

	var info string
	model := m.modelName
	if model != "" {
		mi := provider.LookupModel(model)
		if mi.Limits.Context > 0 {
			pct := float64(m.tokenUsage.TotalInput) / float64(mi.Limits.Context) * 100
			info = fmt.Sprintf("%s (%.0f%%)", session.FormatTokenCount(m.tokenUsage.TotalTokens()), pct)
		}
	}
	if info == "" {
		info = session.FormatTokenCount(m.tokenUsage.TotalTokens()) + " tokens"
	}

	return "\n" + t.StatusBarDim.Render("  " + info)
}

func (m *Model) renderAskQuestion() string {
	t := m.theme
	var b strings.Builder
	b.WriteString(t.AssistantIcon.Render("  ? ") + m.askQuestion)
	if len(m.askOptions) > 0 {
		b.WriteString("\n")
		for i, opt := range m.askOptions {
			b.WriteString(t.StatusBarDim.Render(fmt.Sprintf("    %d. %s\n", i+1, opt)))
		}
	}
	return b.String()
}

// renderProviderKeyInput renders the API key input prompt during provider setup.
func (m *Model) renderProviderEndpointPicker() string {
	t := m.theme
	var b strings.Builder

	boxWidth := min(m.width-8, 60)
	if boxWidth < 4 {
		boxWidth = 4
	}
	innerWidth := boxWidth - 4

	b.WriteString(t.Bold.Render("  Select Plan for ") + t.Brand.Render(m.setupProvider.Name) + "\n")
	b.WriteString(t.Separator.Render(strings.Repeat("─", innerWidth)) + "\n")

	for i, ep := range m.setupProvider.Endpoints {
		prefix := "   "
		nameStyle := t.Text
		if i == m.setupEndpointSelected {
			prefix = " ▸ "
			nameStyle = t.Suggestion
		}
		line := prefix + ep.Name
		b.WriteString(nameStyle.Render(line) + "\n")
	}

	b.WriteString("\n")
	b.WriteString(t.Dim.Render("  ↑/↓ navigate · enter select · esc back") + "\n")
	b.WriteString(t.Separator.Render(strings.Repeat("─", innerWidth)) + "\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#555555")).
		Padding(0, 1).
		Width(boxWidth).
		Render(b.String())

	return box
}

func (m *Model) renderProviderKeyInput() string {
	t := m.theme
	var b strings.Builder

	boxWidth := min(m.width-8, 60)
	if boxWidth < 4 {
		boxWidth = 4
	}
	innerWidth := boxWidth - 4

	b.WriteString(t.Bold.Render("  Configure ") + t.Brand.Render(m.setupProvider.Name) + "\n")
	b.WriteString(t.Separator.Render(strings.Repeat("─", innerWidth)) + "\n")
	b.WriteString("  " + t.Bold.Render("Enter your API key:") + "\n")
	b.WriteString("\n")

	// Masked input with cursor
	var inputLine strings.Builder
	for i := range m.setupInput {
		ch := "•"
		if i == m.setupInputCur && m.setupBlinkOn {
			inputLine.WriteString(t.InverseCursor.Render(ch))
		} else {
			inputLine.WriteString(ch)
		}
	}
	if m.setupInputCur == len(m.setupInput) && m.setupBlinkOn {
		inputLine.WriteString(t.InverseCursor.Render(" "))
	}

	if len(m.setupInput) == 0 {
		placeholder := "sk-..."
		if m.setupBlinkOn {
			inputLine.WriteString(t.InverseCursor.Render(string(placeholder[0])))
			inputLine.WriteString(t.InputHint.Render(placeholder[1:]))
		} else {
			inputLine.WriteString(t.InputHint.Render(placeholder))
		}
	}

	b.WriteString("  " + t.PromptChar.Render("❯ ") + inputLine.String() + "\n")
	b.WriteString("\n")
	b.WriteString(t.Dim.Render("  enter confirm · esc back") + "\n")
	b.WriteString(t.Separator.Render(strings.Repeat("─", innerWidth)) + "\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#555555")).
		Padding(0, 1).
		Width(boxWidth).
		Render(b.String())

	return box
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
	prefix := t.UserPointer.Render(userPointer + " ")
	indent := strings.Repeat(" ", lipgloss.Width(prefix))

	contentWidth := max(20, m.transcriptWidth()-lipgloss.Width(prefix))
	wrapped := wordwrap.String(strings.TrimRight(text, "\n"), contentWidth)
	lines := strings.Split(wrapped, "\n")

	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteString("\n")
		}
		var rawLine string
		if i == 0 {
			rawLine = prefix + t.UserText.Render(line)
		} else {
			rawLine = indent + t.UserText.Render(line)
		}
		padded := padLineToVisualWidth(rawLine, m.transcriptWidth())
		b.WriteString(t.UserBg.Render(padded))
	}
	return b.String()
}

func (m *Model) renderAssistantEntry(text string) string {
	t := m.theme
	dot := t.AssistantDot.Render(assistantDot + " ")
	mdWidth := max(20, m.transcriptWidth()-lipgloss.Width(dot))
	return gutterBlock(
		dot,
		"  ",
		RenderMarkdown(text, mdWidth),
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
						[]string{m.theme.Dim.Render(fmt.Sprintf("click or ctrl+o to expand (%d more lines)", len(tool.Preview)-maxShow))},
						m.theme.Subtle.Render("  "+toolPointer+"  "),
					))
				}
			} else {
				b.WriteString(renderNestedBlock(
					[]string{m.theme.Dim.Render("click or ctrl+o to expand")},
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
	w := m.transcriptWidth()

	b.WriteString("\n")

	// ── Header ──────────────────────────────────────────────
	// Compact brand header that works on narrow terminals
	brandLine := "  " + t.AssistantDot.Render(assistantDot+" ") + t.WelcomeTitle.Render("vibe code")
	if m.version != "" && m.version != "dev" {
		versionTag := t.WelcomeVersion.Render("v" + m.version)
		// Right-align version if there's room
		pad := w - lipgloss.Width(brandLine) - lipgloss.Width(versionTag) - 2
		if pad > 2 {
			brandLine += strings.Repeat(" ", pad) + versionTag
		} else {
			brandLine += "  " + versionTag
		}
	}
	b.WriteString(brandLine + "\n")
	b.WriteString("  " + t.WelcomeSubtitle.Render("AI-powered coding agent for your terminal") + "\n")
	b.WriteString("\n")

	// ── Session info ────────────────────────────────────────
	var infoParts []string
	if m.modelName != "" {
		infoParts = append(infoParts, t.Dim.Render("Model: ")+t.Text.Render(m.modelName))
	}
	if m.providerName != "" {
		infoParts = append(infoParts, t.Dim.Render("Provider: ")+t.Text.Render(m.providerName))
	}
	if m.dir != "" {
		infoParts = append(infoParts, t.Dim.Render("Dir: ")+t.Text.Render(shortenPath(m.dir)))
	}
	if len(infoParts) > 0 {
		infoLine := "  " + strings.Join(infoParts, t.Dim.Render("  ·  "))
		b.WriteString(infoLine + "\n")
		b.WriteString("\n")
	}

	// ── Keyboard Shortcuts ──────────────────────────────────
	b.WriteString("  " + t.Bold.Render("Keyboard shortcuts") + "\n")
	b.WriteString("\n")

	shortcuts := []struct {
		key  string
		desc string
	}{
		{"enter", "Send a message"},
		{"shift+enter", "New line (multi-line input)"},
		{"ctrl+o", "Expand/collapse tool output"},
		{"ctrl+c", "Stop generation or exit"},
		{"shift+↑ / pgup", "Scroll up"},
		{"shift+↓ / pgdn", "Scroll down"},
		{"tab", "Autocomplete slash commands"},
	}

	keyWidth := 0
	for _, s := range shortcuts {
		if kw := lipgloss.Width(s.key); kw > keyWidth {
			keyWidth = kw
		}
	}
	keyWidth += 2 // padding

	for _, s := range shortcuts {
		paddedKey := s.key + strings.Repeat(" ", max(0, keyWidth-lipgloss.Width(s.key)))
		b.WriteString("  " + t.WelcomeKey.Render(paddedKey) + t.WelcomeDesc.Render(s.desc) + "\n")
	}

	b.WriteString("\n")

	// ── Slash Commands ──────────────────────────────────────
	b.WriteString("  " + t.Bold.Render("Slash commands") + "\n")
	b.WriteString("\n")

	cmds := []struct {
		name string
		desc string
	}{
		{"/help", "Show available commands and shortcuts"},
		{"/clear", "Clear conversation history"},
		{"/compact", "Manually trigger context compaction"},
		{"/model", "Switch AI model or provider"},
		{"/config", "Show or edit configuration"},
		{"/usage", "Show token usage and cost"},
	}

	cmdWidth := 0
	for _, c := range cmds {
		if cw := lipgloss.Width(c.name); cw > cmdWidth {
			cmdWidth = cw
		}
	}
	cmdWidth += 2

	for _, c := range cmds {
		paddedCmd := c.name + strings.Repeat(" ", max(0, cmdWidth-lipgloss.Width(c.name)))
		b.WriteString("  " + t.WelcomeKey.Render(paddedCmd) + t.WelcomeDesc.Render(c.desc) + "\n")
	}

	b.WriteString("\n")

	// ── Tips ────────────────────────────────────────────────
	b.WriteString("  " + t.Bold.Render("Tips") + "\n")
	b.WriteString("\n")

	tips := []string{
		"Start with a specific task like \"fix the bug in main.go\"",
		"Use /model anytime to switch providers mid-session",
		"Press ctrl+o to expand tool results and see full output",
		"Multi-line input: type \\ at the end of a line, then press enter",
	}
	for _, tip := range tips {
		b.WriteString("  " + t.WelcomeTip.Render("• ") + t.WelcomeDesc.Render(tip) + "\n")
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

// AppendSystemMessage adds a system message to the transcript (public API).
func (m *Model) AppendSystemMessage(text string, isError bool) {
	m.appendSystemMessage(text, isError)
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

// SetProviderName sets the current provider identifier.
func (m *Model) SetProviderName(name string) {
	m.providerName = name
}

// SetVersion sets the version string displayed on the welcome screen.
func (m *Model) SetVersion(v string) {
	m.version = v
}

// SetModelChangeHandler sets the handler called when the user selects a new model.
func (m *Model) SetModelChangeHandler(fn func(providerID, modelID, baseURL string) error) {
	m.modelChangeHandler = fn
}

// SetHasAPIKeyFunc sets the function used to filter providers by available API keys.
func (m *Model) SetHasAPIKeyFunc(fn func(string) bool) {
	m.hasAPIKey = fn
	m.modelPicker.SetHasAPIKeyFunc(fn)
}

// SetConfig sets the config pointer used to save API keys during provider setup.
func (m *Model) SetConfig(cfg *config.Config) {
	m.config = cfg
	m.providerPicker.SetKeyGetter(func(provider string) string {
		if cfg != nil {
			return cfg.APIKey(provider)
		}
		return ""
	})
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

// SetCommandHandler sets the handler for slash commands.
func (m *Model) SetCommandHandler(fn func(cmdName, args string) (output string, clearHistory bool)) {
	m.commandHandler = fn
}

// SetCommandRegistry sets the command registry used for autocomplete.
func (m *Model) SetCommandRegistry(reg *commands.Registry) {
	m.cmdRegistry = reg
}

// updateAutocomplete refreshes the autocomplete state based on current input.
func (m *Model) updateAutocomplete() {
	if m.cmdRegistry == nil {
		return
	}
	m.autocomplete.Update(m.input.Value(), m.cmdRegistry)
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

	// Detect plan mode changes
	if name == "enter_plan_mode" && err == nil {
		c.program.Send(planModeMsg{active: true})
	} else if name == "exit_plan_mode" && err == nil {
		c.program.Send(planModeMsg{active: false})
	}
}

func (c *TUICallback) OnDone() {
	c.program.Send(doneMsg{})
}

func (c *TUICallback) OnError(err error) {
	c.program.Send(errMsg{err: err})
}

func (c *TUICallback) OnTokenUsage(usage session.SessionUsage) {
	c.program.Send(tokenUsageMsg{usage: usage})
}

func (c *TUICallback) OnCompaction(summary string) {
	c.program.Send(compactionMsg{summary: summary})
}

// AskFunc returns an AskFunc that sends questions to the TUI and waits for answers.
func (c *TUICallback) AskFunc() tool.AskFunc {
	return func(ctx context.Context, question string, options []tool.Option) (string, error) {
		answerCh := make(chan string, 1)
		optLabels := make([]string, len(options))
		for i, o := range options {
			optLabels[i] = o.Label
		}
		c.program.Send(askQuestionMsg{question: question, options: optLabels, answer: answerCh})
		select {
		case answer := <-answerCh:
			return answer, nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
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

// padLineToVisualWidth pads a string with trailing spaces so its visual width
// (ignoring ANSI escape codes) equals targetWidth.
func padLineToVisualWidth(line string, targetWidth int) string {
	w := ansi.StringWidth(line)
	if w < targetWidth {
		return line + strings.Repeat(" ", targetWidth-w)
	}
	return line
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
