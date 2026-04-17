package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/vibecode/vibecode/config"
	"github.com/vibecode/vibecode/internal/agent"
	"github.com/vibecode/vibecode/internal/provider"
	"github.com/vibecode/vibecode/internal/tool"
	"github.com/vibecode/vibecode/internal/tui"
)

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

	rootCmd.Flags().StringVar(&flagProvider, "provider", "", "LLM provider (anthropic, openai, ollama)")
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

	// CLI flag overrides
	if flagProvider != "" {
		cfg.Provider = flagProvider
	}
	if flagModel != "" {
		cfg.Model = flagModel
	}

	// Detect project root
	dir, _ := os.Getwd()

	// Build tool registry
	reg := tool.NewRegistry()
	reg.Register(tool.ReadFile{})
	reg.Register(tool.WriteFile{})
	reg.Register(tool.EditFile{})
	reg.Register(tool.Glob{})
	reg.Register(tool.Grep{})
	reg.Register(tool.Shell{})
	reg.Register(tool.Git{})

	// Build provider
	p, err := buildProvider(cfg)
	if err != nil {
		return err
	}

	// System prompt
	system := buildSystemPrompt(dir)

	// One-shot mode
	if len(args) == 1 {
		return runOneShot(cmd, args[0], p, reg, system, cfg)
	}

	// Interactive mode
	return runInteractive(p, reg, system, cfg, dir)
}

func buildProvider(cfg *config.Config) (provider.Provider, error) {
	switch cfg.Provider {
	case "anthropic", "zhipu":
		key := cfg.APIKey("anthropic")
		if key == "" {
			return nil, fmt.Errorf("set ANTHROPIC_API_KEY or add api_keys.anthropic to ~/.vibecode/config.json")
		}
		model := cfg.Model
		if model == "" {
			model = "claude-sonnet-4-6"
		}
		if cfg.BaseURL != "" {
			return provider.NewAnthropicWithBaseURL(key, model, cfg.BaseURL), nil
		}
		return provider.NewAnthropic(key, model), nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s (only 'anthropic' is implemented in MVP)", cfg.Provider)
	}
}

func buildSystemPrompt(dir string) string {
	return fmt.Sprintf(`You are Vibe Code, an AI coding agent. You help users write, edit, and understand code.

Working directory: %s

You have tools to read, write, and edit files; search code; run shell commands; and interact with git. Use them to accomplish the user's tasks.

Rules:
- Always read files before editing them
- Use edit_file for targeted changes, write_file for new files
- Keep explanations concise
- Run tests after making changes when appropriate`, filepath.Base(dir))
}

func runOneShot(cmd *cobra.Command, message string, p provider.Provider, reg *tool.Registry, system string, cfg *config.Config) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cb := &cliCallback{}
	a := agent.New(p, reg, system, cfg.MaxIterations, cfg.AutoApprove, cb)
	return a.Run(ctx, message)
}

func runInteractive(p provider.Provider, reg *tool.Registry, system string, cfg *config.Config, dir string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	inputChan := make(chan string, 16)

	m := tui.New(inputChan)
	m.SetStatus(cfg.Model, dir)

	pgm := tea.NewProgram(m)

	cb := tui.NewCallback(pgm)
	a := agent.New(p, reg, system, cfg.MaxIterations, cfg.AutoApprove, cb)

	// Agent loop goroutine
	go func() {
		for msg := range inputChan {
			if err := a.Run(ctx, msg); err != nil {
				cb.OnError(err)
			}
		}
	}()

	_, err := pgm.Run()
	close(inputChan)
	return err
}

// cliCallback is a simple callback for one-shot mode that prints to stdout.
type cliCallback struct{}

func (c *cliCallback) OnText(text string)               { fmt.Print(text) }
func (c *cliCallback) OnToolStart(name, id string)       { fmt.Printf("\n◐ %s...\n", name) }
func (c *cliCallback) OnToolOutput(name, id, output string, err error) {
	icon := "✓"
	if err != nil {
		icon = "✗"
	}
	summary := output
	if len(summary) > 120 {
		summary = summary[:120] + "..."
	}
	fmt.Printf("%s %s: %s\n", icon, name, summary)
}
func (c *cliCallback) OnDone()    { fmt.Println() }
func (c *cliCallback) OnError(err error) { fmt.Fprintf(os.Stderr, "Error: %s\n", err) }
