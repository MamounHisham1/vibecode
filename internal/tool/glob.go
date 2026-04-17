package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

type Glob struct{}

func (Glob) Name() string { return "glob" }

func (Glob) Description() string {
	return "Find files matching a glob pattern."
}

func (Glob) Parameters() json.RawMessage {
	return schema(map[string]any{
		"pattern": map[string]any{"type": "string", "description": "Glob pattern (e.g. **/*.go, src/**/*.ts)"},
	}, "pattern")
}

type globInput struct {
	Pattern string `json:"pattern"`
}

func (Glob) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in globInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}

	matches, err := filepath.Glob(in.Pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %w", err)
	}

	if len(matches) == 0 {
		return json.Marshal("No files matched the pattern.")
	}

	return json.Marshal(strings.Join(matches, "\n"))
}
