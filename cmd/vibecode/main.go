package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/vibecode/vibecode/config"
	"github.com/vibecode/vibecode/internal/agent"
	"github.com/vibecode/vibecode/internal/commands"
	"github.com/vibecode/vibecode/internal/hooks"
	"github.com/vibecode/vibecode/internal/provider"
	"github.com/vibecode/vibecode/internal/skills"
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

	// Config subcommand
	rootCmd.AddCommand(buildConfigCmd())

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

	// Agent tool: inject a runner that creates subagents
	reg.Register(tool.NewAgentTool(reg, makeSubagentRunner(p, cfg), dir))

	system := buildSystemPrompt(dir)

	// Load skills and append their prompts to system
	skillStore := loadSkills(dir)
	for name, skill := range skillStore.All() {
		system += fmt.Sprintf("\n\n--- Skill: %s ---\n%s", name, skill.Prompt)
	}

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
	reg.Register(tool.AskUserStdin())
	reg.Register(&tool.TodoWrite{})
	reg.Register(tool.EnterPlanMode{})
	reg.Register(tool.ExitPlanMode{})

	// Web search (optional, requires API key)
	if searchKey := os.Getenv("VIBECODE_SEARCH_API_KEY"); searchKey != "" {
		searchProvider := os.Getenv("VIBECODE_SEARCH_PROVIDER")
		if searchProvider == "" {
			searchProvider = "brave"
		}
		reg.Register(tool.NewWebSearch(searchKey, searchProvider))
	}

	return reg
}

// makeSubagentRunner returns a function that creates and runs a subagent.
func makeSubagentRunner(p provider.Provider, cfg *config.Config) tool.SubagentRunner {
	return func(ctx context.Context, systemPrompt string, reg *tool.Registry, prompt string) (string, error) {
		cb := &tool.SubagentCollector{}
		a := agent.New(p, reg, systemPrompt, 50, nil, cb)
		if err := a.Run(ctx, prompt); err != nil {
			return "", err
		}
		return cb.Text(), nil
	}
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

// loadSkills discovers and loads skills from .vibecode/skills/ directories.
func loadSkills(dir string) *skills.Store {
	store := skills.NewStore()
	if err := store.Load(dir); err == nil && len(store.All()) > 0 {
		log.Printf("Loaded %d skills from %v", len(store.All()), store.Dirs())
	}
	return store
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
	a.SetHooks(buildHooks(cfg))
	return a.Run(ctx, message)
}

func runInteractive(p provider.Provider, reg *tool.Registry, system string, cfg *config.Config, dir string) error {
	inputChan := make(chan string, 16)

	m := tui.New(inputChan)
	m.SetStatus(cfg.Model, dir)

	// Wire slash commands
	cmdReg := commands.NewRegistry()
	m.SetCommandHandler(func(cmdName, args string) (string, bool) {
		cmd, ok := cmdReg.Lookup(cmdName)
		if !ok {
			return fmt.Sprintf("Unknown command: /%s. Type /help for available commands.", cmdName), false
		}
		switch cmd.Type {
		case commands.TypeLocal:
			result := cmd.Handler(args)
			return result.Output, result.Clear
		case commands.TypePrompt:
			inputChan <- cmd.PromptText
			return "", false
		default:
			return "", false
		}
	})

	pgm := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

	cb := tui.NewCallback(pgm)

	// Wire TUI-aware ask function
	if askTool, ok := reg.Get("ask_user"); ok {
		if au, ok := askTool.(*tool.AskUser); ok {
			au.SetAskFunc(cb.AskFunc())
		}
	}

	a := agent.New(p, reg, system, cfg.MaxIterations, cfg.AutoApprove, cb)
	a.SetHooks(buildHooks(cfg))

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
// buildHooks creates a hook manager from config.
func buildHooks(cfg *config.Config) *hooks.Manager {
	hm := hooks.NewManager()
	if len(cfg.Hooks) > 0 {
		hm.LoadFromConfig(cfg.Hooks)
	}
	return hm
}

func buildConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage VibeCode configuration",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "get <key>",
		Short: "Get a config value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			val, err := getConfigValue(cfg, args[0])
			if err != nil {
				return err
			}
			fmt.Println(val)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a config value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := setConfigValue(cfg, args[0], args[1]); err != nil {
				return err
			}
			return cfg.Save()
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all config values",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			data, _ := json.MarshalIndent(cfg, "", "  ")
			fmt.Println(string(data))
			return nil
		},
	})

	return cmd
}

var validConfigKeys = []string{
	"provider", "model", "base_url", "max_iterations", "theme",
}

func getConfigValue(cfg *config.Config, key string) (string, error) {
	switch key {
	case "provider":
		return cfg.Provider, nil
	case "model":
		return cfg.Model, nil
	case "base_url":
		return cfg.BaseURL, nil
	case "max_iterations":
		return fmt.Sprintf("%d", cfg.MaxIterations), nil
	case "theme":
		return cfg.Theme, nil
	default:
		return "", fmt.Errorf("unknown config key: %s (valid: %s)", key, strings.Join(validConfigKeys, ", "))
	}
}

func setConfigValue(cfg *config.Config, key, value string) error {
	switch key {
	case "provider":
		validProviders := []string{"anthropic", "openai", "deepseek", "kimi", "moonshot", "zhipu", "qwen", "ollama"}
		for _, p := range validProviders {
			if p == value {
				cfg.Provider = value
				return nil
			}
		}
		return fmt.Errorf("invalid provider: %s (valid: %s)", value, strings.Join(validProviders, ", "))
	case "model":
		cfg.Model = value
		return nil
	case "base_url":
		cfg.BaseURL = value
		return nil
	case "max_iterations":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("max_iterations must be a number")
		}
		cfg.MaxIterations = n
		return nil
	case "theme":
		cfg.Theme = value
		return nil
	default:
		return fmt.Errorf("unknown config key: %s (valid: %s)", key, strings.Join(validConfigKeys, ", "))
	}
}
