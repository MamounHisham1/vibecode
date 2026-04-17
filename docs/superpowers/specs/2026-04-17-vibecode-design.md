# Vibe Code — Design Spec

## Overview

Vibe Code is an open-source CLI coding agent, similar to Claude Code. It provides full agentic capabilities — reading, writing, understanding, and executing code — powered by multiple LLM providers.

## Core Principles

- Single binary, zero runtime dependencies (except `git` on PATH)
- Multi-provider: Anthropic, OpenAI, Ollama
- Custom agent loop (no framework dependency)
- Modern, minimal terminal UI
- Token-efficient by default
- Open source, community-driven

## Architecture

```
┌─────────────────────────────────────┐
│           CLI Entry Point            │
│        (cobra CLI framework)         │
└──────────────┬──────────────────────┘
               │
┌──────────────▼──────────────────────┐
│          Agent Loop                  │
│  ┌─────────────────────────────┐    │
│  │ 1. Build prompt (system +   │    │
│  │    tools + conversation)     │    │
│  │ 2. Call LLM provider        │    │
│  │ 3. Parse response           │    │
│  │ 4. If tool_call → execute   │    │
│  │ 5. Append result, goto 2    │    │
│  │ 6. If text → display, done  │    │
│  └─────────────────────────────┘    │
└──────────────┬──────────────────────┘
               │
    ┌──────────┼──────────┐
    ▼          ▼          ▼
┌───────┐ ┌───────┐ ┌───────────┐
│ LLM   │ │ Tool  │ │  Renderer │
│ Layer │ │ Layer │ │  (TUI)    │
└───┬───┘ └───┬───┘ └───────────┘
    │         │
    ▼         ▼
┌───────┐ ┌───────────┐
│Provider│ │  Tools    │
│Adapters│ │(Read,Edit,│
│        │ │Grep,Shell,│
│Claude  │ │Git,...)   │
│OpenAI  │ └───────────┘
│Ollama  │
└────────┘
```

Five core packages in `internal/`:

1. **`agent`** — Agentic loop: manages conversation history, orchestrates LLM calls ↔ tool execution
2. **`provider`** — Provider interface + adapters (Anthropic, OpenAI, Ollama). Each adapter normalizes to a common `Message`/`ToolCall`/`ToolResult` shape
3. **`tool`** — Tool registry + built-in tools. Each tool implements a `Tool` interface
4. **`tui`** — Rich terminal rendering: streaming markdown, syntax highlighting, spinners, tool execution indicators
5. **`config`** — Config loading, defaults, precedence (env vars > CLI flags > config file > defaults)

## Provider Abstraction

```go
type Provider interface {
    Stream(ctx context.Context, req Request) (<-chan Event, error)
}

type Request struct {
    System   string
    Messages []Message
    Tools    []ToolDef
}

type Message struct {
    Role       string
    Content    []ContentBlock
}

type ContentBlock struct {
    Type       string          // "text", "tool_use", "tool_result"
    Text       string
    ToolCallID string
    ToolName   string
    Input      json.RawMessage
    Result     json.RawMessage
    IsError    bool
}

type ToolDef struct {
    Name        string
    Description string
    Parameters  json.RawMessage  // JSON Schema
}

type Event interface {
    isEvent()
}

type TextEvent struct { Text string }
type ToolCallEvent struct { ID, Name string; Input json.RawMessage }
type DoneEvent struct{}
type ErrorEvent struct { Err error }
```

**Key decisions:**

- Streaming-first — all providers stream tokens via channels
- ContentBlock model — borrows Anthropic's content block pattern. OpenAI/Ollama adapters translate internally
- JSON Schema for tool parameters — adapters convert to provider-specific format
- Config per provider — API keys, base URLs, model names in `~/.vibecode/config.json`

**MVP adapters:**

1. **Anthropic** — native tool_use protocol, SSE streaming
2. **OpenAI** — function_calling protocol, SSE streaming
3. **Ollama** — local models, `/api/chat` endpoint, OpenAI-compatible tool format

## Tool System

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() json.RawMessage
    Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error)
}

type Registry struct {
    tools map[string]Tool
}

func (r *Registry) Register(tool Tool)
func (r *Registry) All() []Tool
func (r *Registry) Execute(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error)
```

**MVP built-in tools (7):**

| Tool | Purpose |
|------|---------|
| `read_file` | Read file contents with line numbers, offset/limit |
| `write_file` | Create or overwrite a file |
| `edit_file` | Search-and-replace within a file (exact string match) |
| `glob` | Find files by pattern |
| `grep` | Search file contents (regex) |
| `shell` | Execute bash commands with timeout |
| `git` | Git operations (status, diff, log, commit, branch) |

**Key decisions:**

- `edit_file` uses exact string match (not unified diffs) — simpler and more reliable for LLMs
- Shell tool has configurable allowlist/denylist. Default blocks destructive commands
- All tools operate relative to project root (detected via `.git` or `--project` flag)
- Git tool delegates to `git` binary via shell execution

## Agent Loop

```
User input → append to history → select tools → stream LLM response
    │
    ├── text response → display → done
    │
    └── tool calls → execute (parallel if independent)
            → append results to history
            → compact history if over token limit
            → loop back to stream LLM response
