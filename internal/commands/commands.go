package commands

import (
	"fmt"
	"strings"
)

// CommandType determines how a command is handled.
type CommandType int

const (
	// TypeLocal runs a function locally and returns a result.
	TypeLocal CommandType = iota
	// TypePrompt sends text to the model as a user message.
	TypePrompt
)

// Result is returned from a local command execution.
type Result struct {
	Output string // text to display to the user
	Prompt string // if non-empty, send this as a user message to the model
	Clear  bool   // if true, clear conversation history
}

// Command defines a slash command.
type Command struct {
	Name        string
	Aliases     []string
	Description string
	Type        CommandType
	Handler     func(args string) Result // for TypeLocal
	PromptText  string                   // for TypePrompt, with {{args}} placeholder
}

// Registry holds all registered slash commands.
type Registry struct {
	commands map[string]*Command
}

// NewRegistry creates a command registry with built-in commands.
func NewRegistry() *Registry {
	r := &Registry{
		commands: make(map[string]*Command),
	}

	r.registerBuiltins()
	return r
}

func (r *Registry) registerBuiltins() {
	r.registerHelp()
	r.registerClear()
	r.registerModel()
	r.registerConfig()
	r.registerExit()
	r.registerPlan()
	r.registerCompact()
	r.registerStatus()
	r.registerTools()
	r.registerDiff()
	r.registerSkills()
	r.registerUsage()
	r.registerApprove()
}

// Register adds a command to the registry.
func (r *Registry) Register(cmd Command) {
	r.commands[cmd.Name] = &cmd
	for _, alias := range cmd.Aliases {
		r.commands[alias] = &cmd
	}
}

// Lookup finds a command by name or alias.
func (r *Registry) Lookup(name string) (*Command, bool) {
	cmd, ok := r.commands[name]
	return cmd, ok
}

// All returns all unique commands (by primary name).
func (r *Registry) All() []*Command {
	seen := make(map[string]bool)
	var result []*Command
	for _, cmd := range r.commands {
		if !seen[cmd.Name] {
			seen[cmd.Name] = true
			result = append(result, cmd)
		}
	}
	return result
}

// ParseInput checks if input is a slash command and returns the command name and args.
func ParseInput(input string) (cmdName string, args string, ok bool) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return "", "", false
	}

	parts := strings.SplitN(input[1:], " ", 2)
	cmdName = parts[0]
	if len(parts) > 1 {
		args = parts[1]
	}

	return cmdName, args, true
}

func (r *Registry) helpHandler(args string) Result {
	var b strings.Builder
	b.WriteString("Available commands:\n\n")

	for _, cmd := range r.All() {
		aliases := ""
		if len(cmd.Aliases) > 0 {
			aliases = fmt.Sprintf(" (%s)", strings.Join(cmd.Aliases, ", "))
		}
		b.WriteString(fmt.Sprintf("  /%-14s%s%s\n", cmd.Name, cmd.Description, aliases))
	}

	b.WriteString("\nKeyboard shortcuts:\n")
	b.WriteString("  enter         Send message\n")
	b.WriteString("  shift+enter   New line\n")
	b.WriteString("  ctrl+o        Expand/collapse tool output\n")
	b.WriteString("  ctrl+c        Stop generation or exit\n")
	b.WriteString("  pgup/pgdown   Scroll transcript\n")

	return Result{Output: b.String()}
}
