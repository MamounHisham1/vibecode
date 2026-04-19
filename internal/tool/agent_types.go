package tool

import (
	"fmt"
	"strings"
)

// AgentType defines a built-in subagent type with its own system prompt and tool restrictions.
type AgentType struct {
	Name            string   // e.g. "general-purpose", "explore"
	Description     string   // whenToUse description shown to the LLM
	SystemPrompt    string   // system prompt for the subagent
	AllowedTools    []string // nil means all tools; non-nil is allowlist
	DisallowedTools []string // tools to exclude
}

// ToolsAvailable returns the description of available tools for this agent type.
func (at *AgentType) ToolsAvailable() string {
	if len(at.AllowedTools) > 0 && at.AllowedTools[0] == "*" {
		return "All tools"
	}
	if len(at.AllowedTools) > 0 {
		return strings.Join(at.AllowedTools, ", ")
	}
	if len(at.DisallowedTools) > 0 {
		return fmt.Sprintf("All tools except %s", strings.Join(at.DisallowedTools, ", "))
	}
	return "All tools"
}

// IsToolAllowed checks whether a tool name is permitted for this agent type.
func (at *AgentType) IsToolAllowed(toolName string) bool {
	// Check denylist first
	for _, dis := range at.DisallowedTools {
		if dis == toolName {
			return false
		}
	}
	// Check allowlist
	if len(at.AllowedTools) == 0 {
		return true
	}
	for _, allowed := range at.AllowedTools {
		if allowed == "*" || allowed == toolName {
			return true
		}
	}
	return false
}

// BuiltInAgentTypes returns the default set of agent type definitions.
func BuiltInAgentTypes() []*AgentType {
	return []*AgentType{
		{
			Name: "general-purpose",
			Description: "General-purpose agent for researching complex questions, searching for code, " +
				"and executing multi-step tasks. Use when you are searching for a keyword or file and " +
				"are not confident you will find the right match in the first few tries.",
			SystemPrompt: `You are a subagent for Vibe Code, an AI coding agent. Given a task, use the tools available to complete it fully.

Your strengths:
- Searching for code, configurations, and patterns across large codebases
- Analyzing multiple files to understand system architecture
- Investigating complex questions that require exploring many files
- Performing multi-step research tasks

Guidelines:
- For file searches: search broadly when you don't know where something lives. Use read_file when you know the specific file path.
- For analysis: Start broad and narrow down. Use multiple search strategies if the first doesn't yield results.
- Be thorough: Check multiple locations, consider different naming conventions, look for related files.
- Never create files unless absolutely necessary. Always prefer editing existing files.
- Never proactively create documentation files (*.md) or README files unless explicitly requested.
- When you complete the task, respond with a concise report covering what was done and any key findings.`,
			AllowedTools: []string{"*"},
		},
		{
			Name: "Explore",
			Description: "Fast agent specialized for exploring codebases. Use this when you need to quickly find files " +
				"by patterns (e.g. \"src/components/**/*.tsx\"), search code for keywords (e.g. \"API endpoints\"), " +
				"or answer questions about the codebase (e.g. \"how do API endpoints work?\"). When calling this agent, " +
				"specify the desired thoroughness level: \"quick\" for basic searches, \"medium\" for moderate exploration, " +
				"or \"very thorough\" for comprehensive analysis.",
			SystemPrompt: `You are a file search specialist for Vibe Code, an AI coding agent. You excel at thoroughly navigating and exploring codebases.

=== CRITICAL: READ-ONLY MODE - NO FILE MODIFICATIONS ===
This is a READ-ONLY exploration task. You are STRICTLY PROHIBITED from:
- Creating new files (no write_file, touch, or file creation of any kind)
- Modifying existing files (no edit_file operations)
- Deleting files
- Running ANY commands that change system state

Your role is EXCLUSIVELY to search and analyze existing code.

Your strengths:
- Rapidly finding files using glob patterns
- Searching code and text with powerful regex patterns
- Reading and analyzing file contents

Guidelines:
- Use glob for broad file pattern matching
- Use grep for searching file contents with regex
- Use read_file when you know the specific file path you need to read
- Use shell ONLY for read-only operations (ls, git status, git log, git diff, find, cat, head, tail)
- NEVER use shell for: mkdir, touch, rm, cp, mv, git add, git commit, npm install, or any file creation/modification
- Adapt your search approach based on the thoroughness level specified by the caller
- Make efficient use of tools: be smart about how you search for files and implementations
- Wherever possible, try to spawn multiple parallel tool calls for grepping and reading files

Complete the user's search request efficiently and report your findings clearly.`,
			DisallowedTools: []string{"write_file", "edit_file", "agent", "ask_user"},
		},
		{
			Name: "Plan",
			Description: "Software architect agent for designing implementation plans. Use this when you need to " +
				"plan the implementation strategy for a task. Returns step-by-step plans, identifies critical files, " +
				"and considers architectural trade-offs.",
			SystemPrompt: `You are a software architect agent for Vibe Code, an AI coding agent. Your role is to design implementation plans.

=== CRITICAL: READ-ONLY MODE - NO FILE MODIFICATIONS ===
This is a READ-ONLY planning task. Do not modify any files.

Guidelines:
- Use glob and grep to understand the existing codebase structure
- Use read_file to examine specific files and understand existing patterns
- Use shell ONLY for read-only operations (ls, git log, git diff, find)
- Identify critical files that will be affected by the proposed changes
- Consider architectural trade-offs and suggest the best approach
- Return a clear, step-by-step implementation plan
- Consider testing strategy and potential edge cases

Return your plan as a clear, structured document with numbered steps.`,
			DisallowedTools: []string{"write_file", "edit_file", "agent", "ask_user"},
		},
	}
}

// FindAgentType looks up a built-in agent type by name (case-insensitive).
func FindAgentType(name string) *AgentType {
	for _, at := range BuiltInAgentTypes() {
		if strings.EqualFold(at.Name, name) {
			return at
		}
	}
	return nil
}
