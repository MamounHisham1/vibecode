package hooks

import "encoding/json"

// Event identifies a point in the agent lifecycle where hooks can run.
type Event string

const (
	PreToolUse         Event = "PreToolUse"
	PostToolUse        Event = "PostToolUse"
	PostToolUseFailure Event = "PostToolUseFailure"
	UserPromptSubmit   Event = "UserPromptSubmit"
	SessionStart       Event = "SessionStart"
)

// HookType determines how a hook is executed.
type HookType string

const (
	TypeCommand HookType = "command"
	TypeHTTP    HookType = "http"
)

// ResultAction determines what happens after a hook runs.
type ResultAction string

const (
	ActionContinue ResultAction = "continue"
	ActionBlock    ResultAction = "block"
)

// Hook defines a single hook configuration.
type Hook struct {
	Type    HookType          `json:"type"`
	Command string            `json:"command,omitempty"` // for TypeCommand
	URL     string            `json:"url,omitempty"`     // for TypeHTTP
	Matcher string            `json:"matcher,omitempty"` // filter pattern (e.g. tool name)
	Timeout int               `json:"timeout,omitempty"` // seconds, 0 = default (30)
	Once    bool              `json:"once,omitempty"`    // auto-remove after first run
	Headers map[string]string `json:"headers,omitempty"` // for TypeHTTP
}

// Input is the data passed to a hook.
type Input struct {
	Event      Event           `json:"event"`
	ToolName   string          `json:"tool_name,omitempty"`
	ToolInput  json.RawMessage `json:"tool_input,omitempty"`
	ToolOutput string          `json:"tool_output,omitempty"`
	ToolError  string          `json:"tool_error,omitempty"`
	Prompt     string          `json:"prompt,omitempty"`
	SessionID  string          `json:"session_id,omitempty"`
}

// Result is the output from a hook execution.
type Result struct {
	Action       ResultAction    `json:"action"`
	Reason       string          `json:"reason,omitempty"`
	UpdatedInput json.RawMessage `json:"updated_input,omitempty"` // for PreToolUse: modify tool input
}
