# Gi Extension Protocol Design

This document defines the target architecture for Gi packages, extensions,
trusted in-process TUI components, and package-provided out-of-process custom
TUI components. The goal is to support the official Pi extension surface and
popular Pi package behaviors while keeping the protocol open enough for a Rust
implementation such as `ri`. This is behavior compatibility, not package
artifact compatibility: Pi/npm packages are not valid Gi packages unless they
ship `gi.package.json` and speak the Gi protocol.

Status: design target. The current repository does not yet implement the full
`gi-coding-agent` runtime, package manager, extension runner, or RPC component
host.

Normative language follows RFC 2119 style: `MUST`, `MUST NOT`, `SHOULD`,
`SHOULD NOT`, and `MAY` are requirements only when written in uppercase.

The protocol is now split into implementation artifacts under `spec/`:

- [`spec/gi-extension-protocol.md`](spec/gi-extension-protocol.md) is the
  concise normative v1 entry point.
- [`spec/schemas/`](spec/schemas/) contains JSON Schemas for manifests, RPC
  envelopes, host actions, ViewTree nodes, and conformance reports.
- [`spec/registries/`](spec/registries/) contains stable names for profiles,
  capabilities, actions, events, errors, style tokens, nodes, and official
  package derivations.
- [`spec/examples/`](spec/examples/) contains JSONL transcripts for replay.
- [`spec/conformance/README.md`](spec/conformance/README.md) defines the
  conformance evidence model.

## Goals

- Keep Gi core small while allowing packages to add tools, commands, lifecycle
  hooks, providers, skills, prompts, themes, and rich TUI contributions.
- Support Pi-style extension capabilities without requiring TypeScript,
  Node.js, or in-process component loading for third-party packages.
- Make the package and extension standards language-neutral so Go, Rust,
  TypeScript, Python, and other runtimes can implement compatible SDKs.
- Preserve one terminal owner: Gi owns terminal I/O, focus, layout, rendering,
  keyboard parsing, and diff rendering.
- Give trusted built-in code a direct in-process component API while making
  third-party package UI default to out-of-process RPC plus ViewTree events.
- Provide enough conformance tests to make the protocol implementable by both
  Gi and a future Rust `ri`.
- Let community implementations vary in language, package source, renderer, and
  transport while proving compatibility against the same schemas and fixtures.

Non-goals for the base standard:

- Do not standardize Go `plugin`, Rust dynamic library ABI, or Node `jiti` as
  the portable extension mechanism.
- Do not let package-provided components write ANSI directly to the terminal.
- Do not require every Pi example to be implemented in core.
- Do not support `npm:` package sources or `package.json#gi` discovery. Gi uses
  protocol packages, not language-ecosystem package manager metadata.

## Layer Model

```text
Package
  - distribution, versioning, dependencies, resource manifest

Extension
  - runtime process or trusted in-process module
  - registers tools, commands, providers, hooks, resources, UI contributions

TUI contribution
  - header, footer, widget, overlay, editor, message renderer, tool renderer,
    status, autocomplete provider

Execution mode
  - trusted in-process component for core/official/SDK-embedded code
  - out-of-process RPC component for third-party packages by default
```

Packages can contain extensions, skills, prompts, and themes. Extensions can
exist outside packages through local project or user paths. Custom TUI
components are not a separate package resource; they are UI contributions
registered by extensions.

## Open Standard Model

The protocol is standard at the boundary, not in the host internals. Community
implementations can differ in their renderer, package installer, scheduler,
storage, language SDKs, and process sandbox as long as they implement a declared
profile and pass that profile's conformance suite.

The standard is made of these normative artifacts:

- JSON Schemas for package manifests, RPC envelopes, host actions, events,
  diagnostics, ViewTree nodes, and conformance reports.
- Golden JSONL transcripts for handshake, registration, lifecycle events,
  cancellation, crash recovery, and capability denial.
- Golden ViewTree fixtures and rendered terminal snapshots for TUI profiles.
- A capability registry with stable names and compatibility rules.
- A host action registry with stable request/response shapes.
- A conformance runner that emits a signed or reproducible report.

Implementations SHOULD expose a machine-readable conformance report:

```json
{
  "implementation": {
    "name": "ri",
    "version": "0.1.0",
    "language": "rust"
  },
  "profiles": [
    "gi-package-host@1",
    "gi-extension-host@1",
    "gi-viewtree-renderer@1"
  ],
  "features": {
    "transports": ["stdio-ndjson"],
    "packageSources": ["git", "local"],
    "viewNodes": ["text", "markdown", "box", "list", "input"]
  },
  "testResults": {
    "suite": "gi-conformance@1",
    "passed": 184,
    "failed": 0
  }
}
```

### Conformance Profiles

Profiles let small community hosts be compatible without implementing every Gi
feature. A host claims only the profiles it passes.

