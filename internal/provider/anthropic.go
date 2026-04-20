package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

// AnthropicProvider implements Provider for the Anthropic API (and compatible proxies).
type AnthropicProvider struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
	debug   bool
}

func NewAnthropic(apiKey, model string) *AnthropicProvider {
	return &AnthropicProvider{
		apiKey:  apiKey,
		model:   model,
		baseURL: "https://api.anthropic.com/v1/messages",
		client:  &http.Client{},
		debug:   os.Getenv("VIBECODE_DEBUG") != "",
	}
}

func NewAnthropicWithBaseURL(apiKey, model, baseURL string) *AnthropicProvider {
	return &AnthropicProvider{
		apiKey:  apiKey,
		model:   model,
		baseURL: baseURL,
		client:  &http.Client{},
		debug:   os.Getenv("VIBECODE_DEBUG") != "",
	}
}

// anthropicRequest is the JSON body sent to the Anthropic API.
type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    []anthropicContent `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
	Stream    bool               `json:"stream"`
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

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	CacheRead    int `json:"cache_read_input_tokens"`
	CacheWrite   int `json:"cache_creation_input_tokens"`
}

type anthropicSSE struct {
	Type  string          `json:"type"`
	Index int             `json:"index,omitempty"`
	Delta json.RawMessage `json:"delta,omitempty"`

	// For content_block_start
	ContentBlock struct {
		Type  string          `json:"type"`
		ID    string          `json:"id,omitempty"`
		Name  string          `json:"name,omitempty"`
		Input json.RawMessage `json:"input,omitempty"`
		Text  string          `json:"text,omitempty"`
	} `json:"content_block,omitempty"`

	// For message_start
	Message struct {
		ID    string          `json:"id"`
		Model string         `json:"model"`
		Usage anthropicUsage `json:"usage"`
	} `json:"message,omitempty"`

	// For message_delta
	Usage anthropicUsage `json:"usage,omitempty"`
}

type anthropicDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	StopReason  string `json:"stop_reason,omitempty"`
}

func (a *AnthropicProvider) Stream(ctx context.Context, req Request) (<-chan Event, error) {
	body, err := a.buildRequest(req)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	if a.debug {
		log.Printf("REQUEST to %s:\n%s\n", a.baseURL, string(body))
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL, bytes.NewReader(body))
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

	var curBlockType string
	var curToolID string
	var curToolInput strings.Builder
	var usage Usage

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			ch <- DoneEvent{Usage: &usage}
			return
		}

		if a.debug {
			log.Printf("SSE: %s\n", data)
		}

		var sse anthropicSSE
		if err := json.Unmarshal([]byte(data), &sse); err != nil {
			continue
		}

		switch sse.Type {
		case "message_start":
			usage.InputTokens = sse.Message.Usage.InputTokens
			usage.OutputTokens = sse.Message.Usage.OutputTokens
			usage.CacheRead = sse.Message.Usage.CacheRead
			usage.CacheWrite = sse.Message.Usage.CacheWrite

		case "content_block_start":
			curBlockType = sse.ContentBlock.Type
			if curBlockType == "tool_use" {
				curToolID = sse.ContentBlock.ID
				curToolInput.Reset()
				ch <- ToolCallEvent{
					ID:   curToolID,
					Name: sse.ContentBlock.Name,
				}
			}

		case "content_block_delta":
			var delta anthropicDelta
			if err := json.Unmarshal(sse.Delta, &delta); err == nil {
				switch delta.Type {
				case "text_delta":
					ch <- TextEvent{Text: delta.Text}
				case "input_json_delta":
					curToolInput.WriteString(delta.PartialJSON)
				}
			}

		case "content_block_stop":
			if curBlockType == "tool_use" && curToolID != "" {
				input := json.RawMessage(curToolInput.String())
				if len(input) == 0 {
					input = json.RawMessage("{}")
				}
				ch <- ToolCallEvent{
					ID:    curToolID,
					Name:  "",
					Input: input,
				}
				curToolID = ""
				curToolInput.Reset()
			}
			curBlockType = ""

		case "message_delta":
			usage.OutputTokens += sse.Usage.OutputTokens

		case "message_stop":
			ch <- DoneEvent{Usage: &usage}
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
