package tui

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vibecode/vibecode/internal/openrouter"
	"github.com/vibecode/vibecode/internal/provider"
)

// ProviderInfo holds metadata about an AI provider.
type ProviderInfo struct {
	ID          string
	Name        string
	Models      []ModelInfo
	BaseURL     string
	AltBaseURLs map[string]string // option name -> URL, shown as endpoint picker if non-empty
	APIType     string            // "anthropic", "openai", "ollama"
}

// ModelInfo holds metadata about a model.
type ModelInfo struct {
	ID          string
	Name        string
	Description string
}

// SetupConfig is the result of the setup flow.
type SetupConfig struct {
	Provider ProviderInfo
	Model    ModelInfo
	APIKey   string
}

// convertOpenRouterData transforms OpenRouter provider+model data into our ProviderInfo format.
func convertOpenRouterData(data []openrouter.ProviderModels) []ProviderInfo {
	var result []ProviderInfo
	for _, pm := range data {
		meta, ok := provider.ProviderMetaMap[pm.Provider.Slug]
		if !ok {
			continue // skip providers we don't know how to route
		}

		var models []ModelInfo
		for _, m := range pm.Models {
			models = append(models, ModelInfo{
				ID:          openrouter.NormalizeModelID(pm.Provider.Slug, m.ID),
				Name:        strings.TrimPrefix(m.Name, pm.Provider.Name+": "),
				Description: truncate(m.Description, 100),
			})
		}

		result = append(result, ProviderInfo{
			ID:      pm.Provider.Slug,
			Name:    meta.Name,
			APIType: meta.APIType,
			BaseURL: meta.BaseURL,
			Models:  models,
		})
	}
	return result
}

// Providers returns cached provider data, or an empty list if not yet fetched.
func Providers() []ProviderInfo {
	if data, ok := openrouter.GlobalCache.Get(); ok {
		return convertOpenRouterData(data)
	}
	return nil
}

// fetchProvidersCmd is a Bubble Tea command that fetches provider data from OpenRouter.
func fetchProvidersCmd() tea.Msg {
	client := openrouter.NewClient()
	data, err := openrouter.GlobalCache.FetchOrGet(client)
	if err != nil {
		return providersErrMsg{err: err}
	}
	provider.BuildRegistryFromOpenRouter(data)
	return providersLoadedMsg{providers: convertOpenRouterData(data)}
}

type providersLoadedMsg struct {
	providers []ProviderInfo
}

type providersErrMsg struct {
	err error
}

// ─── Setup Flow State ────────────────────────────────────────────

type setupPhase int

const (
	phaseLoading setupPhase = iota
	phaseProvider
	phaseEndpoint
	phaseModel
	phaseToken
	phaseValidating
	phaseDone
	phaseError
)

type validationDoneMsg struct{ err error }

// SetupModel is the bubbletea model for the first-time setup flow.
type SetupModel struct {
	theme       Theme
	phase       setupPhase
	providers   []ProviderInfo
	selected    int
	endpointCur int
	endpoints   []string // ordered keys from AltBaseURLs
	modelCur    int
	input       []rune
	inputCur    int
	blinkOn     bool
	width       int
	height      int
	loadErr     string

	chosenProvider ProviderInfo
	chosenModel    ModelInfo
	apiKey         string
	validationErr  string
	skipKey        bool
}

// NewSetupModel creates the setup flow model.
func NewSetupModel() *SetupModel {
	providers := Providers()
	phase := phaseProvider
	if len(providers) == 0 {
		phase = phaseLoading
	}
	return &SetupModel{
		theme:     DefaultTheme(),
		phase:     phase,
		providers: providers,
		input:     make([]rune, 0),
		blinkOn:   true,
		width:     80,
		height:    24,
	}
}

// Result returns the final setup config if the flow completed.
func (m *SetupModel) Result() *SetupConfig {
	if m.phase != phaseDone {
		return nil
	}
	return &SetupConfig{
		Provider: m.chosenProvider,
		Model:    m.chosenModel,
		APIKey:   m.apiKey,
	}
}

