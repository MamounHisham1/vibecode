package provider

import "math"

type ModelLimits struct {
	Context int
	Input   int
	Output  int
}

type ModelCost struct {
	Input       float64
	Output      float64
	CacheRead   float64
	CacheWrite  float64
}

type ModelInfo struct {
	ID     string
	Limits ModelLimits
	Cost   ModelCost
}

var modelRegistry = map[string]ModelInfo{
	// ─── Anthropic ─────────────────────────────────────────────────
	"claude-opus-4-7": {
		ID:     "claude-opus-4-7",
		Limits: ModelLimits{Context: 1000000, Output: 128000},
		Cost:   ModelCost{Input: 5e-6, Output: 25e-6, CacheRead: 0.5e-6, CacheWrite: 6.25e-6},
	},
	"claude-opus-4-6-fast": {
		ID:     "claude-opus-4-6-fast",
		Limits: ModelLimits{Context: 1000000, Output: 128000},
		Cost:   ModelCost{Input: 30e-6, Output: 150e-6, CacheRead: 3e-6, CacheWrite: 37.5e-6},
	},
	"claude-sonnet-4-6": {
		ID:     "claude-sonnet-4-6",
		Limits: ModelLimits{Context: 1000000, Output: 128000},
		Cost:   ModelCost{Input: 3e-6, Output: 15e-6, CacheRead: 0.3e-6, CacheWrite: 3.75e-6},
	},
	"claude-opus-4-6": {
		ID:     "claude-opus-4-6",
		Limits: ModelLimits{Context: 1000000, Output: 128000},
		Cost:   ModelCost{Input: 5e-6, Output: 25e-6, CacheRead: 0.5e-6, CacheWrite: 6.25e-6},
	},
	"claude-opus-4-5": {
		ID:     "claude-opus-4-5",
		Limits: ModelLimits{Context: 200000, Output: 64000},
		Cost:   ModelCost{Input: 5e-6, Output: 25e-6, CacheRead: 0.5e-6, CacheWrite: 6.25e-6},
	},
	// Legacy Anthropic models
	"claude-sonnet-4-20250514": {
		ID:     "claude-sonnet-4-20250514",
		Limits: ModelLimits{Context: 200000, Output: 16384},
		Cost:   ModelCost{Input: 3e-6, Output: 15e-6, CacheRead: 0.3e-6, CacheWrite: 3.75e-6},
	},
	"claude-opus-4-20250514": {
		ID:     "claude-opus-4-20250514",
		Limits: ModelLimits{Context: 200000, Output: 16384},
		Cost:   ModelCost{Input: 15e-6, Output: 75e-6, CacheRead: 1.5e-6, CacheWrite: 18.75e-6},
	},
	"claude-3-5-sonnet-20241022": {
		ID:     "claude-3-5-sonnet-20241022",
		Limits: ModelLimits{Context: 200000, Output: 8192},
		Cost:   ModelCost{Input: 3e-6, Output: 15e-6, CacheRead: 0.3e-6, CacheWrite: 3.75e-6},
	},
	"claude-3-5-haiku-20241022": {
		ID:     "claude-3-5-haiku-20241022",
		Limits: ModelLimits{Context: 200000, Output: 8192},
		Cost:   ModelCost{Input: 1e-6, Output: 5e-6, CacheRead: 0.1e-6, CacheWrite: 1.25e-6},
	},

	// ─── OpenAI ────────────────────────────────────────────────────
	"gpt-5.4-nano": {
		ID:     "gpt-5.4-nano",
		Limits: ModelLimits{Context: 400000, Output: 128000},
		Cost:   ModelCost{Input: 0.2e-6, Output: 1.25e-6, CacheRead: 0.02e-6},
	},
	"gpt-5.4-mini": {
		ID:     "gpt-5.4-mini",
		Limits: ModelLimits{Context: 400000, Output: 128000},
		Cost:   ModelCost{Input: 0.75e-6, Output: 4.5e-6, CacheRead: 0.075e-6},
	},
	"gpt-5.4-pro": {
		ID:     "gpt-5.4-pro",
		Limits: ModelLimits{Context: 1050000, Output: 128000},
		Cost:   ModelCost{Input: 30e-6, Output: 180e-6},
	},
	"gpt-5.4": {
		ID:     "gpt-5.4",
		Limits: ModelLimits{Context: 1050000, Output: 128000},
		Cost:   ModelCost{Input: 2.5e-6, Output: 15e-6, CacheRead: 0.25e-6},
	},
	"gpt-5.3-chat": {
		ID:     "gpt-5.3-chat",
		Limits: ModelLimits{Context: 128000, Output: 16384},
		Cost:   ModelCost{Input: 1.75e-6, Output: 14e-6, CacheRead: 0.175e-6},
	},
	// Legacy OpenAI models
	"gpt-4.1": {
		ID:     "gpt-4.1",
		Limits: ModelLimits{Context: 1047576, Input: 1047576, Output: 32768},
		Cost:   ModelCost{Input: 2e-6, Output: 8e-6, CacheRead: 0.5e-6},
	},
	"gpt-4.1-mini": {
		ID:     "gpt-4.1-mini",
		Limits: ModelLimits{Context: 1047576, Input: 1047576, Output: 32768},
		Cost:   ModelCost{Input: 0.4e-6, Output: 1.6e-6, CacheRead: 0.1e-6},
	},
	"gpt-4.1-nano": {
		ID:     "gpt-4.1-nano",
		Limits: ModelLimits{Context: 1047576, Input: 1047576, Output: 32768},
		Cost:   ModelCost{Input: 0.1e-6, Output: 0.4e-6, CacheRead: 0.025e-6},
	},
	"gpt-4o": {
		ID:     "gpt-4o",
		Limits: ModelLimits{Context: 128000, Input: 128000, Output: 16384},
		Cost:   ModelCost{Input: 2.5e-6, Output: 10e-6, CacheRead: 1.25e-6},
	},
	"gpt-4o-mini": {
		ID:     "gpt-4o-mini",
		Limits: ModelLimits{Context: 128000, Input: 128000, Output: 16384},
		Cost:   ModelCost{Input: 0.15e-6, Output: 0.6e-6, CacheRead: 0.075e-6},
	},

	// ─── DeepSeek ──────────────────────────────────────────────────
	"deepseek-v3.2-speciale": {
		ID:     "deepseek-v3.2-speciale",
		Limits: ModelLimits{Context: 163840, Output: 163840},
		Cost:   ModelCost{Input: 0.4e-6, Output: 1.2e-6, CacheRead: 0.2e-6},
	},
	"deepseek-v3.2": {
		ID:     "deepseek-v3.2",
		Limits: ModelLimits{Context: 131072, Output: 32768},
		Cost:   ModelCost{Input: 0.252e-6, Output: 0.378e-6, CacheRead: 0.0252e-6},
	},
	"deepseek-v3.2-exp": {
		ID:     "deepseek-v3.2-exp",
		Limits: ModelLimits{Context: 163840, Output: 65536},
		Cost:   ModelCost{Input: 0.27e-6, Output: 0.41e-6},
	},
	"deepseek-v3.1-terminus": {
		ID:     "deepseek-v3.1-terminus",
		Limits: ModelLimits{Context: 163840, Output: 40960},
		Cost:   ModelCost{Input: 0.21e-6, Output: 0.79e-6, CacheRead: 0.13e-6},
	},
	"deepseek-chat-v3.1": {
		ID:     "deepseek-chat-v3.1",
		Limits: ModelLimits{Context: 32768, Output: 7168},
		Cost:   ModelCost{Input: 0.15e-6, Output: 0.75e-6},
	},
	"deepseek-chat": {
		ID:     "deepseek-chat",
		Limits: ModelLimits{Context: 163840, Output: 40960},
		Cost:   ModelCost{Input: 0.32e-6, Output: 0.89e-6},
	},
	// Legacy DeepSeek
	"deepseek-reasoner": {
		ID:     "deepseek-reasoner",
		Limits: ModelLimits{Context: 65536, Output: 8192},
		Cost:   ModelCost{Input: 0.55e-6, Output: 2.19e-6, CacheRead: 0.14e-6},
	},

	// ─── Kimi / Moonshot ───────────────────────────────────────────
	"kimi-k2.6": {
		ID:     "kimi-k2.6",
		Limits: ModelLimits{Context: 262144, Output: 65536},
		Cost:   ModelCost{Input: 0.95e-6, Output: 4e-6, CacheRead: 0.16e-6},
	},
	"kimi-k2.5": {
		ID:     "kimi-k2.5",
		Limits: ModelLimits{Context: 262144, Output: 65535},
		Cost:   ModelCost{Input: 0.44e-6, Output: 2e-6, CacheRead: 0.22e-6},
	},
	"kimi-k2-thinking": {
		ID:     "kimi-k2-thinking",
		Limits: ModelLimits{Context: 262144, Output: 262144},
		Cost:   ModelCost{Input: 0.6e-6, Output: 2.5e-6, CacheRead: 0.15e-6},
	},
	"kimi-k2-0905": {
		ID:     "kimi-k2-0905",
		Limits: ModelLimits{Context: 262144, Output: 65536},
		Cost:   ModelCost{Input: 0.4e-6, Output: 2e-6},
	},
	"kimi-k2": {
		ID:     "kimi-k2",
		Limits: ModelLimits{Context: 131072, Output: 32768},
		Cost:   ModelCost{Input: 0.57e-6, Output: 2.3e-6},
	},
	"moonshot-v1-8k": {
		ID:     "moonshot-v1-8k",
		Limits: ModelLimits{Context: 8192, Output: 4096},
	},

	// ─── Zhipu ─────────────────────────────────────────────────────
	"glm-5.1": {
		ID:     "glm-5.1",
		Limits: ModelLimits{Context: 128000, Output: 16384},
	},
	"glm-5v-turbo": {
		ID:     "glm-5v-turbo",
		Limits: ModelLimits{Context: 128000, Output: 16384},
	},
	"glm-5-turbo": {
		ID:     "glm-5-turbo",
		Limits: ModelLimits{Context: 128000, Output: 16384},
	},
	"glm-5": {
		ID:     "glm-5",
		Limits: ModelLimits{Context: 128000, Output: 16384},
	},
	"glm-4.7-flash": {
		ID:     "glm-4.7-flash",
		Limits: ModelLimits{Context: 128000, Output: 16384},
	},

	// ─── Qwen ──────────────────────────────────────────────────────
	"qwen3.6-plus": {
		ID:     "qwen3.6-plus",
		Limits: ModelLimits{Context: 1000000, Output: 65536},
		Cost:   ModelCost{Input: 0.325e-6, Output: 1.95e-6, CacheWrite: 0.40625e-6},
	},
	"qwen3.5-9b": {
		ID:     "qwen3.5-9b",
		Limits: ModelLimits{Context: 262144, Output: 65536},
		Cost:   ModelCost{Input: 0.1e-6, Output: 0.15e-6},
	},
	"qwen3.5-35b-a3b": {
		ID:     "qwen3.5-35b-a3b",
		Limits: ModelLimits{Context: 262144, Output: 65536},
		Cost:   ModelCost{Input: 0.1625e-6, Output: 1.3e-6},
	},
	"qwen3.5-27b": {
		ID:     "qwen3.5-27b",
		Limits: ModelLimits{Context: 262144, Output: 65536},
		Cost:   ModelCost{Input: 0.195e-6, Output: 1.56e-6},
	},
	"qwen3.5-122b-a10b": {
		ID:     "qwen3.5-122b-a10b",
		Limits: ModelLimits{Context: 262144, Output: 65536},
		Cost:   ModelCost{Input: 0.26e-6, Output: 2.08e-6},
	},
	// Legacy Qwen
	"qwen-turbo": {
		ID:     "qwen-turbo",
		Limits: ModelLimits{Context: 131072, Output: 8192},
	},

	// ─── Ollama (local, no cost) ───────────────────────────────────
	"qwen3:8b": {
		ID:     "qwen3:8b",
		Limits: ModelLimits{Context: 32768, Output: 4096},
	},
	"llama3": {
		ID:     "llama3",
		Limits: ModelLimits{Context: 8192, Output: 4096},
	},
	"mistral": {
		ID:     "mistral",
		Limits: ModelLimits{Context: 32768, Output: 4096},
	},
	"deepseek-coder-v2": {
		ID:     "deepseek-coder-v2",
		Limits: ModelLimits{Context: 128000, Output: 4096},
	},
	"qwen2.5-coder": {
		ID:     "qwen2.5-coder",
		Limits: ModelLimits{Context: 131072, Output: 4096},
	},
}

func LookupModel(modelID string) ModelInfo {
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
	return m.Limits.Context - MaxOutputTokens(modelID)
}
