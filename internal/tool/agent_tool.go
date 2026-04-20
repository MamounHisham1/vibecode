package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/vibecode/vibecode/internal/session"
)

// SubagentRunner is a function that runs a subagent with the given system prompt,
// filtered registry, and prompt. Returns the text result and any error.
type SubagentRunner func(ctx context.Context, systemPrompt string, reg *Registry, prompt string) (string, error)

// AgentTool spawns subagents for parallel or delegated work.
type AgentTool struct {
	reg    *Registry
	runner SubagentRunner
	dir    string
}

// NewAgentTool creates an agent tool with a runner function that creates and executes subagents.
func NewAgentTool(reg *Registry, runner SubagentRunner, dir string) *AgentTool {
	return &AgentTool{
		reg:    reg,
		runner: runner,
		dir:    dir,
	}
}

func (a *AgentTool) Name() string { return "agent" }

func (a *AgentTool) Description() string {
	return "Launch a new agent to handle complex, multi-step tasks. Each agent type has specific capabilities and tools available to it."
}

func (a *AgentTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"description": {
				"type": "string",
				"description": "A short (3-5 word) description of the task"
			},
			"prompt": {
				"type": "string",
				"description": "The task for the agent to perform"
			},
			"subagent_type": {
				"type": "string",
				"description": "The type of specialized agent to use. If omitted, the general-purpose agent is used.",
				"enum": ["general-purpose", "Explore", "Plan"]
			}
		},
		"required": ["description", "prompt"]
	}`)
}

type agentInput struct {
	Description  string `json:"description"`
	Prompt       string `json:"prompt"`
	SubagentType string `json:"subagent_type"`
}

func (a *AgentTool) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in agentInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}

	if in.Prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	// Resolve agent type
	agentType := in.SubagentType
	if agentType == "" {
		agentType = "general-purpose"
	}

	at := FindAgentType(agentType)
	if at == nil {
		return nil, fmt.Errorf("unknown agent type: %s", agentType)
	}

	// Build filtered registry for this agent type
	filteredReg := NewRegistry()
	for _, t := range a.reg.All() {
		if at.IsToolAllowed(t.Name()) {
			filteredReg.Register(t)
		}
	}

	// Build system prompt for the subagent
	systemPrompt := at.SystemPrompt
	if a.dir != "" {
		systemPrompt += fmt.Sprintf("\n\nWorking directory: %s", a.dir)
	}

	// Run subagent via the injected runner
	result, err := a.runner(ctx, systemPrompt, filteredReg, in.Prompt)
	if err != nil {
		return nil, fmt.Errorf("subagent failed: %w", err)
	}

	return json.Marshal(map[string]any{
		"status":      "completed",
		"agent_type":  at.Name,
		"description": in.Description,
		"result":      result,
	})
}

// SubagentCollector is a callback that collects the text output of a subagent.
type SubagentCollector struct {
	mu      sync.Mutex
	textBuf strings.Builder
	tools   []string
	errors  []string
}

func (c *SubagentCollector) OnText(text string) {
	c.mu.Lock()
	c.textBuf.WriteString(text)
	c.mu.Unlock()
}

func (c *SubagentCollector) OnToolStart(name, id string, input json.RawMessage) {
	c.mu.Lock()
	c.tools = append(c.tools, name)
	c.mu.Unlock()
}

func (c *SubagentCollector) OnToolOutput(name, id, output string, err error) {
	if err != nil {
		c.mu.Lock()
		c.errors = append(c.errors, fmt.Sprintf("%s: %s", name, err.Error()))
		c.mu.Unlock()
	}
}

func (c *SubagentCollector) OnDone() {}
func (c *SubagentCollector) OnError(err error) {
	c.mu.Lock()
	c.errors = append(c.errors, err.Error())
	c.mu.Unlock()
}
func (c *SubagentCollector) OnTokenUsage(_ session.SessionUsage) {}
func (c *SubagentCollector) OnCompaction(_ string)               {}
// Text returns the collected text output.
func (c *SubagentCollector) Text() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.textBuf.String()
}
