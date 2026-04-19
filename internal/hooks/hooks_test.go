package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern, s string
		want       bool
	}{
		{"*", "anything", true},
		{"", "anything", true},
		{"read_file", "read_file", true},
		{"read_file", "write_file", false},
		{"read_*", "read_file", true},
		{"read_*", "read_dir", true},
		{"read_*", "write_file", false},
		{"*_file", "read_file", true},
		{"*_file", "write_file", true},
		{"*_file", "shell", false},
	}

	for _, tt := range tests {
		got := matchGlob(tt.pattern, tt.s)
		if got != tt.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", tt.pattern, tt.s, got, tt.want)
		}
	}
}

func TestCommandHookContinue(t *testing.T) {
	m := NewManager()
	m.Register(PreToolUse, Hook{
		Type:    TypeCommand,
		Command: `echo '{"action":"continue"}'`,
	})

	result := m.Run(context.Background(), Input{
		Event:    PreToolUse,
		ToolName: "read_file",
	})

	if result.Action != ActionContinue {
		t.Errorf("Action = %v, want continue", result.Action)
	}
}

func TestCommandHookBlock(t *testing.T) {
	m := NewManager()
	m.Register(PreToolUse, Hook{
		Type:    TypeCommand,
		Command: `echo "forbidden" >&2; exit 2`,
	})

	result := m.Run(context.Background(), Input{
		Event:    PreToolUse,
		ToolName: "write_file",
	})

	if result.Action != ActionBlock {
		t.Errorf("Action = %v, want block", result.Action)
	}
	if result.Reason != "forbidden" {
		t.Errorf("Reason = %q, want 'forbidden'", result.Reason)
	}
}

func TestCommandHookWithMatcher(t *testing.T) {
	m := NewManager()
	m.Register(PreToolUse, Hook{
		Type:    TypeCommand,
		Matcher: "write_*",
		Command: `echo "blocked" >&2; exit 2`,
	})

	// Should match write_file
	result := m.Run(context.Background(), Input{
		Event:    PreToolUse,
		ToolName: "write_file",
	})
	if result.Action != ActionBlock {
		t.Errorf("write_file: Action = %v, want block", result.Action)
	}

	// Should NOT match read_file
	result = m.Run(context.Background(), Input{
		Event:    PreToolUse,
		ToolName: "read_file",
	})
	if result.Action != ActionContinue {
		t.Errorf("read_file: Action = %v, want continue", result.Action)
	}
}

func TestCommandHookOnce(t *testing.T) {
	m := NewManager()
	m.Register(PreToolUse, Hook{
		Type:    TypeCommand,
		Command: `echo "blocked" >&2; exit 2`,
		Once:    true,
	})

	// First call should trigger the hook
	result := m.Run(context.Background(), Input{Event: PreToolUse})
	if result.Action != ActionBlock {
		t.Errorf("first call: Action = %v, want block", result.Action)
	}

	// Second call should skip (once hook already fired)
	result = m.Run(context.Background(), Input{Event: PreToolUse})
	if result.Action != ActionContinue {
		t.Errorf("second call: Action = %v, want continue", result.Action)
	}
}

func TestHTTPHookContinue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(Result{Action: ActionContinue})
	}))
	defer server.Close()

	m := NewManager()
	m.Register(PostToolUse, Hook{
		Type: TypeHTTP,
		URL:  server.URL,
	})

	result := m.Run(context.Background(), Input{
		Event:      PostToolUse,
		ToolName:   "shell",
		ToolOutput: "ok",
	})

	if result.Action != ActionContinue {
		t.Errorf("Action = %v, want continue", result.Action)
	}
}

func TestHTTPHookBlock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, "not allowed")
	}))
	defer server.Close()

	m := NewManager()
	m.Register(PreToolUse, Hook{
		Type: TypeHTTP,
		URL:  server.URL,
	})

	result := m.Run(context.Background(), Input{
		Event:    PreToolUse,
		ToolName: "shell",
	})

	if result.Action != ActionBlock {
		t.Errorf("Action = %v, want block", result.Action)
	}
}

func TestHookTimeout(t *testing.T) {
	m := NewManager()
	m.Register(PreToolUse, Hook{
		Type:    TypeCommand,
		Command: `sleep 10`,
		Timeout: 1, // 1 second timeout
	})

	start := time.Now()
	result := m.Run(context.Background(), Input{Event: PreToolUse})
	elapsed := time.Since(start)

	// Should have timed out quickly, not waited 10 seconds
	if elapsed > 3*time.Second {
		t.Errorf("hook took %v, should have timed out", elapsed)
	}
	if result.Action != ActionContinue {
		t.Errorf("Action = %v, want continue (timeout is non-blocking)", result.Action)
	}
}

func TestLoadFromConfig(t *testing.T) {
	config := map[string]json.RawMessage{
		"PreToolUse":  json.RawMessage(`[{"type":"command","command":"echo ok","matcher":"shell"}]`),
		"PostToolUse": json.RawMessage(`[{"type":"command","command":"echo done"}]`),
	}

	m := NewManager()
	m.LoadFromConfig(config)

	// Should have 1 PreToolUse hook matching "shell"
	result := m.Run(context.Background(), Input{Event: PreToolUse, ToolName: "shell"})
	if result.Action != ActionContinue {
		t.Errorf("PreToolUse shell: Action = %v, want continue", result.Action)
	}

	// Should have no matching hook for "read_file" (only shell matcher)
	result = m.Run(context.Background(), Input{Event: PreToolUse, ToolName: "read_file"})
	if result.Action != ActionContinue {
		t.Errorf("PreToolUse read_file: Action = %v, want continue (no matching hook)", result.Action)
	}

	// PostToolUse should work
	result = m.Run(context.Background(), Input{Event: PostToolUse, ToolName: "anything"})
	if result.Action != ActionContinue {
		t.Errorf("PostToolUse: Action = %v, want continue", result.Action)
	}
}

func TestNoHooks(t *testing.T) {
	m := NewManager()
	result := m.Run(context.Background(), Input{Event: PreToolUse})
	if result.Action != ActionContinue {
		t.Errorf("Action = %v, want continue with no hooks", result.Action)
	}
}
