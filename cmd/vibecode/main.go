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
	"github.com/vibecode/vibecode/internal/session"
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
		// For one-shot mode, fail fast. For interactive mode, open the TUI
		// so the user can fix configuration via /model or /config.
		if len(args) == 1 {
			return err
		}
		p = nil
	}

	// Agent tool: inject a runner that creates subagents
	// Use a closure-captured provider so subagents pick up model switches.
	currentProvider := p
	reg.Register(tool.NewAgentTool(reg, makeSubagentRunner(func() provider.Provider { return currentProvider }, cfg), dir))

	system := buildSystemPrompt(dir)

	// Load skills and append their prompts to system
	skillStore := loadSkills(dir)
	for name, skill := range skillStore.All() {
		system += fmt.Sprintf("\n\n--- Skill: %s ---\n%s", name, skill.Prompt)
	}

	if len(args) == 1 {
		return runOneShot(args[0], p, reg, system, cfg)
	}

	return runInteractive(p, reg, system, cfg, dir, &currentProvider)
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
func makeSubagentRunner(getProvider func() provider.Provider, cfg *config.Config) tool.SubagentRunner {
	return func(ctx context.Context, systemPrompt string, reg *tool.Registry, prompt string) (string, error) {
		p := getProvider()
		if p == nil {
			return "", fmt.Errorf("no provider configured — use /model to select a provider and model")
		}
		cb := &tool.SubagentCollector{}
		a := agent.New(p, reg, systemPrompt, 50, nil, cb)
		if err := a.Run(ctx, prompt); err != nil {
			return "", err
		}
		return cb.Text(), nil
	}
}

func buildProvider(cfg *config.Config) (provider.Provider, error) {
	// Ollama is a special local provider not on OpenRouter.
	if cfg.Provider == "ollama" {
		model := cfg.Model
		if model == "" {
			model = "llama3"
		}
		baseURL := cfg.APIKey("ollama_base_url")
		return provider.NewOllama(model, baseURL), nil
	}

	key := cfg.APIKey(cfg.Provider)
	if key == "" {
		return nil, fmt.Errorf("no API key for %s — set %s_API_KEY or run 'vibecode' to configure", cfg.Provider, cfg.Provider)
	}

	model := cfg.Model
	if model == "" {
		model = "default"
	}

	// Look up provider metadata (base URL, API type) from our minimal map.
	meta, known := provider.ProviderMetaMap[cfg.Provider]

	baseURL := cfg.BaseURL
	apiType := "openai"

	if known {
		if baseURL == "" {
			baseURL = meta.BaseURL
		}
		apiType = meta.APIType
		// If the user has chosen a specific endpoint (custom base_url), infer its API type.
		if baseURL != "" && baseURL != meta.BaseURL {
			for _, ep := range meta.Endpoints {
				if ep.BaseURL == baseURL && ep.APIType != "" {
					apiType = ep.APIType
					break
				}
			}
		}
	} else {
		// Unknown provider — user must set base_url manually.
		if baseURL == "" {
			return nil, fmt.Errorf("provider %s has no known endpoint — set base_url first with: vibecode config set base_url <url>", cfg.Provider)
		}
	}

	if baseURL == "" {
		return nil, fmt.Errorf("provider %s has no endpoint — set base_url with: vibecode config set base_url <url>", cfg.Provider)
	}

	switch apiType {
	case "anthropic":
		return provider.NewAnthropicWithBaseURL(key, model, baseURL), nil
	case "openai":
		return provider.NewOpenAIWithBaseURL(key, model, baseURL), nil
	default:
		return nil, fmt.Errorf("unknown API type %q for provider %s", apiType, cfg.Provider)
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
	a := agent.NewWithConfig(p, reg, system, cfg.MaxIterations, cfg.AutoApprove, cb, cfg)
	a.SetHooks(buildHooks(cfg))
	return a.Run(ctx, message)
}

func runInteractive(p provider.Provider, reg *tool.Registry, system string, cfg *config.Config, dir string, providerPtr *provider.Provider) error {
	inputChan := make(chan string, 16)

	m := tui.New(inputChan)
	m.SetStatus(cfg.Model, dir)
	m.SetProviderName(cfg.Provider)

	// API key checker: determines which providers have configured keys.
	hasKey := func(prov string) bool {
		if prov == "ollama" {
			return true
		}
		// Normalize OpenRouter slugs through aliases (e.g. "z.ai" → "zhipu")
		if alias, ok := provider.ProviderSlugAliases[prov]; ok {
			prov = alias
		}
		if cfg.APIKey(prov) != "" {
			return true
		}
		// Provider-specific env vars not covered by config.Load()
		switch prov {
		case "deepseek":
			return os.Getenv("DEEPSEEK_API_KEY") != ""
		case "moonshotai":
			return os.Getenv("MOONSHOT_API_KEY") != "" || os.Getenv("KIMI_API_KEY") != ""
		case "zhipu":
			return os.Getenv("ZHIPU_API_KEY") != ""
		case "qwen":
			return os.Getenv("DASHSCOPE_API_KEY") != "" || os.Getenv("QWEN_API_KEY") != ""
		}
		return false
	}
	m.SetHasAPIKeyFunc(hasKey)
	m.SetConfig(cfg)

	// Wire slash commands
	cmdReg := commands.NewRegistry()
	m.SetCommandRegistry(cmdReg)
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

	// Agent is created lazily so the TUI can open even without a valid provider.
	var a *agent.Agent
	if p != nil {
		a = agent.NewWithConfig(p, reg, system, cfg.MaxIterations, cfg.AutoApprove, cb, cfg)
		a.SetHooks(buildHooks(cfg))
	} else {
		m.AppendSystemMessage("Configuration incomplete. Use /model to choose a provider and model, or /config to set your API key.", true)
	}

	// Model switch handler: rebuild provider, save config, update or create agent.
	m.SetModelChangeHandler(func(providerID, modelID string) error {
		cfg.Provider = providerID
		cfg.Model = modelID
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		newProvider, err := buildProvider(cfg)
		if err != nil {
			return err
		}
		*providerPtr = newProvider
		if a != nil {
			a.SetProvider(newProvider)
			a.SetModel(modelID)
		} else {
			a = agent.NewWithConfig(newProvider, reg, system, cfg.MaxIterations, cfg.AutoApprove, cb, cfg)
			a.SetHooks(buildHooks(cfg))
		}
		return nil
	})

	// Use a cancellable root context so SIGINT can exit the program.
	// Per-turn cancellation is handled by giving each Run() its own child context.
	rootCtx, rootCancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer rootCancel()

	go func() {
		for msg := range inputChan {
			if *providerPtr == nil {
				cb.OnError(fmt.Errorf("No provider configured. Use /model to select a provider and model, or /config to set your API key."))
				continue
			}

			// Lazily create the agent on first message if it wasn't created upfront.
			if a == nil {
				a = agent.NewWithConfig(*providerPtr, reg, system, cfg.MaxIterations, cfg.AutoApprove, cb, cfg)
				a.SetHooks(buildHooks(cfg))
			}

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
func (c *cliCallback) OnTokenUsage(usage session.SessionUsage) {
	fmt.Fprintf(os.Stderr, "tokens: %d in / %d out | cost: $%.4f\n", usage.TotalInput, usage.TotalOutput, usage.TotalCost)
}
func (c *cliCallback) OnCompaction(summary string) {
	fmt.Fprintf(os.Stderr, "[context compacted]\n")
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
		cfg.Provider = value
		return nil
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
