package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/vibecode/vibecode/config"
	"github.com/vibecode/vibecode/internal/agent"
	"github.com/vibecode/vibecode/internal/provider"
	"github.com/vibecode/vibecode/internal/tool"
	"github.com/vibecode/vibecode/internal/tui"
)

func renderMarkdownCLI(text string) string {
	return tui.RenderMarkdown(text)
}

var (
	flagProvider string
	flagModel    string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "vibecode [message]",
		Short: "AI-powered coding agent for your terminal",
		Long:  "Vibe Code — an open-source CLI coding agent with multi-provider support.",
		Args:  cobra.MaximumNArgs(1),
		RunE:  run,
	}

	rootCmd.Flags().StringVar(&flagProvider, "provider", "", "LLM provider (anthropic, openai, deepseek, kimi, moonshot, zhipu, qwen, ollama)")
	rootCmd.Flags().StringVar(&flagModel, "model", "", "Model name to use")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Check if this is a fresh install — no provider or API key configured.
	// Skip setup if flags are explicitly provided or if using env vars.
	needsSetup := !configFileExists() &&
		flagProvider == "" && flagModel == "" &&
		os.Getenv("ANTHROPIC_API_KEY") == "" &&
		os.Getenv("OPENAI_API_KEY") == "" &&
		os.Getenv("VIBECODE_PROVIDER") == ""

	if needsSetup && len(args) == 0 {
		setupCfg, err := tui.RunSetup()
		if err != nil {
			return fmt.Errorf("setup: %w", err)
		}
		cfg.Provider = setupCfg.Provider.ID
		cfg.Model = setupCfg.Model.ID
		cfg.BaseURL = setupCfg.Provider.BaseURL
		if setupCfg.APIKey != "" {
			if cfg.APIKeys == nil {
				cfg.APIKeys = make(map[string]string)
			}
			cfg.APIKeys[setupCfg.Provider.ID] = setupCfg.APIKey
		}
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
	}

	if flagProvider != "" {
		cfg.Provider = flagProvider
	}
	if flagModel != "" {
		cfg.Model = flagModel
	}

	dir, _ := os.Getwd()

	reg := buildToolRegistry()

	p, err := buildProvider(cfg)
	if err != nil {
		return err
	}

	system := buildSystemPrompt(dir)

	if len(args) == 1 {
		return runOneShot(args[0], p, reg, system, cfg)
	}

	return runInteractive(p, reg, system, cfg, dir)
}

