package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseWithFrontmatter(t *testing.T) {
	content := `---
name: test-skill
description: A test skill
allowed-tools:
  - read_file
  - grep
---
Read the file at {{path}} and summarize it.`

	skill, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if skill.Name != "test-skill" {
		t.Errorf("Name = %q, want 'test-skill'", skill.Name)
	}
	if skill.Description != "A test skill" {
		t.Errorf("Description = %q, want 'A test skill'", skill.Description)
	}
	if len(skill.AllowedTools) != 2 {
		t.Fatalf("AllowedTools = %v, want 2 items", skill.AllowedTools)
	}
	if skill.AllowedTools[0] != "read_file" {
		t.Errorf("AllowedTools[0] = %q, want 'read_file'", skill.AllowedTools[0])
	}
	if skill.Prompt != "Read the file at {{path}} and summarize it." {
		t.Errorf("Prompt = %q", skill.Prompt)
	}
}

func TestParseNoFrontmatter(t *testing.T) {
	content := "Just a simple prompt with no frontmatter."

	skill, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if skill.Name != "" {
		t.Errorf("Name = %q, want empty", skill.Name)
	}
	if skill.Prompt != content {
		t.Errorf("Prompt = %q, want %q", skill.Prompt, content)
	}
}

func TestParseWithArguments(t *testing.T) {
	content := `---
name: search
description: Search the codebase
arguments:
  - name: query
    description: Search query
    required: true
  - name: path
    description: Directory to search
    default: .
---
Search for {{query}} in {{path}}.`

	skill, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if len(skill.Arguments) != 2 {
		t.Fatalf("Arguments = %v, want 2 items", skill.Arguments)
	}
	if skill.Arguments[0].Name != "query" {
		t.Errorf("Arguments[0].Name = %q", skill.Arguments[0].Name)
	}
	if !skill.Arguments[0].Required {
		t.Error("Arguments[0].Required = false, want true")
	}
	if skill.Arguments[1].Default != "." {
		t.Errorf("Arguments[1].Default = %q, want '.'", skill.Arguments[1].Default)
	}
}

func TestStoreLoadFromDir(t *testing.T) {
	// Create a temp skills directory
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".vibecode", "skills")
	os.MkdirAll(skillsDir, 0755)

	// Write a skill file
	os.WriteFile(filepath.Join(skillsDir, "review.md"), []byte(`---
name: review
description: Code review skill
---
Review the code changes.`), 0644)

	os.WriteFile(filepath.Join(skillsDir, "test.md"), []byte(`---
name: test-skill
description: Test skill
---
Run tests.`), 0644)

	store := NewStore()
	err := store.Load(tmpDir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if len(store.skills) != 2 {
		t.Errorf("skills count = %d, want 2", len(store.skills))
	}

	skill, ok := store.Get("review")
	if !ok {
		t.Fatal("review skill not found")
	}
	if skill.Description != "Code review skill" {
		t.Errorf("Description = %q", skill.Description)
	}
}

func TestStoreFilenameAsName(t *testing.T) {
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".vibecode", "skills")
	os.MkdirAll(skillsDir, 0755)

	// Write a skill file with no name in frontmatter
	os.WriteFile(filepath.Join(skillsDir, "my-skill.md"), []byte(`---
description: No name specified
---
Do something.`), 0644)

	store := NewStore()
	store.Load(tmpDir)

	skill, ok := store.Get("my-skill")
	if !ok {
		t.Fatal("my-skill not found")
	}
	if skill.Name != "my-skill" {
		t.Errorf("Name = %q, want 'my-skill'", skill.Name)
	}
}

func TestPromptForSkill(t *testing.T) {
	skill := &Skill{
		Prompt: "Search for {{query}}",
	}

	prompt := PromptForSkill(skill, map[string]string{
		"query": "TODO",
	})

	expected := "Search for {{query}}\n\nquery: TODO\n"
	if prompt != expected {
		t.Errorf("PromptForSkill = %q, want %q", prompt, expected)
	}
}

func TestStoreGetNonExistent(t *testing.T) {
	store := NewStore()
	_, ok := store.Get("nonexistent")
	if ok {
		t.Error("Get(nonexistent) should return false")
	}
}
