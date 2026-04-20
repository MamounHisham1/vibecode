package provider

// ProviderMeta holds the minimal technical metadata needed to route API calls
// to a provider's native endpoint. OpenRouter provides discovery data but not
// routing info, so this small map is the only remaining hardcoded provider data.
type ProviderMeta struct {
	Name    string
	APIType string // "anthropic", "openai", "ollama"
	BaseURL string
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
		BaseURL: "https://open.bigmodel.cn/api/paas/v4/chat/completions",
	},
	"qwen": {
		Name:    "Qwen",
		APIType: "openai",
		BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions",
	},
}

// IsKnownProvider returns true if the slug exists in ProviderMetaMap.
func IsKnownProvider(slug string) bool {
	_, ok := ProviderMetaMap[slug]
	return ok
}