- `gi-package-host@1`: parses manifests, resolves package resources, applies
  precedence, records lock/update metadata, and filters resources.
- `gi-extension-host@1`: supervises out-of-process extensions, negotiates
  protocol versions, enforces capabilities, routes lifecycle events, and handles
  tools, commands, providers, resources, diagnostics, and shutdown.
- `gi-tui-host@1`: provides TUI slots, focus, keyboard events, editor bridge,
  status, dialogs, and autocomplete host actions.
- `gi-viewtree-renderer@1`: renders ViewTree nodes with deterministic layout,
  applies patches, and passes snapshot fixtures.
- `gi-inprocess-host@1`: supports trusted compiled-in extension registration and
  panic/dispose boundaries.
- `gi-extension-process@1`: describes a package-provided extension process that
  can handshake, register contributions, handle events, and shut down cleanly.
- `gi-sdk@1`: SDK-level compatibility. SDKs can provide ergonomic APIs, but
  must emit the same manifest, RPC, actions, and ViewTree messages.

`gi-coding-agent-compatible@1` is an aggregate claim requiring
`gi-package-host@1`, `gi-extension-host@1`, `gi-tui-host@1`,
`gi-viewtree-renderer@1`, and the Pi ecosystem fixture set.

### Extension Points

The protocol must be extensible without fragmenting compatibility:

- Experimental capabilities use `x-<vendor>.<name>` and MUST be ignored by
  hosts that do not grant them.
- Experimental ViewTree nodes use `x-<vendor>.<node>` and MUST include a
  standard `fallback` node.
- Experimental RPC methods use `x/<vendor>/<method>` and MUST fail with
  `unsupported_method` on unknown hosts.
- Stable additions require a schema, at least one fixture, and a conformance
  test before entering the public registry.
- Hosts MUST preserve unknown manifest metadata when rewriting lock files.

This gives room for community innovation while keeping the shared surface
testable.

## Package Manifest v1

Gi packages MUST declare resources in `gi.package.json`. `package.json#gi` and
`npm:` package sources are not part of the Gi protocol and MUST NOT be treated
as compatible package inputs. A JavaScript, Go, Rust, Python, or shell package
can be a Gi package only by shipping the same `gi.package.json` manifest and
speaking the same RPC/ViewTree protocol.

```json
{
  "name": "acme/plan-mode",
  "version": "1.0.0",
  "keywords": ["gi-package"],
  "gi": {
    "manifestVersion": 1,
    "engines": {
      "gi": ">=0.3.0",
      "gi-ext-rpc": "^1.0.0",
      "gi-viewtree": "^1.0.0"
    },
    "extensions": [
      {
        "id": "plan-mode",
        "entry": {
          "kind": "process",
          "command": ["./bin/plan-mode"],
          "transport": "stdio-ndjson",
          "protocol": "gi-ext-rpc@1"
        },
        "capabilities": [
          "session.read",
          "tools.set_active",
          "commands.register",
          "tui.widget",
          "tui.dialog",
          "tui.editor"
        ]
      }
    ],
    "skills": ["skills/**/SKILL.md"],
    "prompts": ["prompts/*.md"],
    "themes": ["themes/*.json"]
  }
}
```

Supported source types:

- git URL with optional ref
- local file or directory
- OCI/tarball source, optional for hosted registries

Hosts MUST NOT support `npm:` as a package source. Reusing npm as an artifact
store would make users expect Pi/npm packages to run directly, while Gi only
guarantees artifacts that implement this protocol. Future Gi package-source
additions must use explicit protocol conformance and must not make npm package
metadata a compatibility boundary.

Resource precedence SHOULD match Pi's useful behavior:

1. Project-local explicit settings
2. Project-local auto-discovered resources
3. User explicit settings
4. User auto-discovered resources
5. Package resources

Package settings MUST support filtering individual resource groups:

```json
{
  "packages": [
    {
      "source": "git:https://github.com/acme/gi-plan-mode.git#v1.0.0",
      "extensions": ["plan-mode"],
      "skills": false,
      "prompts": true,
      "themes": true
    }
  ]
}
```

Missing package behavior is host policy. Interactive Gi SHOULD offer install,
skip, or fail. Non-interactive Gi SHOULD fail unless configured to auto-install.

## Package Lifecycle

Packages are installed resources, not running programs. The host runs only the
resources declared by `gi.package.json`.

### Install

Install resolves a local path, git ref, or approved archive into the user or
project package store, validates `gi.package.json`, records source/ref/digest
metadata in a lock file, and does not execute package code. Hosts MUST NOT run
post-install scripts by default. Any prepare/build step is host policy, must be
declared, and requires explicit approval.

### Resolve

On startup or reload, the host reads enabled package manifests, applies resource
precedence and package filters, and records deterministic source metadata for
extensions, skills, prompts, themes, and assets.

### Activate

