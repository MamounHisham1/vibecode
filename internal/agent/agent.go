package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/vibecode/vibecode/internal/hooks"
	"github.com/vibecode/vibecode/internal/provider"
	"github.com/vibecode/vibecode/internal/tool"
)

// Callback is called by the agent loop to stream events back to the UI.
type Callback interface {
	OnText(text string)
	OnToolStart(name, id string, input json.RawMessage)
	OnToolOutput(name, id string, output string, err error)
	OnDone()
	OnError(err error)
}

type Agent struct {
	provider    provider.Provider
	registry    *tool.Registry
	system      string
	maxIter     int
	history     []provider.Message
	autoApprove map[string]bool
	cb          Callback
	mu          sync.Mutex
	callCounter int

	// Hooks
	hooks *hooks.Manager

	// Plan mode
	planMode bool
}

func New(p provider.Provider, reg *tool.Registry, system string, maxIter int, autoApprove []string, cb Callback) *Agent {
	aa := make(map[string]bool)
	for _, name := range autoApprove {
		aa[name] = true
	}

	return &Agent{
		provider:    p,
		registry:    reg,
		system:      system,
		maxIter:     maxIter,
		autoApprove: aa,
		cb:          cb,
	}
}

// SetHooks sets the hook manager for lifecycle events.
func (a *Agent) SetHooks(h *hooks.Manager) {
	a.hooks = h
}

func (a *Agent) nextCallID() string {
	a.callCounter++
	return fmt.Sprintf("call_%d", a.callCounter)
}

