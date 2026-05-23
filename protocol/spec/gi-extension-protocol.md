# Gi Extension Protocol v1

Status: design target for the future full `gi-coding-agent` runtime.

This specification defines the open boundary for Gi packages, extension
processes, trusted in-process components, and package-provided out-of-process
custom TUI components. A Rust implementation such as Ri can implement the same
wire protocol, schemas, registries, and conformance fixtures without importing
Go code. Pi/npm package artifacts are not compatibility targets; only the
protocol behavior and conformance fixtures are.

Normative keywords `MUST`, `MUST NOT`, `SHOULD`, `SHOULD NOT`, and `MAY` use
RFC 2119 meaning when written in uppercase.

## Artifact Map

The v1 standard is split into machine-checkable artifacts:

- `schemas/package-manifest.schema.json`: package resource manifest.
- `schemas/extension-descriptor.schema.json`: `.gi.json` extension descriptor.
- `schemas/rpc-envelope.schema.json`: extension RPC envelope.
- `schemas/host-action.schema.json`: host action request and response.
- `schemas/viewtree.schema.json`: custom TUI ViewTree.
- `schemas/conformance-report.schema.json`: compatibility report.
- `registries/*.json`: stable names for profiles, capabilities, host
  actions, lifecycle events, ViewTree nodes, and errors.
- `registries/pi-ecosystem-targets.json`: Pi-style ecosystem categories
  that v1 must be able to host.
- `examples/*.jsonl`: replayable transcripts.
- `conformance/README.md`: runner expectations and required suites.

## Conformance Profiles

Hosts and SDKs claim only profiles they pass:

- `gi-package-host@1`: manifest parsing, package source normalization, resource
  precedence, lock metadata, and resource filtering.
- `gi-extension-host@1`: process supervision, protocol negotiation,
  capabilities, lifecycle events, tool/command/provider/resource registration,
  diagnostics, cancellation, and shutdown.
- `gi-tui-host@1`: slots, focus, keyboard events, editor bridge, status,
  dialogs, autocomplete, and renderer registration.
- `gi-viewtree-renderer@1`: deterministic ViewTree render, patch application,
  unknown-node fallback, and snapshot fixtures.
- `gi-inprocess-host@1`: trusted compiled-in extension registration,
  panic/dispose boundaries, and diagnostics.
- `gi-extension-process@1`: package extension process handshake,
  registration, event handling, cancellation, and shutdown.
- `gi-sdk@1`: language SDK that emits compatible manifests, RPC messages,
  host actions, and ViewTrees.
- `gi-coding-agent-compatible@1`: aggregate profile requiring package,
  extension, TUI host, ViewTree renderer, and the Pi ecosystem fixture set.

## Package Layer

A package is a distribution unit. It MAY contain extensions, skills, prompt
templates, themes, and static assets. Packages MUST use `gi.package.json`.
`package.json#gi` and `npm:` sources are not part of v1 and MUST NOT be
treated as compatible package inputs. Hosts MUST NOT discover package metadata
from `package.json`.

V1 package sources are local paths, git URLs with optional refs,
`official:<name>` built-in Gi packages, and approved archive sources such as
tarball or OCI when a host explicitly implements them. Hosts MUST NOT support
`npm:` as a package source. Package compatibility is defined by
`gi.package.json`, extension RPC, capabilities, host actions, and ViewTree, not
by any language ecosystem package manager. Future package source additions must
preserve that boundary and must not make npm package metadata a portable Gi
contract.

Package resolution MUST preserve this resource precedence:

1. Project-local explicit settings.
2. Project-local auto-discovered resources.
3. User explicit settings.
4. User auto-discovered resources.
5. Package resources.

Package resources MUST be filterable by group. Unknown manifest metadata MUST
be preserved in lock files.

Package install MUST fetch or copy the artifact, validate `gi.package.json`,
record source/ref/digest metadata, and avoid executing package code by default.
Hosts MUST NOT run post-install scripts implicitly. Declared prepare/build steps
are host policy and require explicit approval.

