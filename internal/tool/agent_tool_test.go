package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestAgentTypeIsToolAllowed(t *testing.T) {
	tests := []struct {
		name    string
		at      *AgentType
		tool    string
		allowed bool
	}{
		{
			name:    "general-purpose allows all",
			at:      &AgentType{AllowedTools: []string{"*"}},
			tool:    "shell",
			allowed: true,
		},
		{
			name:    "explore blocks write_file",
			at:      &AgentType{DisallowedTools: []string{"write_file", "edit_file"}},
			tool:    "write_file",
			allowed: false,
		},
		{
			name:    "explore allows read_file",
			at:      &AgentType{DisallowedTools: []string{"write_file", "edit_file"}},
			tool:    "read_file",
			allowed: true,
		},
		{
			name:    "allowlist restricts to specific tools",
			at:      &AgentType{AllowedTools: []string{"read_file", "glob", "grep"}},
			tool:    "shell",
			allowed: false,
		},
		{
			name:    "allowlist permits listed tool",
			at:      &AgentType{AllowedTools: []string{"read_file", "glob", "grep"}},
			tool:    "glob",
			allowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.at.IsToolAllowed(tt.tool); got != tt.allowed {
				t.Errorf("IsToolAllowed(%q) = %v, want %v", tt.tool, got, tt.allowed)
			}
		})
	}
}

func TestFindAgentType(t *testing.T) {
	gp := FindAgentType("general-purpose")
	if gp == nil {
		t.Fatal("general-purpose agent type not found")
	}
	if gp.Name != "general-purpose" {
		t.Errorf("Name = %q, want general-purpose", gp.Name)
	}

	explore := FindAgentType("Explore")
	if explore == nil {
		t.Fatal("Explore agent type not found")
	}

	plan := FindAgentType("Plan")
	if plan == nil {
		t.Fatal("Plan agent type not found")
	}

	// Case insensitive
	explore2 := FindAgentType("explore")
	if explore2 == nil {
		t.Fatal("explore (lowercase) not found")
	}

	unknown := FindAgentType("nonexistent")
	if unknown != nil {
		t.Error("nonexistent agent should return nil")
	}
}

func TestAgentTypeToolsAvailable(t *testing.T) {
	gp := FindAgentType("general-purpose")
	if got := gp.ToolsAvailable(); got != "All tools" {
		t.Errorf("general-purpose ToolsAvailable() = %q, want All tools", got)
	}

	explore := FindAgentType("Explore")
	if got := explore.ToolsAvailable(); !strings.Contains(got, "except") {
		t.Errorf("Explore ToolsAvailable() should contain 'except', got %q", got)
	}
}

func TestAgentToolExecute(t *testing.T) {
	// Create a mock runner that echoes the prompt back
	runner := func(ctx context.Context, systemPrompt string, reg *Registry, prompt string) (string, error) {
		return "Mock result: " + prompt, nil
	}

	reg := NewRegistry()
	reg.Register(ReadFile{})
	agentTool := NewAgentTool(reg, runner, "/tmp")

	input := map[string]any{
		"description":   "test agent",
		"prompt":        "Find all Go files",
		"subagent_type": "general-purpose",
	}
	inputJSON, _ := json.Marshal(input)

	result, err := agentTool.Execute(context.Background(), inputJSON)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("Parse result: %v", err)
	}

	if resp["status"] != "completed" {
		t.Errorf("status = %v, want completed", resp["status"])
	}
	if resp["agent_type"] != "general-purpose" {
		t.Errorf("agent_type = %v, want general-purpose", resp["agent_type"])
	}
	if resp["result"] != "Mock result: Find all Go files" {
		t.Errorf("result = %v, want Mock result: Find all Go files", resp["result"])
	}
}

func TestAgentToolExplore(t *testing.T) {
	var receivedSystemPrompt string
	runner := func(ctx context.Context, systemPrompt string, reg *Registry, prompt string) (string, error) {
		receivedSystemPrompt = systemPrompt
		// Verify the registry only has read-only tools
		for _, t := range reg.All() {
			if t.Name() == "write_file" || t.Name() == "edit_file" {
				return "", nil // Should not reach here
			}
		}
		return "Found 5 files", nil
	}

	reg := NewRegistry()
	reg.Register(ReadFile{})
	reg.Register(WriteFile{})
	reg.Register(EditFile{})
	reg.Register(Glob{})
	reg.Register(Grep{})
	agentTool := NewAgentTool(reg, runner, "/tmp")

	input := map[string]any{
		"description":   "explore codebase",
		"prompt":        "Find all test files",
		"subagent_type": "Explore",
	}
	inputJSON, _ := json.Marshal(input)

	result, err := agentTool.Execute(context.Background(), inputJSON)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var resp map[string]any
	json.Unmarshal(result, &resp)

	if resp["agent_type"] != "Explore" {
		t.Errorf("agent_type = %v, want Explore", resp["agent_type"])
	}
	if !strings.Contains(receivedSystemPrompt, "READ-ONLY") {
		t.Error("Explore agent should have READ-ONLY system prompt")
	}
}

func TestAgentToolUnknownType(t *testing.T) {
	runner := func(ctx context.Context, systemPrompt string, reg *Registry, prompt string) (string, error) {
		return "", nil
	}

	reg := NewRegistry()
	agentTool := NewAgentTool(reg, runner, "/tmp")

	input := map[string]any{
		"description":   "bad type",
		"prompt":        "do something",
		"subagent_type": "nonexistent",
	}
	inputJSON, _ := json.Marshal(input)

	_, err := agentTool.Execute(context.Background(), inputJSON)
	if err == nil {
		t.Fatal("Expected error for unknown agent type")
	}
	if !strings.Contains(err.Error(), "unknown agent type") {
		t.Errorf("Error = %v, want 'unknown agent type'", err)
	}
}

func TestAgentToolMissingPrompt(t *testing.T) {
	runner := func(ctx context.Context, systemPrompt string, reg *Registry, prompt string) (string, error) {
		return "", nil
	}

	reg := NewRegistry()
	agentTool := NewAgentTool(reg, runner, "/tmp")

	input := map[string]any{
		"description": "no prompt",
	}
	inputJSON, _ := json.Marshal(input)

	_, err := agentTool.Execute(context.Background(), inputJSON)
	if err == nil {
		t.Fatal("Expected error for missing prompt")
	}
}

func TestSubagentCollector(t *testing.T) {
	c := &SubagentCollector{}
	c.OnText("Hello ")
	c.OnText("World")

	if got := c.Text(); got != "Hello World" {
		t.Errorf("Text() = %q, want Hello World", got)
	}
}
