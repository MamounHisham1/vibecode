package commands

import "os"

func (r *Registry) registerExit() {
	r.Register(Command{
		Name:        "exit",
		Aliases:     []string{"quit", "q", "bye"},
		Description: "Exit the REPL",
		Type:        TypeLocal,
		Handler: func(args string) Result {
			os.Exit(0)
			return Result{}
		},
	})
}