func (m *SetupModel) Init() tea.Cmd {
	if m.phase == phaseLoading {
		return tea.Batch(fetchProvidersCmd, setupTickCmd)
	}
	return setupTickCmd
}

func setupTickCmd() tea.Msg {
	time.Sleep(500 * time.Millisecond)
	return setupTickMsg{}
}

type setupTickMsg struct{}

func (m *SetupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case setupTickMsg:
		// Keep blinkOn always true so cursor never disappears
		m.blinkOn = true
		return m, setupTickCmd

	case validationDoneMsg:
		if msg.err != nil {
			m.phase = phaseError
			m.validationErr = msg.err.Error()
			return m, nil
		}
		m.phase = phaseDone
		return m, tea.Quit

	case providersLoadedMsg:
		m.providers = msg.providers
		m.phase = phaseProvider
		return m, nil

	case providersErrMsg:
		m.loadErr = msg.err.Error()
		m.phase = phaseError
		return m, nil

	case tea.KeyMsg:
		switch m.phase {
		case phaseProvider:
			return m.handleProviderKeys(msg)
		case phaseEndpoint:
			return m.handleEndpointKeys(msg)
		case phaseModel:
			return m.handleModelKeys(msg)
		case phaseToken:
			return m.handleTokenKeys(msg)
		case phaseError:
			if msg.String() == "enter" {
				m.phase = phaseToken
				m.input = m.input[:0]
				m.inputCur = 0
				m.validationErr = ""
				return m, nil
			}
			if msg.String() == "esc" || msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
		case phaseDone:
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *SetupModel) handleProviderKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.selected > 0 {
			m.selected--
		}
	case "down", "j":
		if m.selected < len(m.providers)-1 {
			m.selected++
		}
	case "enter":
		m.chosenProvider = m.providers[m.selected]
		m.skipKey = m.chosenProvider.APIType == "ollama"
		if len(m.chosenProvider.AltBaseURLs) > 0 {
			m.endpoints = make([]string, 0, len(m.chosenProvider.AltBaseURLs))
			for k := range m.chosenProvider.AltBaseURLs {
				m.endpoints = append(m.endpoints, k)
			}
			m.endpointCur = 0
			m.phase = phaseEndpoint
		} else {
			m.modelCur = 0
			m.phase = phaseModel
		}
	case "esc", "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m *SetupModel) handleEndpointKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.endpointCur > 0 {
			m.endpointCur--
		}
	case "down", "j":
		if m.endpointCur < len(m.endpoints)-1 {
			m.endpointCur++
		}
	case "enter":
		chosen := m.endpoints[m.endpointCur]
		m.chosenProvider.BaseURL = m.chosenProvider.AltBaseURLs[chosen]
		// Update APIType based on endpoint
		if strings.Contains(m.chosenProvider.BaseURL, "anthropic") {
			m.chosenProvider.APIType = "anthropic"
		}
		m.modelCur = 0
		m.phase = phaseModel
	case "esc":
		m.phase = phaseProvider
		m.selected = 0
	}
	return m, nil
}

func (m *SetupModel) handleModelKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	models := m.chosenProvider.Models
	switch msg.String() {
	case "up", "k":
		if m.modelCur > 0 {
			m.modelCur--
		}
	case "down", "j":
		if m.modelCur < len(models)-1 {
			m.modelCur++
		}
	case "enter":
		m.chosenModel = models[m.modelCur]
		if m.skipKey {
			m.apiKey = ""
			m.phase = phaseDone
			return m, tea.Quit
		}
		m.phase = phaseToken
		m.input = m.input[:0]
		m.inputCur = 0
	case "esc":
		m.phase = phaseProvider
		m.selected = 0
	}
	return m, nil
}

