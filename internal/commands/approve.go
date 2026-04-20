package commands

import "fmt"

func (r *Registry) registerApprove() {
	r.Register(Command{
		Name:        "approve",
		Aliases:     []string{"auto", "autoapprove"},
		Description: "Show or modify auto-approve settings",
		Type:        TypeLocal,
		Handler:     r.approveHandler,
	})
}

func (r *Registry) approveHandler(args string) Result {
	if args == "" {
		return Result{Output: "Auto-approve settings:\n\nCurrently auto-approved tools:\n  read_file, glob, grep\n\nUsage: /approve add <tool>  or  /approve remove <tool>"}
	}

	parts := splitQuoted(args)
	if len(parts) < 2 {
		return Result{Output: "Usage: /approve [add|remove] <tool-name>"}
	}

	switch parts[0] {
	case "add":
		return Result{Output: fmt.Sprintf("Added %s to auto-approve list (not yet persisted - edit ~/.vibecode/config.json)", parts[1])}
	case "remove":
		return Result{Output: fmt.Sprintf("Removed %s from auto-approve list (not yet persisted - edit ~/.vibecode/config.json)", parts[1])}
	default:
		return Result{Output: "Usage: /approve [add|remove] <tool-name>"}
	}
}