Activation is lazy unless a manifest explicitly requires startup behavior. A
host MAY activate an extension on startup, command invocation, lifecycle event,
TUI slot mount, provider use, or resource discovery. The activation trigger is
host-controlled; packages do not import themselves into core.

### Run

For `entry.kind = "process"`, the host spawns the declared command with the
package root as `cwd`, connects stdio NDJSON, and requires a `hello` handshake
before accepting registrations. The process receives only granted capabilities
and can mutate host state only through RPC methods and `host.*` actions.

### Shutdown

When a package is disabled, a session ends, or the host exits, the host sends a
shutdown event, waits for a bounded grace period, kills the process on timeout,
and records diagnostics. Extensions MUST tolerate duplicate shutdown and stale
cancellation messages.

## Capability Model

Every package declares requested capabilities. The host grants a subset by
policy, project settings, and runtime consent. Capabilities are strings with
optional scoped arguments:

| Capability | Meaning |
| --- | --- |
| `tools.register` | Register LLM-callable tools |
| `tools.set_active` | Change active tool set |
| `commands.register` | Register slash commands |
| `shortcuts.register` | Register keyboard shortcuts |
| `lifecycle.events` | Subscribe to lifecycle events and patch allowed results |
| `providers.register` | Register model providers |
| `session.read` | Read session entries, branch, metadata |
| `session.write` | Append custom entries or labels |
| `agent.send_user_message` | Queue or send user messages |
| `agent.abort` | Abort current turn |
| `resources.discover` | Add skills, prompts, themes at runtime |
| `system_prompt.modify` | Append or replace system prompt sections |
| `compaction.custom` | Provide custom compaction behavior |
| `tui.status` | Set status/footer text |
| `tui.dialog` | Use select, confirm, input, notify dialogs |
| `tui.widget` | Mount above/below editor widgets |
| `tui.header` | Replace or augment header |
| `tui.footer` | Replace or augment footer |
| `tui.overlay` | Mount focused or non-capturing overlays |
| `tui.editor` | Replace or wrap the editor |
| `tui.message_renderer` | Render custom session message types |
| `tui.tool_renderer` | Render tool calls/results |
| `tui.autocomplete` | Provide autocomplete suggestions |
| `fs.read:<scope>` | Read files in allowed scope |
| `fs.write:<scope>` | Write files in allowed scope |
| `process.exec:<scope>` | Execute commands in allowed scope |
| `network:<scope>` | Make outbound network requests |

The host MUST expose granted capabilities to the extension during handshake and
MUST reject operations outside the grant. Sensitive capabilities SHOULD trigger
runtime prompts unless project policy pre-approves them.

## Extension RPC v1

Out-of-process extensions communicate over stdio NDJSON by default. JSON-RPC
2.0 MAY be used as an envelope, but the base conformance suite uses NDJSON
messages with `id`, `type`, `method`, and `params`.

### Handshake

Extension starts and sends:

```json
{
  "type": "hello",
  "protocols": ["gi-ext-rpc@1", "gi-viewtree@1"],
  "sdk": {"language": "rust", "version": "0.1.0"},
  "extension": {"id": "plan-mode", "version": "1.0.0"},
  "requestedCapabilities": ["commands.register", "tui.widget"]
}
```

Host replies:

```json
{
  "type": "hello_result",
  "sessionId": "sess_123",
  "protocols": {"gi-ext-rpc": "1.0.0", "gi-viewtree": "1.0.0"},
  "grantedCapabilities": ["commands.register", "tui.widget"],
  "host": {
    "name": "gi",
    "version": "0.3.0",
    "platform": "darwin-arm64"
  }
}
```

If no compatible protocol version exists, the host MUST stop the extension with
a diagnostic.

### Message Classes

- Request: requires response with same `id`.
- Response: includes `result` or `error`.
- Notification: no response, no `id`.
- Event: host-to-extension notification carrying lifecycle or UI input.

All messages include `protocol: "gi-ext-rpc@1"` after handshake.

### Lifecycle Events

The host sends these events when relevant:

- `extension_start`
- `session_start`
- `session_switch`
- `session_shutdown`
- `before_agent_start`
- `agent_start`
- `message_start`
- `message_update`
- `message_end`
- `agent_end`
- `turn_start`
- `turn_end`
- `tool_call`
- `tool_result`
- `tool_execution_start`
- `tool_execution_end`
- `model_select`
- `thinking_level_select`
- `input`
- `resources_discover`
- `shutdown`

Handlers MAY return mutations when the event contract allows it:

- block, rewrite, or annotate tool calls/results
- transform user input
- append resources
- replace message display metadata
- request compaction

Events carry a monotonically increasing `eventSeq`. Events that allow patches
SHOULD include an `id`; the extension returns a normal response with the same
`id` and a patch result. The host applies accepted patches before invoking the
next handler, so later handlers see the current system prompt or tool result.

Patch contracts are intentionally narrow:

- `before_agent_start` MAY return `message[]` and, with
  `system_prompt.modify`, `systemPrompt`.
