<!-- markdownlint-disable MD013 MD060 -->

# Pi Coding Agent Source Audit

> This document records the v0.78.0-era detailed source audit. The active
> catch-up target is Pi v0.82.0 at `083e6162`; current machine-readable debt is
> tracked in [`docs/pi-parity/v0.82.0-open-gaps.json`](docs/pi-parity/v0.82.0-open-gaps.json).
> The historical green case table is not a v0.82.0 completion claim.

This document tracks source-level parity for Pi `packages/coding-agent/src` in the current local Pi checkout. It complements `PI_CODING_AGENT_TEST_CASE_PARITY.md`: test-case parity is green, but this file is the guardrail for behavior that Pi source implements without an explicit test.

## Completion Criteria

- Every Pi coding-agent source file has a concrete Gi disposition.
- Runtime behavior is either implemented, intentionally replaced by a Go-native design, implemented through the Gi protocol/package boundary, or explicitly excluded.
- Extension, package, and custom TUI surfaces must use `protocol/spec` rather than copying Pi's in-process TypeScript API.
- Gaps must be called out as open work until implemented and verified.

## Current Result

- Pi source files discovered in `packages/coding-agent/src`: `150`.
- Explicit Pi test cases tracked separately: `1225`, with `0` remaining
  `待实现`, `待审计`, or `需要协议 runtime` in the existing case table.
- Source-level status: **active audit, not a blanket completion claim**. The
  case table is useful evidence, but the module-level audit in
  `docs/pi-parity/module-audit.md` remains authoritative for current open
  verification risks and intentional Gi product/protocol differences. Gi now has a default
  Go-native TTY host with startup clear, dynamic cwd/model/context footer,
  Pi-style flow layout for editor/footer input, Pi-style terminal title updates,
  startup `--resume` session picker, model-scope notice, `models.json` error
  warning, session model restore/fallback warnings, Anthropic subscription-auth
  warning, changelog/update/package-update notices, loaded-resource
  startup/reload summaries, streaming editor queueing, Pi-style stop/abort/exit
  keys, model/thinking hotkeys, external editor handoff, clipboard-image paste,
  debug snapshots, graceful signal handling, auto-retry status/error messages,
  compaction lifecycle rendering, orphan streaming cleanup, tmux extended-key
  startup warnings, and a non-TTY single-turn fallback. Built-in slash
  workflows cover settings/resources/theme/thinking/model/scoped-models/queue/
  session/hotkeys/changelog/export/share/import/resume/fork/tree/name/new/
  compact/copy/clone/login/logout/reload/quit plus bash `!` / `!!`. Extension,
  package, and custom TUI behavior is implemented through the Gi protocol
  boundary: protocol-backed dialogs, custom message and tool renderers,
  supervised package-process commands, shortcuts, autocomplete providers,
  editor/dialog host actions, message replay, tool execution, ViewTree
  tick/resize events, trusted in-process Go components, and official package
  slash-command UI feedback all run in the live TTY host.
- Latest dynamic-resource increment: Gi now handles `resources_discover`
  results for Go-native extension factories, merges returned skill, prompt, and
  theme paths into the default resource loader during startup/reload, carries
  deterministic source metadata through the loader, and lets descriptor
  resources declare themes alongside skills and prompts. Package process
  extensions with `resources.discover` are now requested through the same
  event/result contract; the live TTY host merges returned package-local
  resources into the active loader, refreshes the session system prompt, and
  forwards matching `startup`/`reload` reasons to both process `session_start`
  and `resources_discover`. Session switches such as `/new` also re-run process
  resource discovery against the rebound session so package-provided resources
  can follow the active session. Failed process resource handlers surface as
  diagnostics without blocking valid resources from other handlers. This covers
  Pi's dynamic resource event shape inside Gi's protocol boundary without
  npm/TS private APIs.
- Latest model-runtime increment: `ModelRegistry` and
  `model_registry_dynamic.go` now compose an
  instance-scoped `llm.Models` collection for default and `models.json`
  configured Radius providers. A private typed `models.json` structure keeps
  declarative `oauth: "radius"` separate from extension provider input;
  credentials remain owned by `AuthStorage`, dynamic catalogs restore through
  an injected or private JSON `ModelsStore`, and network access is opt-in with
  a bounded context. Cached and fetched models are merged into the registry as
  detached snapshots, with explicit model definitions and `modelOverrides`
  reapplied as the top user layer after every refresh; request auth is resolved
  through the same Radius provider that owns refresh and `pi-messages`
  streaming behavior. Auth selection is deterministic: runtime or persisted
  credentials win first, then `models.json`, then provider environment or
  fallback auth. Credential-scoped environment values remain attached to the
  resolved request and take precedence while configured headers are expanded.
  The provider now also owns Radius OAuth discovery, browser PKCE callback,
  RFC 8628 device-code polling, refresh rotation, and request-token
  derivation. The same provider-neutral state machine now powers the built-in
  Kimi and xAI device flows, including delayed polling, slow-down transitions,
  refresh rotation, and bounded/cancellable HTTP handling. Kimi's request
  timeout and deterministic 429/5xx refresh backoff use the shared typed
  provider retry contract. OpenRouter's provider-owned flow keeps ephemeral
  loopback listener, PKCE, single-claim callback, exchange timeout, and
  permanent-key state behind the same interaction boundary; coding-agent
  remains responsible only for implementing provider-neutral
  `AuthInteraction`.
- Latest interactive increment: default TTY startup now parses changelog entries, records fresh installs without showing them, shows Pi-style full or condensed changelog notices when `lastChangelogVersion` is behind, surfaces a Pi-style update notice from an injectable Gi release checker without forcing tests to hit the network, checks configured Gi git packages for remote HEAD differences before showing a package-update notice, wires the existing Anthropic subscription-auth warning into live startup/model-switch paths, and sets the terminal title from cwd/session name. It also shows Pi-style model scope when scoped models are configured and not silenced, restores an existing session's model/thinking level when possible, surfaces a Pi-style startup warning when that saved model falls back, and surfaces `models.json` load/parse errors through the chat status area. Startup and `/reload` render Pi-style loaded resource sections from the active `AgentSession.ResourceLoader`, including context files, skills, prompt templates, extensions, custom themes, and quiet-startup diagnostics without showing the verbose listing. `/reload` now temporarily replaces the editor area with Pi-style reload feedback, then rebuilds the extension runtime, process-extension specs, autocomplete providers, ViewTree slots, and editor/TUI settings before restoring editor focus and showing the reloaded resource summary. The live TTY regression suite also covers a screenshot-sized 188x56 viewport so startup typing and `/` autocomplete stay in Pi-style flow directly below the startup content rather than being anchored to the terminal bottom.
- Latest package-lifecycle increment: long-lived package processes receive
  `session_switch` plus replacement `session_start` on live session changes
  without restarting, and lifecycle fanout failures from an already-exited
  process are rendered as diagnostics instead of blocking `/new` and related
  session-switch commands.
- Latest slash/session increment: `/session` now renders Pi-style sectioned
  `Session Info`, `Messages`, `Tokens`, and optional `Cost` output instead of a
  long single-line summary that can wrap poorly in narrow viewports. No-argument
  built-in slash commands such as `/session` and `/quit` now require exact
  invocation, matching Pi's command dispatch so `/session extra` and
  `/quit later` are sent as normal prompts rather than being swallowed by the
  TTY host.
- Latest bash-input increment: Escape now clears an unsent `!` / `!!` bash
  draft in the live TTY editor while leaving ordinary non-empty drafts alone,
  matching Pi's bash-mode Escape handling.
- Latest bash-output-accumulator increment: local bash execution now uses a
  Go-native bounded output accumulator. It keeps the existing live chunk
  callback behavior, but both live tool updates and final tool results are
  built from rolling tail snapshots, and large outputs are preserved in a temp
  file as soon as Pi's 2000-line or 50KB threshold is crossed, matching Pi's
  `OutputAccumulator` memory boundary instead of buffering the whole command
  output in RAM.
- Latest command-output increment: interactive `/export` now uses Pi's
  `Session exported to:` status wording while leaving non-interactive export
  output unchanged.
- Latest input increment: slash commands now submit when a terminal sends a
  bare linefeed for Enter, while preserving Pi-style bare-linefeed newline
  insertion for normal multi-line editor content.
- Latest system-prompt-docs increment: the default system prompt now mirrors
  Pi's documentation affordance with Gi-native local documentation entries.
  When the Gi source/docs root is discoverable, it points the model to
  `README.md`, `protocol/README.md`, `protocol/spec/gi-extension-protocol.md`,
  and `PI_COMPATIBILITY.md` only for Gi/protocol/package/extension questions,
  while custom system prompts remain replacement prompts as in Pi.
- Latest tool-wrapper increment: SDK and protocol tools now preserve JSON
  Schema parameters, execution-mode metadata, and a Go-native prepare-arguments
  hook across registration. Print-mode provider calls now pass the active tool
  schemas into `llm.Context.Tools`, and out-of-process tool invocations receive
  both `name`/`toolName` plus cwd/session/source context through `tool.invoke`.
- Latest startup-timing increment: Gi now has Go-native startup timing
  instrumentation. `GI_TIMING=1` enables it, and `PI_TIMING=1` is accepted as a
  migration-compatible alias; timings are printed to stderr with the same
  startup-timings shape as Pi.
- Latest resource-system-prompt increment: CLI resource flags are now consumed
  by the default resource loader instead of only being parsed. `--no-*`
  resource flags disable discovered package/top-level resources while explicit
  CLI resource paths remain enabled, and `--system-prompt`,
  `--append-system-prompt`, and AGENTS/CLAUDE context files feed the session
  system prompt. Context loading now mirrors Pi's global-plus-ancestor order and
  includes the loaded file content, not just startup summary paths.
- Latest CLI-version increment: `gi --version` now prints the configured package
  version, falling back to `DefaultCodingAgentVersion`, instead of a hard-coded
  binary name.