```

**Key behaviors:**

- **Max iterations** — configurable cap (default 50)
- **Parallel tool execution** — independent tool calls run concurrently
- **Streaming display** — text tokens render as they arrive
- **Context compaction** — summarize older turns when history exceeds ~80% of context window
- **Permission model** — read tools auto-approve, write tools ask for confirmation. `--auto-approve` flag or per-tool config to relax
- **Cancellation** — Ctrl+C cancels current LLM call or tool execution context-aware

## Token Efficiency

- **Compact tool descriptions** — 1-2 sentences max, parameter names are self-documenting
- **Minimal JSON Schemas** — required fields only, no verbose metadata
- **Lazy tool loading** — send only relevant tools per turn (3-4 typical, not all 7)
- **Compact tool results** — output only, no re-explanation
- **History compaction** — truncate/summarize older turns
- **Estimated budget** — ~4000-7000 tokens per turn

## Terminal UI

Using **bubbletea** + **lipgloss** + **glamour**.

**Layout:**

```
 vibe code · sonnet · ~/projects/myapp                          4.2k tokens

   fix the login bug

 ◐ reading src/auth/login.go
   142 lines · 2.1ms

 ◐ running tests...
   3 passed · 142ms

   I found the issue. The session token wasn't being validated
   on refresh.

   Fixed in src/auth/login.go:47:

   -  token := ctx.Get("token")
   +  token := validateRefresh(ctx.Get("token"))

 ────────────────────────────────────────────────────────────
 > _
```

**Modern design principles:**

- Dark-mode-first, muted color palette with one accent color
- Thin lines + whitespace instead of heavy ASCII borders
- Braille dot spinners (⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏)
- Indented tool output, visually nested
- Bold for headings, dim for metadata, bright for code
- No alt-screen — runs inline in terminal, copy-paste friendly
- Inline permission prompts with dim hint text

**Components:**

1. Status bar — model, provider, working directory, token count
2. Conversation area — scrollable, markdown with syntax highlighting
3. Tool activity — spinner while executing, ✓/✗ with summary after
4. Input area — multi-line (Shift+Enter for newline, Enter to submit)
5. Permission prompts — inline y/n

## Project Structure

```
vibecode/
├── cmd/
│   └── vibecode/
│       └── main.go
├── internal/
│   ├── agent/
│   │   ├── agent.go
│   │   ├── compact.go
│   │   └── permission.go
│   ├── provider/
│   │   ├── provider.go
│   │   ├── anthropic.go
│   │   ├── openai.go
│   │   └── ollama.go
│   ├── tool/
│   │   ├── registry.go
│   │   ├── read.go
│   │   ├── write.go
│   │   ├── edit.go
│   │   ├── glob.go
│   │   ├── grep.go
│   │   ├── shell.go
│   │   └── git.go
│   └── tui/
│       ├── tui.go
│       ├── renderer.go
│       └── theme.go
├── config/
│   └── config.go
├── go.mod
├── go.sum
└── Makefile
```

## Configuration

**File:** `~/.vibecode/config.json`

```json
{
  "provider": "anthropic",
  "model": "claude-sonnet-4-6",
  "api_keys": {
    "anthropic": "",
    "openai": ""
  },
  "auto_approve": ["read_file", "glob", "grep"],
  "max_iterations": 50,
  "theme": "default"
}
```

**CLI commands:**

```
vibecode                          # start interactive session
vibecode "fix the login bug"      # one-shot mode
vibecode --provider openai        # override provider
vibecode --model gpt-4.1          # override model
vibecode config                   # edit config
vibecode config set provider ollama
```

**Precedence:** env vars > CLI flags > config file > defaults

**Env vars:** `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `OLLAMA_BASE_URL`, `HARNESS_PROVIDER` (legacy compat)

## Key Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/charmbracelet/bubbletea` | TUI event loop |
| `github.com/charmbracelet/lipgloss` | Terminal styling |
| `github.com/charmbracelet/glamour` | Markdown rendering |
| `github.com/charmbracelet/bubbles` | Spinner, textinput components |

## Distribution

- `go install github.com/user/vibecode/cmd/vibecode@latest`
- Single binary, zero runtime deps except `git`
- Makefile targets: `build`, `install`, `test`