- `tool_result` MAY return `content`, `details`, or `isError`; omitted fields
  preserve earlier handler changes.
- cancellation is exposed as a host-owned signal reference in event params, and
  the host emits stale/cancelled diagnostics for late responses.

Extension responses that arrive after the host has invalidated the session MUST
be ignored with a stale context diagnostic.

### Registration Calls

An extension registers contributions through host requests:

- `register_tool`
- `register_command`
- `register_shortcut`
- `register_flag`
- `register_provider`
- `register_message_renderer`
- `register_tool_renderer`
- `register_autocomplete_provider`

Tool parameters use JSON Schema 2020-12. Hosts MAY expose convenience SDK APIs,
but the wire format is JSON Schema.

Tool execution request:

```json
{
  "type": "request",
  "id": "tool_42",
  "method": "tool.execute",
  "params": {
    "toolName": "todo_update",
    "toolCallId": "call_abc",
    "input": {"action": "add", "text": "Write tests"},
    "context": {"cwd": "/repo", "turnId": "turn_7"}
  }
}
```

Tool responses MAY stream updates:

- `tool.update` with partial state/details
- final `tool.result`
- `tool.error`
- `tool.terminate_hint` to skip automatic follow-up when all finalized tools in
  a batch agree

### Process Supervision

The host owns process lifetime:

- start with inherited or controlled environment
- send `shutdown` before killing
- enforce startup timeout, idle timeout, and per-request timeout
- restart only if package policy allows it
- isolate stdout protocol from user-visible logs
- route stderr to diagnostics
- keep an audit log of denied capability calls

The extension MUST tolerate duplicate `shutdown` and stale request cancellation.

## Host Action Surface v1

Lifecycle events tell extensions what happened. Host actions let extensions ask
the host to do controlled work. Official packages must be implementable using
only registered host actions plus granted capabilities.

All host actions are RPC requests under method prefix `host.`. The host MUST
return either `result` or a standard error:

- `unsupported_method`
- `missing_capability`
- `invalid_params`
- `stale_context`
- `busy`
- `cancelled`
- `timeout`
- `policy_denied`
- `internal_error`

Required action groups for `gi-coding-agent-compatible@1`:

- `host.tools.list`: returns built-in, package, and SDK tools with source info,
  active state, descriptions, prompt snippets, and renderer availability.
- `host.tools.set_active`: replaces or patches the active tool set.
- `host.commands.list`: returns slash commands from core, prompts, skills, and
  extensions.
- `host.session.entries`: returns branch or full-tree entries by cursor and
  range, with custom entries preserved.
- `host.session.append_custom`: appends extension-owned custom state.
- `host.session.set_label`: labels a session entry.
- `host.session.set_name`: sets a human-readable session name.
- `host.session.action`: requests `new`, `fork`, `switch`, `reload`, `clear`,
  or `navigate_tree`. The host runs interception hooks before applying it.
- `host.agent.send_user_message`: queues a user message with delivery mode.
- `host.agent.run`: runs a child turn in the current host with bounded context.
- `host.agent.spawn`: creates a subagent session with isolated context,
  inherited capabilities, cancellation, and progress events.
- `host.agent.abort`: aborts current or child work.
- `host.model.list`: lists configured models and auth state.
- `host.model.select`: selects a model after auth and policy checks.
- `host.thinking.get` and `host.thinking.set`: reads or changes thinking level.
- `host.tui.mount`, `host.tui.patch`, `host.tui.unmount`: manages ViewTree
  slots.
- `host.tui.dialog`: runs select, confirm, input, editor, and notification UI.
- `host.tui.editor`: reads, writes, inserts text, and submits the editor.
- `host.tui.status`: updates status text by key.
- `host.policy.request`: requests an additional capability grant.
- `host.process.exec`: runs a command under host policy and streams output.
- `host.fs.read` and `host.fs.write`: scoped filesystem access for extensions
  that do not want to execute arbitrary processes.

Host actions are intentionally more explicit than "give the extension a Go
object". This is the core reason the protocol can be implemented by Gi, Ri, and
other hosts.

Example action:

```json
{
  "type": "request",
  "id": "tools_1",
  "method": "host.tools.set_active",
  "params": {
    "mode": "replace",
    "toolNames": ["read", "grep", "find", "ls"]
  }
}
```

The host response includes the canonical active set:

```json
{
  "type": "response",
  "id": "tools_1",
  "result": {
    "activeToolNames": ["read", "grep", "find", "ls"]
  }
}
```

## TUI ViewTree v1

Out-of-process custom TUI components use a declarative ViewTree plus event
protocol. Extensions never write terminal escape sequences directly.

### Slots

The host exposes Pi-aligned slots:

- `header`
- `footer`
- `widget.aboveEditor`
- `widget.belowEditor`
- `overlay`
- `editor`
- `messageRenderer:<customType>`
- `toolRenderer:<toolName>`
- `status:<key>`
- `autocompleteProvider:<id>`