- Latest CLI-help increment: `gi --help` now loads protocol descriptor flags
  through the same resource-loader boundary and lists them in an
  `Extension CLI Flags` section, matching Pi's help surface without starting
  provider/model execution or process packages.
- Latest frontmatter increment: skill and prompt-template frontmatter parsing now
  accepts folded (`>`) and literal (`|`) block scalar descriptions and ignores
  unknown nested metadata blocks, matching common Pi/Claude-style resource files
  without surfacing false startup `[Skill conflicts]` diagnostics.
- Latest CLI-entry increment: `RunCLI` now consumes parser diagnostics before
  startup/migration dispatch, matching Pi's entrypoint behavior where invalid
  thinking levels print warnings and unknown short flags print errors that
  exit before any mode starts.
- Latest session-cwd increment: print/default runtime creation now resolves the
  final runtime cwd from an explicitly opened or selected session before
  loading project settings, resources, model scope, and system prompt content;
  missing stored session cwd values fail with `MissingSessionCwdError` in
  non-interactive paths. Interactive startup now mirrors Pi's missing-cwd
  prompt by offering Continue/Cancel before runtime creation and uses the
  startup cwd as an explicit override when Continue is selected.
- Latest migration increment: startup migrations now move legacy root-level
  session `.jsonl` files from the agent directory into the cwd-derived
  `sessions/--...--/` directory, migrate legacy `oauth.json` and
  `settings.json` `apiKeys` into `auth.json` with a startup warning, move
  managed `fd`/`rg` binaries from `tools/` to `bin/`, and move legacy
  global/project `commands/` directories to `prompts/` when the destination
  does not already exist. It also reports Pi-style startup warnings for
  deprecated `hooks/` directories and custom `tools/` contents, preserving Pi's
  auth, session, managed-tool, prompt-template, and extension deprecation
  migrations without adding a Node/Bun migration layer.
- Latest in-process UI increment: trusted Go components registered through
  `InProcessUIRegistry` now refresh dynamically after startup. Runtime
  `SetWidget`/`SetHeader`/`SetFooter`/`SetEditor`/`SetOverlay` replacement
  disposes the previous keyed component, `Remove` clears the live TTY slot,
  editor region, or overlay, and the default TTY host rerenders immediately.
  This mirrors Pi's dynamic extension UI affordance for trusted in-process Go
  components while package-provided components remain behind the
  ViewTree/protocol boundary.
- Latest in-process input increment: the trusted in-process component safety
  wrapper now forwards `gitui.SizeAwareComponent`, `gitui.InputHandler`, and
  `gitui.Focusable` behavior plus key-release opt-in to the wrapped Go
  component with panic recovery. Mounted Go-native editor/overlay components can
  now actually receive focus, terminal input, and Kitty key release events when
  requested, and trusted code can use `ShowCustom` for one-shot editor-region or
  overlay workflows with a `done` result, automatic removal, disposer execution,
  and default-editor focus/draft restoration. This matches Pi's interactive
  custom component affordance without exposing that in-process API to
  package-provided UI.
- Latest in-process expansion increment: trusted Go components that implement
  `SetExpanded(bool)` now receive the current tool-output expansion state when
  mounted and whenever `host.tui.tools_expanded` / Ctrl+O toggles. This mirrors
  Pi's custom header/footer behavior while still keeping package-provided UI on
  the ViewTree/protocol path.
- Latest in-process editor increment: trusted Go editor-slot components that
  implement `gitui.EditorComponent` are now treated as the active editor instead
  of only a visual replacement. The live TTY host routes `host.tui.editor`
  read/set/insert/paste/submit actions, external-editor text transfer,
  clear/Escape/queue submission helpers, focus checks, autocomplete provider
  refresh, and settings application through the active editor, and preserves the
  custom editor text when returning to the default editor.
- Latest official-package increment: `gi-plan-mode` now enters read-only
  tool mode through the canonical host tool policy when `/plan` or
  `/plan-review` runs, persists that state as `plan_state`, renders
  `/plan-review` as a distinct review state, and renders plan items through
  official `plan_update`/`plan_read` tool renderers. `gi-tools-ui` also proves
  protocol-bound active-tool patching through `tools_ui_set_active`, persists a
  `tools_state` custom entry, refreshes the session system prompt, and renders
  the updated active tool set through the official `/tools` command state.
  `gi-git-guard` now performs a read-only `git status --short --branch` check
  for the requested cwd or session cwd, persists branch/dirty-file state, and
  renders that guard state through `/git-guard`. `gi-approval-gate` now records
  approve/deny/rewrite decisions with reasons or replacements and renders both
  request and decision state through the official approval renderers.
  `gi-subagents` now reuses a session-scoped host-action RPC host so
  `subagent_abort` can cancel a currently running `subagent_spawn` child and
  render the aborted subagent state; `subagent_spawn` can read an explicitly
  bounded parent context window and render it through official subagent tool and
  command renderers. `gi-powerline-footer` now derives its
  command message and footer ViewTree segment from the bound host session,
  including model, thinking level, git branch, context usage, and active tools.
  Every first-party command package now declares the TUI capability needed for
  its ViewTree slot, and the official command regression suite verifies
  above-editor widgets for plan, subagents, MCP, git guard, approval, todo, and
  tools plus the footer mount for powerline. The live TTY regression suite also
  asserts those command-mounted widgets reach the actual above-editor/footer
  slots for the official package slash commands.
- Latest official-host-action increment: `gi-git-guard` now uses
  `host.process.exec` for its `git status --short --branch` check whenever a
  bound host action executor is available, so policy-enabled official packages
  exercise the same process boundary as out-of-process packages instead of
  directly spawning git from package code. `gi-plan-mode` and `gi-tools-ui`
  active-tool changes now also route through `host.tools.set_active` rather
  than directly mutating the runtime tool set. `gi-subagents` now routes
  `subagent_spawn` and `subagent_abort` through canonical `host.agent.spawn`
  and `host.agent.abort` requests, so official package behavior exercises the
  same child-agent host-action boundary as out-of-process packages.
- Latest resource-selector increment: the default TTY host now exposes
  `/resources`, a searchable selector for package resources plus top-level
  project `.gi` and user-agent skills, prompts, themes, descriptor extensions,
  and process extensions. It persists the same `+/-` filter rules as Pi's
  config selector at the matching package, project, or user scope, then reloads
  the active resource loader/session so changes are reflected in the live TUI.
  The standalone `gi config` command opens the same selector host for
  Pi-style resource configuration outside an active chat session.
- Latest settings-selector increment: the default TTY host now mirrors Pi's
  `enableSkillCommands` semantics for visible command registration: skill
  commands appear in RPC/slash autocomplete only while the setting is enabled,
  while manually typed `/skill:name` prompts still expand through the session
  runtime. Startup and settings changes also apply editor padding,
  autocomplete item count, hardware cursor, clear-on-shrink, and
  thinking-block visibility to the active TUI without waiting for `/reload`.
  Inline terminal image controls now follow Pi's capability gate: `showImages`
  and `imageWidthCells` are only shown when the terminal advertises image
  support, while image input controls such as auto-resize and block-images stay
  available regardless of terminal rendering support, using Pi's
  `auto-resize-images` settings item id.
- Latest theme-selector increment: `/theme <name>` now applies a theme directly,
  and `/theme` without arguments opens a searchable live selector with Pi-style
  theme preview/restore semantics: moving or filtering to a candidate previews
  it without persisting settings, Enter commits it, and Escape restores the
  original theme. The `/settings` Theme submenu uses the same preview boundary.
- Latest theme-state increment: the interactive host now delegates fixed and
  automatic `light/dark` settings, terminal detection and change reports,
  preview/restore, dark fallback, and notification disposal to one Go-native
  controller. Presence-aware raw settings and an immutable parsed pair feed a
  revisioned serialized palette transition, so a delayed terminal query cannot
  overwrite a newer explicit selection.
- Latest tool-image increment: `ToolExecutionComponent` now consumes Pi-style
  terminal image settings. Tool results with image blocks render inline through
  `gi-tui` image protocols when supported, fall back to deterministic image
  indicators otherwise, and live `/settings` changes for `showImages` and
  `imageWidthCells` update existing and future tool components.
- Latest tool-fallback increment: extension or unknown tools without custom
  renderers now use Pi-style default rendering: the tool name, formatted JSON
  args, and plain text result remain visible instead of dropping the call
  arguments from the transcript.
- Latest tool-render-context increment: Go-native and out-of-process tool
  renderers now receive Pi's `showImages` renderer-context flag, so package UI
  can match the active terminal image setting instead of guessing.
- Latest tool-output-sanitize increment: tool result text now follows Pi's
  display sanitization before TUI rendering by stripping ANSI escapes, removing
  carriage returns, and filtering unsafe control/format characters.
- Latest tool-path-display increment: built-in tool call rendering now shortens
  home-directory paths to `~/...` like Pi's shared `shortenPath` helper, while
  keeping slash-normalized paths for other locations.
- Latest compact-read-resource increment: compact read rendering now treats
  `CLAUDE.md` / `CLAUDE.MD` as resource files alongside `AGENTS.md` /
  `AGENTS.MD`, matching Pi's resource-file classification while keeping Gi's
  `.gi` path conventions.
- Latest read/write-display increment: read results and write previews now
  mirror Pi's display normalization by expanding tabs to three spaces and
  removing carriage returns from write preview content before terminal
  measurement/truncation.
- Latest read-truncation-display increment: read result rendering now appends
  Pi-style truncation summaries after collapsed output, so line/byte/requested
  limit truncation stays visible even when the continuation note is below the
  folded 10-line preview.
- Latest search-tool-renderer increment: built-in `grep`, `find`, and `ls`
  calls now use Pi-style Go-native call/result renderers instead of the generic
  JSON fallback, including default paths, limit labels, and collapsed result
  previews with Ctrl+O expansion hints.
- Latest search-tool-limit increment: `find` and `ls` now honor Pi-style
  `limit` arguments with actionable continuation notices and structured limit
  details, while `grep` exposes match-limit details to package/tool renderers.
  Default SDK bindings now preserve those details through `SDKToolResult`.