func (m *SetupModel) handleTokenKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.phase = phaseModel
		return m, nil
	case "enter":
		if len(m.input) == 0 {
			return m, nil
		}
		m.apiKey = string(m.input)
		m.phase = phaseValidating
		return m, m.validateToken()
	case "backspace":
		if m.inputCur > 0 {
			m.input = append(m.input[:m.inputCur-1], m.input[m.inputCur:]...)
			m.inputCur--
		}
	case "delete":
		if m.inputCur < len(m.input) {
			m.input = append(m.input[:m.inputCur], m.input[m.inputCur+1:]...)
		}
	case "left":
		if m.inputCur > 0 {
			m.inputCur--
		}
	case "right":
		if m.inputCur < len(m.input) {
			m.inputCur++
		}
	default:
		if msg.Type == tea.KeyRunes {
			for _, r := range msg.Runes {
				m.input = append(m.input, 0)
				copy(m.input[m.inputCur+1:], m.input[m.inputCur:])
				m.input[m.inputCur] = r
				m.inputCur++
			}
		}
	}
	return m, nil
}

func (m *SetupModel) validateToken() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if m.chosenProvider.APIType == "ollama" {
			return validationDoneMsg{nil}
		}

		switch m.chosenProvider.APIType {
		case "anthropic":
			return validationDoneMsg{validateAnthropicKeyWithBase(ctx, m.chosenProvider.BaseURL, m.apiKey)}
		case "openai":
			return validationDoneMsg{validateOpenAICompatible(ctx, m.chosenProvider.BaseURL, m.apiKey, m.chosenModel.ID)}
		}
		return validationDoneMsg{nil}
	}
}

// ─── View ──────────────────────────────────────────────────────────

func (m *SetupModel) View() string {
	switch m.phase {
	case phaseLoading:
		return m.viewLoading()
	case phaseProvider:
		return m.viewProvider()
	case phaseEndpoint:
		return m.viewEndpoint()
	case phaseModel:
		return m.viewModel()
	case phaseToken:
		return m.viewToken()
	case phaseValidating:
		return m.viewValidating()
	case phaseError:
		return m.viewError()
	}
	return ""
}

func (m *SetupModel) viewLoading() string {
	t := m.theme
	return "\n  " + t.AssistantDot.Render("⠋") + " " + t.BrandLight.Render("Loading providers from OpenRouter...") + "\n"
}

func (m *SetupModel) viewProvider() string {
	t := m.theme
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString("  " + t.Brand.Render("Welcome to Vibe Code!") + "\n")
	b.WriteString("  " + t.Dim.Render("Let's set up your AI provider.") + "\n")
	b.WriteString("\n")
	b.WriteString("  " + t.Bold.Render("Choose your AI provider:") + "\n")
	b.WriteString("\n")

	for i, p := range m.providers {
		cursor := "  "
		name := p.Name
		if i == m.selected {
			cursor = t.Brand.Render("▸ ")
			name = t.Brand.Render(name)
		} else {
			name = t.Text.Render(name)
		}
		b.WriteString("  " + cursor + name + "\n")
	}

	b.WriteString("\n")
	b.WriteString("  " + t.Dim.Render("↑/↓ navigate · enter select · esc quit") + "\n")
	return b.String()
}

func (m *SetupModel) viewEndpoint() string {
	t := m.theme
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString("  " + t.Brand.Render(m.chosenProvider.Name) + " " + t.Dim.Render("— Select your plan") + "\n")
	b.WriteString("\n")

	for i, ep := range m.endpoints {
		cursor := "  "
		name := ep
		if i == m.endpointCur {
			cursor = t.Brand.Render("▸ ")
			name = t.Brand.Render(name)
		} else {
			name = t.Text.Render(name)
		}
		b.WriteString("  " + cursor + name + "\n")
	}

	b.WriteString("\n")
	b.WriteString("  " + t.Dim.Render("↑/↓ navigate · enter select · esc back") + "\n")
	return b.String()
}

func (m *SetupModel) viewModel() string {
	t := m.theme
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString("  " + t.Brand.Render(m.chosenProvider.Name) + " " + t.Dim.Render("— Select a model") + "\n")
	b.WriteString("\n")

	for i, model := range m.chosenProvider.Models {
		cursor := "  "
		name := model.Name
		desc := t.Dim.Render("  " + model.Description)
		if i == m.modelCur {
			cursor = t.Brand.Render("▸ ")
			name = t.Brand.Render(name)
		} else {
			name = t.Text.Render(name)
		}
		b.WriteString("  " + cursor + name + desc + "\n")
	}

	b.WriteString("\n")
	b.WriteString("  " + t.Dim.Render("↑/↓ navigate · enter select · esc back") + "\n")
	return b.String()
}

