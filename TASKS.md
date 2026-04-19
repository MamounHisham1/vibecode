# VibeCode — Feature Gap & Task List

IMPORTANT: Please read the refrence at @../../sites/cluade-code, this is the ai agent i am trying to copy!

> Tasks to make VibeCode a competitive CLI coding agent, derived from Claude Code's architecture.
> Priority: URGENT → HIGH → MEDIUM → LOW
> Use `[x]` / `[ ]` to track progress.

---

## URGENT — Core Missing Features (Agent is Unusable Without These)

### Context/History Compaction
- [x] **Intelligent context compaction** — Summarize old messages when approaching context window limit (~80%), not just truncate
  - Use the LLM to generate a summary of the conversation so far
  - Replace old messages with a single system message containing the summary
  - Track approximate token count per message
  - File: new `internal/agent/compact.go`, updates to `internal/agent/agent.go`

- [x] **Token counting/tracking** — Track tokens per request and total session tokens. Show in status bar.
  - Parse token counts from provider responses (Anthropic: `usage` field, OpenAI: `usage` field)
  - Store on agent struct, display in TUI status bar
  - File: `internal/agent/agent.go`, `internal/provider/*.go`, `internal/tui/tui.go`

### Agent Loop Improvements
- [x] **Proper streaming tool call parsing** — Remove the fragile inline JSON heuristic fallback. All tool calls should come from the provider's native protocol
  - Clean up `handleTextEvent` inline JSON detection in `agent.go`
  - Ensure OpenAI function calling works reliably
  - File: `internal/agent/agent.go`

- [x] **Context cancellation propagation** — Ensure ctrl+C properly cancels in-flight HTTP requests, not just the goroutine
  - Wire `context.Context` through provider `Stream()` calls to `http.Request.WithContext()`
  - File: `internal/provider/anthropic.go`, `internal/provider/openai.go`

---

## HIGH — Essential Features for Real Usage