- Latest grep-semantics increment: `grep` now supports Pi-style regex matching,
  `literal`, `ignoreCase`, `glob` filtering, and `.gitignore` pruning in the
  Go-native executor and SDK binding, while the TUI call renderer surfaces the
  glob filter.
- Latest ls-display increment: `ls` now mirrors Pi's empty-directory output and
  case-insensitive entry sorting before applying entry limits.
- Latest edit-tool-semantics increment: `edit` now strips/restores leading
  UTF-8 BOMs around matching, rejects identical replacements, reports Pi-style
  indexed duplicate/not-found/overlap errors, extends fuzzy matching to Pi's
  smart quote, dash, CR-only newline, and special Unicode-space cases, and
  emits lower-camel JSON details including `firstChangedLine`. Edit diffs now
  render Pi-style line-numbered, context-windowed output instead of raw
  replacement fragments.
- Latest default-file-tools increment: default SDK `read`, `edit`, and `write`
  are now backed by the real Go tool definitions instead of prompt-only or
  direct executor entries, so model tool calls preserve Pi-style prepared
  arguments, legacy `file_path` compatibility where supported, file-mutation
  queueing, and result details.
- Latest tool-update increment: SDK tools can now expose a Go-native
  `ExecuteWithUpdates` callback. `AgentSession` emits Pi-style
  `tool_execution_update` public and protocol lifecycle events for partial
  tool results, the built-in bash tool streams those updates through the SDK
  boundary, and the live TTY renders partial tool results with
  `ToolRenderContext.IsPartial` before replacing them with the final result.
- Latest extension-diagnostics increment: loaded-resource startup/reload output
  now includes Pi-style warnings when a package or extension slash command
  conflicts with a built-in interactive command, explaining whether it is
  skipped from autocomplete or reachable through a disambiguated invocation;
  extension shortcut conflict/override warnings are reported in the same
  `[Extension issues]` section.
- Latest keybindings increment: default TTY app hotkeys, extension shortcut
  activation, and shortcut diagnostics now use the effective keybinding config
  (Pi-style defaults plus user `keybindings.json` overrides) instead of a
  hard-coded shortcut table. The default protocol keybinding map now includes
  Pi's reserved app/editor/select actions for extension conflict checks.
  CLI startup now runs the Go-native keybinding migration before dispatching
  normal modes, matching Pi's startup migration shape without introducing
  TypeScript migration state.
  `/hotkeys` now renders its Navigation/Editing/Other tables from the active
  resolved keybindings and appends extension-registered shortcuts, and the
  `/scoped-models` selector applies the effective `app.models.*` bindings for
  save, enable-all, clear-all, provider toggling, and model reordering while
  preserving Pi-compatible fallback keys. `/tree` now applies the effective
  `app.tree.*` bindings for fold/unfold, filters, filter cycling, and label
  timestamp toggling, and renders its overlay hints from the same resolved
  keys. The `/resume` session selector now also consumes effective
  `app.session.*` bindings for sort, named filtering, path display, rename,
  delete, and non-invasive delete while keeping Pi-compatible fallbacks. The
  default TTY host now also consumes user-bound `app.session.new`,
  `app.session.tree`, `app.session.fork`, and `app.session.resume` actions to
  open the same `/new`, `/tree`, `/fork`, and `/resume` flows as Pi.
  `/login` and `/logout` provider selectors now use the active TUI
  select/input keybindings for move, select, cancel, and clear-search
  behavior, with dynamic hints. Protocol-backed select/input/editor dialogs
  also render active TUI key hints, use `app.tools.expand` for overlay tool
  expansion, and use `app.editor.external` for external-editor handoff.
  Startup compact/expanded help and the scoped-model save header now render
  from the same active keybinding sources.
- Latest TUI anchoring increment: `gi-tui` no longer assumes that a
  non-clearing first render starts at terminal row 1. Diff renders and hardware
  cursor updates now use relative movement when the frame origin is not
  anchored by a clear/home cycle, preventing editor input or slash autocomplete
  from being redrawn at the top of the terminal in fallback startup paths.
- Latest autocomplete-resource increment: prompt templates and skill commands
  now carry source metadata into the live slash command list. The default TTY
  prefixes project/user/package command descriptions with Pi-style source tags
  such as `[p]`, parses git package sources into `git:host/path@ref` display
  tags, and prompt template `argument-hint` frontmatter is preserved for
  autocomplete display.
- Latest model-autocomplete increment: the default TTY `/model` slash command
  now exposes Pi-style argument completions from the active model catalog,
  fuzzy-matches provider/model text, and constrains completions to active scoped
  models when a session model scope is configured.
- Latest protocol-autocomplete increment: live TTY protocol autocomplete
  requests now include the active slash command and zero-based argument index
  when the cursor is inside `/command ...`, giving out-of-process packages the
  same command-argument context Pi extensions receive in-process.
- Latest protocol-command increment: descriptor and process extension command
  registration now carries optional `argumentHint` through
  `ProtocolCommandRegistration`, RPC `get_commands`, and live slash
  autocomplete, preserving Pi-style command argument hints across Gi's
  out-of-process package boundary.
- Latest assistant-message increment: live TTY assistant rendering now mirrors
  Pi's non-tool stop-state display: aborted assistant messages render
  `Operation aborted` or a custom abort message, and error messages render
  `Error: <message>` after any partial visible content while tool-call errors
  remain owned by tool execution components.
- Latest custom-message increment: custom message renderers now receive the
  live `expanded` option, so in-process and out-of-process package renderers can
  mirror Pi's compact/expanded custom message behavior under the same Ctrl+O /
  `host.tui.tools_expanded` state as tool output.
- Latest command-collision increment: protocol extension command invocation
  names now follow Pi's collision resolution when duplicate-generated names
  conflict with explicit command names, incrementing suffixes until every
  invocation name remains unique.
- Latest ViewTree event increment: the protocol host now discovers ViewTree
  nodes that subscribe to `tick` or `resize`, emits capped 10 fps ticks and
  terminal-size changes from the live TTY host, routes those events back to the
  owning package process, and allows the process to update the mounted tree
  through normal `host.tui.patch`. High-frequency tick/resize events are
  delivered to listeners without being retained in the long-lived event
  history, avoiding unbounded growth for animated package UI.
- Latest ViewTree validation increment: `host.tui.mount` and `host.tui.patch`
  now validate mounted trees at the protocol boundary, including required node
  types, experimental-node fallback, registered events/colors, stable list item
  IDs, and table column metadata; invalid patches leave the existing mount
  unchanged.
- Latest fork-selector increment: `/fork` without an explicit entry ID and
  double-Escape `fork` mode now mount a Go-native Pi-style user-message
  selector in the live TTY, defaulting to the most recent user message and
  forking before the selected entry while restoring that text into the editor.
- Latest tree-selector increment: `/tree` without an explicit entry ID and
  double-Escape tree mode now mount the Go-native `TreeSelectorComponent` in
  the live TTY instead of a generic dialog. The selector honors the configured
  initial tree filter, supports Pi-style tree navigation keys, no-ops the
  current leaf with `Already at this point`, and switches the active session
  branch through `AgentSession.NavigateTree`. Before navigation, the live TTY
  now runs Pi's `Summarize branch?` choice flow, including custom instructions,
  `branchSummary.skipPrompt`, and Escape cancellation via branch-summary abort.
  It also consumes effective `app.tree.*` keybindings for filters, fold/unfold,
  filter cycling, and label timestamp visibility while preserving old fallback
  sequences.
- Latest message-component increment: live TTY session replay now renders
  `branchSummary`, `compactionSummary`, and `<skill ...>` user messages as
  Pi-style collapsible components tied to the same Ctrl+O tool-output expansion
  state, instead of falling back to raw role-prefixed text.
- Latest live-session increment: the default TTY host now consumes
  `message_start`, `message_update`, `message_end`, `tool_execution_start`, and
  `tool_execution_end` session events directly, so assistant text and tool
  call/result components stream into the live chat instead of appearing only
  after `Prompt` returns. The host also waits briefly for in-flight prompt
  cleanup during shutdown before stopping package processes, avoiding a race
  where live-rendered final text could be visible while package tool-renderer
  RPC was still finishing. The shutdown path now stops the TUI before emitting
  package process `session_shutdown`, matching Pi's guard against extension
  cleanup repainting the final terminal frame.
- Latest extension-error increment: the live TTY host now subscribes to
  protocol runtime `OnError` events and renders Pi-style extension diagnostics
  in the chat transcript, including `Extension "<path>" error: ...` and
  stack-frame detail lines when the protocol error event supplies a stack
  trace. Out-of-process `diagnostic` envelopes with `severity:"error"` now flow
  through that same runtime error channel and protocol schema. Supervised
  package-process stderr now also stays in the bounded startup/shutdown tail
  while each non-empty stderr line is surfaced as a process-source runtime
  diagnostic in the live TTY. A supervised process that exits unexpectedly
  after the protocol handshake now emits a visible `process.exit` diagnostic
  with the exit error and stderr tail before its owned registrations and UI
  mounts disappear.
- Latest process-env increment: supervised package processes now receive
  stable host-provided metadata environment variables for extension id, source
  path/source/scope/origin, and package directory. Package-declared env entries
  can still add ordinary keys, but they cannot override those `GI_EXTENSION_*`
  host metadata values.
- Latest policy-request increment: `host.policy.request` is now a real host
  action instead of a schema-only method. Hosts deny by default when no policy
  requester is configured, and approved grants are appended only to the
  requesting RPC processor's capability set so later host actions can use the
  newly approved scope without granting other extensions. The protocol registry
  now marks this host action as `none` rather than `session.read`, matching the
  runtime rule that requesting policy is allowed before the grant exists while
  the host decision remains the authorization boundary.
- Latest model-list increment: `host.model.list` now returns provider auth
  status alongside available models and current selection, using an injectable
  auth resolver backed by the active model registry when the CLI host has one.
  Package UI can inspect configured/env/runtime provider state through the
  standard host action instead of reaching into registry internals.
