package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"strings"
)

type Grep struct{}

func (Grep) Name() string { return "grep" }

func (Grep) Description() string {
	return "Search file contents using regex patterns. Uses ripgrep when available, falls back to grep."
}

func (Grep) Parameters() json.RawMessage {
	return schema(map[string]any{
		"pattern": map[string]any{"type": "string", "description": "Regex pattern to search for"},
		"path":    map[string]any{"type": "string", "description": "Directory or file to search in"},
	}, "pattern")
}

type grepInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

func (Grep) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in grepInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}

	searchPath := "."
	if in.Path != "" {
		searchPath = in.Path
	}

	// Try ripgrep first
	cmd := exec.CommandContext(ctx, "rg", "--no-heading", "--line-number", "--color", "never",
		"--max-count", "200", in.Pattern, searchPath)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	if err != nil {
		// rg returns exit code 1 for no matches — that's fine
		if strings.Contains(out.String(), "No files were found") ||
			strings.Contains(out.String(), "no matches") ||
			out.Len() == 0 {
			return json.Marshal("No matches found.")
		}
		// Maybe rg isn't installed, try grep
		return fallbackGrep(ctx, in.Pattern, searchPath)
	}

	result := out.String()
	if result == "" {
		return json.Marshal("No matches found.")
	}

	// Truncate if too long
	lines := strings.Split(result, "\n")
	if len(lines) > 200 {
		result = strings.Join(lines[:200], "\n") + "\n... (truncated)"
	}

	return json.Marshal(result)
}

func fallbackGrep(ctx context.Context, pattern, path string) (json.RawMessage, error) {
	cmd := exec.CommandContext(ctx, "grep", "-rn", "-E", "--color=never",
		"--max-count=200", pattern, path)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	if err != nil {
		return []byte(`"No matches found."`), nil
	}

	result, _ := json.Marshal(out.String())
	return result, nil
}
