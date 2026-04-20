# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

VibeCode is an open-source CLI coding agent written in Go, designed as an alternative to Claude Code. It provides a terminal-based interactive agent with multi-provider LLM support, a bubbletea TUI, and a tool system. Module: `github.com/vibecode/vibecode`, Go 1.26.2.

## Build & Run

```bash
go build -o vibecode ./cmd/vibecode    # Build binary
./vibecode                              # Interactive TUI mode
./vibecode "fix the bug"                # One-shot mode
./vibecode config list                  # Show configuration
```

## Testing

```bash
go test ./...                           # Run all tests
go test ./internal/agent/...            # Run agent package tests
go test -run TestName ./internal/tool/  # Run single test
```

No Makefile or linter config exists. Use `gofmt -w .` and `go vet ./...` for formatting/linting.

## Architecture

```
cmd/vibecode/main.go          CLI entrypoint (cobra commands)
config/config.go               Three-layer settings: user < project < local
internal/
  agent/agent.go               Core agentic loop — sends to provider, processes streaming events, executes tools
  provider/                    Provider abstraction (interface: Stream(ctx, Request) -> <-chan Event)
    provider.go                  Interface & shared types
    anthropic.go                 Anthropic SSE (native Messages API)
    openai.go                    OpenAI-compatible (also DeepSeek, Kimi, Moonshot, Zhipu, Qwen)
    ollama.go                    Local Ollama (reuses OpenAI parser)
    models.go                    Model registry with context limits & per-token costs
  tool/                        Tool system (interface: Name/Description/Parameters/Execute)
    registry.go                  Registry with 50KB result truncation
    read.go, write.go, edit.go   File tools (edit uses diff)
    glob.go, grep.go             Search tools
    shell.go                     Bash execution
    git.go, webfetch.go, websearch.go, notebook.go
    ask.go, todo.go, plan.go     Interactive tools
    agent_tool.go                Subagent spawning (general-purpose, Explore, Plan types)
  tui/                         bubbletea TUI (Elm architecture)
    tui.go                       Main model — streaming text, tool expand/collapse, status bar
    input.go                     Multi-line input with cursor & autocomplete
    setup.go                     First-run provider/model/API key wizard
    theme.go                     Lipgloss styles & diff colors
    diff.go                      LCS-based diff rendering for edits
  hooks/                       Lifecycle hooks (PreToolUse, PostToolUse, etc.)
  skills/                      Loads .md skill files from .vibecode/skills/ with YAML frontmatter
  commands/                    Slash commands (/help, /clear, /model, /config)
  session/                     Token estimation, usage/cost tracking, LLM-based context compaction, overflow detection
```

### Key Patterns

- **Agent loop**: User message -> provider.Stream() -> process SSE events (text + tool calls) -> execute tools via goroutines -> loop until final text or max iterations. Mutex-protected history.
- **Provider interface**: All providers implement `Stream(ctx, Request) (<-chan Event, error)` returning channel of `TextEvent`, `ToolCallEvent`, `DoneEvent`, `ErrorEvent`.
- **Tool interface**: All tools implement `Name()`, `Description()`, `Parameters()`, `Execute(ctx, params)`. Registered in a central registry.
- **Config layering**: User settings (`~/.vibecode/settings.json`) < project (`.vibecode/settings.json`) < local (`.vibecode/settings.local.json`). Env vars override: `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `OLLAMA_BASE_URL`, `VIBECODE_PROVIDER`.
- **Context compaction**: When tokens reach ~80% of model context window, old messages are summarized by the LLM into a single system message.

### Supported Providers

Anthropic, OpenAI, DeepSeek, Kimi (Moonshot), Moonshot AI, Zhipu AI (GLM), Qwen (Alibaba), Ollama (local). OpenAI-compatible providers use the same `openai.go` with base URL override.

## Release

Tag-triggered GoReleaser pipeline (`.github/workflows/release.yml` + `.goreleaser.yml`). Targets: linux/darwin/windows on amd64/arm64. Releases to `mamounhisham1/vibecode`.

## Other Directories

- `remotion/` — Separate TypeScript/React project for a promotional video ad. Not part of the core tool.
- `docs/CLAUDE-CODE-ARCHITECTURE.md` — 975-line reverse-engineered reference of Claude Code internals, used as the blueprint for feature development.
- `TASKS.md` — Master feature gap tracker with priorities (URGENT/HIGH/MEDIUM/LOW).
