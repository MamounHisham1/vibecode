package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type Shell struct{}

func (Shell) Name() string { return "shell" }

func (Shell) Description() string {
	return "Execute a bash command. Has a 2-minute timeout."
}

func (Shell) Parameters() json.RawMessage {
	return schema(map[string]any{
		"command": map[string]any{"type": "string", "description": "Bash command to execute"},
	}, "command")
}

type shellInput struct {
	Command string `json:"command"`
}

var blockedPatterns = []string{
	"rm -rf /",
	"rm -rf /*",
	"mkfs.",
	"dd if=",
	":(){ :|:& };:",
	"> /dev/sd",
	"DROP TABLE",
	"DROP DATABASE",
}

func (Shell) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in shellInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}

	cmdStr := strings.TrimSpace(in.Command)
	if cmdStr == "" {
		return nil, fmt.Errorf("empty command")
	}

	for _, blocked := range blockedPatterns {
		if strings.Contains(cmdStr, blocked) {
			return nil, fmt.Errorf("command blocked for safety: contains %q", blocked)
		}
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", cmdStr)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += "STDERR:\n" + stderr.String()
	}

	if ctx.Err() == context.DeadlineExceeded {
		output += "\nCommand timed out after 2 minutes."
	}

	result := map[string]any{
		"output":  output,
		"success": err == nil,
	}
	if err != nil && ctx.Err() != context.DeadlineExceeded {
		result["error"] = err.Error()
	}

	return json.Marshal(result)
}