- Latest process-exec increment: `host.process.exec` now accepts scoped `cwd`
  and optional `timeoutMillis` parameters. Empty cwd preserves the session cwd
  behavior, relative cwd runs inside the session tree, `../` escapes are
  rejected with `policy_denied`, and over-time local commands preserve partial
  stdout/stderr in a successful result with `exitCode: -1` and `killed: true`,
  matching Pi extension `exec()` result semantics more closely than throwing
  away the process output. The Go executor now starts POSIX commands in their
  own process group, sends graceful termination on timeout, and force-kills the
  process group after a bounded delay; Windows falls back to process kill. RPC
  request cancellation now uses the same termination path, giving Gi extensions
  a Go `context.Context` equivalent to Pi's `AbortSignal`. Local host-process
  execution now also uses explicit stdout/stderr pipes with a bounded post-exit
  read grace, matching Pi's `waitForChildProcess` guard against daemonized
  descendants that inherit stdio handles. Host-action authorization now derives
  scoped `process.exec:<command>` requirements from the requested argv, so
  `process.exec:git` does not accidentally grant arbitrary command execution
  while `process.exec:` remains the explicit broad grant. The official MCP
  stdio adapter now applies the same POSIX process-group cancellation path and
  returns contextual timeout errors with captured stderr, instead of relying on
  default direct child-process cancellation.
- Latest status-coalescing increment: the live TTY host now mirrors Pi's
  back-to-back status-line behavior by updating the previous plain status
  component instead of appending duplicate log noise, while keeping error and
  warning diagnostics as separate transcript entries.
- Latest shortcut-error increment: extension shortcut handler failures now
  surface through the live TTY transcript as Pi-style `Shortcut handler error`
  diagnostics instead of a generic shortcut failure status.
- Latest package-UI lifecycle increment: supervised process packages now keep
  ownership of their `host.tui.mount` ViewTree mounts and automatically unmount
  those widgets, overlays, headers, footers, and editor-slot components when
  the owning process stops, is killed, or exits. This matches Pi's custom
  component dispose/restore semantics without exposing Pi's private in-process
  component API. The RPC processor also rejects cross-process `host.tui.patch`
  and `host.tui.unmount` calls against mounts the caller does not own, and
  denies `host.tui.mount` attempts that would replace another owner by reusing
  its `mountId`. Process-owned slash commands, tools, shortcuts, autocomplete
  providers, flags, and renderers are also removed from the extension runtime
  on shutdown so dead process handlers do not remain discoverable. Process
  startup diagnostics now retain a bounded stderr tail when an extension exits
  before the `hello` handshake or hangs until the handshake timeout, and
  shutdown timeout diagnostics now report that the process was killed instead
  of silently treating a successful kill as a clean shutdown. Package processes
  now also use the same POSIX process-group setup and force-kill path as local
  host process execution, so timeout/cancellation cleanup covers descendants
  spawned by an out-of-process package.
- Latest session-selector increment: `/resume` without an explicit path now
  passes the resolved keybinding config into `SessionSelectorComponent`. The
  selector renders dynamic Pi-style hints, cycles threaded/recent/relevance
  sort modes, toggles named-session filtering, toggles path display, and routes
  rename/delete/non-invasive-delete through `app.session.*` actions while
  preserving the old direct key sequences.
- Latest auth-selector increment: `/login` without a provider now mounts a
  Go-native searchable provider selector for API-key setup guidance, and
  `/logout` without a provider mounts the same selector over stored
  credentials before removing one from Gi auth storage. Gi still does not copy
  Pi's OAuth browser flow; selected login providers render Gi's API-key,
  environment, `auth.json`, and `models.json` guidance. The provider selector
  consumes active TUI keybindings for navigation, confirm, cancel, and
  clear-search behavior instead of hard-coded terminal sequences.
- Latest protocol-dialog increment: protocol-backed select/input/editor
  overlays now render footer hints from the active TUI keybinding manager,
  consume `app.tools.expand` for in-overlay tool expansion, and consume
  `app.editor.external` for editor-dialog external editor handoff while
  retaining legacy Ctrl+O/Ctrl+G fallback behavior.
- Latest compaction-queue increment: manual `/compact` now marks the session as
  compacting while the summarizer runs, keeps the live editor active, queues
  non-extension input for after compaction, displays pending steering/follow-up
  messages, restores those queued messages with Alt+Up, flushes the queue after
  compaction including follow-up delivery, replays flushed-turn output into the
  live chat, and lets Escape request compaction cancellation.
- Latest bash-interaction increment: live TTY `!`/`!!` commands now use an
  injectable Go-native bash operation boundary, render bash components in the
  pending-message area while an agent turn is streaming, restore the attempted
  `!` input to the editor when another bash command is already running, and let
  Escape cancel the running bash command before falling through to broader
  agent abort or double-Escape routing. Local bash execution now starts Unix
  shells in their own process group and cancels that group before falling back
  to killing the shell process, covering Pi's detached-child cleanup intent
  with Go-native process lifecycle control.
- Latest protocol increment: out-of-process extensions can set the live terminal
  or window title through `host.tui.title`, gated by the `tui.title`
  capability.
- Latest editor increment: `host.tui.editor` now supports `action:"paste"` so
  packages can trigger Gi's real editor paste semantics instead of plain text
  insertion.
- Latest loader increment: out-of-process extensions can configure the live
  working loader through `host.tui.working`, gated by `tui.working`.
- Latest thinking-label increment: out-of-process extensions can set the label
  for hidden thinking blocks through `host.tui.thinking_label`, gated by
  `tui.thinking_label`.
- Latest capability increment: `host.tui.mount` is now slot-capability aware, so
  footer/header/overlay/editor mounts require the matching `tui.*` capability
  instead of a broad widget grant.
- Latest descriptor ViewTree increment: `.gi.json` descriptor extensions can
  declare static `viewTrees` mounts. Gi records those mounts in the protocol
  runtime, applies slot-capability checks, and mounts them automatically when a
  live TTY `ViewTreeHost` is bound, so simple package-provided UI does not need
  a process runner.
- Latest status increment: `host.tui.status` now updates both the ViewTree
  footer mount and the live footer data provider used by custom footer
  components.
- Latest status lifecycle increment: supervised package processes now own the
  `host.tui.status` keys they set, and process stop/unexpected exit clears both
  the `status:<key>` ViewTree footer mount and the live footer status provider.
  This mirrors Pi's extension UI reset behavior without exposing in-process
  footer internals to package processes.
- Latest TUI state lifecycle increment: supervised package processes now own
  terminal title, working loader state, and hidden thinking labels they set
  through `host.tui.title`, `host.tui.working`, and
  `host.tui.thinking_label`; process stop/unexpected exit restores each surface
  to the host default, matching Pi's `resetExtensionUI()` semantics.
- Latest theme increment: out-of-process extensions can list, inspect, and
  switch host themes through `host.tui.theme`, gated by `tui.theme`; Gi returns
  serializable metadata and keeps styling on semantic ViewTree tokens.
- Latest tools-expanded increment: in-process and out-of-process extensions can
  read and change live TUI tool-output expansion through
  `getToolsExpanded`/`setToolsExpanded` or `host.tui.tools_expanded`.
- Latest terminal-input increment: process extensions with
  `tui.terminal_input` can observe asynchronous raw terminal input events; Gi
  intentionally keeps consume/transform semantics on focused ViewTree or
  in-process host paths to avoid blocking the core editor on process RPC.
- Latest shortcut increment: process extensions with `shortcuts.register` can
  register package shortcuts through `register_shortcut`; the live TTY host
  resolves them against reserved Pi-style app shortcuts, consumes matching input
  before non-reserved app/editor handlers, and emits `shortcut.invoke` back to
  the owning package process.
- Latest flag increment: process extensions can register CLI flags through
  `register_flag`; Gi normalizes leading `--`, preserves default values in the
  shared protocol runtime, and exposes them through the same `Flags` /
  `FlagValue` APIs as descriptor extensions. CLI unknown long flags are now
  applied to descriptor flags immediately and kept pending so package processes
  can consume matching values when they later call `register_flag`.
- Latest dialog increment: protocol-backed select/input/editor dialogs keep the
  Pi extension-selector behavior that `Ctrl+O` still toggles tool-output
  expansion while a focused extension overlay is open; extension dialog
  `timeout` options also auto-cancel with a live countdown title. Notification
  dialogs now preserve Pi's `info` / `warning` / `error` severity mapping.
- Latest session-action increment: out-of-process packages can request
  `host.session.action` `reload` through the runtime reload callback, and
  `navigate_tree` backed by Gi's Go-native `AgentSession.NavigateTree`
  implementation.
- Latest startup-composition increment: the live TTY host now defers its first
  render during clear-screen startup until resources and providers are wired,
  renders a compact Pi-style startup help header without a duplicate fixed
  title, uses Pi-style flow layout instead of forcing editor/footer rows to a
  synthetic terminal bottom, and lets Ctrl+O expand both the startup help and
  loaded-resource listings in addition to tool output.
- Latest debug increment: the live TTY host now supports Pi-style debug
  snapshots via Shift+Ctrl+D and hidden `/debug`, writing
  `<agentDir>/gi-debug.log` with terminal dimensions, rendered line visible
  widths, and session messages as JSONL.
- Latest shutdown-signal increment: the live TTY host now installs Go-native
  SIGTERM/SIGHUP shutdown watchers on Unix-like platforms. SIGTERM routes
  through the same stop/dispose path as Pi's graceful signal handler, drains
  pending terminal input before stopping the TUI on both signal-triggered and
  normal `/quit` shutdown, and restores the terminal cursor. SIGHUP and
  terminal write failures from both TUI rendering and direct host terminal
  operations that map to Pi's dead-terminal codes (`EIO`/`EPIPE`/`ENOTCONN`)
  now use a Go-native no-render shutdown path so the host disposes
  protocol/package/runtime state without attempting final cursor or layout
  writes to a detached terminal. `AgentSession.Dispose` aborts an
  in-flight bash execution so host shutdown does not leave shell work running
  after the TUI exits.
