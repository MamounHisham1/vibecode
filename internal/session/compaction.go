package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/vibecode/vibecode/internal/provider"
)

const (
	pruneProtect = 40000
	pruneMinimum = 20000
)

type CompactionConfig struct {
	Auto     bool
	Prune    bool
	Reserved int
}

type Compactor struct {
	provider provider.Provider
	model    string
	providerName string
}

func NewCompactor(p provider.Provider, model string, providerName string) *Compactor {
	return &Compactor{
		provider: p,
		model:    model,
		providerName: providerName,
	}
}

func (c *Compactor) Compact(ctx context.Context, history []provider.Message) ([]provider.Message, string, error) {
	prompt := buildCompactionPrompt(history)

	messages := []provider.Message{
		provider.UserMessage(prompt),
	}

	req := provider.Request{
		System:   "You are a helpful assistant that summarizes conversations concisely and accurately.",
		Messages: messages,
		Tools:    nil,
	}

	events, err := c.provider.Stream(ctx, req)
	if err != nil {
		return nil, "", fmt.Errorf("compaction stream failed: %w", err)
	}

	var summary strings.Builder
	for ev := range events {
		switch e := ev.(type) {
		case provider.TextEvent:
			summary.WriteString(e.Text)
		case provider.ErrorEvent:
			return nil, "", e.Err
		case provider.DoneEvent:
			if e.Usage != nil {
				log.Printf("compaction usage: input=%d output=%d", e.Usage.InputTokens, e.Usage.OutputTokens)
			}
		}
	}

	summaryText := summary.String()
	if summaryText == "" {
		return history, "", nil
	}

	compacted := []provider.Message{
		provider.UserMessage("What did we do so far?"),
		provider.AssistantTextMessage(summaryText),
	}

	return compacted, summaryText, nil
}

func buildCompactionPrompt(history []provider.Message) string {
	var conv strings.Builder
	conv.WriteString("Please provide a detailed summary of the following conversation that would allow continuing it effectively.\n\n")
	conv.WriteString("Structure your summary with these sections:\n")
	conv.WriteString("## Goal\n")
	conv.WriteString("## Instructions\n")
	conv.WriteString("## Discoveries\n")
	conv.WriteString("## Accomplished\n")
	conv.WriteString("## Relevant files / directories\n\n")
	conv.WriteString("---\n\n")

	for _, msg := range history {
		switch msg.Role {
		case "user":
			conv.WriteString("USER:\n")
			for _, block := range msg.Content {
				if block.Type == "text" {
					conv.WriteString(block.Text)
					conv.WriteString("\n")
				} else if block.Type == "tool_result" {
					result := string(block.Result)
					if len(result) > 500 {
						result = result[:500] + "..."
					}
					conv.WriteString(fmt.Sprintf("[tool result for %s]: %s\n", block.ToolCallID, result))
				}
			}
		case "assistant":
			conv.WriteString("ASSISTANT:\n")
			for _, block := range msg.Content {
				if block.Type == "text" {
					conv.WriteString(block.Text)
					conv.WriteString("\n")
				} else if block.Type == "tool_use" {
					input := string(block.Input)
					if len(input) > 200 {
						input = input[:200] + "..."
					}
					conv.WriteString(fmt.Sprintf("[called tool %s: %s]\n", block.ToolName, input))
				}
			}
		}
		conv.WriteString("\n")
	}

	return conv.String()
}

func PruneHistory(history []provider.Message, cfg *CompactionConfig) []provider.Message {
	if cfg != nil && !cfg.Prune {
		return history
	}

	pruned := make([]provider.Message, len(history))
	copy(pruned, history)

	totalEstimate := 0
	for i := len(pruned) - 1; i >= 0; i-- {
		msg := pruned[i]
		if msg.Role != "user" {
			continue
		}
		for _, block := range msg.Content {
			if block.Type == "tool_result" {
				totalEstimate += EstimateTokens(string(block.Result))
			}
		}
	}

	if totalEstimate < pruneMinimum {
		return pruned
	}

	runningTotal := 0
	pruneThreshold := totalEstimate - pruneProtect

	for i := 0; i < len(pruned)-4 && runningTotal < pruneThreshold; i++ {
		msg := pruned[i]
		if msg.Role != "user" {
			continue
		}
		for j := range msg.Content {
			block := &msg.Content[j]
			if block.Type == "tool_result" {
				tokenEstimate := EstimateTokens(string(block.Result))
				runningTotal += tokenEstimate
				if runningTotal < pruneThreshold {
					result := "[Old tool result content cleared]"
					block.Result = json.RawMessage(fmt.Sprintf("%q", result))
				}
			}
		}
		pruned[i] = msg
	}

	return pruned
}
