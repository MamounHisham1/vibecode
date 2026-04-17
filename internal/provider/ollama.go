package provider

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type OllamaProvider struct {
	model   string
	baseURL string
	client  *http.Client
}

func NewOllama(model, baseURL string) *OllamaProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434/v1/chat/completions"
	} else {
		baseURL = strings.TrimRight(baseURL, "/") + "/v1/chat/completions"
	}
	return &OllamaProvider{
		model:   model,
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

func (o *OllamaProvider) Stream(ctx context.Context, req Request) (<-chan Event, error) {
	// Ollama uses OpenAI-compatible API, so we reuse the request builder
	openai := &OpenAIProvider{model: o.model, baseURL: o.baseURL}
	body, err := openai.buildRequest(req)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("Ollama error %d: %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan Event, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		// Reuse OpenAI SSE parser since Ollama is compatible
		openai.streamSSE(resp.Body, ch)
	}()

	return ch, nil
}
