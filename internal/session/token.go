package session

import (
	"fmt"

	"github.com/vibecode/vibecode/internal/provider"
)

const charsPerToken = 4

func EstimateTokens(input string) int {
	if input == "" {
		return 0
	}
	return len(input) / charsPerToken
}

func EstimateMessageTokens(msg provider.Message) int {
	total := 0
	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			total += EstimateTokens(block.Text)
		case "tool_use":
			total += EstimateTokens(string(block.Input))
			total += EstimateTokens(block.ToolName)
		case "tool_result":
			total += EstimateTokens(string(block.Result))
		}
	}
	// Message overhead: role label, formatting tokens, etc.
	total += 4
	return total
}

func EstimateRequestTokens(system string, msgs []provider.Message) int {
	total := EstimateTokens(system)
	for _, msg := range msgs {
		total += EstimateMessageTokens(msg)
	}
	return total
}

func EstimateOutputTokens(text string, toolCalls []provider.ToolCallEvent) int {
	total := EstimateTokens(text)
	for _, tc := range toolCalls {
		total += EstimateTokens(string(tc.Input))
		total += EstimateTokens(tc.Name)
	}
	return total
}

func FormatTokenCount(tokens int) string {
	if tokens >= 1000 {
		return fmt.Sprintf("%.1fk", float64(tokens)/1000.0)
	}
	return fmt.Sprintf("%d", tokens)
}

func EstimateStepTokens(system string, history []provider.Message, outputText string, toolCalls []provider.ToolCallEvent) TokenUsage {
	input := EstimateRequestTokens(system, history)
	output := EstimateOutputTokens(outputText, toolCalls)
	return TokenUsage{
		Input:  input,
		Output: output,
	}
}

// EstimateContextSize estimates the current context-window size.
// If anchorUsage/anchorLen are provided, it anchors to the last known
// API-reported size and estimates only the delta, giving much better
// accuracy than estimating the entire history from scratch each time.
func EstimateContextSize(system string, history []provider.Message, anchorSize int, anchorLen int) int {
	if anchorSize > 0 && anchorLen > 0 && anchorLen <= len(history) {
		size := anchorSize
		for i := anchorLen; i < len(history); i++ {
			size += EstimateMessageTokens(history[i])
		}
		return size
	}
	return EstimateRequestTokens(system, history)
}
