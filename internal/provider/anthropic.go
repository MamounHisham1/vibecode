package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const anthropicBaseURL = "https://api.anthropic.com/v1/messages"

// AnthropicProvider implements Provider for the Anthropic API.
type AnthropicProvider struct {
	apiKey string
	model  string
	client *http.Client
}

func NewAnthropic(apiKey, model string) *AnthropicProvider {
	return &AnthropicProvider{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{},
	}
}

// anthropicRequest is the JSON body sent to the Anthropic API.
type anthropicRequest struct {
	Model     string              `json:"model"`
	MaxTokens int                 `json:"max_tokens"`
	System    []anthropicContent  `json:"system,omitempty"`
	Messages  []anthropicMessage  `json:"messages"`
	Tools     []anthropicTool     `json:"tools,omitempty"`
	Stream    bool                `json:"stream"`
}

type anthropicMessage struct {
	Role    string             `json:"role"`
	Content []anthropicContent `json:"content"`
}

type anthropicContent struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// For tool_result blocks
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content2  json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicSSE struct {
	Type  string          `json:"type"`
	Index int             `json:"index,omitempty"`
	Delta json.RawMessage `json:"delta,omitempty"`

	// For message_start
	Message struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Usage   struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message,omitempty"`
}

type anthropicDelta struct {
	Type             string          `json:"type"`
	Text             string          `json:"text,omitempty"`
	PartialJSON      string          `json:"partial_json,omitempty"`
	ToolCallID       string          `json:"id,omitempty"`
	ToolName         string          `json:"name,omitempty"`
	StopReason       string          `json:"stop_reason,omitempty"`
}

func (a *AnthropicProvider) Stream(ctx context.Context, req Request) (<-chan Event, error) {
	body, err := a.buildRequest(req)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicBaseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan Event, 64)

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		a.streamSSE(resp.Body, ch)
	}()

	return ch, nil
}

func (a *AnthropicProvider) buildRequest(req Request) ([]byte, error) {
	ar := anthropicRequest{
		Model:     a.model,
		MaxTokens: 16384,
		Stream:    true,
		System: []anthropicContent{
			{Type: "text", Text: req.System},
		},
	}

	for _, msg := range req.Messages {
		ar.Messages = append(ar.Messages, convertMessage(msg))
	}

	for _, tool := range req.Tools {
		ar.Tools = append(ar.Tools, anthropicTool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.Parameters,
		})
	}

	return json.Marshal(ar)
}

func convertMessage(msg Message) anthropicMessage {
	am := anthropicMessage{Role: msg.Role}
	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			am.Content = append(am.Content, anthropicContent{
				Type: "text",
				Text: block.Text,
			})
		case "tool_use":
			am.Content = append(am.Content, anthropicContent{
				Type:  "tool_use",
				ID:    block.ToolCallID,
				Name:  block.ToolName,
				Input: block.Input,
			})
		case "tool_result":
			am.Content = append(am.Content, anthropicContent{
				Type:      "tool_result",
				ToolUseID: block.ToolCallID,
				Content2:  block.Result,
				IsError:   block.IsError,
			})
		}
	}
	return am
}

func (a *AnthropicProvider) streamSSE(reader io.Reader, ch chan<- Event) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			ch <- DoneEvent{}
			return
		}

		var sse anthropicSSE
		if err := json.Unmarshal([]byte(data), &sse); err != nil {
			ch <- ErrorEvent{Err: fmt.Errorf("parse SSE: %w", err)}
			return
		}

		switch sse.Type {
		case "content_block_start":
			var delta anthropicDelta
			if err := json.Unmarshal(sse.Delta, &delta); err == nil {
				if delta.Type == "tool_use" {
					ch <- ToolCallEvent{
						ID:   delta.ToolCallID,
						Name: delta.ToolName,
					}
				}
			}

		case "content_block_delta":
			var delta anthropicDelta
			if err := json.Unmarshal(sse.Delta, &delta); err == nil {
				switch delta.Type {
				case "text_delta":
					ch <- TextEvent{Text: delta.Text}
				case "input_json_delta":
					// Accumulated tool input — we'll collect in the agent loop
					ch <- TextEvent{Text: delta.PartialJSON}
				}
			}

		case "content_block_stop":
			// Block complete

		case "message_stop":
			ch <- DoneEvent{}
			return

		case "error":
			ch <- ErrorEvent{Err: fmt.Errorf("stream error: %s", data)}
			return
		}
	}

	if err := scanner.Err(); err != nil {
		ch <- ErrorEvent{Err: fmt.Errorf("read stream: %w", err)}
	}
}
