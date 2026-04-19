package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// AskFunc is a function that asks the user a question and returns their answer.
// It's injected from the TUI layer so the ask_user tool works in both TUI and CLI modes.
type AskFunc func(ctx context.Context, question string, options []Option) (string, error)

// Option represents a single choice for the user.
type Option struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// AskUser asks the user a question, optionally with multiple-choice options.
type AskUser struct {
	askFn *AskFunc // pointer so it can be updated after registration
}

// NewAskUser creates an AskUser tool with a custom ask function (for TUI integration).
func NewAskUser(fn AskFunc) *AskUser {
	return &AskUser{askFn: &fn}
}

// SetAskFunc updates the ask function (for late binding in TUI mode).
func (a *AskUser) SetAskFunc(fn AskFunc) {
	a.askFn = &fn
}

// AskUserStdin returns an AskUser that reads from stdin (for CLI/one-shot mode).
func AskUserStdin() AskUser {
	return AskUser{askFn: nil}
}

func (AskUser) Name() string { return "ask_user" }

func (AskUser) Description() string {
	return "Ask the user a question and wait for their response. Use when you need clarification or a decision. " +
		"Supports multiple choice options and free text input."
}

func (AskUser) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"question": {
				"type": "string",
				"description": "The question to ask the user. Should be clear, specific, and end with a question mark."
			},
			"header": {
				"type": "string",
				"description": "Very short label for the question topic (e.g. 'Auth method', 'Library', 'Approach')"
			},
			"options": {
				"type": "array",
				"description": "Available choices for the user. Each option has a label and optional description.",
				"items": {
					"type": "object",
					"properties": {
						"label": {
							"type": "string",
							"description": "Display text for this option (1-5 words)"
						},
						"description": {
							"type": "string",
							"description": "Explanation of what this option means or what happens if chosen"
						}
					},
					"required": ["label"]
				},
				"minItems": 2,
				"maxItems": 4
			}
		},
		"required": ["question"]
	}`)
}

type askInput struct {
	Question string   `json:"question"`
	Header   string   `json:"header"`
	Options  []Option `json:"options"`
}

func (a AskUser) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in askInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}

	if in.Question == "" {
		return nil, fmt.Errorf("question is required")
	}

	// Use injected ask function if available (TUI mode)
	if a.askFn != nil && *a.askFn != nil {
		answer, err := (*a.askFn)(ctx, in.Question, in.Options)
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]any{
			"question": in.Question,
			"answer":   answer,
		})
	}

	// Fallback: stdin-based interaction (CLI/one-shot mode)
	return askStdin(in)
}

func askStdin(in askInput) (json.RawMessage, error) {
	fmt.Printf("\n  %s\n", in.Question)

	if len(in.Options) > 0 {
		for i, opt := range in.Options {
			if opt.Description != "" {
				fmt.Printf("  %d. %s - %s\n", i+1, opt.Label, opt.Description)
			} else {
				fmt.Printf("  %d. %s\n", i+1, opt.Label)
			}
		}
		fmt.Print("\n  Your choice (number or text): ")
	} else {
		fmt.Print("\n  Your answer: ")
	}

	var answer string
	fmt.Scanln(&answer)
	answer = strings.TrimSpace(answer)

	// Map numeric answer to option label
	if len(in.Options) > 0 {
		idx := 0
		if _, err := fmt.Sscanf(answer, "%d", &idx); err == nil && idx >= 1 && idx <= len(in.Options) {
			answer = in.Options[idx-1].Label
		}
	}

	return json.Marshal(map[string]any{
		"question": in.Question,
		"answer":   answer,
	})
}
