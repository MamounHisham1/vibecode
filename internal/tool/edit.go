package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type EditFile struct{}

func (EditFile) Name() string { return "edit_file" }

func (EditFile) Description() string {
	return "Make targeted edits to an existing file by replacing an exact string match. PREFERRED over write_file for modifying existing files — this preserves unchanged content and shows a clean diff."
}

func (EditFile) Parameters() json.RawMessage {
	return schema(map[string]any{
		"path":       map[string]any{"type": "string", "description": "Path to the file"},
		"old_string": map[string]any{"type": "string", "description": "Exact string to find"},
		"new_string": map[string]any{"type": "string", "description": "String to replace with"},
	}, "path", "old_string", "new_string")
}

type editInput struct {
	Path      string `json:"path"`
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

func (EditFile) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in editInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}

	abs, err := filepath.Abs(in.Path)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	content := string(data)
	count := strings.Count(content, in.OldString)

	if count == 0 {
		return nil, fmt.Errorf("old_string not found in %s", in.Path)
	}
	if count > 1 {
		return nil, fmt.Errorf("old_string found %d times in %s — must be unique", count, in.Path)
	}

	newContent := strings.Replace(content, in.OldString, in.NewString, 1)

	if err := os.WriteFile(abs, []byte(newContent), 0644); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	return json.Marshal(map[string]any{
		"output":  fmt.Sprintf("Edited %s: replaced 1 occurrence", in.Path),
		"path":    in.Path,
		"changed": true,
	})
}
