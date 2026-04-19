package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

const defaultTimeout = 30 * time.Second

// Manager holds registered hooks and executes them.
type Manager struct {
	hooks map[Event][]hookEntry
	mu    chan struct{} // used as a mutex via select
}

type hookEntry struct {
	Hook
	id    int
	fired bool // for Once tracking
}

var nextHookID int

// NewManager creates an empty hook manager.
func NewManager() *Manager {
	return &Manager{
		hooks: make(map[Event][]hookEntry),
	}
}

// Register adds a hook for a given event.
func (m *Manager) Register(event Event, h Hook) {
	nextHookID++
	m.hooks[event] = append(m.hooks[event], hookEntry{
		Hook: h,
		id:   nextHookID,
	})
}

// LoadFromConfig loads hooks from a settings map.
// Expected format: { "PreToolUse": [{ "type": "command", "command": "..." }, ...], ... }
func (m *Manager) LoadFromConfig(config map[string]json.RawMessage) {
	for eventName, raw := range config {
		event := Event(eventName)
		var hookList []Hook
		if err := json.Unmarshal(raw, &hookList); err != nil {
			continue
		}
		for _, h := range hookList {
			m.Register(event, h)
		}
	}
}

// Run executes all matching hooks for an event. It returns the aggregated result.
// If any hook returns ActionBlock, the overall result is Block.
func (m *Manager) Run(ctx context.Context, input Input) Result {
	entries := m.matchingHooks(input.Event, input.ToolName)
	result := Result{Action: ActionContinue}

	for i := range entries {
		e := &entries[i]

		// Skip already-fired once hooks
		if e.Once && e.fired {
			continue
		}
		e.fired = true

		timeout := defaultTimeout
		if e.Timeout > 0 {
			timeout = time.Duration(e.Timeout) * time.Second
		}

		hookCtx, cancel := context.WithTimeout(ctx, timeout)
		r, err := m.executeOne(hookCtx, e.Hook, input)
		cancel()

		if err != nil {
			log.Printf("Hook error (%s %s): %v", input.Event, e.Type, err)
			continue
		}

		if r.Action == ActionBlock {
			result.Action = ActionBlock
			result.Reason = r.Reason
			return result
		}

		// Merge updated input
		if len(r.UpdatedInput) > 0 {
			result.UpdatedInput = r.UpdatedInput
		}
	}

	return result
}

func (m *Manager) matchingHooks(event Event, toolName string) []hookEntry {
	entries := m.hooks[event]
	if toolName == "" {
		return entries
	}

	var matched []hookEntry
	for _, e := range entries {
		if e.Matcher == "" {
			matched = append(matched, e)
			continue
		}
		// Simple glob matching: "read_*" matches "read_file", "*" matches everything
		if matchGlob(e.Matcher, toolName) {
			matched = append(matched, e)
		}
	}
	return matched
}

func (m *Manager) executeOne(ctx context.Context, h Hook, input Input) (Result, error) {
	switch h.Type {
	case TypeCommand:
		return m.execCommand(ctx, h, input)
	case TypeHTTP:
		return m.execHTTP(ctx, h, input)
	default:
		return Result{Action: ActionContinue}, fmt.Errorf("unknown hook type: %s", h.Type)
	}
}

func (m *Manager) execCommand(ctx context.Context, h Hook, input Input) (Result, error) {
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return Result{Action: ActionContinue}, fmt.Errorf("marshal input: %w", err)
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", h.Command)
	cmd.Stdin = bytes.NewReader(inputJSON)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		// Exit code 2 = explicit block
		if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 2 {
			return Result{
				Action: ActionBlock,
				Reason: strings.TrimSpace(stderr.String()),
			}, nil
		}
		return Result{Action: ActionContinue}, fmt.Errorf("command failed: %w\n%s", err, stderr.String())
	}

	// Parse JSON output if present
	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return Result{Action: ActionContinue}, nil
	}

	var r Result
	if err := json.Unmarshal([]byte(output), &r); err != nil {
		// Non-JSON output: treat as continue with reason
		return Result{Action: ActionContinue, Reason: truncate(output, 200)}, nil
	}
	if r.Action == "" {
		r.Action = ActionContinue
	}
	return r, nil
}

func (m *Manager) execHTTP(ctx context.Context, h Hook, input Input) (Result, error) {
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return Result{Action: ActionContinue}, fmt.Errorf("marshal input: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.URL, bytes.NewReader(inputJSON))
	if err != nil {
		return Result{Action: ActionContinue}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range h.Headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Result{Action: ActionContinue}, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == 403 {
		return Result{
			Action: ActionBlock,
			Reason: truncate(string(body), 200),
		}, nil
	}

	if resp.StatusCode >= 400 {
		return Result{Action: ActionContinue}, fmt.Errorf("http %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var r Result
	if err := json.Unmarshal(body, &r); err != nil {
		return Result{Action: ActionContinue}, nil
	}
	if r.Action == "" {
		r.Action = ActionContinue
	}
	return r, nil
}

// matchGlob does simple glob matching: "*" matches everything, "prefix_*" matches "prefix_" + anything.
func matchGlob(pattern, s string) bool {
	if pattern == "*" || pattern == "" {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return pattern == s
	}

	parts := strings.SplitN(pattern, "*", 2)
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	if len(parts) > 1 && parts[1] != "" {
		return strings.HasSuffix(s, parts[1])
	}
	return true
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