Mount request:

```json
{
  "type": "request",
  "id": "mount_1",
  "method": "tui.mount",
  "params": {
    "mountId": "plan.todos",
    "slot": "widget.aboveEditor",
    "view": {
      "type": "box",
      "id": "root",
      "border": "rounded",
      "children": [
        {"type": "text", "text": "Plan"},
        {"type": "list", "items": [
          {"id": "1", "text": "Inspect code", "checked": true},
          {"id": "2", "text": "Patch tests", "checked": false}
        ]}
      ]
    }
  }
}
```

Patch request:

```json
{
  "type": "notification",
  "method": "tui.patch",
  "params": {
    "mountId": "plan.todos",
    "ops": [
      {"op": "replace", "path": "/children/1/items/1/checked", "value": true}
    ]
  }
}
```

### View Nodes

Required primitive nodes:

| Node | Purpose |
| --- | --- |
| `text` | Styled text with wrapping/truncation |
| `markdown` | Gi/Pi-compatible markdown rendering |
| `box` | Padding, border, background, title |
| `row` | Horizontal layout |
| `column` | Vertical layout |
| `spacer` | Fixed blank space |
| `list` | Selectable or static list |
| `table` | Width-aware tabular data |
| `tree` | Session/file/tree selector style UI |
| `input` | Single-line input |
| `textarea` | Multi-line input/editor fragment |
| `select` | Select list with filtering |
| `button` | Command target for mouse-capable or keyboard UIs |
| `progress` | Progress bar |
| `spinner` | Animated working indicator |
| `diff` | File/tool diff rendering |
| `image` | Terminal image or fallback |
| `toolCall` | Standardized tool call display |
| `message` | Standardized chat/session message display |
| `keyHint` | Resolved keybinding hint |
| `portal` | Internal target for overlay composition |

Unknown nodes MUST render as a diagnostic placeholder unless `fallback` is
provided. ViewTree MAY include host-specific features under `x-gi-*`, but these
cannot be required for conformance.

### Styling and Theme

ViewTree styles are semantic first:

```json
{"fg": "accent", "bg": "surface", "bold": true, "underline": true}
```

Hosts map semantic tokens to their theme. Raw ANSI is forbidden in ViewTree text
unless the node explicitly opts into `ansi: true` and the host grants
`tui.ansi_text`. Even then, the host must sanitize control sequences that would
write outside the component.

Required semantic colors:

- `default`
- `muted`
- `accent`
- `success`
- `warning`
- `error`
- `dim`
- `border`
- `surface`
- `surfaceAlt`
- `tool`
- `customMessage`

### Events

The host sends component events:

- `mount`
- `unmount`
- `focus`
- `blur`
- `key`
- `textInput`
- `submit`
- `cancel`
- `select`
- `change`
- `resize`
- `theme_change`
- `visibility_change`
- `tick`

Example:

```json
{
  "type": "event",
  "method": "tui.event",
  "params": {
    "mountId": "plan.todos",
    "nodeId": "root",
    "event": "key",
    "data": {"key": "ctrl+alt+p", "raw": "\u001b..."}
  }
}
```

Extensions respond with patches, host actions, or no-op. High-frequency
components SHOULD subscribe to `tick` with a maximum frame rate. Hosts SHOULD
default to 10 fps for package components and MAY allow higher rates for trusted
packages.

### Editor Slot

The `editor` slot is special. A custom editor contribution MUST provide:

- `get_text`
- `set_text`
- `insert_text`
- `submit`
- `change`
- `focus`
- `autocomplete_context`

The host keeps final ownership of submit semantics, paste marker expansion,
large paste handling, autocomplete provider ordering, and slash-command routing.

### Autocomplete

Autocomplete providers receive:

- full editor text
- cursor line and column
- trigger symbol or forced completion reason
- current slash command and argument position when available

They return ranked suggestions with stable IDs, display text, replacement range,
kind, description, and optional detail ViewTree.

### Message and Tool Renderers

Message renderer registration:

```json
{
  "customType": "status-update",
  "slot": "messageRenderer:status-update",
  "schema": {"type": "object"}
}
```

Tool renderer registration:

```json
{
  "toolName": "todo_update",
  "renderCall": true,
  "renderResult": true
}
```

Renderers receive message/tool data and return ViewTree. If the renderer fails
or is unavailable, the host falls back to the built-in markdown/JSON renderer.

## Trusted In-Process Component API

In-process components are for:

- Gi core components
- official Gi extensions shipped with the binary
- enterprise forks
- SDK embeddings where the application owner compiles extensions into the host

They are not the portable third-party package standard.

The base Go contract reuses `gi-tui`:

```go
type Component interface {
    Render(width int) []string
    Invalidate()
}

type InputHandler interface {
    HandleInput(data string)
}

type Focusable interface {
    SetFocused(focused bool)
    Focused() bool
}
```