- Latest terminal-progress increment: the live TTY host now honors Pi's
  `terminal.showTerminalProgress` setting during agent and compaction events,
  driving Go-native OSC 9;4 progress sequences through `gitui.Terminal` and
  clearing progress on event completion or host shutdown.
- Latest session-resume increment: startup `--resume` / `-r` in an interactive
  TTY now opens the same Go-native `SessionSelectorComponent` before creating
  the runtime session; selection is converted into an explicit session path
  and cancellation exits successfully with `No session selected`. `/resume`
  without arguments also mounts that selector in the live TTY, with search,
  current/all scope loading, rename/delete hooks, cancellation, and selected
  session switching; `/resume <path>` keeps the direct path flow.
- Latest official-package increment: official protocol packages now surface
  stateful command UI through display-aware custom-message renderers and mount
  command-scoped ViewTree widgets/footer segments when a live TTY host is
  bound, proving package-provided custom UI without private core APIs.
  `gi-plan-mode` exposes `plan_update`/`plan_read` tools backed by Gi's Go plan
  parser and renders current plan items through `/plan`; `/plan` also enters
  read-only tool mode through the canonical host policy, while `/plan-review`
  records a review state and `plan_update`/`plan_read` have official tool
  renderers for plan items. `gi-approval-gate`
  records structured pending approval requests, approve/deny/rewrite
  decisions, and renders them through `/approvals`. The `gi-todo-widget`
  command reads the latest todo tool state, `gi-tools-ui` shows and patches
  active host tools in the live TTY, and `gi-git-guard` records guard checks
  with read-only git status data without private core APIs; when confirmation
  is requested, it routes a confirm dialog through the same host-action dialog
  boundary and records the confirmed/declined/cancelled decision.
  `gi-approval-gate` now renders approval action, command, risk, diff, reason,
  and rewrite replacement context through official message renderers and
  `approval_gate_request`/`approval_gate_decide` tool renderers.
  `gi-powerline-footer` now reads the bound host session to render model,
  thinking, git branch, context usage, and active-tool count in both its command
  custom message and footer ViewTree mount. `gi-todo-widget` also registers
  official tool renderers for `todo_read`/`todo_write` and rehydrates command
  UI from the currently bound session after runtime session switches.
  `gi-subagents` routes `subagent_spawn` and `subagent_abort` through
  canonical `host.agent.spawn` / `host.agent.abort` host-action requests,
  returning the child session/result payload and rendering completed or aborted
  subagent status through its official command UI; `subagent_spawn` can also
  include an explicitly bounded parent context window in its result/custom
  state, and official subagent tool renderers surface spawn/abort status plus
  parent context. `subagent_abort` can cancel a currently running official child
  session through the same host child registry. When the parent session is
  intentionally in-memory, child
  spawn now stays in-memory as well instead of writing orphan `.jsonl` files
  into the project directory.
  `gi-mcp-adapter` can run a configured stdio MCP server through
  `initialize`/`tools/list`/`tools/call`, capture MCP
  `notifications/progress` and `notifications/tools/list_changed`, register
  discovered MCP tools as Gi dynamic tools through the protocol runtime, record
  discovered tools or call results as session custom state, and render the
  latest MCP status through `/mcp`; the protocol registry now models this as
  `process.stdio:<scope>` rather than one-shot `host.process.exec`, and the
  runtime/policy capability validator accepts that scoped capability. The stdio
  server is host-supervised with process-group cancellation and
  stderr-preserving timeout diagnostics. Registry tests now verify that
  host-actions and official packages only reference known capabilities and host
  actions, that official package descriptors self-declare the exact
  registry-required capabilities, that every registered capability is accepted
  by the runtime, and that the host-action schema method enum matches the
  host-action registry.
  Runtime tests also compare every host-action registry capability declaration
  against `hostActionRequiredCapabilities`, so protocol drift is caught at the
  Go test gate. The
  `ProtocolExtensionRuntime` can now bind
  a `ViewTreeHost`, and live TTY slash-command tests verify official `/plan`,
  `/subagents`, `/mcp`, `/tools`, `/todo`, `/footer`, `/approvals`, and
  `/git-guard` commands consume the same production TTY runtime as RPC package
  components. The git-guard coverage exercises the full protocol chain from
  official tool execution through a live TUI confirm dialog, session custom
  state, and command UI rendering.
- Latest package-process integration increment: one supervised stdio package
  process is now exercised inside the production live TTY while it registers a
  command, shortcut, autocomplete provider, message renderer, tool, and tool
  renderer; the same process drives a lifecycle ViewTree mount, a select dialog,
  editor mutation, message replay rendering, autocomplete insertion, and
  tool/tool-renderer request-response callbacks without private host APIs.
- Latest ViewTree event increment: committed TTY theme switches now dispatch
  `theme_change` with `name` and `preview` data to subscribed ViewTree nodes,
  mount/unmount emits `visibility_change` with `visible`/`reason` data, and
  supervised package processes receive these events through the same owned-mount
  `tui.event` channel as `tick` and `resize`. Package-process mount ownership
  is pre-registered before the host action runs, so synchronous mount-time
  ViewTree events are delivered to the process that requested the mount.
  Focused ViewTree components now also emit semantic `textInput`, `submit`,
  `cancel`, `change`, and `select` events in addition to raw `key` events;
  package-process tests verify the input semantic events can patch a mounted
  editor component through the same RPC channel.
- Latest lifecycle-event increment: the Go runtime now emits the protocol
  registry's `session_switch` event after a replacement session is bound and
  before `session_start`, with `targetSessionFile` and `previousSessionFile`
  populated for new, resume, and fork flows. The live TTY host now rebinds its
  package-process RPC session host during those switches, fans `session_switch`
  and the replacement `session_start` out to already-running package processes,
  and re-subscribes the visible TUI to the replacement session's live message
  events.
- Latest in-process component increment: trusted Go code can now register
  explicit `gi-tui` slot components through `InProcessUIRegistry`. The live TTY
  host passes session/runtime/ViewTree context, merges components into
  header/footer/widget slots, recovers render panics as visible diagnostics,
  and calls disposer callbacks on shutdown; third-party package UI still goes
  through ViewTree/RPC by default.

## Status Meanings

- `已有`: implemented or behaviorally equivalent in Gi with tests.
- `协议化`: intentionally implemented through Gi package/extension/RPC protocol boundaries.
- `Go-native`: intentionally different implementation using Go APIs or explicit dependency injection.
- `不适用`: Pi-specific Node/Bun/browser/vendor/branding surface excluded from Gi core.
- `待补强`: source-level gap marker for future audits; no current source rows use
  this marker.

## Resolved Source-Level Gaps

| Former gap | Pi source | Gi state | Current disposition |
|---|---|---|---|
| Default interactive app launch | `main.ts`, `modes/interactive/interactive-mode.ts`, many interactive components | `RunCLI` handles print, JSON, RPC, package commands, export, list-models, non-TTY fallback, scoped `--models` / settings `enabledModels` resolution, startup `--resume` / `-r` session selection in interactive TTYs, and the default TTY host covers startup, flow layout, editor submit, model/thinking controls, sessions, selectors, resource reload, bash, export/share/import, compaction, signal handling, and live session tree/fork/resume flows | `已有 / Go-native`; remaining differences are Gi product choices or protocolized package surfaces, not open source-port gaps |
| Full interactive component composition | `modes/interactive/components/*.ts` | Go-native components cover the core interactive flows, while trusted in-process `gi-tui` slot/editor/overlay components support size-aware rendering, focus/input/key-release forwarding, one-shot custom editor/overlay workflows, `SetExpanded(bool)` propagation, active-editor host-action routing, panic recovery, disposer lifecycle, and dynamic keyed refresh/removal after startup | `已有 / Go-native / 协议化`; package-provided UI runs through ViewTree/RPC, and direct in-process components are reserved for trusted Go code |
| Package-provided custom UI runtime | extension/custom component pieces under `core/extensions` and interactive components | `ViewTreeHost` can mount, patch, unmount, render, query protocol slot aliases, track focus, dispatch input/focus/tick/resize/theme/visibility events, manage keyed status text, and notify the live TTY host so slot, overlay, and editor-slot content refreshes at runtime. `gi-ext-rpc@1` exposes commands, tools, sessions, model/thinking, fs/exec policy, dialogs, editor actions, autocomplete, ViewTree, message renderers, tool renderers, lifecycle, and bounded supervised process execution. The integrated package-process live TTY test combines lifecycle mount, dialog/editor host actions, message rendering, autocomplete, command/shortcut registration, and tool/tool-renderer callbacks from the same process; official packages exercise stateful plan/approval/todo/tools/git-guard/subagent/powerline/MCP workflows through the same host | `协议化`; future official-package richness is product backlog, while the source parity boundary is implemented and tested |
| Pi TypeScript SDK surface | `core/sdk.ts`, `core/extensions/*` | Replaced by Go protocol runtime and descriptor loading | Keep protocol conformance as acceptance gate; do not add TS/Node private API compatibility |
| npm/Bun self-update and install surfaces | `package-manager-cli.ts`, `config.ts`, `utils/tools-manager.ts`, `bun/*` | Intentionally rejected or replaced by Go/local/git/protocol package sources | Keep rejection tests and docs; no npm backend unless the product decision changes |

## Source Disposition Checklist

### Root and CLI

