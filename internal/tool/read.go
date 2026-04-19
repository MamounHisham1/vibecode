package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/v2"
	chromaLexers "github.com/alecthomas/chroma/v2/lexers"
	chromaStyles "github.com/alecthomas/chroma/v2/styles"
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

	// Extract the content portion for highlighting
	var contentLines []string
	for i := start - 1; i < end && i < len(lines); i++ {
		contentLines = append(contentLines, lines[i])
	}
	content := strings.Join(contentLines, "\n")

	// Syntax highlight
	highlighted := highlightCode(content, abs)

	highlightLines := strings.Split(highlighted, "\n")

	var b strings.Builder
	for i, hl := range highlightLines {
		b.WriteString(strconv.Itoa(start + i))
		b.WriteByte('\t')
		b.WriteString(hl)
		b.WriteByte('\n')
	}

	total := len(lines)
	shown := end - start + 1
	result := fmt.Sprintf("%s(%d of %d lines shown)", b.String(), shown, total)

	return json.Marshal(result)
}

// highlightCode applies terminal syntax highlighting to code based on file extension.
func highlightCode(code, filePath string) string {
	lexer := chromaLexers.Match(filePath)
	if lexer == nil {
		lexer = chromaLexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return code
	}

	style := chromaStyles.Get("monokai")
	if style == nil {
		style = chromaStyles.Fallback
	}

	var b strings.Builder
	for _, token := range iterator.Tokens() {
		entry := style.Get(token.Type)
		b.WriteString(stylizeToken(token.Value, entry))
	}

	return b.String()
}

// stylizeToken applies ANSI styling to a token string based on its chroma style entry.
func stylizeToken(text string, entry chroma.StyleEntry) string {
	if text == "" {
		return ""
	}

	var codes []string
	if entry.Bold == chroma.Yes {
		codes = append(codes, "1")
	}
	if entry.Underline == chroma.Yes {
		codes = append(codes, "4")
	}
	if entry.Italic == chroma.Yes {
		codes = append(codes, "3")
	}
	if entry.Colour.IsSet() {
		codes = append(codes, chromaColourToANSI(entry.Colour))
	}

	if len(codes) == 0 {
		return text
	}

	return fmt.Sprintf("\x1b[%sm%s\x1b[0m", strings.Join(codes, ";"), text)
}

// chromaColourToANSI converts a chroma Colour to an ANSI 256-color escape code.
func chromaColourToANSI(c chroma.Colour) string {
	if !c.IsSet() {
		return ""
	}
	// Use 256-color mode
	r, g, b := c.Red(), c.Green(), c.Blue()
	// Convert to 216-color cube (6x6x6 starting at color 16)
	ri := int(float64(r) * 5.0 / 255.0)
	gi := int(float64(g) * 5.0 / 255.0)
	bi := int(float64(b) * 5.0 / 255.0)
	idx := 16 + 36*ri + 6*gi + bi
	return fmt.Sprintf("38;5;%d", idx)
}
