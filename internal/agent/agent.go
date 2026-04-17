package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/vibecode/vibecode/internal/provider"
	"github.com/vibecode/vibecode/internal/tool"
)

// Callback is called by the agent loop to stream events back to the UI.
type Callback interface {
	OnText(text string)
	OnToolStart(name, id string)
	OnToolOutput(name, id string, output string, err error)
	OnDone()
	OnError(err error)
}

type Agent struct {
	provider   provider.Provider
	registry   *tool.Registry
	system     string
	maxIter    int
	history    []provider.Message
	autoApprove map[string]bool
	cb         Callback
	mu         sync.Mutex
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

// Run processes a user message through the agent loop.
func (a *Agent) Run(ctx context.Context, userMsg string) error {
	a.mu.Lock()
	a.history = append(a.history, provider.UserMessage(userMsg))
	a.mu.Unlock()

	// Collect tool definitions
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

		// Collect the full response from the stream
		var textBuf strings.Builder
		var toolCalls []provider.ToolCallEvent
		var toolInputBufs map[string]string // id -> accumulated JSON
		if toolInputBufs == nil {
			toolInputBufs = make(map[string]string)
		}

		for ev := range events {
			switch e := ev.(type) {
			case provider.TextEvent:
				// If we're accumulating tool input, append there
				if len(toolCalls) > 0 {
					lastID := toolCalls[len(toolCalls)-1].ID
					toolInputBufs[lastID] += e.Text
					continue
				}
				textBuf.WriteString(e.Text)
				a.cb.OnText(e.Text)

			case provider.ToolCallEvent:
				if e.ID != "" && e.Name != "" {
					// New tool call start
					toolCalls = append(toolCalls, e)
					toolInputBufs[e.ID] = ""
				} else if e.Input != nil {
					// Full input in one event
					toolCalls = append(toolCalls, e)
				}

			case provider.DoneEvent:
				// Stream complete

			case provider.ErrorEvent:
				a.cb.OnError(e.Err)
				return e.Err
			}
		}

		// If there's text, append assistant message
		if textBuf.Len() > 0 && len(toolCalls) == 0 {
			a.mu.Lock()
			a.history = append(a.history, provider.AssistantTextMessage(textBuf.String()))
			a.mu.Unlock()
			a.cb.OnDone()
			return nil
		}

		// If there are tool calls, execute them
		if len(toolCalls) > 0 {
			// Append any remaining text first
			if textBuf.Len() > 0 {
				a.cb.OnText(textBuf.String())
			}

			// Build assistant message with tool calls
			// Use the accumulated input buffers
			finalCalls := make([]provider.ToolCallEvent, len(toolCalls))
			for i, tc := range toolCalls {
				input := tc.Input
				if input == nil && toolInputBufs[tc.ID] != "" {
					input = json.RawMessage(toolInputBufs[tc.ID])
				}
				if input == nil {
					input = json.RawMessage("{}")
				}
				finalCalls[i] = provider.ToolCallEvent{
					ID:    tc.ID,
					Name:  tc.Name,
					Input: input,
				}
			}

			a.mu.Lock()
			a.history = append(a.history, provider.AssistantToolCallsMessage(finalCalls))
			a.mu.Unlock()

			// Execute tool calls (parallel)
			var wg sync.WaitGroup
			var histMu sync.Mutex
			var toolResults []provider.Message

			for _, tc := range finalCalls {
				wg.Add(1)
				go func(call provider.ToolCallEvent) {
					defer wg.Done()

					a.cb.OnToolStart(call.Name, call.ID)

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

			// Continue loop — the LLM will see the tool results
			continue
		}

		// Empty response with no tool calls — we're done
		a.cb.OnDone()
		return nil
	}

	a.cb.OnError(fmt.Errorf("max iterations (%d) reached", a.maxIter))
	return fmt.Errorf("max iterations reached")
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

// History returns the current conversation history.
func (a *Agent) History() []provider.Message {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.history
}

// CompactHistory truncates old turns when the history gets too long.
// Keeps the system prompt context and the last N turns.
func (a *Agent) CompactHistory(keepLast int) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if len(a.history) <= keepLast {
		return
	}

	log.Printf("Compacting history: %d messages → keeping last %d", len(a.history), keepLast)
	a.history = a.history[len(a.history)-keepLast:]
}