// Run processes a user message through the agent loop.
func (a *Agent) Run(ctx context.Context, userMsg string) error {
	a.mu.Lock()
	a.history = append(a.history, provider.UserMessage(userMsg))
	a.mu.Unlock()

	toolDefs := a.buildToolDefs()

	for i := 0; i < a.maxIter; i++ {
		req := provider.Request{
			System:   a.system,
			Messages: a.history,
			Tools:    toolDefs,
		}

		events, err := a.provider.Stream(ctx, req)
		if err != nil {
			a.cb.OnError(fmt.Errorf("LLM request failed: %w", err))
			return err
		}

		var textBuf strings.Builder
		var toolCalls []provider.ToolCallEvent

		for ev := range events {
			switch e := ev.(type) {
			case provider.TextEvent:
				textBuf.WriteString(e.Text)
				a.cb.OnText(e.Text)

			case provider.ToolCallEvent:
				if e.Name != "" {
					// Start of a new tool call
					id := e.ID
					if id == "" {
						id = a.nextCallID()
					}
					toolCalls = append(toolCalls, provider.ToolCallEvent{
						ID:    id,
						Name:  e.Name,
						Input: e.Input,
					})
				} else if e.ID != "" && e.Input != nil {
					// Completing a previously started tool call with input data
					for i, tc := range toolCalls {
						if tc.ID == e.ID {
							toolCalls[i].Input = e.Input
							break
						}
					}
				}

			case provider.DoneEvent:
				// Stream complete

			case provider.ErrorEvent:
				a.cb.OnError(e.Err)
				return e.Err
			}
		}

		fullText := textBuf.String()

		// Pure text response, no tool calls
		if len(toolCalls) == 0 {
			a.mu.Lock()
			a.history = append(a.history, provider.AssistantTextMessage(fullText))
			a.mu.Unlock()
			a.cb.OnDone()
			return nil
		}

		// Execute tool calls
		finalCalls := a.resolveToolInputs(toolCalls)

		a.mu.Lock()
		a.history = append(a.history, provider.AssistantToolCallsMessage(finalCalls))
		a.mu.Unlock()

		var wg sync.WaitGroup
		var histMu sync.Mutex
		var toolResults []provider.Message

		for _, tc := range finalCalls {
			wg.Add(1)
			go func(call provider.ToolCallEvent) {
				defer wg.Done()

				a.cb.OnToolStart(call.Name, call.ID, call.Input)

				// Plan mode enforcement
				a.mu.Lock()
				pm := a.planMode
				a.mu.Unlock()
				if pm && isWriteTool(call.Name) {
					output := fmt.Sprintf("blocked: %s is not available in plan mode (read-only)", call.Name)
					a.cb.OnToolOutput(call.Name, call.ID, output, fmt.Errorf("plan mode: %s blocked", call.Name))
					histMu.Lock()
					toolResults = append(toolResults, provider.ToolResultMessage(
						call.ID, json.RawMessage(fmt.Sprintf(`"plan mode: %s is read-only"`, call.Name)), true,
					))
					histMu.Unlock()
					return
				}

				// Check for plan mode toggle tools
				if call.Name == "enter_plan_mode" {
					a.mu.Lock()
					a.planMode = true
					a.mu.Unlock()
				} else if call.Name == "exit_plan_mode" {
					a.mu.Lock()
					a.planMode = false
					a.mu.Unlock()
				}

				// PreToolUse hook
				if a.hooks != nil {
					hookResult := a.hooks.Run(ctx, hooks.Input{
						Event:     hooks.PreToolUse,
						ToolName:  call.Name,
						ToolInput: call.Input,
					})
					if hookResult.Action == hooks.ActionBlock {
						output := fmt.Sprintf("blocked by hook: %s", hookResult.Reason)
						a.cb.OnToolOutput(call.Name, call.ID, output, fmt.Errorf("hook blocked: %s", hookResult.Reason))
						histMu.Lock()
						toolResults = append(toolResults, provider.ToolResultMessage(
							call.ID, json.RawMessage(fmt.Sprintf(`"blocked by hook: %s"`, hookResult.Reason)), true,
						))
						histMu.Unlock()
						return
					}
					if len(hookResult.UpdatedInput) > 0 {
						call.Input = hookResult.UpdatedInput
					}
				}

				result, err := a.registry.Execute(ctx, call.Name, call.Input)

				var output string
				var isError bool
				if err != nil {
					output = err.Error()
					isError = true
				} else {
					output = string(result)
				}

				a.cb.OnToolOutput(call.Name, call.ID, output, err)

				// PostToolUse / PostToolUseFailure hooks
				if a.hooks != nil {
					event := hooks.PostToolUse
					if isError {
						event = hooks.PostToolUseFailure
					}
					errStr := ""
					if isError {
						errStr = output
					}
					a.hooks.Run(ctx, hooks.Input{
						Event:      event,
						ToolName:   call.Name,
						ToolInput:  call.Input,
						ToolOutput: truncateStr(output, 2000),
						ToolError:  errStr,
					})
				}

				histMu.Lock()
				toolResults = append(toolResults, provider.ToolResultMessage(
					call.ID, json.RawMessage(output), isError,
				))
				histMu.Unlock()
			}(tc)
		}
		wg.Wait()

		a.mu.Lock()
		a.history = append(a.history, toolResults...)
		a.mu.Unlock()

		continue
	}

	a.cb.OnError(fmt.Errorf("max iterations (%d) reached", a.maxIter))
	return fmt.Errorf("max iterations reached")
}

func (a *Agent) History() []provider.Message {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.history
}

func (a *Agent) resolveToolInputs(calls []provider.ToolCallEvent) []provider.ToolCallEvent {
	out := make([]provider.ToolCallEvent, len(calls))
	for i, tc := range calls {
		input := tc.Input
		if input == nil {
			input = json.RawMessage("{}")
		}
		out[i] = provider.ToolCallEvent{
			ID:    tc.ID,
			Name:  tc.Name,
			Input: input,
		}
	}
	return out
}

// isWriteTool returns true for tools that modify files or system state.
func isWriteTool(name string) bool {
	switch name {
	case "write_file", "edit_file", "shell", "git", "notebook_edit", "ask_user", "todo_write":
		return true
	default:
		return false
	}
}

// PlanModeActive returns whether the agent is in plan mode.
func (a *Agent) PlanModeActive() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.planMode
}

func (a *Agent) buildToolDefs() []provider.ToolDef {
	tools := a.registry.All()
	defs := make([]provider.ToolDef, len(tools))
	for i, t := range tools {
		defs[i] = provider.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		}
	}
	return defs
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
