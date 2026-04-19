package tui

import (
	"strings"
	"testing"
)

func TestComputeDiff(t *testing.T) {
	tests := []struct {
		name    string
		old     string
		new     string
		added   int
		removed int
	}{
		{
			name:    "single line change",
			old:     "hello\nworld\n",
			new:     "hello\nearth\n",
			added:   1,
			removed: 1,
		},
		{
			name:    "add lines",
			old:     "line1\nline2\n",
			new:     "line1\nline2\nline3\n",
			added:   1,
			removed: 0,
		},
		{
			name:    "remove lines",
			old:     "line1\nline2\nline3\n",
			new:     "line1\nline3\n",
			added:   0,
			removed: 1,
		},
		{
			name:    "no change",
			old:     "same\ncontent\n",
			new:     "same\ncontent\n",
			added:   0,
			removed: 0,
		},
		{
			name:    "complete replacement",
			old:     "old line\nanother old\n",
			new:     "new line\nanother new\n",
			added:   2,
			removed: 2,
		},
		{
			name:    "empty to content",
			old:     "",
			new:     "new file\ncontent\n",
			added:   2,
			removed: 0,
		},
		{
			name:    "content to empty",
			old:     "old content\n",
			new:     "",
			added:   0,
			removed: 1,
		},
		{
			name:    "empty strings",
			old:     "",
			new:     "",
			added:   0,
			removed: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := computeDiff(tt.old, tt.new)
			added, removed := 0, 0
			for _, l := range lines {
				switch l.Kind {
				case "add":
					added++
				case "remove":
					removed++
				}
			}
			if added != tt.added {
				t.Errorf("added = %d, want %d", added, tt.added)
			}
			if removed != tt.removed {
				t.Errorf("removed = %d, want %d", removed, tt.removed)
			}
		})
	}
}

func TestComputeDiffLineNumbers(t *testing.T) {
	old := "line1\nline2\nline3\n"
	new := "line1\nmodified\nline3\n"

	lines := computeDiff(old, new)

	// Should have: context(line1), remove(line2), add(modified), context(line3)
	if len(lines) != 4 {
		t.Fatalf("expected 4 diff lines, got %d", len(lines))
	}

	// Check context lines have correct line numbers
	if lines[0].Kind != "context" || lines[0].OldNum != 1 || lines[0].NewNum != 1 {
		t.Errorf("first line: got kind=%s oldNum=%d newNum=%d", lines[0].Kind, lines[0].OldNum, lines[0].NewNum)
	}
	if lines[3].Kind != "context" || lines[3].OldNum != 3 || lines[3].NewNum != 3 {
		t.Errorf("last line: got kind=%s oldNum=%d newNum=%d", lines[3].Kind, lines[3].OldNum, lines[3].NewNum)
	}

	// Removed line should have OldNum but not NewNum
	if lines[1].Kind != "remove" || lines[1].OldNum != 2 {
		t.Errorf("removed line: got kind=%s oldNum=%d", lines[1].Kind, lines[1].OldNum)
	}

	// Added line should have NewNum but not OldNum
	if lines[2].Kind != "add" || lines[2].NewNum != 2 {
		t.Errorf("added line: got kind=%s newNum=%d", lines[2].Kind, lines[2].NewNum)
	}
}

func TestRenderDiffLines(t *testing.T) {
	theme := DefaultTheme()
	old := "foo\nbar\nbaz\n"
	new := "foo\nqux\nbaz\n"

	lines := computeDiff(old, new)
	summary, preview, added, removed := renderDiffLines(lines, theme, 80, "test.go")

	if added != 1 {
		t.Errorf("added = %d, want 1", added)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}
	if summary == "" {
		t.Error("summary is empty")
	}
	if !strings.Contains(summary, "+1") || !strings.Contains(summary, "-1") {
		t.Errorf("summary missing +/- counts: %s", summary)
	}
	if len(preview) == 0 {
		t.Error("preview is empty")
	}

	// Preview lines should be non-empty strings
	for i, line := range preview {
		if line == "" {
			t.Errorf("preview line %d is empty", i)
		}
	}
}

func TestFormatDiffPreview(t *testing.T) {
	theme := DefaultTheme()
	old := "func main() {\n\tfmt.Println(\"hello\")\n}\n"
	new := "func main() {\n\tfmt.Println(\"world\")\n\tfmt.Println(\"!\")\n}\n"

	summary, preview := formatDiffPreview(old, new, theme, 80, "main.go")

	if summary == "" {
		t.Error("summary is empty")
	}
	if len(preview) == 0 {
		t.Error("preview is empty")
	}
	// Should show "world" as added and "!" as added
	joined := strings.Join(preview, "")
	if !strings.Contains(joined, "world") {
		t.Error("preview should contain 'world'")
	}
}

func TestRenderDiffLinesEmpty(t *testing.T) {
	theme := DefaultTheme()
	summary, preview, added, removed := renderDiffLines(nil, theme, 80, "test.go")

	if summary != "No changes" {
		t.Errorf("summary = %q, want 'No changes'", summary)
	}
	if preview != nil {
		t.Error("preview should be nil for empty diff")
	}
	if added != 0 || removed != 0 {
		t.Errorf("added=%d removed=%d, want 0 0", added, removed)
	}
}
