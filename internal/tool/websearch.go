package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// WebSearch searches the web using a configured search API.
type WebSearch struct {
	apiKey   string
	provider string
	client   *http.Client
}

// NewWebSearch creates a web search tool. Provider can be "brave" or "serpapi".
func NewWebSearch(apiKey, provider string) *WebSearch {
	return &WebSearch{
		apiKey:   apiKey,
		provider: provider,
		client:   &http.Client{Timeout: 15 * time.Second},
	}
}

func (w *WebSearch) Name() string { return "web_search" }

func (w *WebSearch) Description() string {
	return "Search the web for information. Returns titles, URLs, and snippets."
}

func (w *WebSearch) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "The search query"
			},
			"count": {
				"type": "integer",
				"description": "Number of results (default 5, max 10)",
				"default": 5
			}
		},
		"required": ["query"]
	}`)
}

type searchInput struct {
	Query string `json:"query"`
	Count int    `json:"count"`
}

type searchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

func (w *WebSearch) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in searchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}

	if in.Query == "" {
		return nil, fmt.Errorf("query is required")
	}
	if in.Count <= 0 || in.Count > 10 {
		in.Count = 5
	}

	if w.apiKey == "" {
		return nil, fmt.Errorf("web search not configured: set VIBECODE_SEARCH_API_KEY")
	}

	var results []searchResult
	var err error

	switch w.provider {
	case "brave":
		results, err = w.searchBrave(ctx, in)
	case "serpapi":
		results, err = w.searchSerpAPI(ctx, in)
	default:
		return nil, fmt.Errorf("unsupported search provider: %s", w.provider)
	}

	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return json.Marshal(map[string]any{
			"query":   in.Query,
			"results": []searchResult{},
			"message": "No results found",
		})
	}

	return json.Marshal(map[string]any{
		"query":   in.Query,
		"count":   len(results),
		"results": results,
	})
}

func (w *WebSearch) searchBrave(ctx context.Context, in searchInput) ([]searchResult, error) {
	u := fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?q=%s&count=%d",
		url.QueryEscape(in.Query), in.Count)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("X-Subscription-Token", w.apiKey)

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brave search request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("brave search error %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var braveResp struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}

	if err := json.Unmarshal(body, &braveResp); err != nil {
		return nil, fmt.Errorf("parse brave response: %w", err)
	}

	results := make([]searchResult, 0, len(braveResp.Web.Results))
	for _, r := range braveResp.Web.Results {
		results = append(results, searchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Description,
		})
	}

	return results, nil
}

func (w *WebSearch) searchSerpAPI(ctx context.Context, in searchInput) ([]searchResult, error) {
	u := fmt.Sprintf("https://serpapi.com/search.json?q=%s&num=%d&api_key=%s",
		url.QueryEscape(in.Query), in.Count, url.QueryEscape(w.apiKey))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("serpapi request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("serpapi error %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var serpResp struct {
		OrganicResults []struct {
			Title   string `json:"title"`
			Link    string `json:"link"`
			Snippet string `json:"snippet"`
		} `json:"organic_results"`
	}

	if err := json.Unmarshal(body, &serpResp); err != nil {
		return nil, fmt.Errorf("parse serpapi response: %w", err)
	}

	results := make([]searchResult, 0, len(serpResp.OrganicResults))
	for _, r := range serpResp.OrganicResults {
		results = append(results, searchResult{
			Title:   r.Title,
			URL:     r.Link,
			Snippet: r.Snippet,
		})
	}

	return results, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
