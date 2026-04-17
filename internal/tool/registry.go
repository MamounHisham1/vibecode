package tool

import (
	"context"
	"encoding/json"
	"fmt"
)

// Tool is the interface every built-in tool implements.
type Tool interface {
	Name() string
	Description() string
	Parameters() json.RawMessage
	Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error)
}

// Registry holds all registered tools.
type Registry struct {
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

func (r *Registry) Register(tool Tool) {
	r.tools[tool.Name()] = tool
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) All() []Tool {
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}

func (r *Registry) ToolDefs() []struct {
	Name        string
	Description string
	Parameters  json.RawMessage
} {
	all := r.All()
	out := make([]struct {
		Name        string
		Description string
		Parameters  json.RawMessage
	}, len(all))
	for i, t := range all {
		out[i].Name = t.Name()
		out[i].Description = t.Description()
		out[i].Parameters = t.Parameters()
	}
	return out
}

func (r *Registry) Execute(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	return t.Execute(ctx, input)
}

// Helper to create JSON schemas concisely.
func schema(properties map[string]any, required ...string) json.RawMessage {
	s := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		s["required"] = required
	}
	b, _ := json.Marshal(s)
	return b
}
