# Gi Extension Protocol v1

Status: design target for the future full `gi-coding-agent` runtime.

This specification defines the open boundary for Gi packages, extension
processes, trusted in-process components, and package-provided out-of-process
custom TUI components. A Rust implementation such as Ri can implement the same
wire protocol, schemas, registries, and conformance fixtures without importing
Go code.

Normative keywords `MUST`, `MUST NOT`, `SHOULD`, `SHOULD NOT`, and `MAY` use
RFC 2119 meaning when written in uppercase.

## Artifact Map

The v1 standard is split into machine-checkable artifacts:

- `schemas/package-manifest.schema.json`: package resource manifest.
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
templates, themes, and static assets. Packages SHOULD use `gi.package.json`.
Npm-compatible packages MAY embed the same object under the `gi` key in
`package.json`.

Package resolution MUST preserve this resource precedence:

1. Project-local explicit settings.
2. Project-local auto-discovered resources.
3. User explicit settings.
4. User auto-discovered resources.
5. Package resources.

Package resources MUST be filterable by group. Unknown manifest metadata MUST
be preserved in lock files.

## Extension Layer

An extension is executable behavior. The portable default is an out-of-process
stdio NDJSON RPC process. Trusted in-process extensions are allowed only for
core, official compiled-in packages, enterprise forks, or SDK embeddings where
the host owner accepts the memory-safety and crash-risk tradeoff.

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

Capabilities are append-only within major version 1 and are listed in
`registries/capabilities.json`. Experimental capabilities MUST use
`x-<vendor>.<name>` and MUST be ignored or denied by hosts that do not
understand them.

## Host Actions

Host actions are request/response methods under `host.*`. They are the only
portable way for extensions to mutate host state. Required action names are
listed in `registries/host-actions.json`.

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
- `gi-mcp-adapter`: process supervision, tool registration from JSON Schema,
  streaming updates, diagnostics, and scoped process/network grants.
- `gi-git-guard`: session-action interception, `git status`, confirmation UI,
  and persisted decisions.
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
- custom editors use `host.tui.editor`.
- autocomplete providers use registered providers plus suggestion responses.
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
3. Add trusted in-process registration for official Go components.
4. Implement stdio NDJSON supervisor, handshake, capabilities, diagnostics,
   lifecycle events, registration, cancellation, and shutdown.
5. Implement ViewTree schema validation, rendering, patching, slots, focus, and
   input events on top of `gi-tui`.
6. Port Pi ecosystem fixtures for plan mode, todo, footer, editor, approval
   gate, subagents, MCP adapter, and provider extensions.
7. Validate the same fixtures against a minimal Rust Ri host or SDK harness.