### Hooks System
- [x] **Hook framework** — Support lifecycle hooks that run shell commands at key points
  - Events: `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `UserPromptSubmit`, `SessionStart`
  - Hook types: `command` (shell), `prompt` (LLM eval), `http` (POST)
  - Configuration in settings files
  - Hook results: continue/block/suppress/inject
  - File: new `internal/hooks/hooks.go`, `internal/hooks/types.go`

- [x] **Pre-tool-use hooks** — Run hooks before tool execution, allow blocking
  - Filter by tool name pattern
  - Support `once` (auto-remove after first run)
  - Support `timeout` per hook
  - File: `internal/hooks/hooks.go`, integration in `internal/agent/agent.go`

- [x] **Post-tool-use hooks** — Run hooks after tool completion for logging/notification
  - Pass tool name, input summary, output summary
  - File: integration in `internal/agent/agent.go`

### Skills/Commands System
- [x] **Skill loading from directories** — Load skills from `.vibecode/skills/` with SKILL.md frontmatter
  - Parse YAML frontmatter: name, description, allowed-tools, arguments, model, effort
  - Walk from CWD up to home looking for skill directories
  - Register as prompt-type commands
  - File: new `internal/skills/skills.go`, `internal/skills/parser.go`

- [x] **Slash command system** — Parse and route slash commands in user input (e.g., `/help`, `/compact`, `/clear`)
  - Command types: `prompt` (sends to model), `local` (runs locally), `local-jsx` (TUI)
  - Built-in commands: `/help`, `/clear`, `/compact`, `/config`, `/model`, `/usage`
  - Command registry with name, aliases, description, handler
  - File: new `internal/commands/commands.go`, `internal/commands/registry.go`

- [x] **Built-in slash commands** — Implement essential commands:
  - `/help` — show available commands and keyboard shortcuts
  - `/clear` — clear conversation history
  - `/compact` — manually trigger context compaction
  - `/model <name>` — switch model mid-session
  - `/config` — show/edit configuration
  - `/usage` — show token usage and cost
  - File: new `internal/commands/builtins.go`

### Tool Improvements
- [x] **Web search tool** — Add `web_search` tool using a search API
  - Support multiple providers (SerpAPI, Google, Brave, etc.)
  - Return structured results with titles, URLs, snippets
  - File: new `internal/tool/websearch.go`

- [x] **Notebook edit tool** — Add `notebook_edit` for Jupyter notebook support
  - Read/write cells (code + markdown)
  - Support insert/delete/replace operations
  - File: new `internal/tool/notebook.go`

- [x] **Agent tool (subagents)** — Spawn subagents for parallel/delegated work
  - Support inline and background execution
  - Agent definitions from `.vibecode/agents/AGENT.md`
  - Built-in agents: general-purpose, explore, plan
  - File: new `internal/tool/agent.go`, `internal/agent/subagent.go`

- [x] **AskUserQuestion tool** — Proper interactive question UI in TUI
  - Multiple choice options
  - Free text input
  - Multi-select support
  - Render inline in conversation, not just stdin
  - File: rewrite `internal/tool/ask.go`, TUI integration

- [x] **Register todo_write tool** — Currently defined in `todo.go` but not registered in `main.go`
  - Wire up in `buildToolRegistry()`
  - Add TUI rendering for todo items
  - File: `cmd/vibecode/main.go`

- [x] **Tool result size limiting** — Truncate large tool results, optionally persist to disk
  - Add `maxResultSizeChars` to tool interface
  - Auto-truncate with "showing X of Y lines" message
  - File: `internal/tool/registry.go`

### TUI Improvements
- [x] **Tool output expand/collapse per-tool** — Currently ctrl+O toggles all. Add per-tool expand/collapse
  - Click or keyboard shortcut to expand/collapse individual tool results
  - Remember expanded state
  - File: `internal/tui/tui.go`

- [x] **Diff rendering for file edits** — Show before/after diff when files are edited
  - Color-coded additions/deletions
  - Unified diff format
  - File: new `internal/tui/diff.go`

- [x] **Syntax highlighting in file reads** — Highlight code based on file extension
  - Use Chroma or similar Go syntax highlighter
  - File: `internal/tool/read.go` output formatting, TUI rendering

- [ ] **Progress bars for long operations** — Show progress for web fetch, large file ops
  - Spinner with percentage/bytes
  - File: TUI tool progress rendering

- [ ] **Welcome screen improvements** — Add keyboard shortcuts reference, model info, version
  - Already partially implemented, needs polish
  - File: `internal/tui/tui.go`

### Configuration
- [x] **Config subcommand** — `vibecode config set provider ollama`, `vibecode config get model`
  - Get/set/list operations
  - Validate values
  - File: new cobra subcommand in `cmd/vibecode/main.go`

- [x] **Multiple settings sources** — Load from user, project, and local settings files with merge
  - `~/.vibecode/settings.json` (user)
  - `.vibecode/settings.json` (project)
  - `.vibecode/settings.local.json` (local, gitignored)
  - Priority: local > project > user > defaults
  - File: `config/config.go`

- [ ] **VIBECODE.md auto-discovery** — Walk from CWD up to home loading VIBECODE.md files
  - Already partially implemented in main.go
  - Also load `.vibecode/rules/*.md` files
  - File: `cmd/vibecode/main.go`

---

## MEDIUM — Important for Quality & Usability

### Plan Mode
- [ ] **Enter/exit plan mode** — Toggle between execution and planning modes
  - In plan mode: read-only tools only, no file modifications
  - Plan file persisted at `.vibecode/plans/`
  - Visual indicator in status bar
  - File: new `internal/commands/plan.go`, TUI state updates

- [ ] **Plan file management** — Create, list, load plan files
  - Markdown format with numbered steps
  - Track completion status per step
  - File: new `internal/plan/plan.go`

### Worktree Support
- [ ] **Enter/exit worktree** — Create isolated git worktrees for feature work
  - `enter_worktree` tool creates worktree + switches session
  - `exit_worktree` tool cleans up
  - Symlink shared directories (node_modules, etc.)
  - File: new `internal/tool/worktree.go`

### Background Tasks
- [ ] **Background task system** — Run long-running operations in background
  - Task types: bash, agent, workflow
  - Task states: pending → running → completed/failed/killed
  - Task output buffering and retrieval
  - File: new `internal/task/task.go`

- [ ] **Task management tools** — TaskCreate, TaskGet, TaskUpdate, TaskList, TaskStop, TaskOutput
  - Full CRUD for background tasks
  - TUI rendering for task list
  - File: multiple files in `internal/tool/`

### MCP (Model Context Protocol)
- [ ] **MCP client framework** — Connect to MCP servers via stdio/SSE/HTTP
  - Discover tools from MCP servers
  - Proxy tool calls to MCP servers
  - Connection lifecycle management
  - File: new `internal/mcp/client.go`, `internal/mcp/types.go`

- [ ] **MCP server configuration** — Configure MCP servers in settings
  - Per-server config: command, args, env, transport type
  - Auto-start on session start
  - Health monitoring and reconnection
  - File: `config/config.go` additions, new `internal/mcp/manager.go`

- [ ] **MCP tool proxying** — Register MCP server tools in the tool registry
  - Deduplication with built-in tools (built-ins take precedence)
  - Normalized naming
  - File: `internal/mcp/tools.go`, integration in `internal/agent/agent.go`

### Session Persistence
- [ ] **Session save/resume** — Persist conversation history to disk, resume across restarts
  - Session files in `~/.vibecode/sessions/`
  - `/resume` command to list and resume sessions
  - Context restoration with compaction
  - File: new `internal/session/session.go`

- [ ] **Session memory** — Auto-generate notes about the current conversation for future reference
  - Run periodically in background
  - Extract key decisions, files touched, problems solved
  - Store in memory markdown files
  - File: new `internal/memory/memory.go`

### Notification System
- [ ] **OS notifications** — Terminal bell and system notifications when long operations complete
  - Sound on completion
  - Desktop notification with summary
  - Configurable: always/on-background-only/never
  - File: new `internal/notify/notify.go`, TUI integration

- [ ] **Status line customization** — Allow custom status line text via hooks/config
  - Show custom info: git branch, project name, etc.
  - File: `internal/tui/tui.go`, config additions

### Provider Improvements
- [ ] **Google Gemini provider** — Add Gemini API adapter
  - Gemini function calling protocol
  - Streaming support
  - File: new `internal/provider/gemini.go`

- [ ] **Custom provider support** — Allow users to register custom providers via config
  - OpenAI-compatible endpoint configuration
  - Custom headers and auth
  - File: `internal/provider/provider.go`, config additions

- [ ] **Provider health checks** — Validate API keys and connectivity on startup
  - Show error in TUI if provider is unreachable
  - File: provider files, main.go

### Thinking/Reasoning Mode
- [ ] **Extended thinking support** — Show model's thinking process in TUI
  - Collapsible thinking blocks in transcript
  - Toggle visibility
  - File: provider response parsing, TUI rendering

- [ ] **Effort level control** — Allow adjusting response thoroughness
  - Levels: low, medium, high, max
  - Per-request override via `/effort` command
  - File: config, TUI, provider request params

### Output Quality
- [ ] **Brief mode** — Toggle shorter responses with `/brief`
  - Model instruction to be concise
  - Reduced tool output verbosity
  - File: TUI toggle, system prompt modification

- [ ] **Prompt suggestions** — After each response, suggest follow-up prompts
  - Generate 2-3 suggestions based on context
  - Quick-select with number keys
  - File: TUI rendering, agent post-processing

---

## LOW — Polish & Advanced Features

### Voice Mode
- [ ] **Voice input** — Hold-to-talk dictation using Whisper or similar STT
  - Push-to-talk key binding
  - Streaming audio capture and transcription
  - File: new `internal/voice/voice.go`

### Team/Swarm Mode
- [ ] **Multi-agent coordination** — Spawn multiple agents that collaborate
  - Leader/worker pattern via message passing
  - Separate worktrees per worker
  - Tmux integration for panes
  - File: new `internal/team/` package

### Remote/Bridge
- [ ] **Remote session support** — Run sessions on remote machines
  - WebSocket-based event streaming
  - Permission callbacks over bridge
  - File: new `internal/remote/` package

### Speculation
- [ ] **Predictive completion** — Pre-generate completions while user is typing
  - Speculative model call during input idle time
  - Show suggestion in input area
  - File: TUI input integration, background agent call

### Plugin System
- [ ] **Plugin framework** — Load external plugins that provide tools, commands, hooks
  - Plugin discovery from directories
  - Sandboxed execution
  - Dependency resolution
  - File: new `internal/plugin/` package

### Cost Tracking
- [ ] **Token cost estimation** — Track and display estimated API costs
  - Per-model pricing data
  - Session totals
  - Cost threshold warnings
  - File: new `internal/cost/cost.go`

### Testing
- [ ] **Agent loop tests** — Test the agentic loop with mock providers
  - Test tool call parsing, execution, error handling
  - Test parallel execution
  - Test max iterations
  - File: new `internal/agent/agent_test.go`

- [ ] **Provider adapter tests** — Test each provider with mock HTTP servers
  - Anthropic SSE parsing
  - OpenAI function calling
  - Ollama connectivity
  - File: new `internal/provider/*_test.go` files

- [ ] **Tool tests** — Test each tool's execute function
  - Happy path and error cases
  - Edge cases for each tool
  - File: new `internal/tool/*_test.go` files

- [ ] **TUI tests** — Test TUI rendering and state transitions
  - Message rendering
  - Tool output rendering
  - Permission prompts
  - File: new `internal/tui/tui_test.go`

### Distribution
- [ ] **Makefile** — Standard build, test, lint, install targets
  - `make build`, `make test`, `make lint`, `make install`
  - File: new `Makefile`

- [ ] **Homebrew formula** — Install via `brew install`
  - File: new `Formula/vibecode.rb` or tap

- [ ] **Shell completions** — Generate bash/zsh/fish completions
  - File: cobra completion generation in main.go

### Keybindings
- [ ] **Customizable keybindings** — Load from `~/.vibecode/keybindings.json`
  - Rebind keys
  - Chord bindings (multi-key sequences)
  - File: new `internal/input/keybindings.go`

- [ ] **Vim mode** — Full vim emulation for input
  - Motions, operators, text objects
  - Normal/insert/visual modes
  - File: new `internal/input/vim.go`

### Diff Display
- [ ] **Structured diff component** — Rich diff rendering in TUI
  - Side-by-side or unified view
  - Syntax highlighting in diffs
  - File: new `internal/tui/diff.go`

### Sandbox
- [ ] **Configurable sandbox** — Restrict tool execution environment
  - Allowed/blocked directories
  - Network restrictions
  - File size limits
  - File: new `internal/sandbox/sandbox.go`

### Scheduled Tasks
- [ ] **Cron-style scheduling** — Schedule recurring prompts/tasks
  - `ScheduleCron` / `CronDelete` / `CronList` tools
  - Persistent across sessions
  - File: new `internal/cron/cron.go`

### Memory System
- [ ] **Auto memory extraction** — Extract and persist key learnings from conversations
  - Memory types: user, feedback, project, reference
  - MEMORY.md index file
  - Auto-save on key events
  - File: new `internal/memory/` package

---

## Quick Wins (Can Be Done in < 1 Hour Each)

- [ ] Register `todo_write` tool in `main.go` (1 line change)
- [ ] Add token count display to status bar
- [ ] Add `/clear` command (clear conversation slice)
- [ ] Add `/help` command (print keyboard shortcuts + commands)
- [ ] Add `--version` flag to CLI
- [ ] Add `Makefile` with build/test targets
- [ ] Fix `.gitignore` to exclude pre-compiled binaries
- [ ] Remove `hermes-ad/` from repo
- [ ] Add `isReadOnly` / `isDestructive` / `isConcurrencySafe` to Tool interface
- [ ] Wire context.Context through provider HTTP requests
- [ ] Add tool execution duration to tool output rendering

---

## Architecture Decisions Needed

These require design decisions before implementation:

1. **State management**: How to manage app state in Go? (Options: global mutable state, channels, EventBus pattern)
2. **Plugin IPC**: How do external plugins communicate? (Options: stdio JSON-RPC, gRPC, HTTP)
3. **Hook execution**: Sync or async? In-process or subprocess?
4. **Session storage**: JSON files, SQLite, or custom binary format?
5. **MCP integration**: Full protocol support or minimal stdio-only?
6. **Compaction strategy**: LLM summarization (costly) or structural extraction (cheaper)?

---

## Progress

### 2026-04-19 — Intelligent Context Compaction
Implemented the first URGENT task: intelligent context compaction system modeled after Claude Code's architecture.

**What was built:**
- `internal/agent/compact.go` — Core compaction logic with:
  - Auto-compact trigger at 80% of context window (configurable threshold)
  - Rough token estimation using chars/4 heuristic (chars/2 for JSON)
  - LLM-based summarization of old messages with structured prompt
  - Circuit breaker (3 consecutive failures → stop retrying)
  - Compact boundary tracking and compact count
- `internal/agent/compact_test.go` — 8 unit tests covering token estimation, threshold checks, legacy compaction, etc.
- `internal/provider/provider.go` — New `UsageEvent{InputTokens, OutputTokens}` type
- `internal/provider/anthropic.go` — Captures usage from `message_start` and `message_delta` SSE events
- `internal/provider/openai.go` — Captures usage from final SSE chunk via `stream_options.include_usage`
- `internal/agent/agent.go` — Auto-compact check before each provider call, `TokenTracker` for session totals, `OnCompact`/`OnUsage` callback methods
- `internal/tui/tui.go` — Token count display in status bar (e.g. "42k tokens"), compaction notification message
- `config/config.go` — `ContextWindow` field for explicit override
- `cmd/vibecode/main.go` — Per-model context window defaults (Claude: 200K, GPT-4o: 128K, DeepSeek: 64K, etc.)

### 2026-04-19 — Diff Rendering for File Edits
Added color-coded diff display when edit_file modifies files, matching Claude Code's style.

**What was built:**
- `internal/tui/diff.go` — LCS-based line diff algorithm with styled rendering (green additions, red removals, line numbers, +/- markers)
- `internal/tui/diff_test.go` — 5 test cases covering diff computation, line numbers, rendering
- `internal/tui/theme.go` — 8 new diff color styles (DiffAdd, DiffRemove, DiffAddBg, DiffRemoveBg, DiffAddWord, DiffRemoveWord, DiffLineNum, DiffMarker)
- `internal/tui/tui.go` — Diff-aware collapsed view (shows first 8 diff lines inline), expanded view (renders styled diff without Dim wrapper), stores tool Input in toolEntry
- `internal/tool/edit.go` — Structured JSON output with path, old_string, new_string for diff computation

### 2026-04-19 — Token Counting/Tracking Tests
Verified and tested the existing token tracking implementation.

**What was tested:**
- `internal/agent/agent_test.go` — 6 new tests covering:
  - Token tracking from UsageEvent through agent struct
  - Token accumulation across multiple requests
  - Split usage events (Anthropic pattern: input in message_start, output in message_delta)
  - Text response streaming and callback delivery
  - Context cancellation propagation
  - Max iteration enforcement
- Mock provider, callback, and tool types for deterministic agent loop testing
- All existing token tracking infrastructure was already working: UsageEvent in providers, TokenTracker in agent, status bar display in TUI

### 2026-04-19 — Fix Token Count Always Showing 0k
Fixed the status bar always showing "0k tokens" even after receiving responses.

**Root cause:** The Zhipu/GLM provider (via OpenAI-compatible API) doesn't emit `UsageEvent`s because it doesn't support `stream_options.include_usage`. Many OpenAI-compatible providers omit usage data.

**What was fixed:**
- `internal/agent/agent.go` — Added `receivedUsage` flag tracking per iteration. When no UsageEvent is received from the provider, falls back to estimating tokens from request/response content using the existing `roughTokenEstimate` heuristic
- `internal/agent/agent.go` — New `estimateAndReportTokens()` method that estimates input tokens from system prompt + history and output tokens from response text + tool call inputs
- `internal/tui/tui.go` — New `formatTokenCount()` helper that shows "X tokens" for counts < 1000 instead of "0k tokens"
- `internal/agent/agent_test.go` — Added `TestAgentFallbackTokenEstimation` test verifying estimation works when provider sends no UsageEvent

### 2026-04-19 — Hook Framework + Streaming Tool Call Fix
Implemented the hooks framework and fixed Anthropic streaming tool call parsing.

**Hook framework:**
- `internal/hooks/types.go` — Event types (PreToolUse, PostToolUse, PostToolUseFailure, UserPromptSubmit, SessionStart), Hook types (command, http), Result types (continue, block)
- `internal/hooks/hooks.go` — Manager with Register, LoadFromConfig, Run. Command hooks use exit code 2 for blocking. HTTP hooks use 403. Glob matcher for tool name filtering. Once, timeout support.
- `internal/hooks/hooks_test.go` — 10 tests: glob matching, command continue/block, matcher filtering, once hooks, HTTP hooks, timeout, config loading
- `internal/agent/agent.go` — PreToolUse and PostToolUse/PostToolUseFailure hooks integrated into tool execution loop. Hooks can block execution or modify tool input.
- `config/config.go` — `Hooks` field for hook configuration
- `cmd/vibecode/main.go` — `buildHooks()` wired into both one-shot and interactive modes

**Streaming tool call parsing fix:**
- `internal/provider/anthropic.go` — Rewrote SSE parser to track content block type, accumulate input_json_delta into buffer, emit complete ToolCallEvent on content_block_stop
- `internal/agent/agent.go` — Removed fragile inline JSON heuristic (parseInlineToolCalls, stripInlineJSON, findJSONObject). Supports both Anthropic (deferred input) and OpenAI (all-in-one) patterns
- `internal/agent/agent_test.go` — Added TestAgentToolCallWithDeferredInput and TestAgentToolCallWithAllInOne tests

**Context cancellation:**
- Verified already wired: agent passes ctx → provider.Stream → http.NewRequestWithContext on all three providers

### 2026-04-19 — Skills, Commands, Web Search, Notebook, TodoWrite
Implemented multiple features in batch.

**Skills loading:**
- `internal/skills/parser.go` — YAML frontmatter parsing for skill files (name, description, allowed-tools, arguments, model, effort)
- `internal/skills/skills.go` — Store with directory walking (CWD up to home + ~/.vibecode/skills/). Skills appended to system prompt.
- 7 tests covering parsing, loading, filename-as-name, prompt building

**Slash command system:**
- `internal/commands/commands.go` — Registry with built-in commands: /help, /clear, /compact, /model, /config, /usage. Supports aliases and custom commands.
- Wired into TUI via `SetCommandHandler` — slash commands handled locally without sending to model
- 5 tests covering parsing, lookup, builtins, custom commands

**Web search tool:**
- `internal/tool/websearch.go` — web_search tool with Brave Search and SerpAPI providers. Returns structured results (title, URL, snippet). Configured via env vars.

**Notebook edit tool:**
- `internal/tool/notebook.go` — notebook_edit for .ipynb files. Read (cell or all), insert, delete, replace operations. Handles cell types.

**TodoWrite registered:**
- 1-line fix: registered the existing TodoWrite tool in buildToolRegistry()