Packages do not run as a whole. On startup or reload, hosts resolve enabled
package resources and activate extension entries only when needed: startup
policy, command invocation, lifecycle event, TUI slot mount, provider use, or
resource discovery. For `entry.kind = "process"`, hosts spawn the command with
the package root as `cwd`, connect stdio NDJSON, require a `hello` handshake,
grant capabilities, and route all state changes through RPC and `host.*`
actions. Hosts SHOULD inject stable process metadata environment variables:
`GI_EXTENSION_ID`, `GI_EXTENSION_PATH`, `GI_EXTENSION_SOURCE`,
`GI_EXTENSION_SCOPE`, `GI_EXTENSION_ORIGIN`, and `GI_EXTENSION_PACKAGE_DIR`.
Package-declared environment variables MUST NOT override those host-reserved
keys. On disable, session end, or host exit, hosts send shutdown, wait for a
bounded grace period, kill on timeout, remove registrations and UI mounts owned
by that process, and record diagnostics. Startup diagnostics SHOULD include a
bounded stderr tail when a process exits before `hello` or times out before the
handshake. POSIX hosts SHOULD place package processes in their own process group
and kill that group on shutdown timeout or host cancellation. Runtime stderr
lines SHOULD be retained in that tail and surfaced as visible process-source
diagnostics without being mixed into stdout protocol frames. If a process exits
unexpectedly after a successful handshake, hosts SHOULD surface a visible
process-source diagnostic with the exit error and bounded stderr tail. Shutdown
timeout diagnostics SHOULD report that the process was killed after missing the
grace period. A process MAY emit a
`diagnostic` envelope with `severity:"error"`, `code`, `message`, and optional
`stack`; hosts MUST surface that as a visible extension error tied to the
process source.

Long-lived package processes SHOULD survive ordinary session switches. When the
host binds a replacement session for `/new`, `/resume`, `/fork`, or an
equivalent SDK action, it sends `session_switch` and then the replacement
`session_start` to the already-running process. After `session_switch`, all
`host.session.*`, `host.agent.*`, and session-scoped TUI actions MUST resolve
against the replacement session. Hosts SHOULD NOT send `session_shutdown` for a
normal switch unless they are also stopping or restarting that process.
Lifecycle fanout failures from an already-exited process SHOULD be surfaced as
diagnostics and MUST NOT block the host from completing the session switch for
the user.

## Extension Layer

An extension is executable behavior. The portable default is an out-of-process
stdio NDJSON RPC process. Trusted in-process extensions are allowed only for
core, official compiled-in packages, enterprise forks, or SDK embeddings where
the host owner accepts the memory-safety and crash-risk tradeoff.

Gi's trusted Go host path currently supports explicit in-process slot component
registration through `InProcessUIRegistry`. Factories receive session,
runtime-host, and ViewTree context, mount into live `header`, `aboveEditor`,
`belowEditor`, `footer`, focused `editor`, or overlay regions, recover render
panics as visible diagnostics, refresh live slots on keyed replacement/removal
after startup, and run disposer callbacks on replacement, removal, and
shutdown. The same trusted path supports `ShowCustom` for one-shot editor-region
or overlay workflows with a `done` result and automatic removal/restoration, and
trusted components that implement `SetExpanded(bool)` receive host tool-output
expansion updates. Third-party package UI remains out-of-process ViewTree/RPC by
default.

For simple static contributions, a `.gi.json` file MAY use
`extensionProtocol: "descriptor.v1"`. Descriptor extensions can register
commands, tools, message renderers, lifecycle event subscriptions, shortcuts,
flags, declarative ViewTree mounts, skills, prompts, and theme resources without
starting a process. Descriptor resources inherit extension metadata, and
duplicate tools are diagnosed with the first registration winning.

Extension processes MUST perform a `hello` handshake before registration. The
host MUST respond with compatible protocol versions and granted capabilities.
If no compatible version exists, the host MUST stop the extension and emit a
diagnostic.

Extensions MUST NOT receive direct host objects. They interact through standard
RPC methods, lifecycle events, and `host.*` actions. This keeps the same package
usable by Gi, Ri, and other compatible hosts.

## Capability Model

Every sensitive operation requires a granted capability. Hosts MUST reject calls
outside the grant with `missing_capability` or `policy_denied`.

Extensions MAY call `host.policy.request` to ask for additional capabilities at
runtime. Hosts MUST return an explicit decision containing granted and denied
capabilities; hosts without an approval policy MUST deny by default. Granted
capabilities apply only to the requesting extension context and MUST NOT grant
other extensions access. The `host.policy.request` action itself MUST NOT
require a prior session capability; the host's policy decision is the
authorization boundary.

Capabilities are append-only within major version 1 and are listed in
`registries/capabilities.json`. Experimental capabilities MUST use
`x-<vendor>.<name>` and MUST be ignored or denied by hosts that do not
understand them.

