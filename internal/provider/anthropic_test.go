package provider

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExtractUsageFromRaw_MessageStart(t *testing.T) {
	rawData := `{"type":"message_start","message":{"id":"msg_123","model":"claude-3","usage":{"input_tokens":150,"output_tokens":0}}}`
	var raw map[string]json.RawMessage
	json.Unmarshal([]byte(rawData), &raw)

	inputTokens, outputTokens := extractUsageFromRaw(raw)
	if inputTokens != 150 {
		t.Errorf("inputTokens = %d, want 150", inputTokens)
	}
	if outputTokens != 0 {
		t.Errorf("outputTokens = %d, want 0", outputTokens)
	}
}

func TestExtractUsageFromRaw_TopLevelUsage(t *testing.T) {
	// Some proxies send usage at the top level instead of nested in message
	rawData := `{"type":"message_start","usage":{"input_tokens":200,"output_tokens":50}}`
	var raw map[string]json.RawMessage
	json.Unmarshal([]byte(rawData), &raw)

	inputTokens, outputTokens := extractUsageFromRaw(raw)
	if inputTokens != 200 {
		t.Errorf("inputTokens = %d, want 200", inputTokens)
	}
	if outputTokens != 50 {
		t.Errorf("outputTokens = %d, want 50", outputTokens)
	}
}

func TestExtractOutputTokensFromDelta(t *testing.T) {
	rawData := `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":42}}`
	var raw map[string]json.RawMessage
	json.Unmarshal([]byte(rawData), &raw)

	outputTokens := extractOutputTokensFromDelta(raw)
	if outputTokens != 42 {
		t.Errorf("outputTokens = %d, want 42", outputTokens)
	}
}

func TestStreamSSE_UsageFromMessageStart(t *testing.T) {
	sseData := `data: {"type":"message_start","message":{"id":"msg_1","model":"glm-5","usage":{"input_tokens":500,"output_tokens":0}}}

data: {"type":"content_block_start","content_block":{"type":"text","text":""}}

data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"Hello"}}

data: {"type":"content_block_stop"}

data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":10}}

data: {"type":"message_stop"}

`
	ch := captureSSEEvents(sseData)

	// Should have: UsageEvent(500,0), TextEvent("Hello"), UsageEvent(0,10), DoneEvent
	var inputTokens, outputTokens int
	var texts []string
	var doneCount int

	for ev := range ch {
		switch e := ev.(type) {
		case UsageEvent:
			inputTokens += e.InputTokens
			outputTokens += e.OutputTokens
		case TextEvent:
			texts = append(texts, e.Text)
		case DoneEvent:
			doneCount++
		}
	}

	if inputTokens != 500 {
		t.Errorf("inputTokens = %d, want 500", inputTokens)
	}
	if outputTokens != 10 {
		t.Errorf("outputTokens = %d, want 10", outputTokens)
	}
	if len(texts) != 1 || texts[0] != "Hello" {
		t.Errorf("texts = %v, want [Hello]", texts)
	}
	if doneCount != 1 {
		t.Errorf("doneCount = %d, want 1", doneCount)
	}
}

func TestStreamSSE_UsageFromUnknownEvent(t *testing.T) {
	// Simulate a proxy that sends usage in a non-standard event
	sseData := `data: {"type":"custom_usage","usage":{"input_tokens":300,"output_tokens":20}}

data: {"type":"content_block_start","content_block":{"type":"text","text":""}}

data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"Hi"}}

data: {"type":"content_block_stop"}

data: {"type":"message_stop"}

`
	ch := captureSSEEvents(sseData)

	var inputTokens, outputTokens int
	for ev := range ch {
		if u, ok := ev.(UsageEvent); ok {
			inputTokens += u.InputTokens
			outputTokens += u.OutputTokens
		}
	}

	if inputTokens != 300 {
		t.Errorf("inputTokens = %d, want 300", inputTokens)
	}
	if outputTokens != 20 {
		t.Errorf("outputTokens = %d, want 20", outputTokens)
	}
}

func TestStreamSSE_ToolCallWithDeferredInput(t *testing.T) {
	sseData := `data: {"type":"message_start","message":{"id":"msg_1","model":"glm-5","usage":{"input_tokens":100,"output_tokens":0}}}

data: {"type":"content_block_start","content_block":{"type":"tool_use","id":"tool_1","name":"read_file"}}

data: {"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"{\"path\":\"test.go\"}"}}

data: {"type":"content_block_stop"}

data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":30}}

data: {"type":"message_stop"}

`
	ch := captureSSEEvents(sseData)

	var toolCalls []ToolCallEvent
	for ev := range ch {
		if tc, ok := ev.(ToolCallEvent); ok {
			toolCalls = append(toolCalls, tc)
		}
	}

	// Should have 2 tool call events: one with name, one with input
	if len(toolCalls) != 2 {
		t.Fatalf("toolCalls = %d, want 2", len(toolCalls))
	}
	if toolCalls[0].Name != "read_file" {
		t.Errorf("toolCalls[0].Name = %q, want read_file", toolCalls[0].Name)
	}
	if toolCalls[0].ID != "tool_1" {
		t.Errorf("toolCalls[0].ID = %q, want tool_1", toolCalls[0].ID)
	}
	if string(toolCalls[1].Input) != `{"path":"test.go"}` {
		t.Errorf("toolCalls[1].Input = %q, want {\"path\":\"test.go\"}", string(toolCalls[1].Input))
	}
}

func captureSSEEvents(sseData string) <-chan Event {
	p := &AnthropicProvider{
		apiKey:  "test",
		model:   "test",
		baseURL: "http://localhost",
		client:  nil,
		debug:   false,
	}

	ch := make(chan Event, 64)
	go func() {
		defer close(ch)
		p.streamSSE(strings.NewReader(sseData), ch)
	}()
	return ch
}
