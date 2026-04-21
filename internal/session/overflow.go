package session

import (
	"math"

	"github.com/vibecode/vibecode/internal/provider"
)

// autoCompactPercentage is the default fraction of the effective context window
// at which auto-compaction triggers. Aligns with Claude Code / OpenCode behavior.
const autoCompactPercentage = 0.80

// IsOverflow reports whether the current contextSize (in tokens) has exceeded
// the auto-compaction threshold for the given model.
//
// The effective context window is computed as the model's total context minus
// its max output tokens (space that must be reserved for the response).
// By default we trigger at 80% of that effective window. A user can override
// the threshold by setting cfg.Reserved to an explicit token buffer.
func IsOverflow(cfg *CompactionConfig, contextSize int, modelID string) bool {
	if cfg != nil && !cfg.Auto {
		return false
	}
	m := provider.LookupModel(modelID)
	if m.Limits.Context == 0 {
		return false
	}

	// Effective context = total window minus the space we must reserve for
	// the model's next response.
	effectiveContext := m.Limits.Context - provider.MaxOutputTokens(modelID)
	if effectiveContext <= 0 {
		return false
	}

	var threshold int
	if cfg != nil && cfg.Reserved > 0 {
		// Legacy / explicit mode: subtract a fixed buffer from the effective window.
		threshold = effectiveContext - cfg.Reserved
	} else {
		// Percentage-based threshold (default 80%).
		threshold = int(math.Floor(float64(effectiveContext) * autoCompactPercentage))
	}

	if threshold <= 0 {
		return false
	}

	return contextSize >= threshold
}
