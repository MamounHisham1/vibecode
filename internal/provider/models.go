package provider

import (
	"math"
	"strconv"
	"sync"

	"github.com/vibecode/vibecode/internal/openrouter"
)

type ModelLimits struct {
	Context int
	Input   int
	Output  int
}

type ModelCost struct {
	Input      float64
	Output     float64
	CacheRead  float64
	CacheWrite float64
}

type ModelInfo struct {
	ID     string
	Limits ModelLimits
	Cost   ModelCost
}

// modelRegistry is populated dynamically from OpenRouter and protected by mu.
var (
	modelRegistry = make(map[string]ModelInfo)
	registryMu    sync.RWMutex
)

// parsePrice converts an OpenRouter price string (e.g., "0.000005") to float64.
func parsePrice(s string) float64 {
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

// maxCompletionTokens returns a sensible output token limit from OpenRouter data.
func maxCompletionTokens(m openrouter.Model) int {
	if m.TopProvider.MaxCompletionTokens != nil && *m.TopProvider.MaxCompletionTokens > 0 {
		return *m.TopProvider.MaxCompletionTokens
	}
	// Heuristic: context/4, capped at 128k
	out := m.ContextLength / 4
	if out > 128000 {
		out = 128000
	}
	return out
}

// BuildRegistryFromOpenRouter populates the model registry from OpenRouter data.
func BuildRegistryFromOpenRouter(data []openrouter.ProviderModels) {
	registryMu.Lock()
	defer registryMu.Unlock()

	for _, pm := range data {
		for _, m := range pm.Models {
			nativeID := openrouter.NormalizeModelID(pm.Provider.Slug, m.ID)
			modelRegistry[nativeID] = ModelInfo{
				ID: nativeID,
				Limits: ModelLimits{
					Context: m.ContextLength,
					Output:  maxCompletionTokens(m),
				},
				Cost: ModelCost{
					Input:     parsePrice(m.Pricing.Prompt),
					Output:    parsePrice(m.Pricing.Completion),
					CacheRead: parsePrice(m.Pricing.InputCacheRead),
				},
			}
		}
	}
}

func LookupModel(modelID string) ModelInfo {
	registryMu.RLock()
	defer registryMu.RUnlock()

	if m, ok := modelRegistry[modelID]; ok {
		return m
	}
	return ModelInfo{
		ID:     modelID,
		Limits: ModelLimits{Context: 200000, Output: 16384},
	}
}

const OutputTokenMax = 32000

func MaxOutputTokens(modelID string) int {
	m := LookupModel(modelID)
	return int(math.Min(float64(m.Limits.Output), float64(OutputTokenMax)))
}

func UsableInput(modelID string) int {
	m := LookupModel(modelID)
	if m.Limits.Input > 0 {
		return m.Limits.Input
	}
	return m.Limits.Context
}