func configFileExists() bool {
	path, err := config.ConfigPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

func buildToolRegistry() *tool.Registry {
	reg := tool.NewRegistry()
	reg.Register(tool.ReadFile{})
	reg.Register(tool.WriteFile{})
	reg.Register(tool.EditFile{})
	reg.Register(tool.Glob{})
	reg.Register(tool.Grep{})
	reg.Register(tool.Shell{})
	reg.Register(tool.Git{})
	reg.Register(tool.WebFetch{})
	reg.Register(tool.AskUser{})
	return reg
}

func buildProvider(cfg *config.Config) (provider.Provider, error) {
	switch cfg.Provider {
	case "anthropic":
		key := cfg.APIKey("anthropic")
		if key == "" {
			return nil, fmt.Errorf("set ANTHROPIC_API_KEY or run 'vibecode' to configure")
		}
		model := cfg.Model
		if model == "" {
			model = "claude-sonnet-4-6"
		}
		if cfg.BaseURL != "" {
			return provider.NewAnthropicWithBaseURL(key, model, cfg.BaseURL), nil
		}
		return provider.NewAnthropic(key, model), nil

	case "openai":
		key := cfg.APIKey("openai")
		if key == "" {
			return nil, fmt.Errorf("set OPENAI_API_KEY or run 'vibecode' to configure")
		}
		model := cfg.Model
		if model == "" {
			model = "gpt-5.4"
		}
		if cfg.BaseURL != "" {
			return provider.NewOpenAIWithBaseURL(key, model, cfg.BaseURL), nil
		}
		return provider.NewOpenAI(key, model), nil

	case "deepseek":
		key := cfg.APIKey("deepseek")
		if key == "" {
			key = cfg.APIKey("openai")
		}
		if key == "" {
			return nil, fmt.Errorf("run 'vibecode' to configure your DeepSeek API key")
		}
		model := cfg.Model
		if model == "" {
			model = "deepseek-chat"
		}
		baseURL := "https://api.deepseek.com/v1/chat/completions"
		if cfg.BaseURL != "" {
			baseURL = cfg.BaseURL
		}
		return provider.NewOpenAIWithBaseURL(key, model, baseURL), nil

	case "kimi", "moonshot":
		key := cfg.APIKey(cfg.Provider)
		if key == "" {
			key = cfg.APIKey("kimi")
		}
		if key == "" {
			key = cfg.APIKey("moonshot")
		}
		if key == "" {
			key = cfg.APIKey("openai")
		}
		if key == "" {
			return nil, fmt.Errorf("run 'vibecode' to configure your %s API key", cfg.Provider)
		}
		model := cfg.Model
		if model == "" {
			model = "moonshot-v1-8k"
		}
		baseURL := "https://api.moonshot.ai/v1/chat/completions"
		if cfg.BaseURL != "" {
			baseURL = cfg.BaseURL
		}
		return provider.NewOpenAIWithBaseURL(key, model, baseURL), nil

	case "zhipu":
		key := cfg.APIKey("zhipu")
		if key == "" {
			key = cfg.APIKey("anthropic")
		}
		if key == "" {
			return nil, fmt.Errorf("run 'vibecode' to configure your Zhipu AI API key")
		}
		model := cfg.Model
		if model == "" {
			model = "glm-5.1"
		}
		if cfg.BaseURL != "" {
			return provider.NewAnthropicWithBaseURL(key, model, cfg.BaseURL), nil
		}
		return provider.NewOpenAIWithBaseURL(key, model, "https://open.bigmodel.cn/api/paas/v4/chat/completions"), nil

	case "qwen":
		key := cfg.APIKey("qwen")
		if key == "" {
			key = cfg.APIKey("openai")
		}
		if key == "" {
			return nil, fmt.Errorf("run 'vibecode' to configure your Qwen API key")
		}
		model := cfg.Model
		if model == "" {
			model = "qwen-turbo"
		}
		baseURL := "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions"
		if cfg.BaseURL != "" {
			baseURL = cfg.BaseURL
		}
		return provider.NewOpenAIWithBaseURL(key, model, baseURL), nil

	case "ollama":
		model := cfg.Model
		if model == "" {
			model = "llama3"
		}
		baseURL := cfg.APIKey("ollama_base_url")
		return provider.NewOllama(model, baseURL), nil

	default:
		return nil, fmt.Errorf("unsupported provider: %s\nRun 'vibecode' to configure a provider", cfg.Provider)
	}
}

func buildSystemPrompt(dir string) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("You are Vibe Code, an AI coding agent. You help users write, edit, and understand code.\n\n"))
	b.WriteString(fmt.Sprintf("Working directory: %s\n\n", dir))

	// Git context (like Claude Code)
	if gitInfo := getGitContext(dir); gitInfo != "" {
		b.WriteString(gitInfo)
		b.WriteString("\n")
	}

	// Load VIBECODE.md if present
	if instructions := loadVibeCodeMD(dir); instructions != "" {
		b.WriteString("\nProject instructions:\n")
		b.WriteString(instructions)
		b.WriteString("\n")
	}

	// Today's date
	b.WriteString(fmt.Sprintf("\nToday's date: %s\n", getDateString()))

	b.WriteString(`
Tools: read_file, write_file, edit_file, glob, grep, shell, git, web_fetch, ask_user

Rules:
- Always read files before editing them
- Use edit_file for targeted changes, write_file for new files
- Keep explanations concise
- Run tests after making changes when appropriate
- Use ask_user when you need clarification
`)

	return b.String()
}

func getGitContext(dir string) string {
	var b strings.Builder

	// Branch
	if out, err := exec.Command("git", "-C", dir, "branch", "--show-current").Output(); err == nil {
		branch := strings.TrimSpace(string(out))
		if branch != "" {
			b.WriteString(fmt.Sprintf("Git branch: %s\n", branch))
		}
	}

	// Status (truncated)
	if out, err := exec.Command("git", "-C", dir, "status", "--short").Output(); err == nil {
		status := strings.TrimSpace(string(out))
		if status != "" {
			if len(status) > 2000 {
				status = status[:2000] + "\n... (truncated)"
			}
			b.WriteString("Git status:\n" + status + "\n")
		}
	}

	// Recent commits
	if out, err := exec.Command("git", "-C", dir, "log", "--oneline", "-n", "5").Output(); err == nil {
		log := strings.TrimSpace(string(out))
		if log != "" {
			b.WriteString("Recent commits:\n" + log + "\n")
		}
	}

	return b.String()
}

func loadVibeCodeMD(dir string) string {
	// Walk from dir up to home, collecting VIBECODE.md files (like Claude Code's CLAUDE.md)
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	var files []string

	// User-level: ~/.vibecode/VIBECODE.md
	globalPath := filepath.Join(home, ".vibecode", "VIBECODE.md")
	if data, err := os.ReadFile(globalPath); err == nil {
		files = append(files, string(data))
	}

	// Project-level: walk from dir up to home
	cur := dir
	for {
		p := filepath.Join(cur, "VIBECODE.md")
		if data, err := os.ReadFile(p); err == nil {
			files = append(files, string(data))
		}
		p = filepath.Join(cur, ".vibecode", "VIBECODE.md")
		if data, err := os.ReadFile(p); err == nil {
			files = append(files, string(data))
		}
		parent := filepath.Dir(cur)
		if parent == cur || parent == home || parent == "/" {
			break
		}
		cur = parent
	}

	// Also .vibecode/rules/*.md
	rulesDir := filepath.Join(dir, ".vibecode", "rules")
	if entries, err := os.ReadDir(rulesDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
				if data, err := os.ReadFile(filepath.Join(rulesDir, entry.Name())); err == nil {
					files = append(files, string(data))
				}
			}
		}
	}

	return strings.Join(files, "\n\n")
}