## Host Actions

Host actions are request/response methods under `host.*`. They are the only
portable way for extensions to mutate host state. Required action names are
listed in `registries/host-actions.json`.

Lifecycle events that allow mutation MUST be correlated. The host sends the
event with an `id`, then accepts a response patch with the same `id`. If
multiple handlers are registered, the host applies each accepted patch before
building the next event payload. This preserves Pi-style chaining while keeping
the protocol deterministic and replayable.

Required event patch contracts:

- `before_agent_start`: may append `message[]`; may replace `systemPrompt` only
  when `system_prompt.modify` is granted. `getSystemPrompt` in an SDK must read
  the current chained value.
- `before_provider_request`: includes `model` and a serializable `payload`.
  Handlers may return `payload` with `payloadSet: true` to replace the provider
  request body for later handlers and the final provider transport.
- `after_provider_response`: includes `model`, HTTP `status`, and response
  `headers`. It is observational; handlers may return diagnostics or errors,
  but not mutate the already-open provider response.
- `tool_result`: may replace `content`, `details`, or `isError`. Omitted fields
  keep the current value from previous handlers.
- `user_bash`: includes `command`, `cwd`, and `excludeFromContext`. Extensions
  with `bash.intercept` may return `bashResult` to replace host execution.
  Process packages MUST NOT return executable function handles or host-private
  operation objects; they either return a serializable result or omit
  `bashResult` to let the host continue with its normal executor.
- cancellation: event payloads may include a host-owned `signalId`; process
  SDKs map it to their native cancellation primitive, and hosts reject stale
  responses after cancellation or session replacement.

## Registration RPC

Extension processes register portable behavior by sending `request` envelopes
with one of these methods:

- `register_command`: requires `commands.register`; host later emits
  `command.invoke`. Commands may include `argumentHint`, a short display string
  such as `<path>` or `<scope>`, which the host surfaces in slash autocomplete.
- `register_tool`: requires `tools.register`; host later emits `tool.invoke`
  and consumes the response `content` / `details`.
- `register_provider` / `unregister_provider`: requires `providers.register`;
  config mirrors the Go/Pi provider fields that can cross process boundaries:
  `baseUrl`, `apiKey`, `api`, `headers`, `authHeader`, `compat`, and `models`.
- `register_message_renderer`: requires `tui.message_renderer`; host later
  emits `message.render` with renderer options including `expanded`, and
  accepts `lines`, `text`, or a ViewTree `view`.
- `register_tool_renderer`: requires `tui.tool_renderer`; host later emits
  `tool.render_call` and `tool.render_result` and accepts `lines`, `text`, or a
  ViewTree `view`. Renderer `context` includes the current args, tool call id,
  shared state, cwd, args/result lifecycle flags, `expanded`, `showImages`,
  error state, and any edit preflight diff.
- `register_autocomplete_provider`: requires `tui.autocomplete`; host later
  emits `autocomplete.suggest` and consumes a suggestion result.

Registration requests MUST be capability checked before mutating host runtime
state. Renderer errors or timeouts MUST fall back to the built-in/default
renderer where one exists.

## Pi Ecosystem Compatibility Targets

Protocol readiness is judged against the target categories in
`registries/pi-ecosystem-targets.json`. The standard MUST cover permission
gates, path protection, git guards, plan mode, todo widgets, subagents, MCP
adapters, remote execution, sandbox wrappers, custom providers, presets, issue
autocomplete, custom header/footer/editor components, overlays, animated demos,
dynamic resources, prompt mutation, and compaction hooks.

Official packages MUST be implementable with the registered host actions and
capabilities:

- `gi-plan-mode`: commands, tool activation, status/widget UI, editor/dialog
  refinement, session custom state.
- `gi-subagents`: child agent spawn, isolated context, progress, cancellation,
  and session reads.
- `gi-mcp-adapter`: `process.stdio:<scope>` supervision, tool registration
  from JSON Schema, streaming updates, diagnostics, scoped process/network
  grants, and host-supervised stdio cleanup with timeout/cancellation
  guarantees.
  The current Go proof covers stdio `initialize`, `tools/list`, `tools/call`,
  `notifications/progress`, and `notifications/tools/list_changed` for
  configured server commands, then projects discovered MCP tools into Gi
  `register_tool` entries that call back through the same approved server
  command.