func (m *SetupModel) viewToken() string {
	t := m.theme
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString("  " + t.Brand.Render(m.chosenProvider.Name) + " " + t.Dim.Render("· "+m.chosenModel.Name) + "\n")
	b.WriteString("\n")
	b.WriteString("  " + t.Bold.Render("Enter your API key:") + "\n")
	b.WriteString("\n")

	if len(m.input) == 0 {
		placeholder := "sk-..."
		var inputLine strings.Builder
		if m.blinkOn {
			inputLine.WriteString(t.InverseCursor.Render(string(placeholder[0])))
			inputLine.WriteString(t.InputHint.Render(placeholder[1:]))
		} else {
			inputLine.WriteString(t.InputHint.Render(placeholder))
		}
		b.WriteString("  " + t.PromptChar.Render("❯ ") + inputLine.String() + "\n")
	} else {
		// Masked input with cursor
		var inputLine strings.Builder
		for i := range m.input {
			ch := "•"
			if i == m.inputCur && m.blinkOn {
				inputLine.WriteString(t.InverseCursor.Render(ch))
			} else {
				inputLine.WriteString(ch)
			}
		}
		if m.inputCur == len(m.input) && m.blinkOn {
			inputLine.WriteString(t.InverseCursor.Render(" "))
		}
		b.WriteString("  " + t.PromptChar.Render("❯ ") + inputLine.String() + "\n")
	}

	b.WriteString("\n")
	b.WriteString("  " + t.Dim.Render("enter confirm · esc back") + "\n")
	return b.String()
}

func (m *SetupModel) viewValidating() string {
	t := m.theme
	return "\n  " + t.AssistantDot.Render("⠋") + " " + t.BrandLight.Render("Validating API key...") + "\n"
}

func (m *SetupModel) viewError() string {
	t := m.theme
	var b strings.Builder

	b.WriteString("\n")
	if m.loadErr != "" {
		b.WriteString("  " + t.Error.Render("⚠  Failed to load providers") + "\n")
	} else {
		b.WriteString("  " + t.Error.Render("⚠  Authentication failed") + "\n")
	}
	b.WriteString("\n")
	// Truncate long error messages
	errMsg := m.validationErr
	if m.loadErr != "" {
		errMsg = m.loadErr
	}
	if len(errMsg) > 200 {
		errMsg = errMsg[:200] + "..."
	}
	b.WriteString("  " + t.Dim.Render(errMsg) + "\n")
	b.WriteString("\n")
	b.WriteString("  " + t.Dim.Render("enter to try again · esc to quit") + "\n")
	return b.String()
}

// ─── Validation Helpers ────────────────────────────────────────────

func validateAnthropicKey(ctx context.Context, apiKey string) error {
	return validateAnthropicKeyWithBase(ctx, "https://api.anthropic.com/v1/messages", apiKey)
}

func validateAnthropicKeyWithBase(ctx context.Context, baseURL, apiKey string) error {
	body := `{"model":"claude-haiku-4-5-20251001","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("invalid API key")
	}
	if resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("access forbidden — check your API key permissions")
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil
	}
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, truncate(string(respBody), 120))
	}
	return nil
}

func validateOpenAICompatible(ctx context.Context, baseURL, apiKey, model string) error {
	body := fmt.Sprintf(`{"model":"%s","max_completion_tokens":1,"messages":[{"role":"user","content":"hi"}]}`, model)

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("invalid API key")
	}
	if resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("access forbidden — check your API key permissions")
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		// 429 = rate limit or quota — key is valid, just throttled
		return nil
	}
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, truncate(string(respBody), 120))
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// RunSetup launches the first-time setup flow and returns the config.
func RunSetup() (*SetupConfig, error) {
	m := NewSetupModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return nil, err
	}
	result := m.Result()
	if result == nil {
		return nil, fmt.Errorf("setup cancelled")
	}
	return result, nil
}
