package commands

import "fmt"

func (r *Registry) registerModel() {
	r.Register(Command{
		Name:        "model",
		Aliases:     []string{"m"},
		Description: "Switch model via interactive picker",
		Type:        TypeLocal,
		Handler: func(args string) Result {
			if args == "" {
				return Result{Output: "Type /model (no args) to open the interactive model picker."}
			}
			return Result{Output: fmt.Sprintf("To switch to '%s', use the interactive picker by typing /model without arguments.", args)}
		},
	})
}
