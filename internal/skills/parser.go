package skills

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// Skill represents a loaded skill with its metadata and prompt content.
type Skill struct {
	Name         string   `yaml:"name"`
	Description  string   `yaml:"description"`
	AllowedTools []string `yaml:"allowed-tools,omitempty"`
	Arguments    []Arg    `yaml:"arguments,omitempty"`
	Model        string   `yaml:"model,omitempty"`
	Effort       string   `yaml:"effort,omitempty"`
	Prompt       string   `yaml:"-"` // The markdown body after frontmatter
}

// Arg defines a skill argument.
type Arg struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Required    bool   `yaml:"required,omitempty"`
	Default     string `yaml:"default,omitempty"`
}

// Parse parses a skill file content with optional YAML frontmatter.
// Expected format:
//
//	---
//	name: my-skill
//	description: Does something useful
//	---
//	Skill prompt content here...
func Parse(content string) (*Skill, error) {
	s := &Skill{}

	body := content

	// Extract frontmatter
	if strings.HasPrefix(body, "---\n") {
		end := strings.Index(body[4:], "\n---")
		if end != -1 {
			fm := body[4 : 4+end]
			body = body[4+end+4:]

			if err := yaml.Unmarshal([]byte(fm), s); err != nil {
				return nil, err
			}
		}
	}

	s.Prompt = strings.TrimSpace(body)
	return s, nil
}
