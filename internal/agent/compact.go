package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/vibecode/vibecode/internal/provider"
)

const (
	// autoCompactBufferTokens reserves space below the effective window for the
	// compaction summary itself (mirrors Claude Code's 13K buffer).
	autoCompactBufferTokens = 13000

	// minMessagesToKeep is the minimum number of recent messages to preserve
	// during compaction so the model has immediate context to continue.
	minMessagesToKeep = 6

	// maxConsecutiveCompactFailures is the circuit-breaker threshold.
	maxConsecutiveCompactFailures = 3

	// bytesPerToken is the rough heuristic for English text (chars / 4).
	bytesPerToken = 4

	// jsonBytesPerToken is used for dense JSON content (chars / 2).
	jsonBytesPerToken = 2
)

var consecutiveCompactFailures int

// compactPrompt is the system prompt sent to the LLM to generate a summary.
const compactPrompt = `You are a helpful AI assistant tasked with summarizing a conversation so far. Your job is to create a concise but comprehensive summary that preserves all important context needed to continue the conversation.

Include these sections if applicable:
- **Primary Request**: What the user originally asked for
- **Technical Concepts**: Key technical details discussed
- **Files/Code**: Important files touched, code changes made or planned
- **Errors/Fixes**: Any errors encountered and how they were resolved
- **Problem Solving**: Decisions made and reasoning
- **All User Messages**: List every user message (summarized if long)
- **Pending Tasks**: What still needs to be done
- **Current State**: Where we left off and what to do next

Be specific about file paths, function names, and code changes. This summary will replace the full conversation history.`

// estimateMessageTokens returns a rough token estimate for a single message.
func estimateMessageTokens(msg provider.Message) int {
	var total int
	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			total += roughTokenEstimate(block.Text, bytesPerToken)
		case "tool_use":
			nameTokens := roughTokenEstimate(block.ToolName, bytesPerToken)
			inputTokens := roughTokenEstimate(string(block.Input), jsonBytesPerToken)
			total += nameTokens + inputTokens + 5 // overhead for structure
		case "tool_result":
			total += roughTokenEstimate(string(block.Result), jsonBytesPerToken)
		}
	}
	return total
}

// roughTokenEstimate divides content length by the given bytes-per-token ratio.
func roughTokenEstimate(content string, bpt int) int {
	if bpt <= 0 {
		bpt = bytesPerToken
	}
	if len(content) == 0 {
		return 0
	}
	result := len(content) / bpt
	if result == 0 {
		return 1
	}
	return result
}

// estimateTotalTokens returns a rough token count for the entire message history.
func estimateTotalTokens(messages []provider.Message) int {
	var total int
	for _, msg := range messages {
		total += estimateMessageTokens(msg)
	}
	return total
}

// effectiveContextWindow returns the usable context window minus a buffer for
// the compaction summary output.
func (a *Agent) effectiveContextWindow() int {
	return a.contextWindow - autoCompactBufferTokens
}

// autoCompactThreshold returns the token count at which auto-compact triggers.
func (a *Agent) autoCompactThreshold() int {
	threshold := float64(a.effectiveContextWindow()) * a.compactThreshold
	return int(threshold)
}

// checkAutoCompact returns whether compaction should run and the estimated token count.
func (a *Agent) checkAutoCompact() (bool, int) {
	a.mu.Lock()
	defer a.mu.Unlock()

	estimated := estimateTotalTokens(a.history)
	threshold := a.autoCompactThreshold()

	// Don't compact if history is too small
	if len(a.history) <= minMessagesToKeep+2 {
		return false, estimated
	}

	return estimated >= threshold, estimated
}

