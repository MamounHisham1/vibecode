package commands

func (r *Registry) registerHelp() {
	r.Register(Command{
		Name:        "help",
		Aliases:     []string{"h", "?"},
		Description: "Show available commands and keyboard shortcuts",
		Type:        TypeLocal,
		Handler:     r.helpHandler,
	})
}
