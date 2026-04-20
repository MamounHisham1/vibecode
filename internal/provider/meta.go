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
	"alibaba": {
		Name:    "Alibaba (Qwen)",
		APIType: "openai",
		BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions",
	},
	"minimax": {
		Name:    "MiniMax",
		APIType: "anthropic",
		BaseURL: "https://api.minimax.io/anthropic/v1/messages",
		Endpoints: []ProviderEndpoint{
			{Name: "Coding Plan (Anthropic)", BaseURL: "https://api.minimax.io/anthropic/v1/messages", APIType: "anthropic"},
			{Name: "Standard API (OpenAI)", BaseURL: "https://api.minimax.chat/v1/chat/completions", APIType: "openai"},
		},
	},
	"mistral": {
		Name:    "Mistral AI",
		APIType: "openai",
		BaseURL: "https://api.mistral.ai/v1/chat/completions",
	},
	"google-ai-studio": {
		Name:    "Google AI Studio",
		APIType: "openai",
		BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions",
	},
	"groq": {
		Name:    "Groq",
		APIType: "openai",
		BaseURL: "https://api.groq.com/openai/v1/chat/completions",
	},
	"together": {
		Name:    "Together AI",
		APIType: "openai",
		BaseURL: "https://api.together.xyz/v1/chat/completions",
	},
	"fireworks": {
		Name:    "Fireworks AI",
		APIType: "openai",
		BaseURL: "https://api.fireworks.ai/inference/v1/chat/completions",
	},
	"perplexity": {
		Name:    "Perplexity",
		APIType: "openai",
		BaseURL: "https://api.perplexity.ai/chat/completions",
	},
	"xai": {
		Name:    "xAI",
		APIType: "openai",
		BaseURL: "https://api.x.ai/v1/chat/completions",
	},
	"deepinfra": {
		Name:    "DeepInfra",
		APIType: "openai",
		BaseURL: "https://api.deepinfra.com/v1/openai/chat/completions",
	},
	"cerebras": {
		Name:    "Cerebras",
		APIType: "openai",
		BaseURL: "https://api.cerebras.ai/v1/chat/completions",
	},
	"sambanova": {
		Name:    "SambaNova",
		APIType: "openai",
		BaseURL: "https://api.sambanova.ai/v1/chat/completions",
	},
	"siliconflow": {
		Name:    "SiliconFlow",
		APIType: "openai",
		BaseURL: "https://api.siliconflow.cn/v1/chat/completions",
	},
	"nvidia": {
		Name:    "NVIDIA",
		APIType: "openai",
		BaseURL: "https://integrate.api.nvidia.com/v1/chat/completions",
	},
	"ai21": {
		Name:    "AI21 Labs",
		APIType: "openai",
		BaseURL: "https://api.ai21.com/studio/v1/chat/completions",
	},
	"stepfun": {
		Name:    "StepFun",
		APIType: "openai",
		BaseURL: "https://api.stepfun.com/v1/chat/completions",
	},
	"upstage": {
		Name:    "Upstage",
		APIType: "openai",
		BaseURL: "https://api.upstage.ai/v1/chat/completions",
	},
	"hyperbolic": {
		Name:    "Hyperbolic",
		APIType: "openai",
		BaseURL: "https://api.hyperbolic.xyz/v1/chat/completions",
	},
	"nebius": {
		Name:    "Nebius AI",
		APIType: "openai",
		BaseURL: "https://api.studio.nebius.ai/v1/chat/completions",
	},
	"novita": {
		Name:    "Novita AI",
		APIType: "openai",
		BaseURL: "https://api.novita.ai/v3/openai/chat/completions",
	},
	"inference-net": {
		Name:    "InferenceNet",
		APIType: "openai",
		BaseURL: "https://api.inference.net/v1/chat/completions",
	},
	"cohere": {
		Name:    "Cohere",
		APIType: "openai",
		BaseURL: "https://api.cohere.com/v2/chat",
	},
}

// ProviderSlugAliases maps OpenRouter provider slugs to our internal provider IDs.
// This handles rebrands or slug changes on OpenRouter's side without breaking existing configs.
var ProviderSlugAliases = map[string]string{
	"z.ai":    "zhipu",
	"zhipu":   "zhipu",
	"google":  "google-ai-studio",
	"alibaba": "alibaba",
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
