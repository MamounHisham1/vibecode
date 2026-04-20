package commands

import "fmt"

func (r *Registry) registerProviders() {
	r.Register(Command{
		Name:        "providers",
		Aliases:     []string{"p"},
		Description: "Switch provider via interactive picker",
		Type:        TypeLocal,
		Handler:     r.providersHandler,
	})
}

func (r *Registry) providersHandler(args string) Result {
	if args == "" {
		return Result{Output: "Type /providers (no args) to open the interactive provider picker."}
	}
	return Result{Output: fmt.Sprintf("To switch to '%s', use the interactive picker by typing /providers without arguments.", args)}
}
