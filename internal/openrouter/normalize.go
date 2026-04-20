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
		// Anthropic native API uses hyphens: claude-opus-4-7
		return strings.ReplaceAll(id, ".", "-")
	case "openai":
		// OpenAI native API keeps dots: gpt-4.1
		return id
	case "deepseek":
		return id
	case "moonshotai":
		return id
	case "zhipu":
		return id
	case "qwen":
		return id
	default:
		return id
	}
}
