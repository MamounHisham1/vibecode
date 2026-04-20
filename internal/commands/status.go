package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (r *Registry) registerStatus() {
	r.Register(Command{
		Name:        "status",
		Aliases:     []string{"st", "info"},
		Description: "Show current session status",
		Type:        TypeLocal,
		Handler:     r.statusHandler,
	})
}

func (r *Registry) statusHandler(args string) Result {
	var b strings.Builder

	b.WriteString("Session Status\n")
	b.WriteString("==============\n\n")

	// Working directory
	wd, err := os.Getwd()
	if err == nil {
		b.WriteString(fmt.Sprintf("Directory: %s\n", wd))
	}

	// Config file
	home, _ := os.UserHomeDir()
	configPath := filepath.Join(home, ".vibecode", "config.json")
	if _, err := os.Stat(configPath); err == nil {
		b.WriteString(fmt.Sprintf("Config:    %s\n", configPath))
	}

	// Plan mode indicator
	b.WriteString("\nUse /help to see all available commands.\n")

	return Result{Output: b.String()}
}
