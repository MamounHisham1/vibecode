package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/vibecode/vibecode/internal/provider"
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
	usages      []usageRecord
	dones       int
	errors      []error
	mu          sync.Mutex
}

type usageRecord struct {
	input  int
	output int
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

func (c *mockCallback) OnCompact(summary string) {}

func (c *mockCallback) OnUsage(inputTokens, outputTokens int) {
	c.mu.Lock()
	c.usages = append(c.usages, usageRecord{input: inputTokens, output: outputTokens})
	c.mu.Unlock()
}

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

func TestAgentTokenTracking(t *testing.T) {
	cb := &mockCallback{}
	mp := &mockProvider{
		events: []provider.Event{
			provider.UsageEvent{InputTokens: 100, OutputTokens: 50},
			provider.TextEvent{Text: "Hello!"},
			provider.DoneEvent{},
		},
	}

	a := New(mp, tool.NewRegistry(), "test system", 10, nil, cb)
	a.SetContextWindow(200000)

	err := a.Run(context.Background(), "hi")
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	usage := a.TokenUsage()
	if usage.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", usage.InputTokens)
	}
	if usage.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", usage.OutputTokens)
	}
	if usage.Total() != 150 {
		t.Errorf("Total() = %d, want 150", usage.Total())
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()
	if len(cb.usages) != 1 {
		t.Fatalf("OnUsage called %d times, want 1", len(cb.usages))
	}
	if cb.usages[0].input != 100 || cb.usages[0].output != 50 {
		t.Errorf("OnUsage(%d, %d), want (100, 50)", cb.usages[0].input, cb.usages[0].output)
	}
}

func TestAgentTokenTrackingMultipleRequests(t *testing.T) {
	cb := &mockCallback{}
	mp := &mockProvider{
		events: []provider.Event{
			provider.UsageEvent{InputTokens: 200, OutputTokens: 100},
			provider.TextEvent{Text: "Response"},
			provider.DoneEvent{},
		},
	}

	a := New(mp, tool.NewRegistry(), "test system", 10, nil, cb)

	a.Run(context.Background(), "msg1")
	a.Run(context.Background(), "msg2")

	usage := a.TokenUsage()
	if usage.InputTokens != 400 {
		t.Errorf("InputTokens after 2 requests = %d, want 400", usage.InputTokens)
	}
	if usage.OutputTokens != 200 {
		t.Errorf("OutputTokens after 2 requests = %d, want 200", usage.OutputTokens)
	}
	if usage.Total() != 600 {
		t.Errorf("Total() after 2 requests = %d, want 600", usage.Total())
	}
}

func TestAgentTokenTrackingSplitUsage(t *testing.T) {
	// Anthropic sends input tokens in message_start and output tokens in message_delta
	cb := &mockCallback{}
	mp := &mockProvider{
		events: []provider.Event{
			provider.UsageEvent{InputTokens: 500, OutputTokens: 0},
			provider.TextEvent{Text: "Hi!"},
			provider.UsageEvent{InputTokens: 0, OutputTokens: 75},
			provider.DoneEvent{},
		},
	}

	a := New(mp, tool.NewRegistry(), "test system", 10, nil, cb)
	a.Run(context.Background(), "test")

	usage := a.TokenUsage()
	if usage.InputTokens != 500 {
		t.Errorf("InputTokens = %d, want 500", usage.InputTokens)
	}
	if usage.OutputTokens != 75 {
		t.Errorf("OutputTokens = %d, want 75", usage.OutputTokens)
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()
	if len(cb.usages) != 2 {
		t.Fatalf("OnUsage called %d times, want 2", len(cb.usages))
	}
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

func TestAgentFallbackTokenEstimation(t *testing.T) {
	// Provider sends NO UsageEvent — agent should estimate tokens
	cb := &mockCallback{}
	mp := &mockProvider{
		events: []provider.Event{
			provider.TextEvent{Text: "Hello world, this is a test response!"},
			provider.DoneEvent{},
		},
	}

	a := New(mp, tool.NewRegistry(), "test system", 10, nil, cb)
	a.SetContextWindow(200000)

	err := a.Run(context.Background(), "what is 2+2?")
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	usage := a.TokenUsage()
	// Should have estimated tokens (not zero)
	if usage.InputTokens <= 0 {
		t.Errorf("InputTokens = %d, want > 0 (estimated)", usage.InputTokens)
	}
	if usage.OutputTokens <= 0 {
		t.Errorf("OutputTokens = %d, want > 0 (estimated)", usage.OutputTokens)
	}
	if usage.Total() <= 0 {
		t.Errorf("Total() = %d, want > 0 (estimated)", usage.Total())
	}

	// Verify callback received the estimated usage
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if len(cb.usages) != 1 {
		t.Fatalf("OnUsage called %d times, want 1", len(cb.usages))
	}
	if cb.usages[0].input <= 0 || cb.usages[0].output <= 0 {
		t.Errorf("OnUsage(%d, %d), both want > 0", cb.usages[0].input, cb.usages[0].output)
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
