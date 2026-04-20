package session

import (
	"math"

	"github.com/vibecode/vibecode/internal/provider"
)

const compactionBuffer = 20000

func IsOverflow(cfg *CompactionConfig, tokens TokenUsage, modelID string) bool {
	if cfg != nil && !cfg.Auto {
		return false
	}
	m := provider.LookupModel(modelID)
	if m.Limits.Context == 0 {
		return false
	}

	count := TotalTokens(tokens)

	var reserved int
	if cfg != nil && cfg.Reserved > 0 {
		reserved = cfg.Reserved
	} else {
		reserved = int(math.Min(float64(compactionBuffer), float64(provider.MaxOutputTokens(modelID))))
	}

	usable := provider.UsableInput(modelID) - reserved
	if usable <= 0 {
		return false
	}

	return count >= usable
}
