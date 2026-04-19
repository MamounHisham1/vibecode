package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Store holds all discovered skills.
type Store struct {
	skills map[string]*Skill
	dirs   []string // directories searched
}

// NewStore creates an empty skill store.
func NewStore() *Store {
	return &Store{
		skills: make(map[string]*Skill),
	}
}

// Load walks from dir up to home looking for .vibecode/skills/ directories
// and loads any .md files found as skills.
func (s *Store) Load(dir string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	// Walk from dir up to home
	cur := dir
	for {
		skillsDir := filepath.Join(cur, ".vibecode", "skills")
		if err := s.loadDir(skillsDir); err == nil {
			s.dirs = append(s.dirs, skillsDir)
		}

		// Also check user-level skills
		if cur == home {
			break
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}

	// Always check user-level skills
	userDir := filepath.Join(home, ".vibecode", "skills")
	if err := s.loadDir(userDir); err == nil {
		s.dirs = append(s.dirs, userDir)
	}

	return nil
}

func (s *Store) loadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		skill, err := Parse(string(data))
		if err != nil {
			continue
		}

		// Use filename as name if not specified in frontmatter
		if skill.Name == "" {
			skill.Name = strings.TrimSuffix(entry.Name(), ".md")
		}

		// Don't overwrite — first found wins (closer to project wins)
		if _, exists := s.skills[skill.Name]; !exists {
			s.skills[skill.Name] = skill
		}
	}

	return nil
}

// Get returns a skill by name.
func (s *Store) Get(name string) (*Skill, bool) {
	skill, ok := s.skills[name]
	return skill, ok
}

// All returns all loaded skills.
func (s *Store) All() map[string]*Skill {
	return s.skills
}

// Dirs returns the directories that were searched for skills.
func (s *Store) Dirs() []string {
	return s.dirs
}

// PromptForSkill builds the system/user prompt for invoking a skill.
func PromptForSkill(skill *Skill, args map[string]string) string {
	var b strings.Builder

	b.WriteString(skill.Prompt)

	// Append argument values if the skill expects them
	if len(args) > 0 {
		b.WriteString("\n\n")
		for k, v := range args {
			b.WriteString(fmt.Sprintf("%s: %s\n", k, v))
		}
	}

	return b.String()
}
