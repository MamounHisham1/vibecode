package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (r *Registry) registerSkills() {
	r.Register(Command{
		Name:        "skills",
		Aliases:     []string{"skill", "s"},
		Description: "List available skills",
		Type:        TypeLocal,
		Handler:     r.skillsHandler,
	})
}

func (r *Registry) skillsHandler(args string) Result {
	home, err := os.UserHomeDir()
	if err != nil {
		return Result{Output: "Could not determine home directory."}
	}

	skillsDir := filepath.Join(home, ".vibecode", "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return Result{Output: fmt.Sprintf("Skills directory: %s\nNo custom skills found.", skillsDir)}
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Skills directory: %s\n\n", skillsDir))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) == ".md" {
			b.WriteString(fmt.Sprintf("  %s\n", entry.Name()))
		}
	}
	return Result{Output: b.String()}
}
