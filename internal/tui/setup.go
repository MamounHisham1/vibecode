package tui

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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

// Providers returns all available providers with their top models (as of April 2026).
func Providers() []ProviderInfo {
	return []ProviderInfo{
		{
			ID:      "anthropic",
			Name:    "Anthropic",
			APIType: "anthropic",
			BaseURL: "https://api.anthropic.com/v1/messages",
			Models: []ModelInfo{
				{ID: "claude-opus-4-7", Name: "Claude Opus 4.7", Description: "Most capable model, best for complex reasoning and agentic coding"},
				{ID: "claude-sonnet-4-6", Name: "Claude Sonnet 4.6", Description: "Best balance of speed and intelligence, 1M context"},
				{ID: "claude-opus-4-6", Name: "Claude Opus 4.6", Description: "Previous flagship, strong coding and enterprise workflows"},
				{ID: "claude-haiku-4-5-20251001", Name: "Claude Haiku 4.5", Description: "Fastest and most affordable model"},
				{ID: "claude-sonnet-4-5-20250514", Name: "Claude Sonnet 4.5", Description: "Previous generation Sonnet, proven reliable"},
			},
		},
		{
			ID:      "openai",
			Name:    "OpenAI",
			APIType: "openai",
			BaseURL: "https://api.openai.com/v1/chat/completions",
			Models: []ModelInfo{
				{ID: "gpt-5.4", Name: "GPT-5.4", Description: "Latest flagship model for complex professional work"},
				{ID: "gpt-5.4-pro", Name: "GPT-5.4 Pro", Description: "Maximum capability variant with extended compute"},
				{ID: "gpt-5.4-mini", Name: "GPT-5.4 mini", Description: "Fast, efficient model for high-volume workloads"},
				{ID: "gpt-5.4-nano", Name: "GPT-5.4 nano", Description: "Smallest and cheapest, optimized for speed and cost"},
				{ID: "gpt-5.3-codex", Name: "GPT-5.3-Codex", Description: "Agentic coding model combining Codex and GPT-5 stacks"},
			},
		},
		{
			ID:      "deepseek",
			Name:    "DeepSeek",
			APIType: "openai",
			BaseURL: "https://api.deepseek.com/v1/chat/completions",
			Models: []ModelInfo{
				{ID: "deepseek-chat", Name: "DeepSeek-V3.2", Description: "Latest general-purpose model (maps to V3.2)"},
				{ID: "deepseek-reasoner", Name: "DeepSeek-R2", Description: "Advanced reasoning model for complex logic and math"},
				{ID: "deepseek-v3-0324", Name: "DeepSeek-V3-0324", Description: "V3 snapshot with improvements"},
				{ID: "deepseek-r1-0528", Name: "DeepSeek-R1-0528", Description: "R1 reasoning snapshot"},
				{ID: "deepseek-coder", Name: "DeepSeek-Coder V2", Description: "Specialized for coding tasks"},
			},
		},
		{
			ID:      "kimi",
			Name:    "Kimi (Moonshot AI)",
			APIType: "openai",
			BaseURL: "https://api.moonshot.ai/v1/chat/completions",
			Models: []ModelInfo{
				{ID: "kimi-k2.5", Name: "Kimi K2.5", Description: "Most capable multimodal model, 256K context, text and vision"},
				{ID: "kimi-k2-thinking", Name: "Kimi K2 Thinking", Description: "Reasoning model with deep chain-of-thought and agentic capabilities"},
				{ID: "kimi-k2", Name: "Kimi K2", Description: "1T MoE model, 32B active params, strong coding and agents"},
				{ID: "kimi-k2-turbo-preview", Name: "Kimi K2 Turbo", Description: "Faster K2 variant, optimized for speed"},
				{ID: "moonshot-v1-128k", Name: "Moonshot v1 128K", Description: "128K context window, stable production model"},
			},
		},
		{
			ID:      "moonshot",
			Name:    "Moonshot AI",
			APIType: "openai",
			BaseURL: "https://api.moonshot.ai/v1/chat/completions",
			Models: []ModelInfo{
				{ID: "kimi-k2.5", Name: "Kimi K2.5", Description: "Most capable multimodal model, 256K context"},
				{ID: "kimi-k2-thinking", Name: "Kimi K2 Thinking", Description: "Deep reasoning and agentic model"},
				{ID: "kimi-k2", Name: "Kimi K2", Description: "Flagship MoE model for coding and agents"},
				{ID: "moonshot-v1-128k", Name: "Moonshot v1 128K", Description: "128K context window"},
				{ID: "moonshot-v1-32k", Name: "Moonshot v1 32K", Description: "32K context window"},
			},
		},
		{
			ID:      "zhipu",
			Name:    "Zhipu AI / Z.ai (GLM)",
			APIType: "openai",
			BaseURL: "https://open.bigmodel.cn/api/paas/v4/chat/completions",
			AltBaseURLs: map[string]string{
				"Global (z.ai)": "https://api.z.ai/api/anthropic/v1/messages",
				"China (bigmodel.cn)": "https://open.bigmodel.cn/api/paas/v4/chat/completions",
			},
			Models: []ModelInfo{
				{ID: "glm-5.1", Name: "GLM-5.1", Description: "Latest flagship, agentic engineering SOTA (released Apr 2026)"},
				{ID: "glm-5", Name: "GLM-5", Description: "745B MoE, frontier coding and reasoning (released Feb 2026)"},
				{ID: "glm-4.7", Name: "GLM-4.7", Description: "Previous flagship, strong agentic and vision capabilities"},
				{ID: "glm-4.6", Name: "GLM-4.6", Description: "Balanced performance on domestic chips"},
				{ID: "glm-4.5", Name: "GLM-4.5", Description: "Cost-efficient model with strong reasoning"},
			},
		},
		{
			ID:      "qwen",
			Name:    "Qwen (Alibaba Cloud)",
			APIType: "openai",
			BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions",
			Models: []ModelInfo{
				{ID: "qwen3-max", Name: "Qwen3 Max", Description: "Most capable Qwen model with reasoning and tool integration"},
				{ID: "qwen3.5-plus", Name: "Qwen3.5 Plus", Description: "Multimodal model, text/image/video, comparable to Qwen3 Max"},
				{ID: "qwen3.5-flash", Name: "Qwen3.5 Flash", Description: "Fast and efficient multimodal model"},
				{ID: "qwen3-coder-plus", Name: "Qwen3-Coder Plus", Description: "Specialized for agentic coding tasks"},
				{ID: "qwq-plus", Name: "QwQ Plus", Description: "Dedicated reasoning model"},
			},
		},
		{
			ID:      "ollama",
			Name:    "Ollama (Local)",
			APIType: "ollama",
			BaseURL: "http://localhost:11434/v1/chat/completions",
			Models: []ModelInfo{
				{ID: "qwen3:8b", Name: "Qwen3 8B", Description: "Latest Qwen3 series, strong reasoning and coding"},
				{ID: "llama3", Name: "Llama 3", Description: "Meta's open model"},
				{ID: "mistral", Name: "Mistral", Description: "Fast and efficient"},
				{ID: "deepseek-coder-v2", Name: "DeepSeek Coder V2", Description: "Code-specialized"},
				{ID: "qwen2.5-coder", Name: "Qwen 2.5 Coder", Description: "Coding focused"},
			},
		},
	}
}

// ─── Setup Flow State ────────────────────────────────────────────

type setupPhase int

const (
	phaseProvider setupPhase = iota
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
	theme      Theme
	phase      setupPhase
	providers  []ProviderInfo
	selected   int
	endpointCur int
	endpoints  []string // ordered keys from AltBaseURLs
	modelCur   int
	input      []rune
	inputCur   int
	blinkOn    bool
	width      int
	height     int

	chosenProvider ProviderInfo
	chosenModel    ModelInfo
	apiKey         string
	validationErr  string
	skipKey        bool
}

// NewSetupModel creates the setup flow model.
func NewSetupModel() *SetupModel {
	return &SetupModel{
		theme:     DefaultTheme(),
		phase:     phaseProvider,
		providers: Providers(),
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
	b.WriteString("  " + t.Error.Render("⚠  Authentication failed") + "\n")
	b.WriteString("\n")
	// Truncate long error messages
	errMsg := m.validationErr
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
