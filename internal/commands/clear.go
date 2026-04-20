package commands

func (r *Registry) registerClear() {
	r.Register(Command{
		Name:        "clear",
		Aliases:     []string{"cls", "reset", "new"},
		Description: "Clear conversation history and free up context",
		Type:        TypeLocal,
		Handler: func(args string) Result {
			return Result{Clear: true, Output: "Conversation cleared."}
		},
	})
}
