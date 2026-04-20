package commands

func (r *Registry) registerUsage() {
	r.Register(Command{
		Name:        "usage",
		Aliases:     []string{"u", "tokens", "cost"},
		Description: "Show session token usage",
		Type:        TypePrompt,
		PromptText:  "Show me a summary of the token usage and cost for this session so far.",
	})
}