| Pi files | Disposition | Gi evidence |
|---|---|---|
| `index.ts`, `cli.ts`, `main.ts` | `已有 / Go-native` for CLI dispatch, parser diagnostic reporting, version/help output including descriptor extension flags, session-selected runtime cwd resolution, interactive missing-session-cwd prompt, print/RPC/export/package/list-models, a non-TTY single-turn fallback, and full default TTY workflow composition through Gi's Go-native host plus protocol package surfaces | `cmd/gi/main.go`, `gi-coding-agent/cli.go`, `cli_interactive_mode.go`, `cli_missing_cwd.go`, `cli_interactive_tui.go`, `cli_print_mode.go`, `rpc_mode.go`, `cli_session_manager.go` |
| `config.ts`, `migrations.ts` | `已有 / Go-native`; legacy auth-to-`auth.json`, keybinding, root-session relocation, managed `tools/` to `bin/`, legacy `commands/` to `prompts/`, and deprecated extension-dir startup warnings are implemented, while npm/Bun install detection is excluded | `config.go`, `settings_manager.go`, `keybindings.go`, migration tests |
| `package-manager-cli.ts` | `已有 / 协议化`; `gi config` opens the package/top-level resource selector, and npm is intentionally rejected | `cli.go`, `cli_config.go`, `cli_config_test.go`, `package_manager.go`, `package_command_paths_test.go`, `protocol/spec/schemas/package-manifest.schema.json` |
| `bun/cli.ts`, `bun/register-bedrock.ts` | `不适用`; Go binary has no Bun bootstrap | `cmd/gi/main.go`, provider registration in Go packages |
| `bun/restore-sandbox-env.ts` | `已有` | `internal/sandboxenv`, `restore_sandbox_env.go` facade, `restore_sandbox_env_test.go` |
| `cli/args.ts`, `initial-message.ts`, `file-processor.ts`, `list-models.ts`, `session-picker.ts`, `config-selector.ts` | `已有`; config selector resource toggles are Go-native package/top-level filter APIs and the default TTY `/resources` selector rather than copied TS UI code | `args.go`, `initial_message.go`, `file_arguments.go`, `list_models.go`, `session_selector.go`, `package_manager.go`, `cli_interactive_tui.go` |

### Utilities

| Pi files | Disposition | Gi evidence |
|---|---|---|
| `utils/ansi.ts`, `frontmatter.ts`, `paths.ts`, `pi-user-agent.ts`, `shell.ts`, `sleep.ts` | `已有 / Go-native`; frontmatter covers quoted scalars, folded/literal block descriptions, nested metadata skipping, and invalid YAML diagnostics; shell cleanup uses process-group cancellation for local bash instead of Pi's TypeScript child-process tracker | `internal/ansiutil`, `internal/frontmatter`, `internal/pathutil`, `utils.go`, `utils_test.go`, `gi-agent-core/harness/frontmatter.go`, harness skill/prompt tests, `bash_executor.go`, `bash_process_unix.go`, `bash_process_windows.go` |
| `utils/git.ts`, `version-check.ts`, `changelog.ts` | `已有 / Go-native`; Gi uses GitHub/Gi release metadata and legacy aliases | `internal/gitsource`, `git.go`, `internal/versioncheck`, `version_check.go`, `internal/changelog`, `changelog.go`, utility tests |
| `utils/mime.ts`, `image-resize.ts`, `image-convert.ts`, `exif-orientation.ts`, `photon.ts`, `clipboard-image.ts` | `已有 / Go-native`; implemented through Go image helpers and platform command fallbacks | `image_resize.go`, `clipboard_image.go`, `block_images_test.go` |
| `utils/clipboard.ts`, `clipboard-native.ts`, `child-process.ts` | `已有 / Go-native` | `clipboard.go`, `bash_executor.go`, related tests |
| `utils/syntax-highlight.ts`, `html.ts` | `已有`; minimal renderer focused on Pi export/test behavior | `syntax_highlight.go`, `export_html.go` |
| `utils/fs-watch.ts`, `tools-manager.ts`, `highlight-js-lib-index.d.ts` | `不适用 / Go-native`; no Node FSWatcher/downloaded JS tool manager in Gi core | `internal/fswatch`, `footer_data_provider.go`, `package_manager.go`, protocol package docs |

### RPC and Modes

| Pi files | Disposition | Gi evidence |
|---|---|---|
| `modes/rpc/jsonl.ts`, `rpc-mode.ts`, `rpc-client.ts`, `rpc-types.ts` | `已有 / Go-native` | `rpc_jsonl.go`, `rpc_mode.go`, `rpc_client.go`, `rpc_session_host.go` |
| `modes/print-mode.ts` | `已有` | `print_mode.go`, `cli_print_mode.go`, `print_mode_test.go` |
| `modes/index.ts` | `已有 / Go-native` | `cli.go` |

### Interactive TUI

| Pi files | Disposition | Gi evidence |
|---|---|---|
| `modes/interactive/interactive-mode.ts` | `已有 / Go-native / 协议化`; the Go-native TTY host covers startup clear, deferred first render, startup `--resume` / `-r` session selection, startup changelog/update notices, session model restore/fallback startup warnings, compact/expanded startup help, loaded-resource startup sections with quiet diagnostics and Ctrl+O expansion, Pi-style `/reload` transient editor feedback, Pi-style header/chat/editor/footer flow layout, dynamic cwd/model/context footer, initial prompts, editor submit, Pi-style Ctrl+C/Ctrl+D/Escape stop/abort keys, Pi-style streaming Enter steering queue, Alt+Enter follow-up queue, pending queue display, Alt+Up restore-to-editor, Shift+Tab/Ctrl+P/Ctrl+Shift+P/Ctrl+L model-thinking hotkeys, Ctrl+O tool-output expansion plus extension-visible `host.tui.tools_expanded`, Ctrl+T thinking-block visibility toggle, Ctrl+G external editor handoff, Ctrl+Z Unix suspend/restore, Ctrl+V clipboard-image paste-to-temp-file, Shift+Ctrl+D / hidden `/debug` debug snapshots, SIGTERM graceful terminal restoration plus SIGHUP/dead-terminal no-render shutdown, OSC 9;4 terminal progress for agent and compaction events when enabled, scoped model cycling from `--models` / settings `enabledModels`, built-in `/settings`/`/theme`/`/thinking`/`/model`/`/models`/`/scoped-models`/`/queue`/`/session`/`/hotkeys`/`/changelog`/`/export`/`/share`/`/import`/`/resume` picker plus `/resume <path>`/`/fork`/`/tree`/`/name`/`/new`/`/compact`/`/copy`/`/clone`/`/login`/`/logout`/`/reload`/`/quit` slash commands, cancellable `/share` gist creation, interactive `/model`, `/models`, `/scoped-models`, `/theme`, and `/thinking` searchable select-dialog routing, searchable `/settings` SettingsList routing with Pi's terminal/image/transport/editor/warnings controls plus warnings/theme/thinking submenus, `!`/`!!` bash execution including pending-area rendering while streaming, second-bash editor restoration, Escape cancellation, unsent bash-draft clearing, user-bound `app.session.new`/`app.session.tree`/`app.session.fork`/`app.session.resume` action routing, `user_bash` extension result interception, dynamic ViewTree slots/overlays/editor-slot replacement, process-observable `tui.terminal_input`, capped ViewTree `tick` and `resize` events, protocol-backed notify/confirm/select/input/editor dialogs, display-aware custom message renderer/fallback behavior, live `message_*` / `tool_execution_start` / `tool_execution_update` / `tool_execution_end` event rendering, custom tool call/result renderers, aborted/error tool-call rendering, an integrated supervised package-process live TTY workflow, and official package workflows for plan/approval/todo/tools/git-guard/subagent/powerline/MCP | `interactive_mode.go`, `interactive_status.go`, `cli_interactive_tui.go`, focused and integrated live TTY tests |
| `components/assistant-message.ts`, `user-message.ts`, `custom-message.ts`, `skill-invocation-message.ts`, `branch-summary-message.ts`, `compaction-summary-message.ts` | `已有` for message rendering and session-context behavior | `message_components.go`, `interactive_mode.go`, `export_html.go` |
| `components/tool-execution.ts`, `bash-execution.ts`, `diff.ts`, `visual-truncate.ts` | `已有`; updated pending tool behavior uses in-place indexed updates when available, edit diffs use Pi-style line colors and single-line intra-line inverse highlighting, and tool image blocks honor `showImages` / `imageWidthCells` with inline protocol rendering or image fallback text | `tool_execution_component.go`, `diff_render.go`, `internal/diffrender`, `bash_execution.go`, `edit_tool_definition.go` |
| `components/model-selector.ts`, `scoped-models-selector.ts`, `thinking-selector.ts`, `settings-selector.ts`, `config-selector.ts`, `oauth-selector.ts`, `theme-selector.ts`, `show-images-selector.ts` | `已有` for tested behavior including default `/scoped-models`, `/resources`, `/theme`, `/model` argument autocomplete with scoped-model filtering, and Go-native `/login`/`/logout` provider selector TTY routing; richer host wiring remains tied to the remaining default interactive gap | `model_selector_component.go`, `oauth_selector.go`, `interactive_status.go`, `cli_interactive_tui.go`, settings/resource/auth selector tests |
| `components/session-selector.ts`, `session-selector-search.ts`, `tree-selector.ts`, `user-message-selector.ts` | `已有` for tested selection/search/tree/user-message semantics, including Pi-style Ctrl+N named-session filter toggling, startup `--resume` and live `/resume` TTY mounting through `SessionSelectorComponent`, live `/tree` / double-Escape tree mounting through `TreeSelectorComponent`, and live `/fork` / double-Escape fork mounting through `UserMessageSelectorComponent` | `session_selector.go`, `session_selector_search.go`, `tree_selector.go`, `user_message_selector.go`, `cli_resume_selector.go`, `cli_interactive_tui.go` |
| `components/footer.ts`, `countdown-timer.ts`, `bordered-loader.ts`, `dynamic-border.ts`, `keybinding-hints.ts` | `已有 / Go-native` | `footer.go`, `footer_data_provider.go`, `gi-tui` helpers |
| `components/custom-editor.ts`, `extension-editor.ts`, `extension-input.ts`, `extension-selector.ts`, `login-dialog.ts` | `协议化 / Go-native`; protocol `host.tui.dialog` covers notify/confirm/searchable-select/input/editor overlays including external-editor handoff and live process-driven select dialogs, ViewTree `editor` mounts can replace the default editor region with focus/key events, `host.tui.editor` exposes text, submit, cursor, focus, custom-editor-active state, paste semantics, and autocomplete context to both in-host and stdio package processes, process editor set/insert/paste/submit is verified against the live TTY host, `register_autocomplete_provider` stores priority-ordered suggestion providers that the live default TUI editor adapts both before and after TUI startup, and Gi login UI is a Go-native provider credential guidance flow rather than Pi's OAuth dialog copy | `protocol_extension_runtime.go`, `host_actions.go`, `protocol_extension_process_test.go`, `cli_tui_dialog.go`, `cli_interactive_tui.go`, `viewtree.go`, `internal/authguide`, `auth_guidance.go`, `protocol/spec` |
| `components/armin.ts`, `daxnuts.ts`, `earendil-announcement.ts`, `assets/clankolas.png` | `不适用`; Pi branding/mascot UI is not copied into Gi core | no Gi equivalent by design |
| `theme/*.json`, `theme.ts`, `theme-schema.json` | `已有 / Go-native / 协议化`; theme behavior is represented as settings, resource discovery, export helpers, and `host.tui.theme` metadata/switching for package processes | `interactive_status.go`, `theme_export.go`, `resource_loader.go`, `host_actions.go`, `cli_interactive_tui.go`, `protocol/spec` |

