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
	"claude-sonnet-4-6": {
		ID: "claude-sonnet-4-6",
		Limits: ModelLimits{Context: 200000, Output: 16384},
		Cost:   ModelCost{Input: 3e-6, Output: 15e-6, CacheRead: 0.3e-6, CacheWrite: 3.75e-6},
	},
	"claude-sonnet-4-20250514": {
		ID: "claude-sonnet-4-20250514",
		Limits: ModelLimits{Context: 200000, Output: 16384},
		Cost:   ModelCost{Input: 3e-6, Output: 15e-6, CacheRead: 0.3e-6, CacheWrite: 3.75e-6},
	},
	"claude-opus-4-20250514": {
		ID: "claude-opus-4-20250514",
		Limits: ModelLimits{Context: 200000, Output: 16384},
		Cost:   ModelCost{Input: 15e-6, Output: 75e-6, CacheRead: 1.5e-6, CacheWrite: 18.75e-6},
	},
	"claude-3-5-sonnet-20241022": {
		ID: "claude-3-5-sonnet-20241022",
		Limits: ModelLimits{Context: 200000, Output: 8192},
		Cost:   ModelCost{Input: 3e-6, Output: 15e-6, CacheRead: 0.3e-6, CacheWrite: 3.75e-6},
	},
	"claude-3-5-haiku-20241022": {
		ID: "claude-3-5-haiku-20241022",
		Limits: ModelLimits{Context: 200000, Output: 8192},
		Cost:   ModelCost{Input: 1e-6, Output: 5e-6, CacheRead: 0.1e-6, CacheWrite: 1.25e-6},
	},
	"gpt-4.1": {
		ID: "gpt-4.1",
		Limits: ModelLimits{Context: 1047576, Input: 1047576, Output: 32768},
		Cost:   ModelCost{Input: 2e-6, Output: 8e-6, CacheRead: 0.5e-6, CacheWrite: 0},
	},
	"gpt-4.1-mini": {
		ID: "gpt-4.1-mini",
		Limits: ModelLimits{Context: 1047576, Input: 1047576, Output: 32768},
		Cost:   ModelCost{Input: 0.4e-6, Output: 1.6e-6, CacheRead: 0.1e-6, CacheWrite: 0},
	},
	"gpt-4.1-nano": {
		ID: "gpt-4.1-nano",
		Limits: ModelLimits{Context: 1047576, Input: 1047576, Output: 32768},
		Cost:   ModelCost{Input: 0.1e-6, Output: 0.4e-6, CacheRead: 0.025e-6, CacheWrite: 0},
	},
	"gpt-4o": {
		ID: "gpt-4o",
		Limits: ModelLimits{Context: 128000, Input: 128000, Output: 16384},
		Cost:   ModelCost{Input: 2.5e-6, Output: 10e-6, CacheRead: 1.25e-6, CacheWrite: 0},
	},
	"gpt-4o-mini": {
		ID: "gpt-4o-mini",
		Limits: ModelLimits{Context: 128000, Input: 128000, Output: 16384},
		Cost:   ModelCost{Input: 0.15e-6, Output: 0.6e-6, CacheRead: 0.075e-6, CacheWrite: 0},
	},
	"deepseek-chat": {
		ID: "deepseek-chat",
		Limits: ModelLimits{Context: 65536, Output: 8192},
		Cost:   ModelCost{Input: 0.27e-6, Output: 1.1e-6, CacheRead: 0.07e-6, CacheWrite: 0},
	},
	"deepseek-reasoner": {
		ID: "deepseek-reasoner",
		Limits: ModelLimits{Context: 65536, Output: 8192},
		Cost:   ModelCost{Input: 0.55e-6, Output: 2.19e-6, CacheRead: 0.14e-6, CacheWrite: 0},
	},
	"glm-5.1": {
		ID: "glm-5.1",
		Limits: ModelLimits{Context: 128000, Output: 16384},
	},
	"moonshot-v1-8k": {
		ID: "moonshot-v1-8k",
		Limits: ModelLimits{Context: 8192, Output: 4096},
	},
	"qwen-turbo": {
		ID: "qwen-turbo",
		Limits: ModelLimits{Context: 131072, Output: 8192},
	},
	"llama3": {
		ID: "llama3",
		Limits: ModelLimits{Context: 8192, Output: 4096},
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
