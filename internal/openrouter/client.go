package openrouter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const BaseURL = "https://openrouter.ai/api/v1"

// Provider represents an AI provider from the OpenRouter providers endpoint.
type Provider struct {
	Name             string   `json:"name"`
	Slug             string   `json:"slug"`
	PrivacyPolicyURL *string  `json:"privacy_policy_url"`
	TermsOfServiceURL *string `json:"terms_of_service_url"`
	StatusPageURL    *string  `json:"status_page_url"`
	Headquarters     string   `json:"headquarters"`
	Datacenters      []string `json:"datacenters"`
}

// Model represents a model from the OpenRouter models endpoint.
type Model struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Created       int64  `json:"created"`
	Description   string `json:"description"`
	ContextLength int    `json:"context_length"`
	Pricing       struct {
		Prompt         string `json:"prompt"`
		Completion     string `json:"completion"`
		InputCacheRead string `json:"input_cache_read"`
	} `json:"pricing"`
	TopProvider struct {
		ContextLength       int  `json:"context_length"`
		MaxCompletionTokens *int `json:"max_completion_tokens"`
	} `json:"top_provider"`
}

// ProviderModels combines a provider with its available models.
type ProviderModels struct {
	Provider Provider
	Models   []Model
}

type providersResponse struct {
	Data []Provider `json:"data"`
}

type modelsResponse struct {
	Data []Model `json:"data"`
}

// Client is a lightweight HTTP client for the OpenRouter API.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a new OpenRouter client.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    BaseURL,
	}
}

// FetchProviders returns all providers from the OpenRouter API.
func (c *Client) FetchProviders() ([]Provider, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/providers")
	if err != nil {
		return nil, fmt.Errorf("fetch providers: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch providers: status %d", resp.StatusCode)
	}

	var r providersResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("decode providers: %w", err)
	}
	return r.Data, nil
}

// FetchModels returns all models from the OpenRouter API.
func (c *Client) FetchModels() ([]Model, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/models")
	if err != nil {
		return nil, fmt.Errorf("fetch models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch models: status %d", resp.StatusCode)
	}

	var r modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("decode models: %w", err)
	}
	return r.Data, nil
}

// FetchProviderModels fetches both providers and models, grouping models by provider.
func (c *Client) FetchProviderModels() ([]ProviderModels, error) {
	providers, err := c.FetchProviders()
	if err != nil {
		return nil, err
	}
	models, err := c.FetchModels()
	if err != nil {
		return nil, err
	}

	// Build provider lookup
	providerMap := make(map[string]Provider, len(providers))
	for _, p := range providers {
		providerMap[p.Slug] = p
	}

	// Group models by provider slug
	modelMap := make(map[string][]Model)
	for _, m := range models {
		parts := strings.SplitN(m.ID, "/", 2)
		if len(parts) != 2 {
			continue
		}
		slug := parts[0]
		modelMap[slug] = append(modelMap[slug], m)
	}

	// Build result: only include providers that have models
	var result []ProviderModels
	for slug, prov := range providerMap {
		if ms, ok := modelMap[slug]; ok && len(ms) > 0 {
			result = append(result, ProviderModels{
				Provider: prov,
				Models:   ms,
			})
		}
	}
	return result, nil
}