The in-process extension contract SHOULD look like:

```go
type Extension interface {
    ID() string
    Register(ctx *ExtensionContext) error
}

type ComponentFactory func(ctx ComponentContext) (
    component gitui.Component,
    dispose func(),
    err error,
)

type UIRegistry interface {
    SetWidget(key string, factory ComponentFactory, options WidgetOptions)
    SetHeader(factory ComponentFactory)
    SetFooter(factory ComponentFactory)
    ShowOverlay(factory ComponentFactory, options OverlayOptions) OverlayHandle
    SetEditor(factory EditorFactory)
    RegisterMessageRenderer(customType string, renderer MessageRenderer)
    RegisterToolRenderer(toolName string, renderer ToolRenderer)
}
```

Host requirements:

- recover panics from render, input, event, tool, and command handlers
- call `dispose` on reload, session replacement, and shutdown
- route all terminal writes through Gi
- require explicit registration rather than import side effects
- surface diagnostics in the same format as RPC extensions
- run race tests for mutable components

In-process components can use richer Go APIs than ViewTree, but official
components SHOULD avoid behavior that cannot be represented by ViewTree unless
there is a core-only need. This keeps later RPC parity practical.

## Package-Provided Out-of-Process Components

Package-provided UI defaults to out-of-process RPC. Mapping from Pi concepts:

- `ctx.ui.setWidget(key, lines)` maps to a `tui.mount` request with text or
  markdown nodes in a widget slot.
- `ctx.ui.setWidget(key, componentFactory)` maps to a `tui.mount` request with
  ViewTree content in a widget slot.
- `ctx.ui.setFooter(factory)` maps to a `footer` slot mount.
- `ctx.ui.setHeader(factory)` maps to a `header` slot mount.
- `ctx.ui.custom(factory, {overlay})` maps to a focused `overlay` mount plus the
  component event loop.
- `ctx.ui.setEditorComponent(factory)` maps to an `editor` slot contribution.
- `ctx.ui.addAutocompleteProvider(factory)` maps to
  `register_autocomplete_provider`.
- `pi.registerMessageRenderer(type, fn)` maps to
  `register_message_renderer` returning ViewTree.
- Custom tool `renderCall` and `renderResult` map to
  `register_tool_renderer` returning ViewTree.

The user experience target is Pi-like for common extensions: plan mode, todo
widgets, tool picker, presets, custom footer/header, approval dialogs, message
renderers, autocomplete, and provider setup. Game-like overlays and high-frame
animations are supported through `tick` subscriptions, but hosts MAY cap frame
rate for untrusted packages.

## Pi Ecosystem Coverage Targets

The protocol must support these Pi extension categories before it is considered
ready for Gi's full coding-agent layer:

- Permission gates and path protection: `tool_call`, dialogs, block/rewrite
  result, file/path capabilities.
- Git checkpoint and dirty repo guard: lifecycle events, process exec, and
  session switch interception.
- Plan mode: commands, active-tool mutation, status, widget, editor dialog.
- Todo tools/widgets: tool registration, session persistence, custom message and
  tool renderers.
- Subagents: tools, session/agent capabilities, child turn orchestration,
  progress UI.
- MCP adapter: process supervision, tool registration, JSON Schema, streaming
  updates.
- SSH/remote execution: process exec, tool override, capability prompts, custom
  renderers.
- Sandbox execution: process capability, policy config, status/footer UI.
- Custom providers: provider registration, auth prompts, model registry events.
- Presets/model workflows: flags, commands, model/thinking/tool mutation, and
  selector UI.
- GitHub issue autocomplete: process exec/network, autocomplete provider, and
  diagnostics.
- Custom footer/header: TUI slots, theme, resize, session/model data.
- Custom editor: editor slot, text APIs, key events, autocomplete integration.
- Overlays and wizards: overlay slot, focus, key/input events, submit/cancel.
- Games/animated demos: overlay, key events, tick subscription, frame-rate
  policy.
- Dynamic resources: `resources_discover`, package-local paths, source metadata.
- System prompt and compaction: prompt mutation hooks, compaction hook, model
  access.

Official Gi packages SHOULD be built on this protocol instead of core-only code
where practical:

- `gi-plan-mode`
- `gi-subagents`
- `gi-mcp-adapter`
- `gi-git-guard`
- `gi-approval-gate`
- `gi-todo-widget`
- `gi-tools-ui`
- `gi-powerline-footer`
- `gi-web-access`
- `gi-provider-kit`

### Official Package Derivation

This section is the implementation proof. If any package below requires private
host APIs, the protocol is incomplete.

`gi-plan-mode`:

- Registers `/plan` and `/todos` commands.
- Uses `host.tools.set_active` to enter read-only mode.
- Uses `host.tui.status` and `host.tui.mount` for progress.
- Uses `host.tui.dialog` and `host.tui.editor` for refinement.
- Persists plan state with `host.session.append_custom`.

