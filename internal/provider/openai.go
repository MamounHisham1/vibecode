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

type OpenAIProvider struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

func NewOpenAI(apiKey, model string) *OpenAIProvider {
	return &OpenAIProvider{
		apiKey:  apiKey,
		model:   model,
		baseURL: "https://api.openai.com/v1/chat/completions",
		client:  &http.Client{},
	}
}

func NewOpenAIWithBaseURL(apiKey, model, baseURL string) *OpenAIProvider {
	return &OpenAIProvider{
		apiKey:  apiKey,
		model:   model,
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

type openAIRequest struct {
	Model       string           `json:"model"`
	Messages    []openAIMessage  `json:"messages"`
	Tools       []openAITool     `json:"tools,omitempty"`
	Stream      bool             `json:"stream"`
	MaxTokens   int              `json:"max_completion_tokens"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []openAIContent
}

type openAIContent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ID       string `json:"id,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
}

type openAITool struct {
	Type     string      `json:"type"`
	Function openAIFunc  `json:"function"`
}

type openAIFunc struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type openAISSE struct {
	Choices []openAIChoice `json:"choices"`
}

type openAIChoice struct {
	Delta openAIDelta `json:"delta"`
}

type openAIDelta struct {
	Role      string          `json:"role,omitempty"`
	Content   string          `json:"content,omitempty"`
	ToolCalls []openAIToolCall `json:"tool_calls,omitempty"`
}

type openAIToolCall struct {
	Index    int             `json:"index"`
	ID       string          `json:"id,omitempty"`
	Type     string          `json:"type,omitempty"`
	Function openAIFuncCall  `json:"function"`
}

type openAIFuncCall struct {
	Name      string          `json:"name,omitempty"`
	Arguments string          `json:"arguments,omitempty"`
}

func (o *OpenAIProvider) Stream(ctx context.Context, req Request) (<-chan Event, error) {
	body, err := o.buildRequest(req)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.client.Do(httpReq)
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
		o.streamSSE(resp.Body, ch)
	}()

	return ch, nil
}

func (o *OpenAIProvider) buildRequest(req Request) ([]byte, error) {
	messages := make([]openAIMessage, 0, len(req.Messages)+1)

	// System message
	if req.System != "" {
		messages = append(messages, openAIMessage{Role: "system", Content: req.System})
	}

	// Convert messages
	for _, msg := range req.Messages {
		om := openAIMessage{Role: msg.Role}
		if len(msg.Content) == 1 && msg.Content[0].Type == "text" {
			om.Content = msg.Content[0].Text
		} else if len(msg.Content) > 0 {
			parts := make([]openAIContent, 0, len(msg.Content))
			for _, block := range msg.Content {
				switch block.Type {
				case "text":
					parts = append(parts, openAIContent{Type: "text", Text: block.Text})
				case "tool_result":
					parts = append(parts, openAIContent{
						Type:       "text",
						Text:       string(block.Result),
						ToolCallID: block.ToolCallID,
					})
				}
			}
			om.Content = parts
		} else {
			om.Content = ""
		}
		messages = append(messages, om)
	}

	// Tools
	tools := make([]openAITool, len(req.Tools))
	for i, t := range req.Tools {
		tools[i] = openAITool{
			Type: "function",
			Function: openAIFunc{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		}
	}

	or := openAIRequest{
		Model:     o.model,
		Messages:  messages,
		Tools:     tools,
		Stream:    true,
		MaxTokens: 16384,
	}

	return json.Marshal(or)
}

func (o *OpenAIProvider) streamSSE(reader io.Reader, ch chan<- Event) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	// Track tool calls by index
	toolCalls := make(map[int]ToolCallEvent)

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			// Emit any pending tool calls
			for _, tc := range toolCalls {
				ch <- tc
			}
			ch <- DoneEvent{}
			return
		}

		var sse openAISSE
		if err := json.Unmarshal([]byte(data), &sse); err != nil {
			continue
		}

		for _, choice := range sse.Choices {
			delta := choice.Delta

			if delta.Content != "" {
				ch <- TextEvent{Text: delta.Content}
			}

			for _, tc := range delta.ToolCalls {
				existing, ok := toolCalls[tc.Index]
				if !ok {
					// New tool call
					existing = ToolCallEvent{
						ID:    tc.ID,
						Name:  tc.Function.Name,
						Input: json.RawMessage(tc.Function.Arguments),
					}
					toolCalls[tc.Index] = existing
				} else {
					// Accumulating arguments
					if tc.Function.Name != "" {
						existing.Name = tc.Function.Name
					}
					if tc.ID != "" {
						existing.ID = tc.ID
					}
					if tc.Function.Arguments != "" {
						// Append to input
						existing.Input = append(existing.Input, json.RawMessage(tc.Function.Arguments)...)
					}
					toolCalls[tc.Index] = existing
				}
			}
		}
	}

	// If we got here without [DONE], emit pending tool calls
	for _, tc := range toolCalls {
		ch <- tc
	}

	if err := scanner.Err(); err != nil {
		ch <- ErrorEvent{Err: fmt.Errorf("read stream: %w", err)}
	}
}
