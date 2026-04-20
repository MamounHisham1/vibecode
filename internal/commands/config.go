package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (r *Registry) registerConfig() {
	r.Register(Command{
		Name:        "config",
		Aliases:     []string{"settings", "cfg"},
		Description: "Show or edit configuration",
		Type:        TypeLocal,
		Handler:     r.configHandler,
	})
}

func (r *Registry) configHandler(args string) Result {
	home, err := os.UserHomeDir()
	if err != nil {
		return Result{Output: "Error: could not determine home directory"}
	}

	configPath := filepath.Join(home, ".vibecode", "config.json")

	if args == "" {
		return Result{Output: fmt.Sprintf("Config file: %s\n\nUse /config get <key> or /config set <key> <value> to modify.", configPath)}
	}

	parts := splitArgs(args)
	if len(parts) == 0 {
		return Result{Output: "Usage: /config [get|set] <key> [value]"}
	}

	switch parts[0] {
	case "get":
		if len(parts) < 2 {
			return Result{Output: "Usage: /config get <key>"}
		}
		return Result{Output: fmt.Sprintf("Key: %s (not yet implemented - edit %s directly)", parts[1], configPath)}
	case "set":
		if len(parts) < 3 {
			return Result{Output: "Usage: /config set <key> <value>"}
		}
		return Result{Output: fmt.Sprintf("Key: %s, Value: %s (not yet implemented - edit %s directly)", parts[1], parts[2], configPath)}
	default:
		return Result{Output: "Usage: /config [get|set] <key> [value]"}
	}
}

func splitArgs(s string) []string {
	return splitQuoted(s)
}

func splitQuoted(s string) []string {
	var parts []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)

	for _, r := range s {
		switch {
		case !inQuote && (r == '"' || r == '\''):
			inQuote = true
			quoteChar = r
		case inQuote && r == quoteChar:
			inQuote = false
			quoteChar = 0
		case !inQuote && r == ' ':
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}
