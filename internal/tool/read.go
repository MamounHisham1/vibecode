package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type ReadFile struct{}

func (ReadFile) Name() string { return "read_file" }

func (ReadFile) Description() string {
	return "Read file contents with line numbers. Supports offset and limit for partial reads."
}

func (ReadFile) Parameters() json.RawMessage {
	return schema(map[string]any{
		"path":   map[string]any{"type": "string", "description": "Path to the file"},
		"offset": map[string]any{"type": "integer", "description": "Line number to start reading from (1-based)"},
		"limit":  map[string]any{"type": "integer", "description": "Max number of lines to read"},
	})
}

type readInput struct {
	Path   string `json:"path"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
}

func (ReadFile) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in readInput
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

	lines := strings.Split(string(data), "\n")

	start := 1
	if in.Offset > 0 {
		start = in.Offset
	}

	end := len(lines)
	if in.Limit > 0 && start+in.Limit-1 < end {
		end = start + in.Limit - 1
	}

	var b strings.Builder
	for i := start - 1; i < end && i < len(lines); i++ {
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteByte('\t')
		b.WriteString(lines[i])
		b.WriteByte('\n')
	}

	total := len(lines)
	shown := end - start + 1
	result := fmt.Sprintf("%s\n(%d of %d lines shown)", b.String(), shown, total)

	return json.Marshal(result)
}
