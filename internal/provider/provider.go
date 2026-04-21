package provider

import (
	"context"
	"encoding/json"
)

// Provider is the interface every LLM backend implements.
type Provider interface {
	Stream(ctx context.Context, req Request) (<-chan Event, error)
}

// Request is sent to the LLM provider.
type Request struct {
	System   string    `json:"system"`
	Messages []Message `json:"messages"`
	Tools    []ToolDef `json:"tools"`
}

// Message is a single turn in the conversation.
type Message struct {
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
}

// ContentBlock represents a unit of content — text, a tool call, or a tool result.
type ContentBlock struct {
	Type       string          `json:"type"`
	Text       string          `json:"text,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolName   string          `json:"tool_name,omitempty"`
	Input      json.RawMessage `json:"input,omitempty"`
	Result     json.RawMessage `json:"result,omitempty"`
	IsError    bool            `json:"is_error,omitempty"`
}

// ToolDef describes a tool the LLM can invoke.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// Event is a streaming event from the provider.
type Event interface {
	isEvent()
}

type TextEvent struct {
	Text string
}

func (TextEvent) isEvent() {}

type ToolCallEvent struct {
	ID    string
	Name  string
	Input json.RawMessage
}

func (ToolCallEvent) isEvent() {}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	CacheRead    int `json:"cache_read_input_tokens,omitempty"`
	CacheWrite   int `json:"cache_creation_input_tokens,omitempty"`
	// TotalTokens is the complete context-window size reported by the provider.
	// For Anthropic this is input+output+cache; for OpenAI it is input+output
	// (cache is already included in input). Providers must set this explicitly.
	TotalTokens int `json:"total_tokens,omitempty"`
}

type DoneEvent struct {
	Usage *Usage
}

func (DoneEvent) isEvent() {}

type ErrorEvent struct {
	Err error
}

func (ErrorEvent) isEvent() {}

// Helper constructors.

func UserMessage(text string) Message {
	return Message{
		Role: "user",
		Content: []ContentBlock{
			{Type: "text", Text: text},
		},
	}
}

func AssistantTextMessage(text string) Message {
	return Message{
		Role: "assistant",
		Content: []ContentBlock{
			{Type: "text", Text: text},
		},
	}
}

func AssistantToolCallsMessage(calls []ToolCallEvent) Message {
	blocks := make([]ContentBlock, len(calls))
	for i, call := range calls {
		input := call.Input
		if input == nil || !json.Valid(input) {
			input = json.RawMessage(`{}`)
		}
		blocks[i] = ContentBlock{
			Type:       "tool_use",
			ToolCallID: call.ID,
			ToolName:   call.Name,
			Input:      input,
		}
	}
	return Message{Role: "assistant", Content: blocks}
}

func ToolResultMessage(toolCallID string, result json.RawMessage, isError bool) Message {
	if result == nil || !json.Valid(result) {
		result = json.RawMessage(`""`)
	}
	return Message{
		Role: "user",
		Content: []ContentBlock{
			{
				Type:       "tool_result",
				ToolCallID: toolCallID,
				Result:     result,
				IsError:    isError,
			},
		},
	}
}