`gi-subagents`:

- Registers a `subagent_run` tool and commands for configured agents.
- Uses `host.agent.spawn` for isolated child sessions.
- Streams child progress through tool updates and ViewTree status.
- Reads parent context through `host.session.entries`.
- Cancels children through `host.agent.abort`.

`gi-mcp-adapter`:

- Starts MCP server processes through `host.process.exec` or a supervised
  process capability.
- Converts MCP tools to `register_tool` calls with JSON Schema.
- Streams MCP progress through `tool.update`.
- Surfaces server diagnostics through standard extension diagnostics.
- Enforces process and network capabilities per configured server.

`gi-git-guard`:

- Subscribes to session action events for `switch`, `fork`, `clear`, and
  `reload`.
- Uses `host.process.exec` for `git status --porcelain`.
- Blocks or confirms unsafe actions through `host.tui.dialog`.
- Stores guard decisions in custom session entries when useful.

`gi-approval-gate`:

- Subscribes to `tool_call` and `host.session.action` interception points.
- Renders file diffs or command details through `diff` and `toolCall` nodes.
- Uses `host.tui.dialog` or `overlay` for approve/deny.
- Caches decisions through `host.session.append_custom` with explicit scope.
- Returns block/rewrite decisions through the event response contract.

`gi-powerline-footer`:

- Mounts a `footer` ViewTree.
- Subscribes to model, branch, context usage, token, status, and tool activity
  updates through host events.
- Uses only semantic theme tokens, so Gi and Ri can render it differently while
  preserving meaning.

`gi-todo-widget`:

- Registers a todo tool with JSON Schema.
- Persists todo state through custom entries and tool result details.
- Registers tool/message renderers for compact and expanded views.
- Mounts `widget.aboveEditor` for current todo state.
- Rehydrates state from `host.session.entries` on session start/switch.

`gi-tools-ui`:

- Registers `/tools`.
- Reads tools through `host.tools.list`.
- Renders a selectable settings UI with `select`, `list`, and `keyHint` nodes.
- Applies changes through `host.tools.set_active`.
- Persists choices through host settings or session custom entries depending on
  scope.

This derivation shows the minimum host action surface needed for the proposed
official packages. `gi-subagents` is the strongest test because it requires
child agent orchestration, cancellation, scoped capabilities, progress events,
and session isolation.

## Rust and Ri Portability

The standard is portable because:

- manifests are JSON
- tools use JSON Schema
- extension transport is process RPC
- ViewTree is declarative JSON
- no Go interface is required for package-provided components
- conformance tests operate on wire messages and rendered snapshots

Rust `ri` can implement:

- `ri-package` resolver compatible with Package Manifest v1
- `ri-ext-host` RPC supervisor
- `ri-viewtree` renderer mapping nodes to a Rust TUI engine
- Rust SDK for extension authors

SDK ergonomics may differ, but the wire contract must not:

```rust
#[tokio::main]
async fn main() -> anyhow::Result<()> {
    ri_ext::serve(PlanMode::new()).await
}
```

```go
func main() {
    giextext.Serve(PlanMode{})
}
```

Both SDKs compile down to the same manifest, RPC, capability, and ViewTree
contracts.

## Versioning and Compatibility

- Protocols are named `gi-ext-rpc@MAJOR` and `gi-viewtree@MAJOR`.
- Minor versions add optional capabilities or nodes.
- Major versions can break wire semantics.
- Handshake negotiates the highest mutually supported minor version.
- Hosts must gracefully degrade unsupported optional features.
- Packages can require minimum host and protocol versions in the manifest.

Deprecation process:

1. Add replacement feature.
2. Emit diagnostics when old feature is used.
3. Keep conformance tests for at least one major version.
4. Remove only in a major protocol revision.

### Registry Governance

Stable protocol registries are append-only within a major version:

- capability names
- RPC method names
- host action names
- lifecycle event names
- ViewTree node types
- semantic style tokens
- diagnostic error codes
- conformance profile names

Changing the meaning of an existing registry item requires a new major version.
Adding an optional item requires:

1. A written schema.
2. At least one positive fixture.
3. At least one negative or fallback fixture when applicable.
4. A documented downgrade path for hosts that do not support it.

This prevents community implementations from depending on hidden host behavior
or undocumented Pi/Gi internals.

## Security and Policy

Install-time UI SHOULD show:

- package source and version/ref
- publisher or repository origin
- requested capabilities
- declared prepare/build behavior, if any
- whether package has native binaries

Runtime policy SHOULD support:

- project allowlist/denylist
- organization policy file
- one-shot grants
- scoped grants, such as `fs.write:repo` but not home directory
- per-extension timeout/resource limits
- extension stderr diagnostics
- audit log for denied calls

