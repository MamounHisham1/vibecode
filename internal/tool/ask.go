package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type AskUser struct{}

func (AskUser) Name() string { return "ask_user" }

func (AskUser) Description() string {
	return "Ask the user a question and wait for their response. Use when you need clarification or a decision."
}

func (AskUser) Parameters() json.RawMessage {
	return schema(map[string]any{
		"question": map[string]any{"type": "string", "description": "Question to ask the user"},
		"options": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "Optional list of choices for the user to pick from",
		},
	}, "question")
}

type askInput struct {
	Question string   `json:"question"`
	Options  []string `json:"options"`
}

func (AskUser) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in askInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}

	if in.Question == "" {
		return nil, fmt.Errorf("question is required")
	}

	fmt.Printf("\n  %s\n", in.Question)

	if len(in.Options) > 0 {
		for i, opt := range in.Options {
			fmt.Printf("  %d. %s\n", i+1, opt)
		}
		fmt.Print("\n  Your choice: ")
	} else {
		fmt.Print("\n  Your answer: ")
	}

	var answer string
	fmt.Fscanln(os.Stdin, &answer)
	answer = strings.TrimSpace(answer)

	// If they typed a number and we have options, map it
	if len(in.Options) > 0 {
		idx := 0
		if _, err := fmt.Sscanf(answer, "%d", &idx); err == nil && idx >= 1 && idx <= len(in.Options) {
			answer = in.Options[idx-1]
		}
	}

	return json.Marshal(answer)
}