- `gi-git-guard`: session-action interception, `git status`, confirmation UI,
  scoped `host.process.exec` with host-enforced `timeoutMillis`, partial output
  preservation, RPC cancellation, process-group termination before force kill
  where supported, bounded inherited-stdio handling after direct child exit,
  `killed` result metadata, and persisted decisions.
- `gi-approval-gate`: tool-call interception, diff/tool renderers, approval
  dialogs, scoped decision cache, and block/rewrite responses.
- `gi-powerline-footer`: footer ViewTree driven by model, branch, token,
  context, and tool-activity events.
- `gi-todo-widget`: todo tool, custom entries, tool/message renderers, widget
  mount, and rehydration from session entries.
- `gi-tools-ui`: command registration, tool listing, selectable tool settings,
  active-tool patching, and persisted scope.

The package derivation registry in
`registries/official-packages.json` is the acceptance checklist. If one of
these packages needs a private host API, v1 is incomplete.

`resources_discover` handlers MAY return `skillPaths`, `promptPaths`, and
`themePaths` as resource extension entries. Hosts MUST normalize relative paths
against the declaring extension/package base, attach deterministic source
metadata, and merge discovered resources before rebuilding skills, prompt
templates, themes, and the system prompt. Startup and reload MUST use the same
result shape so Go, Rust, and process-based SDKs can be conformance-tested with
the same fixtures. Hosts that activate an out-of-process package for resource
discovery SHOULD request `resources_discover` after handshake and session
binding, then refresh prompt/resource-derived UI state before the first user
turn. The lifecycle reason, including `startup`, `reload`, or a session-switch
reason like `new`, MUST match the reason sent with the adjacent `session_start`
event. A handler failure or invalid result from one process MUST be surfaced as
a diagnostic without discarding valid resources returned by other handlers in
the same discovery pass.

The Go descriptor/runtime path MAY bind a host `ViewTreeHost` for trusted or
official package commands. When bound, command handlers SHOULD mount ViewTree
widgets or footer segments through the same slot model used by
`host.tui.mount`; when no live TUI exists, the mount MUST be a no-op and the
command's custom-message/session effects remain authoritative.

## TUI ViewTree

Out-of-process custom TUI components MUST return declarative ViewTree JSON.
They MUST NOT write ANSI, OSC, or terminal-control bytes directly. Gi or Ri owns
terminal I/O, focus, keyboard parsing, layout, theme mapping, image protocols,
and diff rendering.

Standard slots are:

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

Unknown ViewTree nodes MUST render through `fallback` when present. Without
fallback, hosts MUST render a diagnostic placeholder and continue.
Experimental nodes MUST use `x-<vendor>.<node>` and include `fallback`.
Hosts MUST validate mounted and patched ViewTrees at the host-action boundary:
`type` is required on every node, registered event names and semantic colors
must be known, table columns must declare stable `id` and `title`, list items
must declare stable `id`, and invalid patches MUST leave the previous mounted
tree unchanged.

For mounted ViewTrees owned by a process extension, the host emits `tui.event`
envelopes back to that process when the live TUI focuses, blurs, unmounts, or
routes keyboard input to the mounted component. The event params are
`mountId`, `nodeId`, `event`, and optional `data`. Process extensions MUST use
`host.tui.patch` or another host action to update UI state; they MUST NOT write
terminal bytes in response to input. Hosts MUST unmount ViewTree mounts owned
by a process extension when that process stops, is killed, or exits
unexpectedly. Process extensions MUST only patch or unmount ViewTree mounts
they own and MUST NOT replace existing mounts owned by another process or host;
compliant hosts reject cross-owner `host.tui.mount`, `host.tui.patch`, and
`host.tui.unmount` requests.

Process extensions with `tui.terminal_input` MAY observe asynchronous
`tui.terminal_input` events containing raw `data`, but they MUST NOT consume or
transform the core input stream. Focused ViewTree events are the standard path
for interactive components that need ownership of key handling.

## In-Process Components

In-process components are not the portable package standard. A host MAY expose
a trusted API that reuses its native component interfaces, but it MUST preserve
these invariants:

- recover panics from render, input, command, tool, and event handlers
- call dispose hooks on reload, session replacement, and shutdown
- route all terminal writes through the host renderer
- require explicit registration instead of import side effects
- surface diagnostics using the same error registry as RPC extensions
- keep official components close to ViewTree semantics when practical

