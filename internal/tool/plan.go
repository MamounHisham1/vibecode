package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PlanModeState tracks the current plan mode status.
type PlanModeState struct {
	Active   bool   `json:"active"`
	PlanFile string `json:"plan_file,omitempty"`
}

// EnterPlanMode switches the agent to plan mode (read-only).
type EnterPlanMode struct{}

func (EnterPlanMode) Name() string { return "enter_plan_mode" }

func (EnterPlanMode) Description() string {
	return "Enter plan mode for exploring the codebase and designing an implementation approach. " +
		"In plan mode, only read-only tools are available. Present your plan to the user for approval."
}

func (EnterPlanMode) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"plan_title": {
				"type": "string",
				"description": "Short title for the plan"
			}
		},
		"required": ["plan_title"]
	}`)
}

type planModeInput struct {
	PlanTitle string `json:"plan_title"`
}

func (EnterPlanMode) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in planModeInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}

	// Create plan file
	dir, _ := os.Getwd()
	planDir := filepath.Join(dir, ".vibecode", "plans")
	os.MkdirAll(planDir, 0755)

	slug := strings.ReplaceAll(strings.ToLower(in.PlanTitle), " ", "-")
	if len(slug) > 50 {
		slug = slug[:50]
	}
	timestamp := time.Now().Format("2006-01-02-150405")
	planFile := filepath.Join(planDir, fmt.Sprintf("%s-%s.md", slug, timestamp))

	planContent := fmt.Sprintf("# %s\n\n> Created: %s\n\n## Plan\n\n1. \n\n## Notes\n\n", in.PlanTitle, time.Now().Format("2006-01-02 15:04"))

	if err := os.WriteFile(planFile, []byte(planContent), 0644); err != nil {
		return nil, fmt.Errorf("create plan file: %w", err)
	}

	return json.Marshal(map[string]any{
		"status":    "entered_plan_mode",
		"plan_file": planFile,
		"message":   "Entered plan mode. Only read-only tools are available. Explore the codebase and design your approach.",
	})
}

// ExitPlanMode switches the agent back to normal execution mode.
type ExitPlanMode struct{}

func (ExitPlanMode) Name() string { return "exit_plan_mode" }

func (ExitPlanMode) Description() string {
	return "Exit plan mode and return to normal execution. The plan file is preserved for reference."
}

func (ExitPlanMode) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {}
	}`)
}

func (ExitPlanMode) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]any{
		"status":  "exited_plan_mode",
		"message": "Exited plan mode. All tools are now available for implementation.",
	})
}
