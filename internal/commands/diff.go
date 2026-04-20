package commands

import (
	"fmt"
	"os/exec"
	"strings"
)

func (r *Registry) registerDiff() {
	r.Register(Command{
		Name:        "diff",
		Aliases:     []string{"d", "changes"},
		Description: "Show git diff of uncommitted changes",
		Type:        TypeLocal,
		Handler:     r.diffHandler,
	})
}

func (r *Registry) diffHandler(args string) Result {
	cmd := exec.Command("git", "diff", "--stat")
	stat, err := cmd.Output()
	if err != nil {
		return Result{Output: "No git repository found or no changes."}
	}

	cmd = exec.Command("git", "diff")
	out, err := cmd.Output()
	if err != nil {
		return Result{Output: "Could not get diff."}
	}

	var b strings.Builder
	if len(stat) > 0 {
		b.WriteString("Changed files:\n")
		b.WriteString(string(stat))
		b.WriteString("\n")
	}

	diff := string(out)
	if strings.TrimSpace(diff) == "" {
		b.WriteString("No uncommitted changes.")
	} else {
		lines := strings.Split(diff, "\n")
		if len(lines) > 100 {
			b.WriteString(strings.Join(lines[:100], "\n"))
			b.WriteString(fmt.Sprintf("\n\n... (%d more lines)", len(lines)-100))
		} else {
			b.WriteString(diff)
		}
	}

	return Result{Output: b.String()}
}
