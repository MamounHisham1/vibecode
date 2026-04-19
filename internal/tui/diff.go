package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// DiffLine represents a single line in a diff.
type DiffLine struct {
	Kind   string // "add", "remove", "context"
	Text   string
	OldNum int // line number in old file (0 if added)
	NewNum int // line number in new file (0 if removed)
}

// computeDiff produces a unified-style diff between two strings.
// Uses a line-level LCS (longest common subsequence) algorithm.
func computeDiff(oldStr, newStr string) []DiffLine {
	oldLines := strings.Split(oldStr, "\n")
	newLines := strings.Split(newStr, "\n")

	// Trim trailing empty lines that come from trailing newlines
	if len(oldLines) > 0 && oldLines[len(oldLines)-1] == "" {
		oldLines = oldLines[:len(oldLines)-1]
	}
	if len(newLines) > 0 && newLines[len(newLines)-1] == "" {
		newLines = newLines[:len(newLines)-1]
	}

	// Compute LCS table
	m, n := len(oldLines), len(newLines)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if oldLines[i-1] == newLines[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] > dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}

	// Backtrack to produce diff
	var result []DiffLine
	i, j := m, n

	// Collect in reverse, then reverse
	type diffEntry struct {
		kind   string
		oldIdx int
		newIdx int
	}
	var entries []diffEntry

	for i > 0 || j > 0 {
		if i > 0 && j > 0 && oldLines[i-1] == newLines[j-1] {
			entries = append(entries, diffEntry{"context", i - 1, j - 1})
			i--
			j--
		} else if j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]) {
			entries = append(entries, diffEntry{"add", -1, j - 1})
			j--
		} else {
			entries = append(entries, diffEntry{"remove", i - 1, -1})
			i--
		}
	}

	// Reverse entries
	for left, right := 0, len(entries)-1; left < right; left, right = left+1, right-1 {
		entries[left], entries[right] = entries[right], entries[left]
	}

	// Convert to DiffLines with line numbers
	oldNum, newNum := 1, 1
	for _, e := range entries {
		switch e.kind {
		case "context":
			result = append(result, DiffLine{
				Kind:   "context",
				Text:   oldLines[e.oldIdx],
				OldNum: oldNum,
				NewNum: newNum,
			})
			oldNum++
			newNum++
		case "remove":
			result = append(result, DiffLine{
				Kind:   "remove",
				Text:   oldLines[e.oldIdx],
				OldNum: oldNum,
			})
			oldNum++
		case "add":
			result = append(result, DiffLine{
				Kind:   "add",
				Text:   newLines[e.newIdx],
				NewNum: newNum,
			})
			newNum++
		}
	}

	return result
}

// renderDiffLines converts diff lines into styled strings for TUI display.
// Returns (summary, previewLines, addedCount, removedCount).
func renderDiffLines(lines []DiffLine, theme Theme, width int, filePath string) (string, []string, int, int) {
	if len(lines) == 0 {
		return "No changes", nil, 0, 0
	}

	added, removed := 0, 0
	for _, l := range lines {
		switch l.Kind {
		case "add":
			added++
		case "remove":
			removed++
		}
	}

	shortPath := filePath
	if len(shortPath) > 40 {
		shortPath = "..." + shortPath[len(shortPath)-37:]
	}
	summary := fmt.Sprintf("%s  +%d added, -%d removed", shortPath, added, removed)

	// Compute line number width
	maxNum := 0
	for _, l := range lines {
		if l.OldNum > maxNum {
			maxNum = l.OldNum
		}
		if l.NewNum > maxNum {
			maxNum = l.NewNum
		}
	}
	numWidth := len(fmt.Sprintf("%d", maxNum))

	// Account for the fixed-width prefix: "NNN +/- " = numWidth + 3 visual chars
	prefixWidth := numWidth + 3 // lineNum + marker + space
	contentWidth := width - prefixWidth
	if contentWidth < 20 {
		contentWidth = 20
	}

	// Render up to maxLines diff lines
	maxLines := 25
	lineCount := 0
	var preview []string

	for _, l := range lines {
		if lineCount >= maxLines {
			preview = append(preview, theme.Dim.Render(fmt.Sprintf("  ... +%d more lines", len(lines)-maxLines)))
			break
		}

		var lineNumStr string
		switch l.Kind {
		case "remove":
			lineNumStr = fmt.Sprintf("%*d", numWidth, l.OldNum)
		case "add":
			lineNumStr = fmt.Sprintf("%*d", numWidth, l.NewNum)
		default:
			lineNumStr = fmt.Sprintf("%*d", numWidth, l.OldNum)
		}

		var marker, styledText string
		switch l.Kind {
		case "remove":
			marker = theme.DiffRemove.Render("-")
			styledText = theme.DiffRemove.Render(truncateVisual(l.Text, contentWidth))
		case "add":
			marker = theme.DiffAdd.Render("+")
			styledText = theme.DiffAdd.Render(truncateVisual(l.Text, contentWidth))
		default:
			marker = " "
			styledText = theme.Text.Render(truncateVisual(l.Text, contentWidth))
		}

		numField := theme.DiffLineNum.Render(lineNumStr)
		rendered := numField + " " + marker + " " + styledText

		preview = append(preview, rendered)
		lineCount++
	}

	return summary, preview, added, removed
}

// truncateVisual truncates a string to fit within maxVisual characters,
// preserving ANSI escape codes.
func truncateVisual(s string, maxVisual int) string {
	w := ansi.StringWidth(s)
	if w <= maxVisual {
		return s
	}
	runes := []rune(s)
	var result strings.Builder
	visual := 0
	inEscape := false
	for _, r := range runes {
		if r == '\x1b' {
			inEscape = true
			result.WriteRune(r)
			continue
		}
		if inEscape {
			result.WriteRune(r)
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		if visual >= maxVisual {
			break
		}
		result.WriteRune(r)
		visual++
	}
	return result.String()
}

// formatDiffPreview is the public entry point for creating diff preview content.
// It takes old/new strings and returns styled lines suitable for toolEntry.Preview.
func formatDiffPreview(oldStr, newStr string, theme Theme, width int, filePath string) (summary string, preview []string) {
	lines := computeDiff(oldStr, newStr)
	if len(lines) == 0 {
		return "No changes", nil
	}
	s, p, _, _ := renderDiffLines(lines, theme, width, filePath)
	return s, p
}
