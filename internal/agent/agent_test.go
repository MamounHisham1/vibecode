package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/vibecode/vibecode/internal/provider"
	"github.com/vibecode/vibecode/internal/session"
	"github.com/vibecode/vibecode/internal/tool"
)

// mockProvider sends a canned sequence of events.
type mockProvider struct {
	events []provider.Event
}

func (m *mockProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	ch := make(chan provider.Event, 64)
	go func() {
		defer close(ch)
		for _, ev := range m.events {
			select {
			case <-ctx.Done():
				ch <- provider.ErrorEvent{Err: ctx.Err()}
				return
			default:
				ch <- ev
			}
		}
	}()
	return ch, nil
}

// mockCallback records callback invocations.
type mockCallback struct {
	texts       []string
	toolStarts  []string
	toolOutputs []string
	dones       int
	errors      []error
	mu          sync.Mutex
}

func (c *mockCallback) OnText(text string) {
	c.mu.Lock()
	c.texts = append(c.texts, text)
	c.mu.Unlock()
}

func (c *mockCallback) OnToolStart(name, id string, input json.RawMessage) {
	c.mu.Lock()
	c.toolStarts = append(c.toolStarts, name)
	c.mu.Unlock()
}

func (c *mockCallback) OnToolOutput(name, id, output string, err error) {
	c.mu.Lock()
	c.toolOutputs = append(c.toolOutputs, name)
	c.mu.Unlock()
}

func (c *mockCallback) OnDone() {
	c.mu.Lock()
	c.dones++
	c.mu.Unlock()
}

func (c *mockCallback) OnError(err error) {
	c.mu.Lock()
	c.errors = append(c.errors, err)
	c.mu.Unlock()
}
func (c *mockCallback) OnTokenUsage(_ session.SessionUsage) {}
func (c *mockCallback) OnCompaction(_ string)               {}

// mockTool implements tool.Tool for testing.
type mockTool struct {
	toolName string
}

func (m *mockTool) Name() string        { return m.toolName }
func (m *mockTool) Description() string { return "mock tool" }
func (m *mockTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}
func (m *mockTool) Execute(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`"ok"`), nil
}

func TestAgentTextResponse(t *testing.T) {
	cb := &mockCallback{}
	mp := &mockProvider{
		events: []provider.Event{
			provider.TextEvent{Text: "Hello "},
			provider.TextEvent{Text: "world!"},
			provider.DoneEvent{},
		},
	}

	a := New(mp, tool.NewRegistry(), "test system", 10, nil, cb)
	err := a.Run(context.Background(), "hi")
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	fullText := strings.Join(cb.texts, "")
	if fullText != "Hello world!" {
		t.Errorf("text = %q, want %q", fullText, "Hello world!")
	}
	if cb.dones != 1 {
		t.Errorf("OnDone called %d times, want 1", cb.dones)
	}
}

func TestAgentContextCancellation(t *testing.T) {
	cb := &mockCallback{}
	mp := &mockProvider{
		events: []provider.Event{
			provider.TextEvent{Text: "partial"},
			provider.DoneEvent{},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	a := New(mp, tool.NewRegistry(), "test system", 10, nil, cb)
	err := a.Run(ctx, "hi")
	if err == nil {
		t.Error("expected error from cancelled context, got nil")
	}
}

func TestAgentMaxIterations(t *testing.T) {
	cb := &mockCallback{}
	callCount := 0

	// Provider that always returns a tool call
	mp := &mockProviderFunc{
		fn: func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
			callCount++
			ch := make(chan provider.Event, 4)
			go func() {
				defer close(ch)
				ch <- provider.ToolCallEvent{
					ID:    "call_1",
					Name:  "read_file",
					Input: json.RawMessage(`{"path": "/tmp/test"}`),
				}
				ch <- provider.DoneEvent{}
			}()
			return ch, nil
		},
	}

	reg := tool.NewRegistry()
	reg.Register(&mockTool{toolName: "read_file"})

	a := New(mp, reg, "test system", 3, []string{"read_file"}, cb)
	err := a.Run(context.Background(), "hi")

	if err == nil {
		t.Error("expected max iterations error, got nil")
	}
	if !strings.Contains(err.Error(), "max iterations") {
		t.Errorf("error = %q, want 'max iterations'", err.Error())
	}
	if callCount > 3 {
		t.Errorf("provider called %d times, want <= 3", callCount)
	}
}

func TestAgentToolCallWithDeferredInput(t *testing.T) {
	// Simulates Anthropic's pattern: ToolCallEvent(name only) then ToolCallEvent(input only)
	cb := &mockCallback{}
	reg := tool.NewRegistry()
	reg.Register(&mockTool{toolName: "read_file"})

	callCount := 0
	mp := &mockProviderFunc{
		fn: func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
			ch := make(chan provider.Event, 8)
			go func() {
				defer close(ch)
				callCount++
				if callCount == 1 {
					// First call: text + tool call (deferred input pattern)
					ch <- provider.TextEvent{Text: "Let me read that file."}
					ch <- provider.ToolCallEvent{
						ID:   "toolu_01",
						Name: "read_file",
					}
					ch <- provider.ToolCallEvent{
						ID:    "toolu_01",
						Input: json.RawMessage(`{"path": "/tmp/test.go"}`),
					}
				} else {
					// Second call: just text (response to tool result)
					ch <- provider.TextEvent{Text: "Done!"}
				}
				ch <- provider.DoneEvent{}
			}()
			return ch, nil
		},
	}

	a := New(mp, reg, "test system", 10, []string{"read_file"}, cb)
	err := a.Run(context.Background(), "read test.go")
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if len(cb.toolStarts) != 1 || cb.toolStarts[0] != "read_file" {
		t.Errorf("toolStarts = %v, want [read_file]", cb.toolStarts)
	}
	if len(cb.toolOutputs) != 1 {
		t.Errorf("toolOutputs count = %d, want 1", len(cb.toolOutputs))
	}
	if cb.dones != 1 {
		t.Errorf("dones = %d, want 1", cb.dones)
	}
}

func TestAgentToolCallWithAllInOne(t *testing.T) {
	// OpenAI pattern: tool call with name + input in one event
	cb := &mockCallback{}
	reg := tool.NewRegistry()
	reg.Register(&mockTool{toolName: "shell"})

	callCount := 0
	mp := &mockProviderFunc{
		fn: func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
			ch := make(chan provider.Event, 4)
			go func() {
				defer close(ch)
				callCount++
				if callCount == 1 {
					ch <- provider.ToolCallEvent{
						ID:    "call_abc",
						Name:  "shell",
						Input: json.RawMessage(`{"command": "ls -la"}`),
					}
				} else {
					ch <- provider.TextEvent{Text: "Done!"}
				}
				ch <- provider.DoneEvent{}
			}()
			return ch, nil
		},
	}

	a := New(mp, reg, "test system", 10, []string{"shell"}, cb)
	err := a.Run(context.Background(), "list files")
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if len(cb.toolStarts) != 1 || cb.toolStarts[0] != "shell" {
		t.Errorf("toolStarts = %v, want [shell]", cb.toolStarts)
	}
}

// mockProviderFunc is a provider implemented as a function.
type mockProviderFunc struct {
	fn func(ctx context.Context, req provider.Request) (<-chan provider.Event, error)
}

func (m *mockProviderFunc) Stream(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	return m.fn(ctx, req)
}