### Core Runtime

| Pi files | Disposition | Gi evidence |
|---|---|---|
| `core/agent-session.ts`, `agent-session-runtime.ts`, `agent-session-services.ts`, `event-bus.ts`, `messages.ts` | `已有` for tested runtime, queue, event, message lifecycle, and partial tool-result update behavior | `agent_session_runtime.go`, `agent_session_runtime_events.go`, runtime tests |
| `core/session-manager.ts`, `session-cwd.ts`, `source-info.ts`, `timings.ts` | `已有 / Go-native`; runtime startup uses selected-session cwd for resources, reports missing stored cwd values, prompts for an interactive cwd override before runtime creation, and supports `GI_TIMING`/`PI_TIMING` startup timing output | `session_manager.go`, `internal/sessioncwd`, `session_cwd.go`, `internal/startuptiming`, `startup_timings.go`, `cli_print_mode.go`, `cli_missing_cwd.go`, session manager suites |
| `core/model-registry.ts`, `model-resolver.ts`, `provider-display-names.ts`, `auth-storage.ts`, `auth-guidance.ts`, `defaults.ts`, `settings-manager.ts`, `resolve-config-value.ts` | `已有 / Go-native`; `model-resolver.ts` and auth guidance/warning helpers now map to focused internal packages plus root compatibility facades | `model_registry.go`, `model_resolver.go`, `internal/modelresolver/model_resolver.go`, `auth_storage.go`, `internal/authguide`, `internal/authwarning`, `auth_guidance.go`, `anthropic_warning.go`, `settings_manager.go` |
| `core/package-manager.ts`, `resource-loader.ts`, `diagnostics.ts`, `skills.ts`, `prompt-templates.ts`, `slash-commands.ts` | `已有 / 协议化`; `.gi` paths and protocol package manifests replace Pi npm/TS discovery, CLI `--no-*` resource flags still preserve explicit CLI resource paths, and `resources_discover` now merges dynamic skill/prompt/theme paths during startup/reload | `package_manager.go`, `resource_loader.go`, `protocol_package_resolver.go`, prompt/skill tests |
| `core/sdk.ts`, `extensions/index.ts`, `extensions/loader.ts`, `extensions/runner.ts`, `extensions/types.ts`, `extensions/wrapper.ts` | `协议化`; no in-process TS SDK compatibility; `user_bash` is represented as a serializable lifecycle event with optional `bashResult`, and process packages request it through `bash.intercept` rather than passing Pi's JS function handles across RPC | `protocol_extension_runtime.go`, `protocol_extension_descriptor.go`, `protocol_extension_process.go`, `host_actions.go`, `viewtree.go`, `protocol/spec` |
| `core/system-prompt.ts`, `compaction/index.ts`, `compaction/branch-summarization.ts`, `compaction/compaction.ts`, `compaction/utils.ts` | `已有`; default prompts include Gi-native documentation pointers for Gi/protocol/package/extension questions, loaded AGENTS/CLAUDE context files, custom replacement prompts, and appended prompt sections. Compaction now matches Pi's default reserve/keep token settings, strict trigger threshold, `totalTokens` precedence, hook-generated summary exclusion, read-only vs modified file-list computation, exact structured history/turn-prefix prompt text, branch-summary prompt shape, empty-branch result, and XML file-operation tags. | `system_prompt.go`, `system_prompt_docs.go`, `resource_loader.go`, `gi-agent-core/harness/compaction.go`, `branch_summary.go`, compaction tests, `docs/pi-parity/coding-agent-file-map.md` |
| `core/bash-executor.ts`, `exec.ts`, `output-guard.ts` | `已有 / Go-native`; explicit writers replace global stdout patching, and bash output uses a bounded accumulator plus temp-file preservation for large output | `bash_executor.go`, `internal/tooloutput/output_accumulator.go`, `bash_output_accumulator.go`, `stdout_cleanliness_test.go` |
| `core/footer-data-provider.ts`, `keybindings.ts`, `telemetry.ts` | `已有 / Go-native` | `footer_data_provider.go`, `keybindings.go`, `internal/telemetry`, `telemetry.go` |

### Core Tools

| Pi files | Disposition | Gi evidence |
|---|---|---|
| `tools/read.ts`, `write.ts`, `edit.ts`, `edit-diff.ts`, `file-mutation-queue.ts`, `tool-definition-wrapper.ts`, `render-utils.ts`, `truncate.ts` | `已有 / 部分协议化`; Go SDK/protocol wrappers preserve tool schemas, prepare hooks, execution metadata, and provider-facing active tool definitions instead of copying Pi's TypeScript wrapper API. `file-mutation-queue.ts` now maps to a focused `internal/toolqueue` package plus root facade. `read` now preserves Pi's first-line-over-50KB guard and expanded truncation metadata. `write` execution/rendering is present, but Pi's incremental partial highlight cache is still a source-shape difference. | `file_tools.go`, `edit_tool_definition.go`, `internal/toolqueue/file_mutation_queue.go`, `file_mutation_queue.go`, `sdk_session.go`, read/write/edit tests, `docs/pi-parity/coding-agent-file-map.md` |
| `tools/bash.ts`, `grep.ts`, `find.ts`, `ls.ts`, `path-utils.ts`, `output-accumulator.ts`, `index.ts` | `已有 / Go-native`; bash live updates and final results use bounded tail snapshots plus temp-file preservation, with `output-accumulator.ts` now mapped to `internal/tooloutput` plus a root compatibility facade | `bash_tool.go`, `internal/tooloutput/output_accumulator.go`, `bash_output_accumulator.go`, `search_tools.go`, `file_tool_definitions.go`, tools tests |

### Export HTML

| Pi files | Disposition | Gi evidence |
|---|---|---|
| `core/export-html/ansi-to-html.ts` | `已有`; Go converter now matches Pi's styled SGR handling, HTML escaping, empty-line rendering, and ANSI-only spacing-line trimming for exported tool output | `export_html.go`, `export_html_whitespace_test.go` |
| `core/export-html/index.ts`, `tool-renderer.ts`, `template.css` | `部分已有 / Go-native`; file export, safe HTML helpers, custom result HTML conversion, and whitespace CSS are represented, but Gi still has a smaller generated HTML shell than Pi's full static export app | `export_html.go`, export tests |
| `core/export-html/template.html`, `template.js`, `vendor/highlight.min.js`, `vendor/marked.min.js` | `缺口或产品决策点`; Gi does not yet port Pi's rich browser-side static viewer, Markdown renderer, or syntax-highlighter asset pipeline | `docs/pi-parity/coding-agent-file-map.md` |

## Completion Audit Notes

Latest flag-source increment: Pi keeps duplicate extension flags in their
extension records and resolves them with first-wins lookup, so a later duplicate
can become visible after the first source is removed. Gi now preserves duplicate
protocol flag registrations internally, exposes only the first visible flag, and
recomputes the flag value when `RemoveSource` removes that visible owner.

Latest descriptor-conflict increment: Pi reports duplicate extension tools and
flags as resource diagnostics while keeping all extensions loaded. Gi descriptor
loading now follows that shape for protocol packages: conflicting tools and
flags emit discovery errors, but duplicate registrations are preserved so runtime
first-wins and source removal can still reveal later registrations.

Latest renderer-source increment: Pi stores custom message renderers per
extension and resolves the first matching renderer at display time. Gi now
preserves duplicate protocol message renderers internally and rebuilds the
visible first-wins renderer map when a source is removed, so a later package
renderer can become active without a full runtime rebuild.

Latest tool-renderer-source increment: Gi's package-provided tool renderer
protocol now uses the same internal shape as message renderers: duplicate
registrations are retained, the visible map is first-wins, and source removal
can reveal the next renderer for the same tool.

Latest flag-read increment: Pi extensions can call `getFlag(name)` for flags
they registered. Gi now exposes the same source-scoped behavior in-process and
over stdio RPC through `get_flag`, with CLI-provided values applied before the
owning package reads the flag.

Latest handler-source increment: process package shutdown calls `RemoveSource`;
that cleanup now removes lifecycle and input handlers in addition to commands,
tools, providers, renderers, flags, shortcuts, autocomplete providers, and
ViewTree mounts, avoiding stale callbacks into stopped packages.

Latest provider-source increment: Gi provider registrations are now retained as
source-aware replay records. When a process package is stopped or a source is
removed, only that source's provider contribution is removed; remaining
registrations for the same provider are replayed into the model registry so an
earlier package/provider configuration can become active again.

Latest same-source-registration increment: Pi extension loader stores commands
and tools in per-extension maps, so registering the same command or tool name
again from the same source replaces the previous definition. Gi now mirrors that
for protocol commands and tools while still preserving cross-source duplicate
commands/tools for suffixing, first-wins lookup, and source-removal fallback.

