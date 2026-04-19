package tool

import (
	"bytes"
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
	result, err := t.Execute(ctx, input)
	if err != nil {
		return nil, err
	}

	// Truncate large results
	const maxResultSize = 50000
	if len(result) > maxResultSize {
		truncated := result[:maxResultSize]
		// Try to find a clean break point
		if idx := bytes.LastIndex(truncated, []byte("\n")); idx > maxResultSize/2 {
			truncated = truncated[:idx]
		}
		truncated = append(truncated, []byte(
			fmt.Sprintf("\n\n... (%d bytes truncated, showing %d of %d total)",
				len(result)-len(truncated), len(truncated), len(result)))...)
		return truncated, nil
	}

	return result, nil
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
