package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type WriteFile struct{}

func (WriteFile) Name() string { return "write_file" }

func (WriteFile) Description() string {
	return "Create a new file or completely rewrite an existing file. IMPORTANT: Prefer edit_file for modifying existing files — only use write_file when creating a brand new file or when a full rewrite is needed."
}

func (WriteFile) Parameters() json.RawMessage {
	return schema(map[string]any{
		"path":    map[string]any{"type": "string", "description": "Path to the file"},
		"content": map[string]any{"type": "string", "description": "Content to write"},
	}, "path", "content")
}

type writeInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (WriteFile) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in writeInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}

	abs, err := filepath.Abs(in.Path)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}

	// Read existing content for diff (if file exists)
	var oldContent string
	if data, err := os.ReadFile(abs); err == nil {
		oldContent = string(data)
	}

	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return nil, fmt.Errorf("create directories: %w", err)
	}

	if err := os.WriteFile(abs, []byte(in.Content), 0644); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	return json.Marshal(map[string]any{
		"output":      fmt.Sprintf("Wrote %d bytes to %s", len(in.Content), in.Path),
		"path":        in.Path,
		"old_content": oldContent,
		"new_content": in.Content,
	})
}
