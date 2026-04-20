# AGENTS.md

## Build & Run

```bash
go build -o vibecode ./cmd/vibecode    # Build binary
./vibecode                              # Interactive TUI mode
./vibecode "fix the bug"                # One-shot mode (prints to stdout, no TUI)
./vibecode config list                  # Show configuration
./vibecode config set provider ollama   # Change config values
```

No Makefile. No linter config. Format with `gofmt -w .`, vet with `go vet ./...`.

## Testing

```bash
go test ./...                           # All tests
go test ./internal/agent/...            # Single package
go test -run TestName ./internal/tool/  # Single test
```

No integration tests, no fixtures, no external services required.

## Architecture

Module: `github.com/vibecode/vibecode`, Go 1.26.2. Single binary, no plugins.

**Entrypoint**: `cmd/vibecode/main.go` — cobra CLI, wires everything together.

**Core loop** (`internal/agent/agent.go`): User message → `provider.Stream()` → process SSE events (text + tool calls) → execute tools via goroutines → loop until final text or max iterations. History is mutex-protected.

**Provider interface** (`internal/provider/provider.go`): All providers implement `Stream(ctx, Request) (<-chan Event, error)`. Returns a channel of `TextEvent`, `ToolCallEvent`, `UsageEvent`, `DoneEvent`, `ErrorEvent`.

**Tool interface** (`internal/tool/registry.go`): `Name()`, `Description()`, `Parameters()`, `Execute(ctx, input)`. Registered in a central `Registry`. Results truncated at 50KB.

**Config layering** (`config/config.go`): `~/.vibecode/config.json` base → `~/.vibecode/settings.json` → `.vibecode/settings.json` → `.vibecode/settings.local.json`. Env vars override: `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_BASE_URL`, `OPENAI_API_KEY`, `OLLAMA_BASE_URL`, `VIBECODE_PROVIDER`.

**Context compaction** (`internal/session/compaction.go`): Auto-triggers at ~80% of model context window. LLM summarizes old messages into a single system message. Has a circuit breaker (3 consecutive failures → stop retrying).

### Key directories

| Path | Purpose |
|------|---------|
| `cmd/vibecode/` | CLI entrypoint, provider wiring, system prompt construction |
| `config/` | Config loading, layering, settings merge |
| `internal/agent/` | Core agentic loop + compaction |
| `internal/provider/` | Provider adapters: `anthropic.go`, `openai.go`, `ollama.go`, `meta.go` |
| `internal/tool/` | All tools (read, write, edit, glob, grep, shell, git, web_fetch, ask_user, todo, plan, agent_tool, websearch, notebook) |
| `internal/tui/` | bubbletea TUI (Elm architecture), theme, diff rendering, input, setup wizard |
| `internal/hooks/` | Lifecycle hooks (PreToolUse, PostToolUse, etc.) |
| `internal/skills/` | Loads `.vibecode/skills/*.md` with YAML frontmatter |
| `internal/commands/` | Slash commands (/help, /clear, /model, /config, etc.) |
| `internal/session/` | Token estimation, usage tracking, context compaction |
| `internal/openrouter/` | OpenRouter API client for dynamic model registry |

### Provider details

- `anthropic.go`: Native Anthropic Messages API with SSE. Handles `input_json_delta` accumulation across chunks.
- `openai.go`: OpenAI-compatible (also DeepSeek, Kimi, Moonshot, Zhipu, Qwen, Meta, etc.). Distinguished by `apiType` from `ProviderMetaMap`.
- `ollama.go`: Local Ollama, reuses OpenAI parser. No API key needed.
- `models.go`: Model registry populated dynamically from OpenRouter. Stores context limits and per-token costs.
- `meta.go`: `ProviderMetaMap` — maps provider IDs to base URLs, API types, and endpoint variants. This is where new providers are added.

### Adding a new provider

1. Add entry to `ProviderMetaMap` in `internal/provider/meta.go` with `BaseURL` and `APIType` ("openai" or "anthropic").
2. If it's OpenAI-compatible, that's it — `openai.go` handles it via base URL override.
3. If it needs a custom protocol, create a new file implementing the `Provider` interface.

### Adding a new tool

1. Create a file in `internal/tool/` implementing the `Tool` interface.
2. Register it in `buildToolRegistry()` in `cmd/vibecode/main.go`.

### Adding a new slash command

1. Create a file in `internal/commands/` and register in `commands.go`.
2. Command types: `TypeLocal` (runs locally, returns text), `TypePrompt` (expands to text sent to model).

## Important patterns & gotchas

- **Two API protocols**: Anthropic uses `content_block_start/stop` with deferred `input_json_delta`. OpenAI sends complete tool calls in one event. The agent handles both.
- **Fallback token estimation**: Many OpenAI-compatible providers don't emit `UsageEvent`. The agent falls back to `roughTokenEstimate()` (chars/4 heuristic) when no usage is received.
- **Provider is swappable at runtime**: The TUI holds a `*provider.Provider` pointer. `/model` rebuilds the provider and swaps it via `SetProvider()`. Subagents use a closure-captured reference that picks up changes.
- **Agent created lazily**: In TUI mode, the agent is nil until the first message or until `/model` is used. Don't assume the agent exists.
- **Per-turn cancellation**: Each `a.Run()` gets its own child context so Ctrl+C cancels the current turn without killing subsequent turns.
- **Web search is opt-in**: Requires `VIBECODE_SEARCH_API_KEY` env var. Defaults to Brave Search.
- **VIBECODE.md auto-discovery**: Walks from CWD up to home loading `VIBECODE.md` and `.vibecode/rules/*.md` into the system prompt. Project instructions are appended, not prepended.
- **Skills**: Loaded from `.vibecode/skills/` with YAML frontmatter. Appended to system prompt after the main prompt.

## Non-core directories

- `remotion/` — Separate TypeScript/React project for a promotional video. Not part of the Go build.
- `docs/CLAUDE-CODE-ARCHITECTURE.md` — 975-line reverse-engineered reference of Claude Code internals. Used as the blueprint for feature development.
- `TASKS.md` — Feature gap tracker with priorities. The source of truth for what's implemented vs. planned.

## Release

Tag-triggered GoReleaser pipeline. Push a `v*` tag → `.github/workflows/release.yml` builds for linux/darwin/windows on amd64/arm64. Releases to `mamounhisham1/vibecode`.
