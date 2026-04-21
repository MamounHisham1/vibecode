package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/vibecode/vibecode/internal/provider"
)

// pruneProtectRatio and pruneMinimumRatio define the default fractions of the
// effective context window used for pruning decisions. They replace the prior
// hard-coded absolute values so pruning scales correctly across model sizes.
const (
	pruneProtectRatio = 0.20 // protect newest ~20% of context
	pruneMinimumRatio = 0.10 // only prune if we can free ~10%
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

func PruneHistory(history []provider.Message, cfg *CompactionConfig, modelID string) []provider.Message {
	if cfg != nil && !cfg.Prune {
		return history
	}

	m := provider.LookupModel(modelID)
	contextWindow := m.Limits.Context
	if contextWindow == 0 {
		contextWindow = 200000
	}
	effectiveContext := contextWindow - provider.MaxOutputTokens(modelID)
	if effectiveContext <= 0 {
		effectiveContext = contextWindow
	}

	pruneProtect := int(float64(effectiveContext) * pruneProtectRatio)
	pruneMinimum := int(float64(effectiveContext) * pruneMinimumRatio)

	pruned := make([]provider.Message, len(history))
	copy(pruned, history)

	// Walk backward (newest first) to protect recent tool results.
	protected := 0
	var toPrune []struct{ msgIdx, blockIdx int; estimate int }
	prunableTotal := 0

	for i := len(pruned) - 1; i >= 0; i-- {
		msg := pruned[i]
		if msg.Role != "user" {
			continue
		}
		for j := range msg.Content {
			block := &msg.Content[j]
			if block.Type != "tool_result" {
				continue
			}
			estimate := EstimateTokens(string(block.Result))
			if protected+estimate > pruneProtect {
				toPrune = append(toPrune, struct{ msgIdx, blockIdx int; estimate int }{i, j, estimate})
				prunableTotal += estimate
			} else {
				protected += estimate
			}
		}
	}

	if prunableTotal < pruneMinimum {
		return pruned
	}

	for _, idx := range toPrune {
		block := &pruned[idx.msgIdx].Content[idx.blockIdx]
		result := "[Old tool result content cleared]"
		block.Result = json.RawMessage(fmt.Sprintf("%q", result))
	}

	return pruned
}