Latest same-source-shortcut-autocomplete increment: same-source shortcut
registration now replaces the previous key binding instead of warning about a
self-conflict, matching Pi's per-extension shortcut map behavior. Gi's
protocol-only autocomplete providers also replace by source/id, preventing
duplicate callbacks when a process re-registers the same provider.

Latest provider-hook increment: the OpenAI Responses transport now honors the
same provider hooks as the other Go providers. `OnPayload` can inspect or
replace the request body before `postSSE`, and `OnResponseStatus` observes the
HTTP status plus response headers before stream/error handling; hook failures
return assistant error streams. This keeps the default OpenAI Responses path
compatible with Pi's `before_provider_request` and `after_provider_response`
extension surface while still using Gi's Go-native provider API.

Latest provider-hook-session increment: print-mode and default live TTY provider
calls now bridge protocol lifecycle handlers into `SimpleStreamOptions.OnPayload`
and `OnResponseStatus`. `before_provider_request` handlers can inspect and
replace the provider payload, while `after_provider_response` handlers observe
status and cloned headers. The protocol event registry and docs now list both
events as first-class lifecycle hooks.

Latest session-info increment: the live TTY `/session` command now renders
Pi-style session detail output instead of a one-line status. It shows session
name, file, ID, active model/thinking level, user/assistant/tool message counts,
token totals, context usage when available, and cost when non-zero.

Latest scoped-model-selector increment: the Go-native `/scoped-models` overlay
now mirrors Pi's selector search semantics. Printable input filters the model
list with fuzzy matching, Ctrl+C clears the active search before cancelling, and
bulk enable/clear actions apply only to the filtered model set when a search is
active while preserving ordered scoped-model updates.

Latest model-selector increment: selecting a model through the interactive
`/model` selector now saves the chosen provider/model as the default, matching
Pi's `ModelSelectorComponent.handleSelect` behavior. Direct exact-match
`/model provider/model` commands still only switch the active session model.

Latest scoped-model-command increment: direct `/model <pattern>` exact matching
now uses the same candidate set as Pi's `getModelCandidates`: active scoped
models when a scope is configured, otherwise all available models. This prevents
typed model commands from bypassing the session's scoped model set; users can
still change scope through `/scoped-models` before selecting outside it.

Latest model-selector-scope increment: the live `/model` selector is now a
dedicated Go-native overlay instead of a generic select dialog. When scoped
models are configured it starts in `scoped`, supports Tab toggling to `all`,
keeps searchable filtering in both scopes, preserves scoped ordering, and saves
the selected provider/model as the default after selection.

Latest compaction-startup increment: initial live TTY rendering now matches
Pi's compacted-session notice. When the active session already contains one or
more compaction entries, startup shows `Session compacted 1 time` or
`Session compacted N times` after replaying existing messages.

Latest theme-preview increment: Gi now mirrors Pi's `ThemeSelectorComponent`
and `SettingsSelectorComponent` preview boundary. Select-list search changes
emit selection-change callbacks, `/theme` previews the currently highlighted
theme without writing settings, Enter commits, Escape restores the original
theme, and the `/settings` Theme submenu uses the same preview/restore path.

Latest compaction-live increment: manual compaction cancellation now emits a
Pi-style `compaction_end` event with `aborted: true` and
`Compaction cancelled` instead of a generic failed-compaction event. The live
TTY host handles compaction start/end like Pi: manual and auto labels are
distinct, success rebuilds the chat from compacted session messages, cancelled
manual/auto compactions render the matching status, and slash `/compact`
does not append a second submit error after Escape cancellation.

Latest agent-end increment: live TTY `agent_end` now mirrors Pi's defensive
cleanup path by stopping terminal progress, removing any orphan streaming
assistant component, clearing the streaming message pointer, and clearing
pending tool bookkeeping even when an abnormal event sequence skips the normal
`message_end` cleanup.

Latest bash-component increment: `BashExecutionComponent` now carries Pi-style
context-truncation metadata through live TTY bash rendering. Extension-provided
and host-executed bash results with `truncated` and `fullOutputPath` render the
same `Output truncated. Full output: ...` status that Pi shows while preserving
the recorded session metadata.

Latest retry-status increment: live TTY auto-retry status is now tracked as a
transient status component. `auto_retry_start` replaces any prior retry status,
and `auto_retry_end` removes it before either returning silently on success or
showing the final retry failure/cancellation message, matching Pi's retry
loader cleanup semantics.

Latest bash-running increment: `BashExecutionComponent` now displays Pi's
running-state cancel affordance, `Running... (Esc to cancel)`, while a command
is still active.

Latest bash-context-truncation increment: expanded bash output rendering now
uses the shared Go harness `TruncateTail` limits before display, mirroring Pi's
component-level context truncation for very large live bash output while still
showing the full-output path warning when available.

Latest bash-byte-truncation increment: bash tool result formatting now uses
Pi's combined tail limits of 2000 lines or 50KB, whichever is hit first.
Byte-truncated multiline and long-single-line outputs persist the full output
and render Pi-style `Full output:` summaries instead of returning oversized
text directly to the model/context.

Latest bash-output-accumulator increment: local bash execution now uses a
bounded Go accumulator instead of a full in-memory buffer. It keeps rolling
UTF-8-safe tails for live tool-update and final display snapshots, writes the
complete raw output to a temp file once Pi's line/byte threshold is crossed, and
preserves the existing sanitized live chunk callback surface for TTY updates.

Latest tool-renderer-safety increment: `ToolExecutionComponent` now treats
custom call/result renderer panics as protocol-bound renderer failures rather
than TUI crashes. It recovers and falls back to the built-in call label or
plain result text, matching Pi's try/catch behavior around package-provided
tool renderers.

Latest tool-fallback increment: unknown or extension tools now keep Pi's
default `formatToolExecution` shape in the Go TUI fallback: tool title,
pretty-printed JSON args, and text output. Renderer panic fallback uses the
same call fallback, so failed package renderers do not hide tool arguments.

Latest tool-render-context increment: `ToolRenderContext` and the
`tool.render_call` / `tool.render_result` RPC context now include Pi's
`showImages` flag alongside `expanded`, `isPartial`, args lifecycle, cwd, and
error state, allowing renderer packages to honor the live terminal image
preference.

Latest tool-output-sanitize increment: `fileToolResultText` now mirrors Pi's
`getTextOutput` display safety path for text blocks: ANSI escapes are stripped,
carriage returns are removed, and unsafe control / Unicode format characters
are filtered before the TUI renderer measures or wraps the output.

Latest tool-path-display increment: `shortenDisplayPath` now mirrors Pi's
`shortenPath` home-directory display rule for read/write/edit call rows, so
absolute paths under the user's home render as `~/...` without affecting paths
outside the home directory.

Latest compact-read-resource increment: `compactReadClassification` now uses
Pi's resource filename set for read result elision, including `CLAUDE.md` and
uppercase `.MD` variants. Gi still labels cwd-relative paths with its `.gi`
resource layout where applicable.

Latest read/write-display increment: built-in read result rendering and write
preview rendering now apply Pi's `replaceTabs` / `normalizeDisplayText` display
rules, replacing tabs with three spaces and removing carriage returns before
the text is measured, truncated, or rendered.

Latest read-truncation-display increment: read result components now append a
Pi-style truncation summary based on `ReadToolTruncation` details after the
collapsed preview line, keeping the truncation reason visible even when the
tool output itself is folded to 10 display lines.

Latest search-tool-renderer increment: `ToolExecutionComponent` now registers
Go-native built-in renderers for `grep`, `find`, and `ls`, matching Pi's
specialized search/list call labels and collapsed result limits instead of
falling back to generic JSON args for those core tools.

Latest search-tool-limit increment: `FindToolInput` and `LsToolInput` now carry
`limit`, the executors cap results with Pi-style continuation notices, and
`FileToolDetails` records match/result/entry limits so TUI and package
renderers can surface `[Truncated: ...]` summaries after collapsed output.

Latest grep-semantics increment: `GrepToolInput` now carries `glob`,
`ignoreCase`, and `literal`; the executor compiles regexes by default, supports
literal matching when requested, filters files by glob, prunes `.gitignore`
matches during directory walks, and the default SDK/TUI renderer paths expose
those semantics.

Latest ls-display increment: `LsTool` now returns `(empty directory)` for empty
directories and sorts entries case-insensitively before formatting directory
suffixes and applying `limit`, matching Pi's directory listing display.

Latest edit-tool-semantics increment: the Go `edit` tool now strips a leading
UTF-8 BOM before matching and restores it after writing, rejects no-op
replacements, reports Pi-style indexed duplicate/not-found/overlap errors, and
extends fuzzy matching across Pi's smart quote, dash, CR-only newline, and
special Unicode-space normalization cases. Edit results also carry
`firstChangedLine` with lower-camel JSON detail fields for protocol consumers,
and edit diffs now render Pi-style line-numbered context windows with
added/removed/context colors and single-line intra-line inverse highlighting
instead of raw replacement fragments.

Latest default-file-tools increment: default SDK `read`, `edit`, and `write`
tools now bind the real Go tool definitions instead of prompt-only or direct
executor entries, so model tool calls preserve prepared arguments, supported
legacy `file_path` compatibility, file-mutation queueing, renderer details,
and protocol-safe result shapes.

Completion audit decision:

- The default non-print launch path has grown from scaffold into the core
  multi-turn TTY workflows listed above, with focused live TTY tests for startup,
  input, slash commands, selectors, sessions, resource reload, bash, compaction,
  package UI, and signal/error behavior.
- Custom TUI components have an integrated supervised-process proof through
  lifecycle mount, editor/dialog host actions, autocomplete, message renderer,
  command/shortcut registration, tool execution, and tool-renderer callbacks in
  the same production TTY runtime.
- No open source rows remain. Pi private TypeScript SDK, npm/Bun installation,
  and branding-specific components are intentionally excluded or replaced by
  Gi's Go-native/protocol package design.