## Package-Provided Out-of-Process Components

Package UI maps Pi-style component concepts to RPC and ViewTree:

- widget lines or factories become `host.tui.mount` calls.
- headers and footers become slot mounts.
- overlays and wizards become focused ViewTree mounts plus event handling.
- extension dialogs may carry `timeout` in milliseconds; compliant hosts
  auto-cancel timed-out dialogs and may surface a countdown in the title.
- notification dialogs may carry `type` as `info`, `warning`, or `error`;
  hosts should surface warning/error severity through their native status UI.
- status updates use `host.tui.status`; process-owned status keys MUST be
  cleared from the host status surfaces when that process exits or is stopped.
- ViewTree mount authorization is slot-specific: widget slots require
  `tui.widget`, header/footer slots require `tui.header` or `tui.footer`,
  overlays require `tui.overlay`, and editor-slot mounts require `tui.editor`.
- custom editors use `host.tui.editor` for text mutation, paste semantics,
  submit, cursor queries, focus, active custom-editor state, and autocomplete
  context.
- terminal title updates use `host.tui.title`; process-owned titles MUST be
  restored to the host default when that process exits or is stopped.
- working loader updates use `host.tui.working` for message, visibility, and
  indicator frames; process-owned working loader state MUST be reset to the
  host default when that process exits or is stopped.
- hidden thinking labels use `host.tui.thinking_label`; process-owned labels
  MUST be reset when that process exits or is stopped.
- theme listing and switching use `host.tui.theme`; process extensions receive
  serializable theme metadata and still style ViewTree nodes with semantic
  tokens instead of raw ANSI.
- tool-output expansion uses `host.tui.tools_expanded`, matching Pi's
  `getToolsExpanded` and `setToolsExpanded` UI context behavior.
- autocomplete providers use `register_autocomplete_provider`, priority-ordered
  provider lookup, first non-empty suggestion responses, host-owned native
  editor adaptation that refreshes when providers register after TUI startup,
  and slash argument context (`slashCommand`, zero-based `argumentIndex`) when
  the cursor is inside `/command ...` input.
- shortcuts use `register_shortcut`; hosts resolve conflicts against reserved
  native shortcuts before activation and emit `shortcut.invoke` to the owning
  process when the shortcut fires.
- flags use `register_flag` and `get_flag`; hosts normalize leading `--`, store
  default values in the shared extension runtime, apply CLI-provided extension
  flag values to registered flags, keep CLI values pending for out-of-process
  packages that register after startup, and only expose a value to a requesting
  source that registered that flag.
- message and tool renderers return ViewTree and fall back to built-in renderers.

This preserves Pi's extension value while avoiding a hard dependency on
TypeScript in-process components.

## Versioning

Protocols are named `gi-ext-rpc@MAJOR` and `gi-viewtree@MAJOR`. Minor versions
add optional capabilities, actions, events, or nodes. Major versions can change
wire semantics.

Registries are append-only within a major version. Stable additions require:

1. A schema entry or schema update.
2. At least one positive fixture.
3. At least one negative or fallback fixture when applicable.
4. A documented downgrade path.

## Security

Out-of-process execution is not a sandbox by itself. Hosts MUST enforce
capabilities at the host-action boundary and SHOULD provide scoped grants,
project policy, organization policy, one-shot approvals, timeout limits, stderr
diagnostics, and audit logs.

## Determinism

Conformance runs MUST be reproducible. Hosts MUST make event sequence numbers,
time, random IDs, viewport size, theme, cwd, and environment injectable. Default
fixtures MUST NOT require live network, credentials, or ambient user
configuration.

## Implementation Roadmap

1. Implement package manifest parsing and resource resolution.
2. Add extension registries for tools, commands, hooks, providers, resources,
   and UI contributions.
3. Add trusted in-process registration for official Go components. The current
   Go host covers dynamic keyed slot, editor-region, overlay, and one-shot
   custom component workflows.
4. Implement stdio NDJSON supervisor, handshake, capabilities, diagnostics,
   lifecycle events, registration, cancellation, and shutdown.
5. Implement ViewTree schema validation, rendering, patching, slots, focus, and
   input events on top of `gi-tui`.
6. Port Pi ecosystem fixtures for plan mode, todo, footer, editor, approval
   gate, subagents, MCP adapter, and provider extensions.
7. Validate the same fixtures against a minimal Rust Ri host or SDK harness.
