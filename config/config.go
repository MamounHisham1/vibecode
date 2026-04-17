package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Provider      string            `json:"provider"`
	Model         string            `json:"model"`
	APIKeys       map[string]string `json:"api_keys"`
	AutoApprove   []string          `json:"auto_approve"`
	MaxIterations int               `json:"max_iterations"`
	Theme         string            `json:"theme"`
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
	}
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".vibecode", "config.json"), nil
}

func Load() (*Config, error) {
	path, err := configPath()
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

	// Env overrides
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
		cfg.APIKeys["anthropic"] = v
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
	path, err := configPath()
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
