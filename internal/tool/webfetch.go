package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type WebFetch struct{}

func (WebFetch) Name() string { return "web_fetch" }

func (WebFetch) Description() string {
	return "Fetch a web page and return its text content. Use for reading documentation or APIs."
}

func (WebFetch) Parameters() json.RawMessage {
	return schema(map[string]any{
		"url":     map[string]any{"type": "string", "description": "URL to fetch"},
		"format":  map[string]any{"type": "string", "enum": []string{"text", "markdown"}, "description": "Output format (default: text)"},
		"timeout": map[string]any{"type": "integer", "description": "Timeout in seconds (default: 30)"},
	}, "url")
}

type webFetchInput struct {
	URL     string `json:"url"`
	Format  string `json:"format"`
	Timeout int    `json:"timeout"`
}

func (WebFetch) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in webFetchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}

	if in.URL == "" {
		return nil, fmt.Errorf("url is required")
	}
	if !strings.HasPrefix(in.URL, "http://") && !strings.HasPrefix(in.URL, "https://") {
		return nil, fmt.Errorf("url must start with http:// or https://")
	}

	timeout := 30
	if in.Timeout > 0 {
		timeout = in.Timeout
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, in.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "VibeCode/0.1")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024)) // 512KB limit
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	content := string(body)
	ct := resp.Header.Get("Content-Type")

	// Strip HTML tags for text output
	if strings.Contains(ct, "html") && in.Format != "markdown" {
		content = stripHTML(content)
	}

	if len(content) > 50000 {
		content = content[:50000] + "\n... (truncated)"
	}

	return json.Marshal(map[string]any{
		"url":         in.URL,
		"status":      resp.StatusCode,
		"content":     content,
		"content_len": len(content),
	})
}

func stripHTML(s string) string {
	var b strings.Builder
	inTag := false
	inScript := false
	for i := 0; i < len(s); i++ {
		if s[i] == '<' {
			inTag = true
			tag := ""
			j := i + 1
			for j < len(s) && s[j] != '>' {
				tag += string(s[j])
				j++
			}
			if strings.HasPrefix(tag, "script") || strings.HasPrefix(tag, "style") {
				inScript = true
			}
			if strings.HasPrefix(tag, "/script") || strings.HasPrefix(tag, "/style") {
				inScript = false
			}
			continue
		}
		if s[i] == '>' {
			inTag = false
			continue
		}
		if inTag || inScript {
			continue
		}
		b.WriteByte(s[i])
	}

	// Collapse whitespace
	result := b.String()
	for strings.Contains(result, "  ") {
		result = strings.ReplaceAll(result, "  ", " ")
	}
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(result)
}