// compactHistory performs intelligent context compaction by summarizing old messages
// and keeping recent messages intact.
func (a *Agent) compactHistory(ctx context.Context) error {
	a.mu.Lock()
	if a.compacting {
		a.mu.Unlock()
		return fmt.Errorf("compaction already in progress")
	}
	a.compacting = true
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		a.compacting = false
		a.mu.Unlock()
	}()

	// Circuit breaker
	if consecutiveCompactFailures >= maxConsecutiveCompactFailures {
		return fmt.Errorf("too many consecutive compaction failures, giving up")
	}

	a.mu.Lock()
	history := a.history
	a.mu.Unlock()

	if len(history) <= minMessagesToKeep {
		return nil
	}

	// Split history: messages to summarize vs messages to keep
	splitIdx := len(history) - minMessagesToKeep
	if splitIdx < 1 {
		return nil
	}

	toSummarize := history[:splitIdx]
	toKeep := history[splitIdx:]

	// Build the conversation text for summarization
	var convBuilder strings.Builder
	for _, msg := range toSummarize {
		role := msg.Role
		if role == "user" && len(msg.Content) > 0 && msg.Content[0].Type == "tool_result" {
			role = "tool_result"
		}
		convBuilder.WriteString(fmt.Sprintf("[%s]\n", role))
		for _, block := range msg.Content {
			switch block.Type {
			case "text":
				convBuilder.WriteString(block.Text)
				convBuilder.WriteString("\n")
			case "tool_use":
				convBuilder.WriteString(fmt.Sprintf("Tool call: %s\n", block.ToolName))
				if len(block.Input) > 0 {
					convBuilder.WriteString(fmt.Sprintf("Input: %s\n", truncateStr(string(block.Input), 500)))
				}
			case "tool_result":
				convBuilder.WriteString(fmt.Sprintf("Tool result: %s\n", truncateStr(string(block.Result), 500)))
			}
		}
		convBuilder.WriteString("\n---\n\n")
	}

	conversationText := convBuilder.String()
	if len(conversationText) > 100000 {
		conversationText = conversationText[:100000] + "\n[...truncated for summarization...]"
	}

	// Ask the LLM to summarize
	summary, err := a.summarizeWithLLM(ctx, conversationText)
	if err != nil {
		consecutiveCompactFailures++
		log.Printf("Compaction LLM call failed: %v", err)
		return err
	}

	consecutiveCompactFailures = 0

	// Build the summary message
	summaryMsg := provider.UserMessage(
		fmt.Sprintf("[Context Compaction]\nThe following is a summary of the earlier part of this conversation (compacted at %s):\n\n%s",
			time.Now().Format("15:04:05"),
			summary,
		),
	)

	// Replace history with [summary, ...recent messages]
	a.mu.Lock()
	a.history = make([]provider.Message, 0, 1+len(toKeep))
	a.history = append(a.history, summaryMsg)
	a.history = append(a.history, toKeep...)
	a.compactCount++
	a.mu.Unlock()

	log.Printf("Compaction complete: %d messages summarized, %d kept, compact #%d",
		len(toSummarize), len(toKeep), a.compactCount)

	// Notify UI
	if a.cb != nil {
		a.cb.OnCompact(summary)
	}

	return nil
}

// summarizeWithLLM sends the conversation text to the provider for summarization.
func (a *Agent) summarizeWithLLM(ctx context.Context, conversationText string) (string, error) {
	messages := []provider.Message{
		provider.UserMessage(fmt.Sprintf("Please summarize this conversation:\n\n%s", conversationText)),
	}

	req := provider.Request{
		System:   compactPrompt,
		Messages: messages,
		// No tools during compaction
	}

	events, err := a.provider.Stream(ctx, req)
	if err != nil {
		return "", fmt.Errorf("summarize request failed: %w", err)
	}

	var summary strings.Builder
	for ev := range events {
		switch e := ev.(type) {
		case provider.TextEvent:
			summary.WriteString(e.Text)
		case provider.ErrorEvent:
			return "", e.Err
		}
	}

	result := strings.TrimSpace(summary.String())
	if result == "" {
		return "", fmt.Errorf("empty summary from LLM")
	}

	return result, nil
}

// CompactCount returns how many times compaction has run this session.
func (a *Agent) CompactCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.compactCount
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// ResetCompactFailures resets the consecutive failure counter (for testing).
func ResetCompactFailures() {
	consecutiveCompactFailures = 0
}

// messageToJSON converts a message to a compact JSON representation for estimation.
func messageToJSON(msg provider.Message) string {
	data, _ := json.Marshal(msg)
	return string(data)
}
