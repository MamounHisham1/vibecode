package openrouter

import "strings"

// NormalizeModelID converts an OpenRouter model ID to a native API model ID.
// OpenRouter IDs are "provider/model.id" (e.g., "anthropic/claude-opus-4.7").
// Native APIs may use different formatting rules.
func NormalizeModelID(providerSlug, openrouterModelID string) string {
	// Remove provider prefix
	id := strings.TrimPrefix(openrouterModelID, providerSlug+"/")

	// Provider-specific normalization rules
	switch providerSlug {
	case "anthropic":
		return strings.ReplaceAll(id, ".", "-")
	case "openai", "deepseek", "moonshotai", "zhipu", "qwen", "alibaba",
		"minimax", "mistral", "groq", "together", "fireworks", "perplexity",
		"xai", "deepinfra", "cerebras", "sambanova", "siliconflow", "nvidia",
		"ai21", "stepfun", "upstage", "hyperbolic", "nebius", "novita",
		"inference-net", "cohere", "google-ai-studio":
		return id
	default:
		return id
	}
}