Out-of-process is safer than in-process, but it is not a sandbox by itself.
Capabilities must be enforced at host APIs, and optional OS sandboxing should be
used for package processes when available.

## Performance Rules

- Textual RPC default: stdio NDJSON.
- Host batches ViewTree patches per render frame.
- Extension updates can request `priority: "interactive"` for input latency.
- Host applies backpressure when patch queues grow.
- Host may drop stale animation frames.
- Host should cap untrusted `tick` subscriptions.
- Large binary payloads use temp files or content handles, not inline JSON.
- Component render snapshots are host-owned and testable.

## Determinism and Verifiability

Compatibility is not a branding claim. It is a reproducible test result.

Every normative behavior must have one of these verification forms:

- JSON Schema validation for static artifacts and RPC payloads.
- JSONL transcript replay for process/RPC behavior.
- Golden session fixtures for lifecycle, tool, and host action ordering.
- Golden ViewTree snapshots for layout and rendering.
- Negative fixtures for denied capabilities, stale contexts, unknown nodes,
  malformed payloads, and crashed extensions.
- Cross-host fixture replay, where the same extension transcript must pass on
  Gi and Ri.

To keep results deterministic:

- Hosts MUST assign stable event sequence numbers.
- Hosts MUST include deterministic source info for package resources.
- Hosts MUST make time, random IDs, viewport size, theme, cwd, and environment
  injectable in conformance runs.
- Hosts MUST define ordering for conflicting tools, commands, flags, shortcuts,
  renderers, and resources.
- Hosts MUST expose fallback rendering for unsupported optional features.
- Extension tests MUST be able to run without live network, credentials, or
  ambient user configuration unless the fixture explicitly declares otherwise.

The conformance runner SHOULD support three modes:

- `schema`: validates manifests and payloads only.
- `transcript`: replays host/extension JSONL interactions.
- `render`: mounts ViewTree fixtures and compares virtual terminal snapshots.

## Conformance Suite

The protocol is not complete without a shared test suite. Required suites:

1. Manifest parsing and resource precedence
2. Package install source normalization
3. Capability grant/deny behavior
4. Extension handshake and version negotiation
5. Lifecycle event ordering
6. Tool registration, execution, streaming updates, cancellation
7. Command, flag, shortcut registration and conflict resolution
8. Provider registration and auth prompt flow
9. Resource discovery for skills, prompts, themes
10. ViewTree render snapshots for every primitive
11. ViewTree patch application and stale mount behavior
12. Focus, key, text input, submit, cancel, resize, theme events
13. Editor slot text/autocomplete/submit behavior
14. Message and tool renderer fallback behavior
15. Extension crash, timeout, restart, and shutdown behavior
16. Pi extension fixture ports for plan mode, todo, custom footer, custom
    editor, approval gate, subagent, MCP adapter, and provider extension

Both Gi and Ri must pass the protocol conformance suite to claim compatibility.

## Implementation Roadmap

Phase 1: Host foundations

- Define manifest parser and resource resolver.
- Add extension registry abstractions for tools, commands, hooks, providers,
  and UI contributions.
- Add trusted in-process extension registration for official Go components.

Phase 2: RPC core

- Implement stdio NDJSON supervisor.
- Implement handshake, capability grants, lifecycle events, registration calls,
  diagnostics, and shutdown.
- Add conformance tests for non-UI extension behavior.

Phase 3: ViewTree renderer

- Implement ViewTree schema, primitive renderer, patch application, slots,
  focus and input events.
- Map ViewTree nodes to existing `gi-tui` components.
- Add golden TUI snapshots through `gi-tui` virtual terminal.

Phase 4: Pi ecosystem compatibility fixtures

- Port representative Pi examples as Gi packages:
  plan mode, todo, custom footer, modal editor, permission gate, dynamic tools,
  provider extension, and MCP adapter.
- Track feature gaps in a case-level parity report.

Phase 5: Rust/Ri validation

- Build a small Rust SDK and fixture extension.
- Run the same conformance suite against Gi and Ri host implementations.
- Publish package authoring docs that avoid Go-specific assumptions.

## Acceptance Criteria

The protocol is ready for implementation when:

- package manifest, extension RPC, capability, and ViewTree schemas are
  versioned and published
- host action, event, diagnostic, and conformance report schemas are versioned
  and published
- conformance profiles are defined so partial community implementations can
  claim precise compatibility without implying full coding-agent coverage
- every Pi extension category in the coverage matrix maps to concrete protocol
  features
- every official package derivation above can be implemented without private
  host APIs
- official Gi packages can be built without private host APIs except where
  intentionally in-process
- third-party package UI can run out-of-process without writing terminal escape
  sequences
- a Rust extension can register a command, tool, widget, overlay, and message
  renderer using the same wire protocol
- conformance fixtures cover lifecycle, tool, package, and TUI behavior
- Gi and a minimal Ri host can both run at least one shared extension process
  fixture and produce compatible conformance reports
