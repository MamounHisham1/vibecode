package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type Git struct{}

func (Git) Name() string { return "git" }

func (Git) Description() string {
	return "Run git commands (status, diff, log, commit, branch, add)."
}

func (Git) Parameters() json.RawMessage {
	return schema(map[string]any{
		"command": map[string]any{"type": "string", "description": "Git command to run (e.g. 'status', 'diff', 'log --oneline -10')"},
	}, "command")
}

type gitInput struct {
	Command string `json:"command"`
}

// Allowed git subcommands — only these are permitted.
var allowedSubcommands = map[string]bool{
	"status": true, "diff": true, "log": true, "branch": true,
	"add": true, "commit": true, "show": true, "stash": true,
	"remote": true, "fetch": true, "pull": true, "push": true,
	"checkout": true, "switch": true, "merge": true, "rebase": true,
	"reset": true, "restore": true, "worktree": true, "tag": true,
	"init": true, "blame": true, "shortlog": true, "reflog": true,
}

func (Git) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in gitInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}

	cmdStr := strings.TrimSpace(in.Command)
	if cmdStr == "" {
		return nil, fmt.Errorf("empty command")
	}

	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty command")
	}

	// Handle "git status" or just "status"
	subcmd := parts[0]
	if subcmd == "git" && len(parts) > 1 {
		subcmd = parts[1]
		parts = parts[1:]
	}

	if !allowedSubcommands[subcmd] {
		return nil, fmt.Errorf("git %q is not allowed", subcmd)
	}

	cmd := exec.CommandContext(ctx, "git", parts...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}

	if output == "" && err != nil {
		return nil, fmt.Errorf("git %s failed: %w", subcmd, err)
	}

	return json.Marshal(output)
}
