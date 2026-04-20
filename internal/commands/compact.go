package commands

func (r *Registry) registerCompact() {
	r.Register(Command{
		Name:        "compact",
		Aliases:     []string{"summary", "summarize"},
		Description: "Compact conversation context into a summary",
		Type:        TypePrompt,
		PromptText:  "Please provide a concise summary of our conversation so far, capturing the key decisions, context, and current state. This will be used to compact the conversation context.",
	})
}
