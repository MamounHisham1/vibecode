package provider

import "strings"

// ProviderEndpoint represents one of potentially multiple API endpoints for a provider.
type ProviderEndpoint struct {
	Name    string // e.g. "Standard API", "Coding Plan (Global)"
	BaseURL string
	APIType string // overrides provider default if non-empty
}

// ProviderMeta holds the minimal technical metadata needed to route API calls
// to a provider's native endpoint. OpenRouter provides discovery data but not
// routing info, so this small map is the only remaining hardcoded provider data.
type ProviderMeta struct {
	Name      string
	APIType   string // "anthropic", "openai", "ollama"
	BaseURL   string
	Endpoints []ProviderEndpoint // optional; if empty, BaseURL/APIType are used
}

// ProviderMetaMap maps OpenRouter provider slugs to their native API metadata.
var ProviderMetaMap = map[string]ProviderMeta{
	"anthropic": {
		Name:    "Anthropic",
		APIType: "anthropic",
		BaseURL: "https://api.anthropic.com/v1/messages",
	},
	"openai": {
		Name:    "OpenAI",
		APIType: "openai",
		BaseURL: "https://api.openai.com/v1/chat/completions",
	},
	"deepseek": {
		Name:    "DeepSeek",
		APIType: "openai",
		BaseURL: "https://api.deepseek.com/v1/chat/completions",
	},
	"moonshotai": {
		Name:    "Moonshot AI",
		APIType: "openai",
		BaseURL: "https://api.moonshot.ai/v1/chat/completions",
	},
	"zhipu": {
		Name:    "Zhipu AI",
		APIType: "openai",
		BaseURL: "https://api.z.ai/api/paas/v4/chat/completions",
		Endpoints: []ProviderEndpoint{
			{Name: "Standard API (OpenAI)", BaseURL: "https://api.z.ai/api/paas/v4/chat/completions", APIType: "openai"},
			{Name: "Coding Plan — Global", BaseURL: "https://api.z.ai/api/anthropic/v1/messages", APIType: "anthropic"},
			{Name: "Coding Plan — Chinese", BaseURL: "https://open.bigmodel.cn/api/anthropic", APIType: "anthropic"},
		},
	},
	"qwen": {
		Name:    "Qwen",
		APIType: "openai",
		BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions",
	},
}

// ProviderSlugAliases maps OpenRouter provider slugs to our internal provider IDs.
// This handles rebrands or slug changes on OpenRouter's side without breaking existing configs.
var ProviderSlugAliases = map[string]string{
	"z.ai":   "zhipu",
	"zhipu":  "zhipu",
}

// ProviderNameMatches maps lowercase provider name substrings to internal provider IDs.
// Used as a fallback when slug matching fails (OpenRouter sometimes changes slugs).
var ProviderNameMatches = map[string]string{
	"zhipu": "zhipu",
	"z.ai":  "zhipu",
}

// IsKnownProvider returns true if the slug (or its alias) exists in ProviderMetaMap.
func IsKnownProvider(slug string) bool {
	if alias, ok := ProviderSlugAliases[slug]; ok {
		slug = alias
	}
	_, ok := ProviderMetaMap[slug]
	return ok
}

// ResolveProviderID tries to find an internal provider ID from a slug and name.
// It first checks slug aliases, then falls back to name matching.
func ResolveProviderID(slug, name string) string {
	if alias, ok := ProviderSlugAliases[slug]; ok {
		return alias
	}
	lowerName := strings.ToLower(name)
	for substr, id := range ProviderNameMatches {
		if strings.Contains(lowerName, substr) {
			return id
		}
	}
	return slug
}
