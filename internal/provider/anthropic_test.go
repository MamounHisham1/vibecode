package provider

import (
	"strings"
	"testing"
)

func TestStreamSSE_ToolCallWithDeferredInput(t *testing.T) {
	sseData := `data: {"type":"message_start","message":{"id":"msg_1","model":"glm-5"}}

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
