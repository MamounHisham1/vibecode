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

func EstimateRequestTokens(system string, msgs []provider.Message) int {
	total := EstimateTokens(system)
	for _, msg := range msgs {
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
