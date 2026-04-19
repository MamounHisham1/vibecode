# Claude Code — Full Architecture Reference

> Complete reverse-engineered documentation of Claude Code's internals.
> Use this as the blueprint for building VibeCode's features.

---

## Table of Contents

1. [Tool System](#1-tool-system)
2. [Commands & Skills System](#2-commands--skills-system)
3. [Tasks & Agent System](#3-tasks--agent-system)
4. [Permissions System](#4-permissions-system)
5. [Hooks System](#5-hooks-system)
6. [App State Management](#6-app-state-management)
7. [MCP (Model Context Protocol)](#7-mcp-model-context-protocol)
8. [Configuration System](#8-configuration-system)
9. [Sessions](#9-sessions)
10. [TUI (Terminal UI)](#10-tui-terminal-ui)
11. [Plugins System](#11-plugins-system)
12. [System Prompt Construction](#12-system-prompt-construction)
13. [Query Engine](#13-query-engine)
14. [CLI Entrypoints](#14-cli-entrypoints)
15. [Remote / Bridge System](#15-remote--bridge-system)
16. [Cost Tracking](#16-cost-tracking)
17. [Other Features](#17-other-features)
18. [Feature Flags](#18-feature-flags)

---

## 1. Tool System

### Tool Interface

Every tool implements a rich interface with these properties:

```typescript
interface Tool {
  // Identity
  name: string;                    // unique identifier
  aliases: string[];               // backward-compatible names
  searchHint: string;              // 3-10 word keyword hint for deferred search

  // Core
  call(args, context, canUseTool, parentMessage, onProgress): Promise<ToolResult>;
  inputSchema: ZodSchema;          // input validation schema
  inputJSONSchema: object;         // JSON Schema for MCP

  // Permissions
  checkPermissions(input, context): PermissionResult;  // allow/ask/deny
  validateInput(input): ValidationResult;              // pre-permission validation

  // UI Rendering (React/Ink components)
  renderToolUseMessage: Component;
  renderToolResultMessage: Component;
  renderToolUseProgressMessage: Component;   // live progress during execution
  renderToolUseRejectedMessage: Component;
  renderToolUseErrorMessage: Component;
  renderGroupedToolUse: Component;           // grouped parallel tool rendering

  // Behavioral Metadata
  isConcurrencySafe: boolean;      // can run in parallel with others
  isReadOnly: boolean;             // doesn't modify filesystem
  isDestructive: boolean;          // potentially dangerous
  interruptBehavior: 'cancel' | 'block';  // behavior when user sends new msg
  shouldDefer: boolean;            // lazy-load via ToolSearch
  alwaysLoad: boolean;             // always include in prompt

  // Display
  description(input, options): string;
  prompt(options): string;         // model-facing prompt string
  getToolUseSummary(input): string;
  getActivityDescription(input): string;
  userFacingName: string;

  // MCP
  mcpInfo?: { serverName: string; toolName: string };

  // Other
  maxResultSizeChars: number;      // disk persistence threshold for large results
  strict: boolean;                 // strict tool instruction adherence
  preparePermissionMatcher(input): Rule;  // hook pattern matching
}
```

### Tool Registry

`getAllBaseTools()` returns all built-in tools. `getTools(permissionContext)` filters by:
1. Simple mode (`CLAUDE_CODE_SIMPLE`) → only Bash/Read/Edit
2. Deny rules from permission context
3. `isEnabled()` checks

`assembleToolPool(permissionContext, mcpTools)` combines built-in + MCP tools with deduplication (built-ins take precedence), sorted by name for prompt-cache stability.

### Tool Use Context

Every tool execution receives a rich context object:
- `options.commands`, `options.tools`, `options.thinkingConfig`, `options.mcpClients`
- `abortController` for cancellation
- `readFileState` — LRU cache for file contents
- `getAppState()` / `setAppState()` — state management
- `setToolJSX` — custom UI during tool execution
- `addNotification` — OS/system notifications
- `sendOSNotification` — terminal bell/notifications
- `agentId` / `agentType` — subagent context
- `requestPrompt` — interactive user prompts
- `messages` — full conversation history
- `toolDecisions` — cached permission decisions
- `contentReplacementState` — tool result budget management

### Complete Tool List

| Tool | Description |
|------|-------------|
| AgentTool | Spawns subagents (fork, worktree, background) |
| BashTool | Shell command execution with security |
| FileReadTool | Read file contents with line numbers |
| FileEditTool | Exact string replacement editing |
| FileWriteTool | Write full file contents |
| GlobTool | File pattern matching |
| GrepTool | Content search (ripgrep) |
| NotebookEditTool | Jupyter notebook editing |
| WebFetchTool | HTTP GET with HTML stripping |
| WebSearchTool | Web search via API |
| SkillTool | Invoke skills/commands |
| MCPTool | Proxy for MCP server tools |
| TaskCreateTool | Create background tasks |
| TaskGetTool | Get task details |
| TaskUpdateTool | Update task status |
| TaskListTool | List all tasks |
| TaskStopTool | Stop running tasks |
| TaskOutputTool | Get task output |
| TodoWriteTool | Todo list management |
| ConfigTool | Configuration management |
| EnterPlanModeTool | Enter planning mode |
| ExitPlanModeTool | Exit planning mode |
| EnterWorktreeTool | Create git worktree |
| ExitWorktreeTool | Exit git worktree |
| AskUserQuestionTool | Interactive user questions |
| SendMessageTool | Send message to agent/team |
| TeamCreateTool | Create agent teams |
| TeamDeleteTool | Delete agent teams |
| BriefTool | Toggle brief mode |
| LSPTool | Language server protocol |
| ListMcpResourcesTool | List MCP resources |
| ReadMcpResourceTool | Read MCP resources |
| ToolSearchTool | Deferred tool loading |
| ScheduleCronTool | Schedule recurring tasks |
| CronDeleteTool | Delete scheduled tasks |
| CronListTool | List scheduled tasks |

### Key Design Patterns

- **Deferred Loading**: Tools with `shouldDefer=true` aren't sent to the model initially. `ToolSearchTool` finds and loads them on demand, reducing prompt size.
- **Parallel Execution**: Tools with `isConcurrencySafe=true` run concurrently via `sync.WaitGroup`-style patterns.
- **Permission Flow**: check → allow/deny/ask → interactive prompt if needed.
- **Result Budgeting**: Large tool results get truncated or persisted to disk with lazy loading.

---

## 2. Commands & Skills System

### Command Types

Three distinct command types:

1. **PromptCommand** (`type: 'prompt'`)
   - Expands into text sent to the model
   - `getPromptForCommand(args, context)` returns `ContentBlockParam[]`
   - Supports: `allowedTools`, `model`, `effort`, `context` (inline/fork), `agent`, `hooks`, `paths`
   - This is what "skills" are

2. **LocalCommand** (`type: 'local'`)
   - Runs locally, returns text output
   - Lazy-loaded via `load()` → `{ call(args, context) }`
   - Examples: `/help`, `/clear`, `/compact`

3. **LocalJSXCommand** (`type: 'local-jsx'`)
   - Renders Ink (React) UI components
   - Lazy-loaded via `load()` → `{ call(onDone, context, args) }`
   - Examples: `/config`, `/review`

### Skill Directory Structure

Skills are directories containing `SKILL.md` with YAML frontmatter:

```
.claude/skills/
  my-skill/
    SKILL.md
    [reference-files...]
```

### SKILL.md Frontmatter Schema

```yaml
---
name: skill-name
description: One-line description
when_to_use: Trigger description
allowed-tools: Bash(git*), Read, Edit  # tool restrictions
argument-hint: "<description>"          # shown in UI
arguments:                              # structured arguments
  - name: arg1
    description: Description
    required: true
model: inherit | claude-opus-4-7        # model override
effort: low | medium | high             # effort level
context: inline | fork                  # execution context
agent: agent-type                       # use specific agent
user-invocable: true                    # can user invoke it
disable-model-invocation: false         # can model invoke it
paths:                                  # conditional activation
  - "src/**/*.ts"
hooks:                                  # lifecycle hooks
  pre: "command"
shell: bash                             # shell for commands
version: "1.0"
---

Skill content in markdown...
```

### Skill Loading Priority

1. **Managed skills** — `managed-settings.json/.claude/skills/` (policy level)
2. **User skills** — `~/.claude/skills/` (user level)
3. **Project skills** — `.claude/skills/` walked up from CWD to home
4. **Additional dirs** — `--add-dir` CLI paths
5. **Legacy commands** — `.claude/commands/` (backward compat)

### Dynamic Skill Discovery

- When files are touched during a session, walks from file's parent up to CWD looking for `.claude/skills/` directories
- Skills in deeper directories take precedence
- Gitignored directories are skipped
- Conditional skills (with `paths` frontmatter) are stored but not activated until a matching file is touched

### Bundled Skills

Registered programmatically at startup. Can include reference files extracted to disk on first invocation with symlink/traversal protection (`O_NOFOLLOW|O_EXCL`).

### MCP Skills

MCP servers can provide skills. Remote and untrusted — shell commands within their markdown body are never executed.

### Skill Tool

The `SkillTool` allows the model to invoke any prompt-type command. Fetches commands from AppState (including MCP skills) and runs them, supporting both inline and fork execution contexts.

---

## 3. Tasks & Agent System

### Task Types

| Type | Description |
|------|-------------|
| `local_bash` | Shell command execution |
| `local_agent` | In-process subagent (Agent tool) |
| `remote_agent` | Agent in remote CCR session |
| `in_process_teammate` | Teammate via TeamCreate |
| `local_workflow` | Workflow script execution |
| `monitor_mcp` | MCP monitoring |
| `dream` | Background processing |

### Task Lifecycle

```
pending → running → completed | failed | killed
```

Task IDs: prefix + 8 random bytes encoded in base-36 (e.g., `b` for bash, `a` for agent).

### Agent Tool Features

- **Fork subagents**: share parent's prompt cache for efficiency
- **Auto-background**: agents auto-background after 120 seconds
- **Worktree isolation**: agents can work in git worktrees
- **Agent definitions**: loaded from `.claude/agents/` with `AGENT.md` files
- **Built-in agents**: `general-purpose`, `Explore`, `Plan`, etc.
- **MCP requirements**: agents can declare required MCP servers
- **Color management**: each agent gets a unique color in TUI
- **Team swarms**: multi-agent coordination via tmux

### Agent Definition (AGENT.md)

```yaml
---
name: agent-name
description: One-line description
model: model-name
tools: [list of allowed tools]
mcpServers: [list of required MCP servers]
---

Agent instructions...
```

### Team System

- `TeamCreateTool` / `TeamDeleteTool` / `SendMessageTool`
- Teammates run in separate tmux panes with own worktrees
- Leader coordinates via message passing (inbox system)
- Worker sandbox permission requests flow through leader

---

## 4. Permissions System

### Permission Modes

| Mode | Description |
|------|-------------|
| `default` | Standard — prompts for unknown operations |
| `plan` | Plan mode — read-only, no tool execution |
| `acceptEdits` | Auto-accept file edits |
| `bypassPermissions` | Skip all permission prompts |
| `dontAsk` | Never ask, auto-accept |
| `auto` | AI classifier decides |

### Permission Rules

Rules have a `toolName` and optional `ruleContent` (pattern like `Bash(git *)`):

```json
{
  "permissions": {
    "allow": ["Bash(git *)", "Read", "Glob", "Grep"],
    "deny": ["Bash(rm -rf *)"],
    "ask": ["Bash(*)"]
  }
}
```

### Rule Sources (Priority Order)

1. `policySettings` — enterprise/managed
2. `userSettings` — `~/.claude/settings.json`
3. `projectSettings` — `.claude/settings.json`
4. `localSettings` — `.claude/settings.local.json`
5. `flagSettings` — CLI flags
6. `cliArg` — direct CLI args
7. `command` — from running command
8. `session` — session-only rules

### Permission Flow

```
1. hasPermissionsToUseTool() → check rules → allow/deny/ask
2. If allow → execute immediately
3. If deny → log and stop
4. If ask:
   a. Coordinator workers await automated checks
   b. Swarm workers try classifier auto-approval
   c. Speculative bash classifier runs during 2s grace period
   d. Falls through to interactive permission dialog (y/n)
```

### Security Classifiers

- **Bash classifier**: AI-based classification of bash commands
- **YOLO classifier**: transcript-based auto-mode classifier
- **Auto-mode denial tracking**: falls back to prompting when denial limits exceeded

---

## 5. Hooks System

### Hook Events

| Event | When |
|-------|------|
| `PreToolUse` | Before tool execution |
| `PostToolUse` | After successful tool execution |
| `PostToolUseFailure` | After failed tool execution |
| `UserPromptSubmit` | When user submits a prompt |
| `SessionStart` | When session starts |
| `Setup` | During setup |
| `SubagentStart` | When subagent starts |
| `PermissionDenied` | When permission is denied |
| `Notification` | For notifications |
| `PermissionRequest` | When permission is requested |
| `Elicitation` | User question asked |
| `ElicitationResult` | User answered question |
| `CwdChanged` | Working directory changed |
| `FileChanged` | File changed on disk |
| `WorktreeCreate` | Worktree created |

### Hook Types

1. **`command`** — shell command hook
   ```json
   { "type": "command", "command": "echo 'tool used'", "shell": "bash", "timeout": 5000 }
   ```

2. **`prompt`** — LLM prompt evaluation hook
   ```json
   { "type": "prompt", "prompt": "Is this safe?", "model": "claude-haiku-4-5" }
   ```

3. **`http`** — HTTP POST hook
   ```json
   { "type": "http", "url": "https://...", "headers": {}, "allowedEnvVars": [] }
   ```

4. **`agent`** — agentic verifier hook (runs a sub-agent)
   ```json
   { "type": "agent", "prompt": "Verify this change is safe" }
   ```

### Hook Configuration

All hooks support:
- `if` — permission rule syntax to filter (e.g., `Bash(git *)`)
- `statusMessage` — custom spinner text during hook execution
- `once` — run once then auto-remove
- `timeout` — per-hook timeout
- `async` — non-blocking execution
- `asyncRewake` — re-wake after async completion

### Hook Results

```typescript
interface HookJSONOutput {
  continue: boolean;           // allow tool to proceed
  suppressOutput: boolean;     // hide tool output from model
  stopReason?: string;         // override stop reason
  decision?: 'approve' | 'block';  // permission decision
  reason?: string;             // explanation
  systemMessage?: string;      // inject into conversation
  hookSpecificOutput?: any;    // event-specific fields
}
```

### Session Hooks

Temporary, in-memory hooks:
- Command hooks via `addSessionHook()` with matchers
- Function hooks via `addFunctionHook()` with timeout and error message
- Per-session, keyed by session ID
- Uses Map mutation for O(1) performance

---

## 6. App State Management

### State Store

Central state via `useSyncExternalStore` (React-like pattern):

```typescript
interface AppState {
  // Settings & UI
  settings: SettingsJson;
  verbose: boolean;
  mainLoopModel: string;
  statusLineText: string;
  expandedView: boolean;
  isBriefOnly: boolean;
  fastMode: boolean;
  effortValue: string;

  // Tasks
  tasks: Record<string, TaskState>;
  agentNameRegistry: Map<string, AgentId>;
  foregroundedTaskId: string;
  viewingAgentTaskId: string;

  // MCP
  mcp: {
    clients: Map<string, MCPClient>;
    tools: MCPTool[];
    commands: MCPCommand[];
    resources: MCPResource[];
  };

  // Plugins
  plugins: {
    enabled: Plugin[];
    disabled: Plugin[];
    commands: PluginCommand[];
    errors: PluginError[];
    installationStatus: Map<string, Status>;
  };

  // Permissions
  toolPermissionContext: PermissionContext;

  // Remote/Bridge
  remoteSessionUrl: string;
  remoteConnectionStatus: string;

  // Speculation
  speculation: { state: 'idle' | 'active' };

  // Other
  thinkingEnabled: boolean;
  promptSuggestionEnabled: boolean;
  todos: Todo[];
  inbox: Message[];
  notifications: Notification[];
  sessionHooks: Map<string, SessionStore>;
  fileHistory: Map<string, string>;
  agentDefinitions: Map<string, AgentDefinition>;
  initialMessage: string;
}
```

### State Hooks

- `useAppState(selector)` — subscribes to a slice, re-renders only on change
- `useSetAppState()` — stable updater reference
- `useAppStateStore()` — direct store access

---

## 7. MCP (Model Context Protocol)

### Transport Types

| Transport | Description |
|-----------|-------------|
| `stdio` | Standard input/output |
| `sse` | Server-Sent Events |
| `sse-ide` | SSE via IDE |
| `http` | HTTP POST |
| `ws` | WebSocket |
| `sdk` | In-process SDK |
| `claudeai-proxy` | Claude.ai proxy |

### Connection States

```
pending → connected | failed | needs-auth | disabled
```

### Config Scopes

| Scope | Source |
|-------|--------|
| `local` | `.claude/settings.local.json` |
| `user` | `~/.claude/settings.json` |
| `project` | `.claude/settings.json` |
| `dynamic` | Runtime |
| `enterprise` | Managed settings |
| `claudeai` | Claude.ai |

### MCP Features

- OAuth support per server
- Cross-App Access (XAA/SEP-990)
- Enterprise allowlist/denylist
- Channel notifications (`claude/channel` capability)
- IDE integration transports
- SDK transport for in-process MCP servers
- Resource management
- Normalized tool name mapping (built-ins take precedence)

---

## 8. Configuration System

### Settings Schema

Loaded from multiple sources with merge semantics:

1. **Policy** — `managed-settings.json` (enterprise)
2. **User** — `~/.claude/settings.json`
3. **Project** — `.claude/settings.json`
4. **Local** — `.claude/settings.local.json`

### Key Configuration Categories

```typescript
interface Settings {
  // Core
  model: string;
  availableModels: string[];
  modelOverrides: Record<string, string>;
  apiKeyHelper: string;
  env: Record<string, string>;

  // Permissions
  permissions: {
    allow: string[];
    deny: string[];
    ask: string[];
    defaultMode: string;
    additionalDirectories: string[];
  };

  // Hooks
  hooks: HookSettings;
  disableAllHooks: boolean;
  allowedHttpHookUrls: string[];

  // MCP
  enableAllProjectMcpServers: boolean;
  enabledMcpjsonServers: string[];
  disabledMcpjsonServers: string[];

  // Plugins
  enabledPlugins: string[];
  pluginConfigs: Record<string, any>;

  // Agent/Model
  agent: string;
  advisorModel: string;
  alwaysThinkingEnabled: boolean;
  effortLevel: string;
  fastMode: boolean;
  promptSuggestionEnabled: boolean;

  // Memory
  autoMemoryEnabled: boolean;
  autoMemoryDirectory: string;

  // Appearance
  outputStyle: string;
  language: string;
  prefersReducedMotion: boolean;
  spinnerTipsEnabled: boolean;
  spinnerVerbs: string[];

  // Remote
  remote: { defaultEnvironmentId: string };
  sshConfigs: SSHConfig[];

  // Attribution
  attribution: { commit: string; pr: string };
  includeCoAuthoredBy: boolean;
  includeGitInstructions: boolean;
}
```

---

## 9. Sessions

### Session History

Sessions are persisted via API with paginated events (100 per page):
- `fetchLatestEvents()` — newest page
- Older pages via `before_id` cursor

### Session Memory

Automatically maintains a markdown file with notes about the current conversation:
- Runs periodically in background using forked subagent
- Initialization threshold (number of tool calls before first extraction)
- Update threshold (minimum tool calls between updates)

### Context System

- **System context** (cached per conversation): git status, branch, recent commits
- **User context** (cached per conversation): CLAUDE.md content, current date
- System prompt injection for cache breaking
- `--bare` mode skips auto-discovery

### Session Resume

- `/resume` command to resume previous sessions
- Session state persisted across restarts
- Context window restoration with compaction

---

## 10. TUI (Terminal UI)

### Framework

Claude Code uses a customized fork of Ink (React renderer for terminals):

**Core Components:**
- `App.tsx`, `Box.tsx`, `Text.tsx` — basic layout
- `ScrollBox.tsx`, `Spacer.tsx`, `Button.tsx`
- `AlternateScreen.tsx` — fullscreen mode

**Layout Engine:**
- Yoga-based layout (`layout/engine.ts`, `layout/yoga.ts`)
- Node-based rendering (`layout/node.ts`)
- Geometry calculations (`layout/geometry.ts`)

**Terminal I/O:**
- ANSI/CSI/ESC/OSC parser and tokenizer
- Keypress parsing (`parse-keypress.ts`)
- Hit testing (`hit-test.ts`)
- Hyperlink support (`supports-hyperlinks.ts`)

**Text Processing:**
- Colorization (`colorize.ts`)
- Text wrapping (`wrap-text.ts`, `wrap-ansi.ts`)
- Search highlighting (`searchHighlight.ts`)
- String width measurement (`stringWidth.ts`)
- Bidirectional text (`bidi.ts`)
- Text selection (`selection.ts`)

### Application Components (200+)

- `App.tsx` — main shell
- `Messages.tsx`, `Message.tsx`, `MessageRow.tsx` — message display
- `PromptInput/` — text input with typeahead, vim mode
- `permissions/` — permission dialogs
- `mcp/` — MCP server dialogs
- `memory/` — memory management UI
- `skills/` — skill-related UI
- `tasks/` — task list and management
- `teams/` — team coordination UI
- `diff/` — structured diff display
- `design-system/` — shared design tokens
- `shell/` — shell-related UI
- `wizard/` — setup wizards

### Keybindings

Customizable keybinding system via `~/.claude/keybindings.json`. Supports:
- Single key bindings
- Chord bindings (multi-key sequences)
- Vim mode with full emulation

### Vim Mode

Full vim emulation: motions, operators, text objects, transitions.

### Input Features

- Multi-line input (shift+enter)
- Typeahead/autocomplete
- Command history with search
- Vim mode
- Paste handling
- Undo/redo

---

## 11. Plugins System

### Plugin Capabilities

Plugins provide:
- **Commands** — slash commands
- **Skills** — model-invocable prompts
- **Hooks** — lifecycle event handlers
- **MCP Servers** — tool providers
- **LSP Servers** — language intelligence
- **Output Styles** — response formatting

### Plugin Types

| Type | Description |
|------|-------------|
| `BuiltinPluginDefinition` | Ships with CLI |
| `LoadedPlugin` | From marketplace or local path |

### Plugin Features

- Marketplace system with allowlists/blocklists
- Git-based plugin loading
- Dependency resolution
- MCPB (MCP Binary) support for compiled bundles
- Plugin configuration persistence
- Installation status tracking

---

## 12. System Prompt Construction

### Priority Order

0. **Override** — loop mode, replaces everything
1. **Coordinator** — if coordinator mode
2. **Agent** — if agent definition set
3. **Custom** — `--system-prompt` flag
4. **Default** — built-in system prompt

`appendSystemPrompt` is always appended (except override mode).

### Context Injection

The system prompt includes:
- Working directory
- Git context (branch, status, recent commits)
- CLAUDE.md content (walked up from CWD to home)
- Date/time
- Tool descriptions and behavioral rules
- Permission context

---

## 13. Query Engine

### Main Loop

The query engine handles:
1. Message normalization and API call construction
2. Tool use processing (parallel and sequential)
3. Streaming response handling
4. Permission checking integration
5. Hook execution (PreToolUse, PostToolUse)
6. Compact/compaction management
7. Speculation (predictive completion)
8. Content replacement/budget management

### Streaming Protocol

- SSE (Server-Sent Events) for streaming
- Event types: `TextEvent`, `ToolCallEvent`, `ToolResultEvent`, `DoneEvent`, `ErrorEvent`
- Content blocks: text, tool_use, tool_result
- Accumulation of partial tool calls across delta events

---

## 14. CLI Entrypoints

### Main CLI

```bash
claude                          # Interactive mode
claude "fix the bug"            # One-shot mode
claude --provider anthropic     # Provider override
claude --model opus             # Model override
claude --system-prompt "..."    # Custom system prompt
claude --bare                   # Skip auto-discovery
claude --add-dir /path          # Additional directories
claude -p "prompt"              # Print mode (non-interactive)
```

### Special Entrypoints

- `--version` / `-v` — zero module loading fast path
- `--dump-system-prompt` — output prompt and exit
- `--claude-in-chrome-mcp` — Chrome extension MCP server
- `--chrome-native-host` — Chrome native messaging host
- `--computer-use-mcp` — computer use MCP server
- `--daemon-worker` — lean worker process
- `--mcp` — run as MCP server itself

### Agent SDK

Public SDK API for programmatic usage:
- `tool()` — define custom tools
- `createSdkMcpServer()` — create MCP server instances
- Session management (`listSessions`, `getSessionInfo`, `forkSession`)

---

## 15. Remote / Bridge System

### CCR (Claude Code Remote)

```
connecting → connected → reconnecting → disconnected
```

- Remote sessions via WebSocket with event streaming
- Always-on bridge state (`replBridgeEnabled/Connected/SessionActive`)
- Permission callbacks for bidirectional bridge checks
- Channel permission callbacks for Telegram/iMessage

### Teleport

- `teleportToRemote()` — move local session to remote
- `TeleportStash` — stashes local changes before teleporting
- Resume handling for remote sessions

---

## 16. Cost Tracking

- Token usage tracking per session
- Cost threshold dialogs
- Usage reporting (`/usage`, `/stats` commands)
- Rate limit monitoring

---

## 17. Other Features

### Worktree Mode

- `EnterWorktreeTool` / `ExitWorktreeTool` — git worktree isolation
- Symlink directories from main repo (e.g., `node_modules`)
- Sparse checkout for large monorepos

### Compact/Compaction

- Automatic and manual context compaction
- Context window-aware compaction (~80% threshold)
- Compact progress tracking
- Summary-based compaction (not just truncation)

### Plan Mode

- `EnterPlanModeTool` / `ExitPlanModeTool` — planning without execution
- Plan verification via background agent
- Clear context option on plan acceptance
- Plan file at `.claude/plans/`

### Voice Mode

- Hold-to-talk dictation
- Voice stream STT processing
- Voice keyterms extraction

### Speculation (Predictive Completion)

- Pre-generates completions while user is typing
- Speculation state tracking (idle/active)
- Time-saved metrics

### Sandbox

- Configurable sandbox settings
- Sandbox violation detection and display
- Worker sandbox permission requests

### Thinking Mode

- Configurable thinking/reasoning mode
- Thinking summaries in transcript view
- Extended thinking support

### Prompt Suggestions

- AI-generated prompt suggestions after responses
- Suggestion acceptance tracking
- Contextual suggestions based on conversation

### Effort Levels

| Level | Description |
|-------|-------------|
| `low` | Quick, brief responses |
| `medium` | Balanced |
| `high` | Thorough, detailed |
| `max` | Maximum effort |

### Fast Mode

- Toggle for faster responses (lower quality)
- Per-session opt-in

### OS Notifications

- Terminal bell on completion
- System notifications
- Sound alerts

### Diff Display

- Structured diff rendering
- Before/after file comparison
- Color-coded additions/deletions

---

## 18. Feature Flags

Key feature flags used for dead code elimination:

| Flag | Purpose |
|------|---------|
| `PROACTIVE` / `KAIROS` | Proactive/assistant mode |
| `BRIDGE_MODE` / `DAEMON` | Remote bridge |
| `COORDINATOR_MODE` | Multi-agent coordination |
| `WORKFLOW_SCRIPTS` | Workflow execution |
| `VOICE_MODE` | Voice dictation |
| `TRANSCRIPT_CLASSIFIER` | Auto-mode AI classifier |
| `WEB_BROWSER_TOOL` | Web browser tool |
| `MCP_SKILLS` | MCP-provided skills |
| `FORK_SUBAGENT` | Fork-based subagent optimization |
| `TOOL_SEARCH` | Deferred tool loading |
| `AGENT_TRIGGERS` | Scheduled tasks |