func getDateString() string {
	out, err := exec.Command("date", "+%Y-%m-%d").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func runOneShot(message string, p provider.Provider, reg *tool.Registry, system string, cfg *config.Config) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cb := &cliCallback{}
	a := agent.New(p, reg, system, cfg.MaxIterations, cfg.AutoApprove, cb)
	if cw := effectiveContextWindow(cfg); cw > 0 {
		a.SetContextWindow(cw)
	}
	return a.Run(ctx, message)
}

func runInteractive(p provider.Provider, reg *tool.Registry, system string, cfg *config.Config, dir string) error {
	inputChan := make(chan string, 16)

	m := tui.New(inputChan)
	m.SetStatus(cfg.Model, dir)

	pgm := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

	cb := tui.NewCallback(pgm)
	a := agent.New(p, reg, system, cfg.MaxIterations, cfg.AutoApprove, cb)
	if cw := effectiveContextWindow(cfg); cw > 0 {
		a.SetContextWindow(cw)
	}

	// Use a cancellable root context so SIGINT can exit the program.
	// Per-turn cancellation is handled by giving each Run() its own child context.
	rootCtx, rootCancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer rootCancel()

	go func() {
		for msg := range inputChan {
			// Create a fresh child context for each turn so that cancelling one
			// turn (e.g. user presses Ctrl+C to stop generation) does not kill
			// subsequent turns.
			turnCtx, turnCancel := context.WithCancel(rootCtx)
			m.SetCancelFunc(turnCancel)

			if err := a.Run(turnCtx, msg); err != nil {
				cb.OnError(err)
			}
			turnCancel()
		}
	}()

	_, err := pgm.Run()
	close(inputChan)
	return err
}

type cliCallback struct {
	buf strings.Builder
}

func (c *cliCallback) OnText(text string) { c.buf.WriteString(text) }
func (c *cliCallback) OnToolStart(name, id string, input json.RawMessage) {
	if c.buf.Len() > 0 {
		fmt.Print(renderMarkdownCLI(c.buf.String()))
		c.buf.Reset()
	}
	fmt.Printf("\n● %s...\n", name)
}
func (c *cliCallback) OnToolOutput(name, id, output string, err error) {
	icon := "●"
	if err != nil {
		icon = "✗"
	}
	summary := output
	if len(summary) > 120 {
		summary = summary[:120] + "..."
	}
	fmt.Printf("%s %s: %s\n", icon, name, summary)
}
func (c *cliCallback) OnDone() {
	if c.buf.Len() > 0 {
		fmt.Print(renderMarkdownCLI(c.buf.String()))
		c.buf.Reset()
	}
	fmt.Println()
}
func (c *cliCallback) OnError(err error) {
	if c.buf.Len() > 0 {
		fmt.Print(renderMarkdownCLI(c.buf.String()))
		c.buf.Reset()
	}
	fmt.Fprintf(os.Stderr, "Error: %s\n", err)
}
func (c *cliCallback) OnCompact(summary string) {
	if c.buf.Len() > 0 {
		fmt.Print(renderMarkdownCLI(c.buf.String()))
		c.buf.Reset()
	}
	fmt.Println("ℹ Conversation compacted")
}
func (c *cliCallback) OnUsage(inputTokens, outputTokens int) {}

// effectiveContextWindow returns the context window size for the configured model.
// Falls back to defaults per provider if not explicitly set.
func effectiveContextWindow(cfg *config.Config) int {
	if cfg.ContextWindow > 0 {
		return cfg.ContextWindow
	}

	// Default context windows per model family
	model := strings.ToLower(cfg.Model)
	switch {
	case strings.Contains(model, "claude-3-5-sonnet"), strings.Contains(model, "claude-3.5-sonnet"):
		return 200000
	case strings.Contains(model, "claude-3-opus"), strings.Contains(model, "claude-3.5-opus"):
		return 200000
	case strings.Contains(model, "claude-3-haiku"):
		return 200000
	case strings.Contains(model, "claude-sonnet-4"), strings.Contains(model, "claude-sonnet-4-6"):
		return 200000
	case strings.Contains(model, "claude-opus-4"), strings.Contains(model, "claude-opus-4-7"):
		return 200000
	case strings.Contains(model, "claude"):
		return 200000
	case strings.Contains(model, "gpt-4o"), strings.Contains(model, "gpt-5"):
		return 128000
	case strings.Contains(model, "gpt-4-turbo"):
		return 128000
	case strings.Contains(model, "gpt-4"):
		return 8192
	case strings.Contains(model, "deepseek"):
		return 64000
	case strings.Contains(model, "moonshot"), strings.Contains(model, "kimi"):
		return 8192
	case strings.Contains(model, "qwen"):
		return 32768
	case strings.Contains(model, "glm"):
		return 128000
	default:
		return 128000
	}
}
