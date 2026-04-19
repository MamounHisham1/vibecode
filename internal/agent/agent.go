package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
	OnCompact(summary string)
	OnUsage(inputTokens, outputTokens int)
	OnEstimatedUsage(inputTokens, outputTokens int)
}

// TokenTracker accumulates token counts from provider responses.
type TokenTracker struct {
	InputTokens  int
	OutputTokens int
}

func (t *TokenTracker) Add(input, output int) {
	t.InputTokens += input
	t.OutputTokens += output
}

func (t *TokenTracker) Total() int {
	return t.InputTokens + t.OutputTokens
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

	// Token tracking
	tokens TokenTracker

	// Compaction
	contextWindow    int
	compactThreshold float64 // fraction of context window (e.g. 0.80)
	compacting       bool
	compactCount     int
}

func New(p provider.Provider, reg *tool.Registry, system string, maxIter int, autoApprove []string, cb Callback) *Agent {
	aa := make(map[string]bool)
	for _, name := range autoApprove {
		aa[name] = true
	}

	return &Agent{
		provider:         p,
		registry:         reg,
		system:           system,
		maxIter:          maxIter,
		autoApprove:      aa,
		cb:               cb,
		contextWindow:    200000,
		compactThreshold: 0.80,
	}
}

// SetContextWindow sets the model's context window size in tokens.
func (a *Agent) SetContextWindow(tokens int) {
	a.contextWindow = tokens
}

// SetHooks sets the hook manager for lifecycle events.
func (a *Agent) SetHooks(h *hooks.Manager) {
	a.hooks = h
}

// SetCompactThreshold sets the fraction of the context window at which auto-compact triggers.
func (a *Agent) SetCompactThreshold(threshold float64) {
	a.compactThreshold = threshold
}

// TokenUsage returns a snapshot of the accumulated token counts.
func (a *Agent) TokenUsage() TokenTracker {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.tokens
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
		// Check if auto-compact is needed before sending the request
		if !a.compacting {
			if shouldCompact, _ := a.checkAutoCompact(); shouldCompact {
				log.Printf("Auto-compact triggered: history has grown beyond threshold")
				if err := a.compactHistory(ctx); err != nil {
					log.Printf("Auto-compact failed: %v", err)
				}
			}
		}

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
		receivedUsage := false

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

			case provider.UsageEvent:
				receivedUsage = true
				a.mu.Lock()
				a.tokens.Add(e.InputTokens, e.OutputTokens)
				a.mu.Unlock()
				a.cb.OnUsage(e.InputTokens, e.OutputTokens)

			case provider.DoneEvent:
				// Stream complete

			case provider.ErrorEvent:
				a.cb.OnError(e.Err)
				return e.Err
			}
		}

		// Fallback: estimate tokens if provider did not report usage
		if !receivedUsage {
			a.estimateAndReportTokens(req, textBuf.String(), toolCalls)
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

// Compact triggers a manual compaction of the conversation history.
func (a *Agent) Compact(ctx context.Context) error {
	return a.compactHistory(ctx)
}

// CompactHistory removes old messages keeping only the last N (legacy API).
func (a *Agent) CompactHistory(keepLast int) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if len(a.history) <= keepLast {
		return
	}

	log.Printf("Compacting history: %d messages -> keeping last %d", len(a.history), keepLast)
	a.history = a.history[len(a.history)-keepLast:]
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

// estimateAndReportTokens provides a fallback token estimate when the provider
// does not report usage in its streaming response (common with OpenAI-compatible APIs).
func (a *Agent) estimateAndReportTokens(req provider.Request, responseText string, toolCalls []provider.ToolCallEvent) {
	// Estimate input tokens from system prompt + message history
	inputEst := roughTokenEstimate(req.System, bytesPerToken)
	for _, msg := range req.Messages {
		inputEst += estimateMessageTokens(msg)
	}

	// Estimate output tokens from response text + tool call inputs
	outputEst := roughTokenEstimate(responseText, bytesPerToken)
	for _, tc := range toolCalls {
		outputEst += roughTokenEstimate(string(tc.Input), jsonBytesPerToken)
	}

	a.mu.Lock()
	a.tokens.Add(inputEst, outputEst)
	a.mu.Unlock()
	a.cb.OnEstimatedUsage(inputEst, outputEst)
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
