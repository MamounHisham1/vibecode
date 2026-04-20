package commands

func (r *Registry) registerPlan() {
	r.Register(Command{
		Name:        "plan",
		Aliases:     []string{"p"},
		Description: "Enter plan mode (read-only exploration)",
		Type:        TypePrompt,
		PromptText:  "Enter plan mode. Explore the codebase and design an implementation approach. Only read-only tools are available. Present your plan to the user for approval.",
	})

	r.Register(Command{
		Name:        "unplan",
		Aliases:     []string{"up", "execute"},
		Description: "Exit plan mode and resume execution",
		Type:        TypePrompt,
		PromptText:  "Exit plan mode. We are ready to implement the plan. Proceed with execution.",
	})
}
