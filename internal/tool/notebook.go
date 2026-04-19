package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// NotebookEdit handles Jupyter notebook (.ipynb) cell operations.
type NotebookEdit struct{}

func (n NotebookEdit) Name() string { return "notebook_edit" }

func (n NotebookEdit) Description() string {
	return "Edit Jupyter notebook cells. Supports read, insert, delete, and replace operations on .ipynb files."
}

func (n NotebookEdit) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Path to the .ipynb file"
			},
			"operation": {
				"type": "string",
				"enum": ["read", "insert", "delete", "replace"],
				"description": "Operation to perform"
			},
			"cell_index": {
				"type": "integer",
				"description": "0-based index of the cell (for delete/replace/read)"
			},
			"cell_type": {
				"type": "string",
				"enum": ["code", "markdown"],
				"description": "Type of cell to insert/replace"
			},
			"source": {
				"type": "string",
				"description": "Cell source content (for insert/replace)"
			}
		},
		"required": ["path", "operation"]
	}`)
}

type notebookInput struct {
	Path      string `json:"path"`
	Operation string `json:"operation"`
	CellIndex int    `json:"cell_index"`
	CellType  string `json:"cell_type"`
	Source    string `json:"source"`
}

type notebookCell struct {
	CellType string   `json:"cell_type"`
	Source   []string `json:"source"`
	Metadata any      `json:"metadata,omitempty"`
}

type notebookFile struct {
	Cells         []notebookCell `json:"cells"`
	Metadata      any            `json:"metadata,omitempty"`
	NBFormat      int            `json:"nbformat"`
	NBFormatMinor int            `json:"nbformat_minor"`
}

func (n NotebookEdit) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in notebookInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}

	if in.Path == "" {
		return nil, fmt.Errorf("path is required")
	}

	switch in.Operation {
	case "read":
		return n.readNotebook(in)
	case "insert":
		return n.insertCell(in)
	case "delete":
		return n.deleteCell(in)
	case "replace":
		return n.replaceCell(in)
	default:
		return nil, fmt.Errorf("unknown operation: %s", in.Operation)
	}
}

func (n NotebookEdit) readNotebook(in notebookInput) (json.RawMessage, error) {
	data, err := os.ReadFile(in.Path)
	if err != nil {
		return nil, fmt.Errorf("read notebook: %w", err)
	}

	var nb notebookFile
	if err := json.Unmarshal(data, &nb); err != nil {
		return nil, fmt.Errorf("parse notebook: %w", err)
	}

	if in.CellIndex >= 0 && in.CellIndex < len(nb.Cells) {
		cell := nb.Cells[in.CellIndex]
		return json.Marshal(map[string]any{
			"index":     in.CellIndex,
			"cell_type": cell.CellType,
			"source":    strings.Join(cell.Source, ""),
			"total":     len(nb.Cells),
		})
	}

	// Return summary of all cells
	type cellSummary struct {
		Index    int    `json:"index"`
		CellType string `json:"cell_type"`
		Preview  string `json:"preview"`
	}

	summaries := make([]cellSummary, len(nb.Cells))
	for i, cell := range nb.Cells {
		src := strings.Join(cell.Source, "")
		if len(src) > 80 {
			src = src[:80] + "..."
		}
		summaries[i] = cellSummary{
			Index:    i,
			CellType: cell.CellType,
			Preview:  src,
		}
	}

	return json.Marshal(map[string]any{
		"path":  in.Path,
		"cells": summaries,
		"total": len(nb.Cells),
	})
}

func (n NotebookEdit) insertCell(in notebookInput) (json.RawMessage, error) {
	nb, err := n.loadNotebook(in.Path)
	if err != nil {
		return nil, err
	}

	cellType := in.CellType
	if cellType == "" {
		cellType = "code"
	}

	source := splitSource(in.Source)
	newCell := notebookCell{
		CellType: cellType,
		Source:   source,
	}

	idx := in.CellIndex
	if idx < 0 {
		idx = len(nb.Cells)
	}
	if idx > len(nb.Cells) {
		idx = len(nb.Cells)
	}

	nb.Cells = append(nb.Cells[:idx], append([]notebookCell{newCell}, nb.Cells[idx:]...)...)

	if err := n.saveNotebook(in.Path, nb); err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{
		"action": "inserted",
		"index":  idx,
		"type":   cellType,
		"total":  len(nb.Cells),
	})
}

func (n NotebookEdit) deleteCell(in notebookInput) (json.RawMessage, error) {
	nb, err := n.loadNotebook(in.Path)
	if err != nil {
		return nil, err
	}

	if in.CellIndex < 0 || in.CellIndex >= len(nb.Cells) {
		return nil, fmt.Errorf("cell_index %d out of range (0-%d)", in.CellIndex, len(nb.Cells)-1)
	}

	nb.Cells = append(nb.Cells[:in.CellIndex], nb.Cells[in.CellIndex+1:]...)

	if err := n.saveNotebook(in.Path, nb); err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{
		"action": "deleted",
		"index":  in.CellIndex,
		"total":  len(nb.Cells),
	})
}

func (n NotebookEdit) replaceCell(in notebookInput) (json.RawMessage, error) {
	nb, err := n.loadNotebook(in.Path)
	if err != nil {
		return nil, err
	}

	if in.CellIndex < 0 || in.CellIndex >= len(nb.Cells) {
		return nil, fmt.Errorf("cell_index %d out of range (0-%d)", in.CellIndex, len(nb.Cells)-1)
	}

	cellType := in.CellType
	if cellType == "" {
		cellType = nb.Cells[in.CellIndex].CellType
	}

	nb.Cells[in.CellIndex] = notebookCell{
		CellType: cellType,
		Source:   splitSource(in.Source),
	}

	if err := n.saveNotebook(in.Path, nb); err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{
		"action": "replaced",
		"index":  in.CellIndex,
		"type":   cellType,
	})
}

func (n NotebookEdit) loadNotebook(path string) (*notebookFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read notebook: %w", err)
	}

	var nb notebookFile
	if err := json.Unmarshal(data, &nb); err != nil {
		return nil, fmt.Errorf("parse notebook: %w", err)
	}

	if len(nb.Cells) == 0 && nb.NBFormat == 0 {
		// Initialize new notebook
		nb.NBFormat = 4
		nb.NBFormatMinor = 5
		nb.Metadata = map[string]any{}
	}

	return &nb, nil
}

func (n NotebookEdit) saveNotebook(path string, nb *notebookFile) error {
	data, err := json.MarshalIndent(nb, "", " ")
	if err != nil {
		return fmt.Errorf("marshal notebook: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

func splitSource(source string) []string {
	if source == "" {
		return []string{""}
	}

	lines := strings.Split(source, "\n")
	for i := 0; i < len(lines)-1; i++ {
		lines[i] += "\n"
	}
	return lines
}
