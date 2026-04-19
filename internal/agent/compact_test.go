package agent

import (
	"encoding/json"
	"testing"

	"github.com/vibecode/vibecode/internal/provider"
)

func TestRoughTokenEstimate(t *testing.T) {
	tests := []struct {
		content string
		bpt     int
		want    int
	}{
		{"hello world", 4, 2},
		{"", 4, 0},
		{"a", 4, 1}, // minimum 1
		{"abcd", 4, 1},
		{"abcdefgh", 4, 2},
		{"{\"key\": \"value\"}", 2, 8},
	}

	for _, tt := range tests {
		got := roughTokenEstimate(tt.content, tt.bpt)
		if got != tt.want {
			t.Errorf("roughTokenEstimate(%q, %d) = %d, want %d", tt.content, tt.bpt, got, tt.want)
		}
	}
}

func TestEstimateMessageTokens(t *testing.T) {
	// Text message
	msg := provider.UserMessage("hello world, this is a test message")
	tokens := estimateMessageTokens(msg)
	if tokens <= 0 {
		t.Errorf("estimateMessageTokens(text) = %d, want > 0", tokens)
	}

	// Tool call message
	toolMsg := provider.AssistantToolCallsMessage([]provider.ToolCallEvent{
		{
			ID:    "call_1",
			Name:  "read_file",
			Input: json.RawMessage(`{"path": "/tmp/test.go"}`),
		},
	})
	toolTokens := estimateMessageTokens(toolMsg)
	if toolTokens <= 0 {
		t.Errorf("estimateMessageTokens(tool_use) = %d, want > 0", toolTokens)
	}

	// Tool result message
	resultMsg := provider.ToolResultMessage("call_1", json.RawMessage(`"file contents here"`), false)
	resultTokens := estimateMessageTokens(resultMsg)
	if resultTokens <= 0 {
		t.Errorf("estimateMessageTokens(tool_result) = %d, want > 0", resultTokens)
	}
}

func TestEstimateTotalTokens(t *testing.T) {
	messages := []provider.Message{
		provider.UserMessage("short"),
		provider.AssistantTextMessage("response"),
	}
	total := estimateTotalTokens(messages)
	if total <= 0 {
		t.Errorf("estimateTotalTokens = %d, want > 0", total)
	}

	// Empty messages
	emptyTotal := estimateTotalTokens(nil)
	if emptyTotal != 0 {
		t.Errorf("estimateTotalTokens(nil) = %d, want 0", emptyTotal)
	}
}

func TestCheckAutoCompact(t *testing.T) {
	ResetCompactFailures()

	a := &Agent{
		contextWindow:    200000,
		compactThreshold: 0.80,
	}

	// Small history should not trigger (fewer than minMessagesToKeep + 2)
	a.history = []provider.Message{
		provider.UserMessage("hi"),
		provider.AssistantTextMessage("hello"),
	}
	should, est := a.checkAutoCompact()
	if should {
		t.Errorf("checkAutoCompact() = true with small history, want false")
	}
	if est <= 0 {
		t.Errorf("checkAutoCompact() estimated %d tokens, want > 0", est)
	}

	// History below threshold should not trigger
	// effective = 200000-13000=187000, threshold = 149600
	// 20 msgs * 200 chars / 4 = 1000 tokens, well below 149600
	a.history = generateMessages(20, 200)
	should, _ = a.checkAutoCompact()
	if should {
		t.Errorf("checkAutoCompact() = true below threshold, want false")
	}

	// History above threshold should trigger
	// 10000 msgs * 100 chars / 4 = 250000 tokens, above 149600
	a.history = generateMessages(10000, 100)
	should, _ = a.checkAutoCompact()
	if !should {
		t.Errorf("checkAutoCompact() = false above threshold, want true")
	}
}

func TestEffectiveContextWindow(t *testing.T) {
	a := &Agent{
		contextWindow:    200000,
		compactThreshold: 0.80,
	}

	// Effective window = 200000 - 13000 = 187000
	got := a.effectiveContextWindow()
	want := 187000
	if got != want {
		t.Errorf("effectiveContextWindow() = %d, want %d", got, want)
	}

	// Auto-compact threshold = 187000 * 0.80 = 149600
	threshold := a.autoCompactThreshold()
	wantThreshold := int(float64(187000) * 0.80)
	if threshold != wantThreshold {
		t.Errorf("autoCompactThreshold() = %d, want %d", threshold, wantThreshold)
	}
}

func TestTokenTracker(t *testing.T) {
	tracker := &TokenTracker{}
	tracker.Add(100, 50)
	tracker.Add(200, 75)

	if tracker.InputTokens != 300 {
		t.Errorf("InputTokens = %d, want 300", tracker.InputTokens)
	}
	if tracker.OutputTokens != 125 {
		t.Errorf("OutputTokens = %d, want 125", tracker.OutputTokens)
	}
	if tracker.Total() != 425 {
		t.Errorf("Total() = %d, want 425", tracker.Total())
	}
}

func TestCompactHistoryLegacy(t *testing.T) {
	a := &Agent{
		contextWindow:    200000,
		compactThreshold: 0.80,
	}

	a.history = generateMessages(20, 50)

	a.CompactHistory(5)

	if len(a.history) != 5 {
		t.Errorf("CompactHistory(5) left %d messages, want 5", len(a.history))
	}

	// CompactHistory with keepLast > len(history) should be a no-op
	a.CompactHistory(100)
	if len(a.history) != 5 {
		t.Errorf("CompactHistory(100) changed history length to %d, want 5", len(a.history))
	}
}

func TestTruncateStr(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello..."},
		{"", 5, ""},
		{"hi", 5, "hi"},
	}

	for _, tt := range tests {
		got := truncateStr(tt.input, tt.max)
		if got != tt.want {
			t.Errorf("truncateStr(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
		}
	}
}

func TestUsageEventInterface(t *testing.T) {
	// Verify UsageEvent implements the Event interface
	var _ provider.Event = provider.UsageEvent{}
	var _ provider.Event = provider.TextEvent{}
	var _ provider.Event = provider.ToolCallEvent{}
	var _ provider.Event = provider.DoneEvent{}
	var _ provider.Event = provider.ErrorEvent{}
}

// generateMessages creates n messages, each with a text block of the given length.
func generateMessages(n, textLen int) []provider.Message {
	messages := make([]provider.Message, n)
	text := make([]byte, textLen)
	for i := range text {
		text[i] = 'a'
	}
	for i := 0; i < n; i++ {
		if i%2 == 0 {
			messages[i] = provider.UserMessage(string(text))
		} else {
			messages[i] = provider.AssistantTextMessage(string(text))
		}
	}
	return messages
}
