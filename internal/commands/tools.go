package commands

import (
	"fmt"
	"strings"
)

func (r *Registry) registerTools() {
	r.Register(Command{
		Name:        "tools",
		Aliases:     []string{"t", "tool"},
		Description: "List available tools",
		Type:        TypeLocal,
		Handler:     r.toolsHandler,
	})
}

func (r *Registry) toolsHandler(args string) Result {
	tools := []struct {
		name string
		desc string
	}{
		{"read_file", "Read file contents with optional offset/limit"},
		{"write_file", "Create or overwrite a file"},
		{"edit_file", "Replace text in a file"},
		{"shell", "Execute bash commands"},
		{"git", "Run git commands"},
		{"glob", "Find files by pattern"},
		{"grep", "Search file contents by regex"},
		{"web_fetch", "Fetch content from a URL"},
		{"web_search", "Search the web (requires API key)"},
		{"ask_user", "Ask the user a question"},
		{"todo_write", "Manage a todo list"},
		{"enter_plan_mode", "Enter read-only plan mode"},
		{"exit_plan_mode", "Exit plan mode"},
	}

	var b strings.Builder
	b.WriteString("Available tools:\n\n")
	for _, t := range tools {
		b.WriteString(fmt.Sprintf("  %-18s%s\n", t.name, t.desc))
	}
	return Result{Output: b.String()}
}
