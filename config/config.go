package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type CompactionConfig struct {
	Auto     bool `json:"auto"`
	Prune    bool `json:"prune"`
	Reserved int  `json:"reserved,omitempty"`
}

type Config struct {
	Provider      string                     `json:"provider"`
	Model         string                     `json:"model"`
	BaseURL       string                     `json:"base_url,omitempty"`
	BaseURLs      map[string]string          `json:"base_urls,omitempty"`
	APIKeys       map[string]string          `json:"api_keys"`
	AutoApprove   []string                   `json:"auto_approve"`
	MaxIterations int                        `json:"max_iterations"`
	Theme         string                     `json:"theme"`
	Hooks         map[string]json.RawMessage `json:"hooks,omitempty"`
	Compaction    *CompactionConfig          `json:"compaction,omitempty"`
}

func Default() *Config {
	return &Config{
		Provider: "anthropic",
		Model:    "claude-sonnet-4-6",
		APIKeys: map[string]string{
			"anthropic": "",
			"openai":    "",
		},
		AutoApprove:   []string{"read_file", "glob", "grep"},
		MaxIterations: 50,
		Theme:         "default",
		Compaction: &CompactionConfig{
			Auto:     true,
			Prune:    true,
			Reserved: 0,
		},
	}
}

func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".vibecode", "config.json"), nil
}

func Load() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return nil, err
	}

	cfg := Default()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Apply layered settings: user → project → local
	settingsPaths := settingsFilePaths(".")
	for _, sp := range settingsPaths {
		if sd, err := os.ReadFile(sp); err == nil {
			var s Settings
			if json.Unmarshal(sd, &s) == nil {
				mergeSettings(cfg, &s)
			}
		}
	}

	// Env overrides
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
		cfg.APIKeys["anthropic"] = v
	}
	if v := os.Getenv("ANTHROPIC_AUTH_TOKEN"); v != "" {
		cfg.APIKeys["anthropic"] = v
	}
	if v := os.Getenv("ANTHROPIC_BASE_URL"); v != "" {
		if cfg.BaseURLs == nil {
			cfg.BaseURLs = make(map[string]string)
		}
		cfg.BaseURLs["anthropic"] = v
	}
	if v := os.Getenv("OPENAI_API_KEY"); v != "" {
		cfg.APIKeys["openai"] = v
	}
	if v := os.Getenv("OLLAMA_BASE_URL"); v != "" {
		cfg.APIKeys["ollama_base_url"] = v
	}
	if v := os.Getenv("VIBECODE_PROVIDER"); v != "" {
		cfg.Provider = v
	}

	return cfg, nil
}

func (c *Config) Save() error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// APIKey returns the API key for the given provider.
func (c *Config) APIKey(provider string) string {
	if c.APIKeys == nil {
		return ""
	}
	return c.APIKeys[provider]
}

func (c *Config) GetBaseURL(provider string) string {
	if c.BaseURLs != nil {
		if u, ok := c.BaseURLs[provider]; ok && u != "" {
			return u
		}
	}
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return ""
}

func (c *Config) SetBaseURL(provider, url string) {
	if c.BaseURLs == nil {
		c.BaseURLs = make(map[string]string)
	}
	c.BaseURLs[provider] = url
}

// ─── Settings Layer ────────────────────────────────────────────

// Settings is a partial config that can be layered on top of the base config.
// Only non-zero fields are applied.
type Settings struct {
	Provider      string                     `json:"provider,omitempty"`
	Model         string                     `json:"model,omitempty"`
	BaseURL       string                     `json:"base_url,omitempty"`
	BaseURLs      map[string]string          `json:"base_urls,omitempty"`
	APIKeys       map[string]string          `json:"api_keys,omitempty"`
	AutoApprove   []string                   `json:"auto_approve,omitempty"`
	MaxIterations int                        `json:"max_iterations,omitempty"`
	Theme         string                     `json:"theme,omitempty"`
	Hooks         map[string]json.RawMessage `json:"hooks,omitempty"`
	Compaction    *CompactionConfig          `json:"compaction,omitempty"`
}

// settingsFilePaths returns settings file paths in priority order (low to high).
// Priority: user < project < local
func settingsFilePaths(cwd string) []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	var paths []string

	// User-level settings
	paths = append(paths, filepath.Join(home, ".vibecode", "settings.json"))

	// Project-level settings
	paths = append(paths, filepath.Join(cwd, ".vibecode", "settings.json"))

	// Local (gitignored) settings — highest priority
	paths = append(paths, filepath.Join(cwd, ".vibecode", "settings.local.json"))

	return paths
}

// mergeSettings overlays non-zero settings fields onto the config.
func mergeSettings(cfg *Config, s *Settings) {
	if s.Provider != "" {
		cfg.Provider = s.Provider
	}
	if s.Model != "" {
		cfg.Model = s.Model
	}
	if s.BaseURL != "" {
		cfg.BaseURL = s.BaseURL
	}
	if len(s.BaseURLs) > 0 {
		if cfg.BaseURLs == nil {
			cfg.BaseURLs = make(map[string]string)
		}
		for k, v := range s.BaseURLs {
			cfg.BaseURLs[k] = v
		}
	}
	if len(s.APIKeys) > 0 {
		if cfg.APIKeys == nil {
			cfg.APIKeys = make(map[string]string)
		}
		for k, v := range s.APIKeys {
			cfg.APIKeys[k] = v
		}
	}
	if len(s.AutoApprove) > 0 {
		cfg.AutoApprove = s.AutoApprove
	}
	if s.MaxIterations > 0 {
		cfg.MaxIterations = s.MaxIterations
	}
	if s.Theme != "" {
		cfg.Theme = s.Theme
	}
	if len(s.Hooks) > 0 {
		if cfg.Hooks == nil {
			cfg.Hooks = make(map[string]json.RawMessage)
		}
		for k, v := range s.Hooks {
			cfg.Hooks[k] = v
		}
	}
	if s.Compaction != nil {
		cfg.Compaction = s.Compaction
	}
}
