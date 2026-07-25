# Pi Parity Module Audit

> The detailed mappings below originated from the v0.78.0 audit. Gi is now
> targeting the immutable Pi v0.82.0 baseline declared in `baseline.json`.
> `v0.82.0-open-gaps.json` is authoritative for the current unclassified delta;
> these tables are not a v0.82.0 completion claim.

This document is the working audit for aligning Gi with Pi `v0.82.0` at commit
`083e61621276bff9f6faefab87ce07fcd98734e2`. It intentionally records current
evidence and open gaps; it is not a completion claim.

## Module Map

| Pi module | Gi module | Current abstraction status |
| --- | --- | --- |
| `packages/ai` | `gi-llm-provider` | Same responsibility: model catalog, provider registry, payload conversion, streaming, OAuth helpers, images. File layout differs because Go groups protocol-specific helpers and tests by provider. |
| `packages/agent` | `gi-agent-core`, `gi-agent-core/harness` | Same responsibility: core agent loop plus harness/session/compaction/env helpers. Gi splits harness under the same Go module package tree instead of a TS submodule. |
| `packages/tui` | `gi-tui` | Same responsibility: component model, editor/input, key parsing, terminal/image helpers, headless terminal, render tests. Gi exposes parity through Go constructors/types while incrementally splitting Pi component boundaries into focused Go files. |
| `packages/coding-agent` | `gi-coding-agent`, `cmd/gi`, `protocol` | Same responsibility: CLI, interactive/print/RPC modes, session runtime, tools, resource loading, packages/extensions, UI components. Gi additionally separates the cross-language extension/view protocol into `protocol/`. |
| `packages/web-ui` | none | No Gi equivalent currently observed. If Pi web UI is in coding-agent scope, this is an explicit product gap; if out of scope, document that boundary. |
| `packages/storage/sqlite-node` | none | Explicitly excluded by `baseline.json`: this is a separately published optional adapter. Its two tests under `packages/agent/test/harness` are reported as cross-package exclusions; agent session-storage contracts and JSONL/in-memory implementations remain in scope. |

## Directory Abstraction Map

The parity target is module ownership, not a byte-for-byte path clone. A Gi
directory is considered abstraction-consistent only when every Pi source file in
that directory is either mapped to a same-purpose Go implementation, an
intentional Go-native consolidation, the Gi protocol boundary, or an explicit
gap.

### LLM Provider

| Pi directory | Gi ownership boundary | Abstraction status |
| --- | --- | --- |
| `packages/ai/src` | `gi-llm-provider` public package: model catalog, registry, stream entrypoints, base types, image API | consistent; Go keeps one provider package instead of TS root/barrel files. |
| `packages/ai/src/api` | `gi-llm-provider` provider, payload, stream, registry, and transport files | Go-native consistent; eager Go linking replaces lazy TS modules, while typed provider interfaces preserve the transport boundary and request snapshot ownership. |
| `packages/ai/src/auth` | `auth.go`, `credential_store.go`, `models_runtime.go`, and provider-owned auth hooks | Go-native consistent; one canonical credential shape, request-scoped auth context, serialized store mutation, and typed model errors cover provider login, refresh, request auth, and availability snapshots. |
| `packages/ai/src/auth/oauth` | provider-owned `*_oauth.go` flows, shared `oauth_authorization_code.go`, `oauth_device_code.go`, `oauth_pkce.go`, and `internal/oauthpage` | Go-native consistent; provider packages own protocol state and token exchange, `context.Context` owns cancellation, `ModelRuntime` owns credential persistence and provider recomposition, and the coding-agent `AuthInteraction` adapter owns only terminal/browser presentation. |
| `packages/ai/src/compat` | `types.go` model compatibility metadata plus provider conversion/config helpers | consolidated; generated compatibility flags are typed model data in Go and consumed by the relevant protocol adapter. |
| `packages/ai/src/providers` | `gi-llm-provider/*_provider.go`, `*_payload.go`, `*_stream.go`, `*_config.go` | consistent; provider files are split by protocol responsibility rather than nested directories. |
| `packages/ai/src/providers/images` | `images.go`, `image_models.go`, `openrouter_images*.go` | consistent; image provider registry stays in the same Go package. |
| `packages/ai/src/utils` | focused Go helpers in `diagnostics.go`, `env.go`, `event_stream.go`, `message_transform.go`, `overflow.go`, `validation.go`, `http_proxy.go`, `internal/envkeys`, `internal/eventstream`, `internal/httpproxy` | consistent; one request-scoped provider environment lookup feeds authentication, cache policy, and Azure/Bedrock/Vertex configuration. Generic event stream mechanics and HTTP proxy resolution use focused Go subpackages with root-level compatibility wrappers, while helpers that depend on provider message types remain in the public package to avoid import cycles. |

### Agent Core

| Pi directory | Gi ownership boundary | Abstraction status |
| --- | --- | --- |
| `packages/agent/src` | `gi-agent-core` public package: loop, stateful agent, proxy, core types | consistent; root agent loop/state/proxy remain one reusable core package. |
| `packages/agent/src/harness` | `gi-agent-core/harness` package | consistent; harness remains a subpackage because it owns sessions/resources/local execution around the core loop. |
| `packages/agent/src/harness/compaction` | `gi-agent-core/harness/compaction.go`, `branch_summary.go` | consistent; Go folds compaction helpers into the harness package while preserving exported functions/types. |
| `packages/agent/src/harness/env` | `gi-agent-core/harness/env`, `gi-agent-core/harness/local_env.go` facade | Go-native consistent; Node execution classes map to Go file/process interfaces behind a focused harness env subpackage. |
| `packages/agent/src/harness/session` | `session.go`, `session_repo.go`, `session_storage.go`, `harness/sessionid` UUID helpers | consistent; session tree/repo/storage are still owned by harness, while UUID generation now has a focused Go subpackage with harness-level compatibility wrappers. |
| `packages/agent/src/harness/tools` | `gi-agent-core/harness/tools` reusable read/write/edit/bash implementations plus `gi-agent-core/harness/tool.go` per-turn context binding; `gi-coding-agent` compatibility adapters | consistent; reusable execution, mutation state, path/image handling, shell capture, and result contracts now live below coding-agent, while its public tool APIs delegate through thin adapters. |
| `packages/agent/src/harness/utils` | `harness/utils` truncate helpers, `harness/env` shell-output helpers, root facades | consistent; reusable truncation and shell-capture logic now have focused Go subpackages with harness-level compatibility wrappers. |

### TUI

| Pi directory | Gi ownership boundary | Abstraction status |
| --- | --- | --- |
| `packages/tui/src` | `gi-tui` public package: TUI, terminal, editor/input, key parsing, image, autocomplete, utils | consistent; Go exposes constructors/types directly without `index.ts`. |
| `packages/tui/src/components` | `components_basic.go`, `components_loader.go`, `components_input.go`, `components_image.go`, `components_select_list.go`, `components_settings_list.go`, `components_editor.go`, `components_markdown.go`, plus focused support files | increasingly structured and consistent; Pi's low-coupling `text`, `spacer`, `truncated-text`, and `box` component files now map to `components_basic.go`, Pi's loader components now map to `components_loader.go`, Pi's single-line input maps to `components_input.go`, Pi's terminal image component maps to `components_image.go`, Pi's select list maps to `components_select_list.go`, Pi's settings list maps to `components_settings_list.go`, Pi's editor maps to `components_editor.go`, and Pi's markdown component maps to `components_markdown.go` plus `markdown_goldmark.go`. |
| Pi headless test terminal behavior | `gi-tui/virtual_terminal.go`, `gi-tui/internal/vtemu`, shared width helpers in `gi-tui/internal/width` | Gi-specific but aligned; this is a test/emulation boundary rather than a Pi source directory. |
| Pi history helpers | `gi-tui/internal/history`, `gi-tui/history.go` | consistent; kill-ring and undo-stack implementation now has a focused Go subpackage while the public constructors/types remain on `gi-tui`. |

### Coding Agent

| Pi directory | Gi ownership boundary | Abstraction status |
| --- | --- | --- |
| `packages/coding-agent/src` | `cmd/gi/main.go`, `gi-coding-agent/cli.go`, `config.go`, mode dispatch files | split but consistent; Go separates binary entrypoint from reusable package logic. |
| `packages/coding-agent/src/bun` | `internal/sandboxenv`, `restore_sandbox_env.go` facade; Bun bootstrap/provider files are excluded | Go-native; binary/runtime bootstrap does not apply to Gi, while sandbox environment restoration now lives behind a focused package. |
| `packages/coding-agent/src/cli` | `gi-coding-agent/internal/cli`, `cli_api.go`, `file_arguments.go`, `list_models.go`, `cli_config.go`, resume/session selector files | consistent; argument parsing and initial-message assembly now have a focused Go subpackage with root-level compatibility wrappers. |
| `packages/coding-agent/src/core` | `agent_session_*`, `usage_totals.go`, `cache_stats.go`, `http_runtime.go`, auth/settings/model/resource/system/footer/session manager files, `internal/authguide`, `internal/authwarning`, `internal/attribution`, `internal/modelresolver`, `internal/planmode`, `internal/sessioncwd`, `internal/telemetry` | consistent but still partly flattened; Go keeps tightly-coupled session/runtime pieces in one package to avoid artificial import cycles, while `session_file_store.go` isolates JSONL I/O policy from mutable session-tree state. Billed usage, active context pressure, model attribution, cache waste, and cache-miss notices are pure projections from one locked session snapshot and flow through the canonical `llm.Usage` and `FileEntry` types. Session discovery uses bounded header reads, explicit opens retain an authoritative streaming fallback, and validated entries plus derived indexes are applied as one locked state transition. Provider requests capture one validated settings snapshot and inject a reusable, independently owned HTTP client plus presence-aware timeouts; transport replacement is synchronized and retires old idle connections. Derived notices remain outside the append-only session log. OAuth and generic HTTP transport mechanics live in `gi-llm-provider`. |
| `packages/coding-agent/src/core/compaction` | `gi-agent-core/harness` compaction plus `agent_session_compaction*.go` | split but consistent; reusable compaction lives in harness, session-trigger wiring lives in coding-agent. |
| `packages/coding-agent/src/core/export-html` | `export_html.go` and export tests | partial; ANSI/session-data/custom-tool paths are mapped, while Pi's full static template/vendor asset pipeline remains a documented gap. |
| `packages/coding-agent/src/core/export-html/vendor` | `ExportHTMLTemplateJS` safe DOM markdown/highlight helpers | partial; Gi intentionally does not embed Pi's `marked.min.js` and `highlight.min.js` assets yet, while keeping their browser-rendering responsibilities documented in the file map. |
| `packages/coding-agent/src/core/extensions` | `protocol_extension_*`, `host_actions.go`, `viewtree.go`, `protocol/spec` | protocol-consistent; Pi's in-process TS API is intentionally replaced by process RPC/ViewTree/capability contracts. |
| `packages/coding-agent/src/core/tools` | `bash_*`, `file_tools.go`, `search_tools.go`, `edit_tool_definition.go`, `internal/toolqueue`, `internal/tooloutput`, root facades, tool renderers | consistent; built-in tool ownership is preserved with Go-native execution, and pure per-file mutation queueing plus streaming output accumulation now have focused tool subpackages. |
| `packages/coding-agent/src/extensions` | `extension_discovery.go`, `extension_selector.go`, and `protocol_extension_*` | partial; Gi owns generic extension discovery and process-protocol execution, while v0.82.0 bundled extensions are tracked separately from the host runtime. |
| `packages/coding-agent/src/extensions/llama` | no bundled Gi equivalent | explicit product gap; Pi's local Hugging Face/Llama extension requires a Go provider/runtime design before it can be shipped safely. |
| `packages/coding-agent/src/modes` | `print_mode.go`, `interactive_mode.go`, `cli_interactive_mode.go`, `rpc_mode.go` | consistent; mode boundaries are preserved by entrypoint files. |
| `packages/coding-agent/src/modes/interactive` | `cli_interactive_tui.go`, `cli_interactive_editor_host.go`, `cli_interactive_dialog_host.go`, `cli_interactive_status.go`, `cli_interactive_settings.go`, `cli_interactive_model.go`, `cli_interactive_auth.go`, `cli_interactive_help.go`, `cli_interactive_autocomplete.go`, `cli_interactive_keybindings.go`, `cli_interactive_session.go`, `cli_interactive_reload.go`, resources/signals/theme/component files | consistent but flattened; host, editor, dialog, status, settings, model-selection, auth/login, help/debug, autocomplete, keybinding/action routing, session/navigation, and reload/resource lifecycle state mirror Pi's interactive-mode ownership. |
| `packages/coding-agent/src/modes/interactive/assets` | `gi-coding-agent/assets`, embedded asset helpers | consistent; Pi's interactive announcement image asset is embedded and rendered through Gi's Go image component. |
| `packages/coding-agent/src/modes/interactive/components` | message/component/selector/footer/tool/theme files, `internal/diffrender`, plus `coding-agent-interactive-components.md` | consistent by component map; each Pi component file is mapped or protocol-consolidated, and the Pi diff component's pure line/word rendering now has a focused Go subpackage with theme injection. |
| `packages/coding-agent/src/modes/interactive/theme` | `tui_theme.go`, `theme_export.go`, `theme-schema.json`, embedded asset/theme tests | consistent; theme token/schema ownership is preserved with Gi branding. |
| `packages/coding-agent/src/modes/rpc` | `rpc_mode.go`, `rpc_client.go`, `rpc_session_host.go`, `internal/rpcwire`, `internal/mcpstdio`, `rpc_jsonl.go` and `mcp_stdio.go` facades | consistent; JSONL framing and MCP stdio request mechanics now have focused Go subpackages while client/host/session types remain in the parent package until shared RPC type boundaries can be split without cycles. |
| `packages/coding-agent/src/utils` | `utils.go`, `clipboard*.go` facades, `git.go`, `internal/ansiutil`, `internal/changelog`, `internal/clipboard`, `internal/frontmatter`, `internal/fswatch`, `internal/gitsource`, `internal/imageresize`, `internal/pathutil`, `internal/share`, `internal/startuptiming`, `internal/syntaxhighlight`, `internal/tmux`, `internal/versioncheck`, root facades, process/watch helpers | consistent or Go-native; pure clipboard text/image, file-watch, git-source parsing, image resize/EXIF, syntax-highlight, changelog, version-check/user-agent, frontmatter, path/ANSI, share URL parsing, startup timing, and tmux keyboard checks now have focused Go subpackages, while Node-only helpers are explicit exclusions. |

## Go Directory Structure Migration

The original parity pass proved that Pi source directories and symbols were
covered, but it intentionally tolerated a flat Go package layout where file
prefixes carried most ownership information. The current structural migration
keeps public Go APIs stable while moving pure, low-cycle implementation areas
behind focused subpackages:

| Gi subpackage | Pi boundary it clarifies | Public compatibility surface |
| --- | --- | --- |
| `gi-llm-provider/internal/envkeys` | `packages/ai/src/env-api-keys.ts` and provider environment-key lookup helpers | `gi-llm-provider/env.go` exposes process-only compatibility calls plus `FindEnvKeysWithOverrides` and `GetEnvAPIKeyWithOverrides` for request-scoped resolution. |
| `gi-llm-provider/internal/oauthpage` | `packages/ai/src/utils/oauth/oauth-page.ts` | `gi-llm-provider/oauth_page.go` aliases `OAuthPageOptions` and forwards OAuth callback page render helpers. |
| `gi-llm-provider/internal/eventstream` | `packages/ai/src/utils/event-stream.ts` | `gi-llm-provider/event_stream.go` wraps the generic stream type and preserves `AssistantMessageEventStream` constructors. |
| `gi-llm-provider/internal/httpproxy` | `packages/ai/src/utils/node-http-proxy.ts` proxy resolution helpers | `gi-llm-provider/http_proxy.go` forwards the public proxy resolver and unsupported-protocol message. |
| `gi-tui/internal/history` | `packages/tui/src/kill-ring.ts`, `undo-stack.ts` | `gi-tui/history.go` aliases `KillRing`, `KillRingPushOptions`, and generic `UndoStack`, and forwards constructors. |
| `gi-tui/internal/width` | `packages/tui/src/utils.ts` grapheme width/truncation helper boundary | `gi-tui/utils.go` and `gi-tui/internal/vtemu/width.go` keep package-local compatibility wrappers while shared grapheme segmentation, width calculation, and plain fragment truncation live in one TUI utility subpackage. |
| `gi-coding-agent/internal/cli` | `packages/coding-agent/src/cli/args.ts`, `initial-message.ts` | `gi-coding-agent/cli_api.go` aliases `Args`, `Mode`, `ThinkingLevel`, `ParseArgs`, and `BuildInitialMessage`. |
| `gi-coding-agent/internal/authguide` | `packages/coding-agent/src/core/auth-guidance.ts` login/API-key guidance strings | `gi-coding-agent/auth_guidance.go` keeps package-local helper names while provider guidance formatting lives in the auth helper subpackage. |
| `gi-coding-agent/internal/authwarning` | Anthropic subscription-auth warning detection used by interactive startup/model changes | `gi-coding-agent/anthropic_warning.go` preserves the checker type and warning constant while the provider/key decision logic lives in the auth warning subpackage. |
| `gi-coding-agent/internal/attribution` | `packages/coding-agent/src/core/sdk.ts` attribution header helpers | `gi-coding-agent/sdk_attribution.go` forwards `GetAttributionHeaders` and `BuildSDKStreamHeaders` while provider-specific attribution header merging lives in the SDK metadata subpackage. |
| `gi-coding-agent/internal/modelresolver` | `packages/coding-agent/src/core/model-resolver.ts` | `gi-coding-agent/model_resolver.go` aliases model-resolution types/constants and forwards model pattern/scope/initial selection helpers. |
| `gi-coding-agent/internal/planmode` | plan-mode parsing/status helpers used by coding-agent tests and UI | `gi-coding-agent/plan_mode_api.go` aliases `PlanTodoItem` and forwards plan parsing helpers. |
| `gi-coding-agent/internal/sessioncwd` | `packages/coding-agent/src/core/session-cwd.ts` missing stored-cwd detection and prompt formatting | `gi-coding-agent/session_cwd.go` preserves `MissingSessionCwd*` types and runtime factory helpers while pure cwd validation lives in the session-cwd subpackage. |
| `gi-coding-agent/internal/telemetry` | `packages/coding-agent/src/core/telemetry.ts` install-telemetry env parsing | `gi-coding-agent/telemetry.go` forwards install telemetry checks while env precedence/truthy parsing lives in the telemetry subpackage. |
| `gi-coding-agent/internal/rpcwire` | `packages/coding-agent/src/modes/rpc/jsonl.ts` | `gi-coding-agent/rpc_jsonl.go` forwards JSONL serialization and line-reader functions. |
| `gi-coding-agent/internal/mcpstdio` | MCP stdio JSON-RPC request/notification mechanics used by Gi protocol packages | `gi-coding-agent/mcp_stdio.go` injects host process hooks and preserves package-local helper names while stdio JSON-RPC framing, timeout, stderr, and notification handling live in a focused subpackage. |
| `gi-coding-agent/internal/sandboxenv` | `packages/coding-agent/src/bun/restore-sandbox-env.ts` | `gi-coding-agent/restore_sandbox_env.go` forwards sandbox env restoration while `/proc/self/environ` restoration mechanics live in the sandbox env subpackage. |
| `gi-coding-agent/internal/imageresize` | `packages/coding-agent/src/utils/image-resize.ts`, `image-convert.ts` EXIF/PNG conversion responsibilities | `gi-coding-agent/image_resize.go` aliases image resize types and forwards conversion, resize, dimension-note, and EXIF-orientation helpers used by existing tests. |
| `gi-coding-agent/internal/syntaxhighlight` | `packages/coding-agent/src/utils/syntax-highlight.ts` and the coding-agent highlight.js adapter boundary | `gi-coding-agent/syntax_highlight.go` aliases highlight types and forwards render/support helpers. |
| `gi-coding-agent/internal/fswatch` | `packages/coding-agent/src/utils/fs-watch.ts` | `gi-coding-agent/fs_watch.go` aliases watcher types/constants and forwards watch/close helpers. |
| `gi-coding-agent/internal/gitsource` | `packages/coding-agent/src/utils/git.ts` | `gi-coding-agent/git.go` aliases `GitSource` and forwards `ParseGitURL`. |
| `gi-coding-agent/internal/diffrender` | `packages/coding-agent/src/modes/interactive/components/diff.ts` | `gi-coding-agent/diff_render.go` forwards `RenderDiff` with Gi/Pi theme functions injected by the parent interactive package. |
| `gi-coding-agent/internal/toolqueue` | `packages/coding-agent/src/core/tools/file-mutation-queue.ts` | `gi-coding-agent/file_mutation_queue.go` forwards `WithFileMutationQueue` while the canonical path queue implementation lives in the tool subpackage. |
| `gi-coding-agent/internal/tooloutput` | `packages/coding-agent/src/core/tools/output-accumulator.ts` | `gi-coding-agent/bash_output_accumulator.go` keeps compatibility aliases while bounded tail, sanitization, and temp-file preservation live in the tool subpackage. |
| `gi-coding-agent/internal/changelog` | `packages/coding-agent/src/utils/changelog.ts` | `gi-coding-agent/changelog.go` keeps package-local compatibility wrappers while changelog parsing, new-entry selection, and display formatting live in the utility subpackage. |
| `gi-coding-agent/internal/share` | `packages/coding-agent/src/config.ts` share viewer URL and gist output parsing helpers | `gi-coding-agent/share.go` preserves share helper names while GitHub CLI gist creation and share URL parsing live in the share subpackage. |
| `gi-coding-agent/internal/startuptiming` | `packages/coding-agent/src/core/timings.ts` startup timing entries and env gates | `gi-coding-agent/startup_timings.go` keeps unexported compatibility aliases while timing collection/printing lives in the startup timing subpackage. |
| `gi-coding-agent/internal/tmux` | interactive terminal startup warning for tmux extended key settings | `gi-coding-agent/tmux_keyboard.go` preserves the public option-reader/checker names while tmux command probing and warning text live in a focused terminal helper subpackage. |
| `gi-coding-agent/internal/versioncheck` | `packages/coding-agent/src/utils/version-check.ts`, `packages/coding-agent/src/utils/pi-user-agent.ts` | `gi-coding-agent/version_check.go` and `utils.go` keep compatibility wrappers while version comparison, latest-release fetching, skip/offline handling, and Gi/Pi user-agent formatting live in the utility subpackage. |
| `gi-coding-agent/internal/frontmatter` | `packages/coding-agent/src/utils/frontmatter.ts` | `gi-coding-agent/utils.go` aliases `FrontmatterResult` and forwards frontmatter parse/strip/newline helpers while the YAML-frontmatter implementation lives in the utility subpackage. |
| `gi-coding-agent/internal/pathutil` | `packages/coding-agent/src/utils/paths.ts` and Gi read-path normalization helpers | `gi-coding-agent/utils.go` forwards expand/resolve/canonical/local/relative path helpers and same-package test compatibility helpers while the path implementation lives in the utility subpackage. |
| `gi-coding-agent/internal/ansiutil` | `packages/coding-agent/src/utils/ansi.ts` | `gi-coding-agent/utils.go` forwards `StripAnsi` while the OSC/CSI stripping implementation lives in the utility subpackage. |
| `gi-coding-agent/internal/clipboard` | `packages/coding-agent/src/utils/clipboard.ts`, `clipboard-image.ts`, `clipboard-native.ts` | `gi-coding-agent/clipboard.go` and `clipboard_image.go` keep compatibility aliases while text clipboard routing, OSC 52 fallback, Wayland/X11/WSL image reads, MIME selection, and BMP-to-PNG conversion live in the utility subpackage. |
| `gi-agent-core/harness/utils` | `packages/agent/src/harness/utils/truncate.ts` | `gi-agent-core/harness/truncate.go` aliases truncation types/constants and forwards functions. |
| `gi-agent-core/harness/sessionid` | `packages/agent/src/harness/session/uuid.ts` | `gi-agent-core/harness/uuid.go` aliases UUID callback types and forwards UUID functions. |
| `gi-agent-core/harness/env` | `packages/agent/src/harness/env/nodejs.ts` and `harness/utils/shell-output.ts` | `gi-agent-core/harness/local_env.go` aliases local execution env types/constants and forwards constructors/shell-capture helpers. |

This is intentionally incremental. Provider transports, coding-agent core
session/runtime code, and interactive TUI components still contain many
cross-file dependencies, so they should be split only after the shared type
boundaries are made explicit enough to avoid import cycles.

## Current v0.82.0 Inventory Evidence

All generated evidence below uses the clean, immutable Pi checkout at
`/private/tmp/pi-v0.82.0`, whose HEAD is the commit declared in
`baseline.json`.

The member-level source inventory currently reports:

| Module | Pi source files | Gi production files | Pi symbols | Missing Pi files | Missing Pi symbols |
| --- | ---: | ---: | ---: | ---: | ---: |
| LLM provider | 169 | 101 | 632 | 0 | 0 |
| Agent core | 35 | 35 | 327 | 0 | 0 |
| TUI | 28 | 32 | 449 | 0 | 0 |
| Coding agent | 177 | 180 | 2115 | 17 | 303 |

`docs/pi-parity/member-symbol-inventory.md` is the generated per-file detail.
A mentioned symbol means its ownership or gap has been classified; it does not
by itself prove equivalent behavior.

The directory-boundary verifier sees 8 LLM, 7 agent, 2 TUI, and 18 coding-agent
source directories. Every directory now has an ownership or explicit-gap row
above. Regenerate the inventory with:

```sh
node docs/pi-parity/verify-module-boundaries.mjs \
  --pi-root /private/tmp/pi-v0.82.0 \
  --format markdown \
  --out docs/pi-parity/module-boundary-inventory.md
```

The test-case inventory currently reports:

| Module | In-scope Pi test files | In-scope Pi cases | Candidate files | Candidate cases | No-candidate files | No-candidate cases |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| LLM provider | 111 | 1186 | 111 | 1186 | 0 | 0 |
| Agent core | 16 | 212 | 16 | 212 | 0 | 0 |
| TUI | 27 | 700 | 27 | 700 | 0 | 0 |
| Coding agent | 181 | 1649 | 178 | 1631 | 3 | 18 |

Candidate matching is an audit lead, not proof. Behavioral parity still
requires the mapped Go tests, implementation review, and the release gate in
`docs/pi-parity/README.md`.

## Superseded v0.78.0 Inventory Evidence

> The section below is retained as historical audit provenance only. Its
> “current” counts refer to the old v0.78.0 pass and must not be used for the
> v0.82.0 release decision.

<details>
<summary>Show the superseded v0.78.0 evidence</summary>

Current TypeScript/function source inventory from the local checkouts:

| Module | Pi TS source files | Gi production Go files | Directory abstraction note |
| --- | ---: | ---: | --- |
| LLM provider | 51 | 49 | Pi splits by TS source role; Gi now has focused env-key, OAuth callback-page, generic event-stream, and HTTP proxy resolver subpackages, while provider-specific payload, stream, and config helpers stay together until common provider types can be split without import cycles. |
| Agent core | 25 | 22 | Pi has `src/harness/*`; Gi preserves that as the `gi-agent-core/harness` package plus focused harness subpackages, and keeps loop/proxy/types in `gi-agent-core`. |
| TUI | 25 | 27 | Pi has one TS file per component; Gi now splits history helpers, headless terminal emulation, shared width/grapheme helpers, basic UI components, loader components, single-line input, terminal image rendering, select list, settings list, editor, and markdown into focused files inside the public `gi-tui` package plus focused internal helpers. |
| Coding agent | 141 | 157 | Pi is deeply nested by CLI/core/modes/utils; Gi now has focused internal subpackages for low-cycle CLI, auth guidance/warnings, attribution, model-resolution, plan-mode, OAuth flow helpers, session-cwd checks, telemetry, RPC wire, MCP stdio, sandbox env, share URL handling, startup timing, tmux keyboard checks, clipboard text/image handling, file watching, git-source parsing, image resize, syntax-highlight, changelog, version-check/user-agent, frontmatter, path/ANSI utilities, diff-rendering, file-mutation queueing, and streaming tool-output accumulation logic, while the remaining tightly-coupled internals stay behind `gi-coding-agent` plus `cmd/gi` and protocol files. |

Directory-boundary coverage is tracked separately by
`docs/pi-parity/module-boundary-inventory.md` and generated with:

```sh
node docs/pi-parity/verify-module-boundaries.mjs --pi-root ~/Projects/agents/pi --format markdown --out docs/pi-parity/module-boundary-inventory.md
```

This verifier scans every file under the four Pi `src` trees, including static
assets, JSON theme files, HTML/CSS templates, `.d.ts` shims, and vendor JS that
the TypeScript symbol verifier intentionally ignores. Current output:

```text
llm: piFiles=51 piDirectories=5 missingAuditRows=0
agent: piFiles=25 piDirectories=6 missingAuditRows=0
tui: piFiles=25 piDirectories=2 missingAuditRows=0
coding: piFiles=150 piDirectories=16 missingAuditRows=0
```

The current source audit has three levels:

- source-file coverage: every Pi source file must be mentioned in the relevant
  file map or in an explicit scope-exclusion row;
- exported-symbol coverage: every Pi exported function/type/constant must be
  mentioned by name and mapped to a Gi symbol, a consolidated Go
  implementation, or a documented gap.
- implementation-symbol coverage: every Pi top-level helper and class/member
  method extracted by the verifier must be named in the relevant map, with
  direct, split, protocol, Go-native, partial, gap, or not-applicable status.

Current evidence from the local checkouts:

| Module | Pi source-file coverage in docs | Pi exported symbols | Exported symbols still not named in docs | Meaning |
| --- | --- | ---: | ---: | --- |
| LLM provider | complete | 275 | 0 | File ownership and exported-symbol ownership are both mapped; remaining work is provider transport gaps and test-case parity. |
| Agent core | complete | 234 | 0 | File ownership and exported-symbol ownership are both mapped; remaining work is test-case parity and Go-native Result/error boundary confirmation. |
| TUI | complete | 215 | 0 | File ownership and exported-symbol ownership are both mapped; remaining work is test-case and fixture parity. |
| Coding agent | complete | 1368 | 0 | File ownership and exported-symbol ownership are both mapped; remaining work is test-case parity and the documented product-scope gaps. |

Current verification for the source/export rows above uses
`docs/pi-parity/verify-source-map.mjs`. It scans Pi source files, extracts
top-level exported TS symbols including multi-line re-export blocks, and checks
that each source file and exported name appears in the matching Gi parity map:

```text
llm: piFiles=51 giProductionFiles=49 symbols=275 missingFiles=0 missingSymbols=0
agent: piFiles=25 giProductionFiles=22 symbols=234 missingFiles=0 missingSymbols=0
tui: piFiles=25 giProductionFiles=27 symbols=215 missingFiles=0 missingSymbols=0
coding: piFiles=141 giProductionFiles=157 symbols=1368 missingFiles=0 missingSymbols=0
```

This check is intentionally textual, so it proves audit coverage rather than
runtime behavior. Behavior still requires the focused tests and manual code
comparison listed in each module section.

It also does not prove that Gi's physical directory tree matches Pi's
`packages/*/src` tree. For example, `coding: missingAuditRows=0` means every
Pi coding-agent source directory has an explicit ownership row in this audit.
The current `gi-coding-agent/` implementation is still a flatter Go package:
the root package owns the tightly coupled session/runtime/TUI files, while
low-cycle helpers are incrementally extracted into focused internal
subpackages such as `internal/cli`, `internal/authguide`,
`internal/authwarning`, `internal/clipboard`, `internal/mcpstdio`,
`internal/modelresolver`, `internal/sessioncwd`,
`internal/toolqueue`, and `internal/tooloutput`.

The same verifier can also expose non-exported top-level implementation
functions/classes:

```sh
node docs/pi-parity/verify-source-map.mjs --scope top-level --pi-root ~/Projects/agents/pi
```

Current top-level implementation inventory is partially complete as a mapped
function-by-function audit:

| Module | Pi top-level implementation symbols | Currently not named in maps | Meaning |
| --- | ---: | ---: | --- |
| LLM provider | 364 | 0 | LLM provider top-level private helper ownership is grouped in `llm-provider-file-map.md`; remaining work is behavioral parity and release-gate validation rather than source-symbol coverage. |
| Agent core | 161 | 0 | Agent-core top-level private helper ownership is now grouped in `agent-core-file-map.md`; remaining work is behavior/test-case parity and Go-native boundary validation, not source-symbol coverage. |
| TUI | 121 | 0 | TUI top-level private helper ownership is now grouped in `tui-file-map.md`; remaining work is behavior/test-case parity, not source-symbol coverage. |
| Coding agent | 660 | 0 | Coding-agent top-level private helper ownership is now grouped in `coding-agent-file-map.md`; remaining work is behavior/test-case parity and the documented protocol/product-scope gaps. |

Current top-level verifier output:

```text
llm: piFiles=51 giProductionFiles=49 symbols=364 missingFiles=0 missingSymbols=0
agent: piFiles=25 giProductionFiles=22 symbols=161 missingFiles=0 missingSymbols=0
tui: piFiles=25 giProductionFiles=27 symbols=121 missingFiles=0 missingSymbols=0
coding: piFiles=141 giProductionFiles=157 symbols=660 missingFiles=0 missingSymbols=0
```

This top-level helper audit is now complete as a source-coverage check. The
goal is still not complete until behavior and UI parity are verified against
the mapped test cases and the remaining partial/gap rows are either implemented
or accepted as explicit product-scope differences.

Member-level implementation coverage is tracked by
`docs/pi-parity/member-symbol-inventory.md`. It is generated with
`--include-covered`, so the inventory lists every extracted Pi source file,
the member/top-level symbols found in that file, and a per-file
`Missing from map` column instead of only aggregate counts:

```sh
node docs/pi-parity/verify-source-map.mjs --scope members --pi-root ~/Projects/agents/pi --format markdown --include-covered --out docs/pi-parity/member-symbol-inventory.md
```

Current member-level verifier output:

```text
llm: piFiles=51 giProductionFiles=49 symbols=372 missingFiles=0 missingSymbols=0
agent: piFiles=25 giProductionFiles=22 symbols=265 missingFiles=0 missingSymbols=0
tui: piFiles=25 giProductionFiles=27 symbols=395 missingFiles=0 missingSymbols=0
coding: piFiles=141 giProductionFiles=157 symbols=1637 missingFiles=0 missingSymbols=0
```

This closes the file/exported/top-level/member source-coverage pass for the
four target modules. It does not close behavior parity by itself: Pi/Gi UI
screenshots, provider streams, tool execution, packages/extensions, and the
test-case map remain the next source of truth for runtime equivalence.

Test-case-level tracking now lives in
`docs/pi-parity/test-case-map.md`. The generated inventory in
`docs/pi-parity/test-case-inventory.md` is produced by
`docs/pi-parity/verify-test-case-map.mjs`; it currently extracts 2755 Pi
`test`/`it`/conditional-skip cases. The current file-level candidate coverage
is 67/67 LLM provider test files, 16/16 agent-core files, 22/22 TUI files, and
121/121 coding-agent files. The same verifier now reports weak case-name
coverage too: 788/788 LLM cases, 150/150 agent-core cases, 592/592 TUI cases,
and 1225/1225 coding-agent cases have an obvious Go test/subtest name
candidate. It also records confirmed high-risk Gi coverage for interactive UI,
thinking visibility, real provider streaming, compaction/retry status,
model/thinking selection, runtime switching, retry lifecycle behavior, and the
Go-native provider dependency/dispatch boundary corresponding to Pi's
`lazy-module-load.test.ts`. No Pi test file or extracted Pi test case remained
without a Gi candidate in that historical pass.

</details>

## Current Module Findings

### LLM Provider

Pi source roots inspected:

- `packages/ai/src`
- `packages/ai/test`

Gi source roots inspected:

- `gi-llm-provider`

Observed direct coverage:

- Detailed file/function mapping is tracked in
  `docs/pi-parity/llm-provider-file-map.md`.
- Provider/model core: Pi `api-registry.ts`, `models.ts`, `models.generated.ts`, `types.ts`, `stream.ts` map to Gi `registry.go`, `models.go`, `pi_models_generated.go`, `types.go`, `event_stream.go`.
- Pi's generated model catalog now comes from the official
  `@earendil-works/pi-ai@0.82.0` release package (npm shasum
  `b1f33e7cb81ef6a55918baaef2764ff03c37a925`). The tag intentionally omits the
  generated provider JSON, so Gi's checked-in `internal/cmd/modelgen` consumes
  the published data, preserves source order, rejects unknown fields, and
  verifies the published manifest schema/timestamp, exact shard set, per-file
  SHA-256 values, and normalized model/API structure hash. The validated
  generation timestamp is embedded into the generated Go catalog. This aligns
  the catalog with v0.82.0, including the Fireworks
  `kimi-k2p6-turbo` router, GPT-5.6 pricing tiers, and `max` thinking metadata.
- Gi model registry initialization now mirrors Pi's generated-catalog path:
  `models.go` calls `registerPiGeneratedModels()` directly instead of first
  registering hand-written model rows that were immediately discarded by the
  generated registry reset.
- Pi `api-registry.ts` source lifecycle and mismatch wrapper now map to Gi
  `registry.go` source-aware registration, provider listing, source
  unregistration, clearing, and retrieved-provider API validation.
- Built-in providers: Pi Anthropic, Azure OpenAI Responses, Bedrock, Faux, Google, Google Vertex, GitHub Copilot headers, Mistral, OpenAI Codex Responses, OpenAI Completions, OpenAI Responses, OpenRouter images map to same-named Gi files.
- Cross-provider behavior: Pi transform/overflow/env/OAuth/image helpers map to Gi `message_transform.go`, `overflow.go`, `env.go`, `oauth.go`, `images.go`, `image_models.go`, `openrouter_images.go`.
- Cloudflare base URL helpers in Pi `providers/cloudflare.ts` now map to Gi `cloudflare.go`, and are wired into Anthropic/OpenAI Completions/OpenAI Responses provider request paths.
- Pi assistant-message diagnostics in `utils/diagnostics.ts` now map to Gi `diagnostics.go` plus the `Message.Diagnostics` field. Gi uses this for OpenAI Codex transport fallback reporting.
- Pi OpenAI Codex WebSocket transport maps to Gi's synchronized connection leases, 55-minute age rotation, five-minute idle cleanup, continuation deltas, retryable backend state errors, and session debug/reset/close helpers. A `provider_transport_failure` diagnostic accompanies SSE fallback only when the WebSocket fails before stream start.
- Pi OpenAI Codex request timing and retry policy map to one immutable Go execution snapshot shared by SSE and WebSocket paths. SSE header deadlines stop once headers arrive without cancelling body streaming; retry parsing accepts fractional and HTTP-date headers, rejects terminal quota failures, and enforces the default 60-second server-delay ceiling through a typed error.
- Pi OpenAI Codex SSE request compression maps to a reusable pure-Go level-3 zstd encoder. A prepared request clones headers and owns one compressed byte slice reused across attempts, while the WebSocket path continues to send plain JSON frames.
- Pi simple-stream normalization maps to one cloned Go `StreamOptions` snapshot before provider-specific request construction. It defaults `MaxTokens` from the model, reserves 4,096 tokens for context safety, and carries legacy Anthropic/Bedrock reasoning as a typed total-plus-thinking allocation so budget expansion cannot exceed the remaining context.
- Pi `utils/json-parse.ts` maps to Gi `RepairJSON`, `UnmarshalJSONWithRepair`, and `parseStreamingJSONObject`; the streaming parser now repairs common partial objects so tool arguments can appear before the final JSON delta.
- Pi `utils/hash.ts`, `headers.ts`, `node-http-proxy.ts`, `sanitize-unicode.ts`, and `validation.ts` map to Gi `shortHash`, `responseHeaders`, `http_proxy.go`, `SanitizeSurrogates`, and `validation.go`. `ValidateToolCall` is present for the Pi-level "find tool by name, then validate arguments" contract.
- Pi `utils/typebox-helpers.ts` `StringEnum` maps to Gi `StringEnum` / `StringEnumWithOptions`, and Gi validation now enforces `Schema.Enum`.
- Pi OpenAI Responses reasoning stream behavior now maps to Gi
  `openai_responses_stream.go`: reasoning items emit thinking events, completed
  reasoning items keep replay signatures, and completed text items keep
  Pi-style text signatures. Text, thinking, and tool-call stream events now
  carry the Pi `contentIndex`, `delta`, `content`, and `toolCall` lifecycle
  fields. Gi also follows Pi's summary-part guard, so bare
  `response.reasoning_summary_text.delta` events do not create visible thinking
  content unless a `response.reasoning_summary_part.added` event opened that
  summary part.
- Pi OpenAI-compatible Chat Completions stream lifecycle now maps to Gi
  `openai_completions_stream.go`: text, reasoning, and tool-call blocks emit
  Pi-style start/delta/end events, including `contentIndex`, streamed `delta`,
  final content, and finalized tool-call arguments.
- Pi Bedrock Converse stream processing now maps to Gi `bedrock_stream.go` for
  assistant start, text/thinking/tool-call deltas, `toolConfig` conversion,
  stop-reason mapping, usage, and cost calculation through an injectable
  transport boundary. Default AWS SDK/SigV4 live transport is still a
  deliberate provider gap.

Open verification items:

- Pi `session-resources.ts` cleanup registry now maps to Gi
  `session_resources.go`. Codex session cleanup closes cached WebSocket
  connections and their idle timers through the same registry; debug stats,
  fallback state, connection reuse, and cached-input delta requests have
  focused race-tested coverage.
- Pi utility files currently have direct Gi counterparts or documented Go-specific equivalents.

### Agent Core

Pi source roots inspected:

- `packages/agent/src`
- `packages/agent/test`

Gi source roots inspected:

- `gi-agent-core`
- `gi-agent-core/harness`

Observed direct coverage:

- Detailed file/function mapping is tracked in
  `docs/pi-parity/agent-core-file-map.md`.
- Core loop/types: Pi `agent-loop.ts`, `agent.ts`, `types.ts` map to Gi `agent_loop.go`, `agent.go`, `types.go`.
- Harness/session: Pi memory/jsonl storage/repo/session files map to Gi `session.go`, `session_repo.go`, `session_storage.go`, `uuid.go`.
- Harness resources: Pi prompt templates, skills, compaction, branch summarization, local env, message formatting, shell output/truncate map to Gi `prompt_templates.go`, `skills.go`, `compaction.go`, `branch_summary.go`, `harness/env`, `local_env.go` facade, `format.go`, `truncate.go`.
- Pi `harness/system-prompt.ts` `formatSkillsForSystemPrompt` maps directly to Gi `harness/format.go` `FormatSkillsForSystemPrompt`.
- Pi `harness/messages.ts` maps to Gi `harness/messages.go`: `BashExecutionText`, branch/compaction summary prompt wrappers, custom-message session projection, and `ConvertToLLM` for `bashExecution` / `custom` / `branchSummary` / `compactionSummary`.
- Pi `harness/utils/shell-output.ts` `sanitizeBinaryOutput` maps to Gi `harness/env` `SanitizeBinaryOutput` with a root `harness/local_env.go` facade; `ExecuteShellWithCapture` now applies the same control-character and carriage-return cleanup before inline/full-output capture.
- Pi `node.ts` re-exports Node-specific execution env APIs; Gi's equivalent is `harness/env` plus a root facade and is intentionally Go-native.

Open verification items:

- Pi `proxy.ts` now maps to Gi `gi-agent-core/proxy.go`: `StreamProxy` and
  `NewProxyStreamFn` preserve Pi's server-managed-auth `/api/stream` embedding
  pattern and reconstruct stripped proxy events into assistant message events.

### TUI

Pi source roots inspected:

- `packages/tui/src`
- `packages/tui/test`

Gi source roots inspected:

- `gi-tui`

Observed direct coverage:

- Detailed file/function mapping is tracked in
  `docs/pi-parity/tui-file-map.md`.
- Pi public exports in `packages/tui/src/index.ts` are represented by Gi public constructors/types and guarded by `gi-tui/api_parity_test.go`.
- Pi small component files `box`, `text`, `spacer`, and `truncated-text` now live in Gi `components_basic.go`; `loader` and `cancellable-loader` now live in `components_loader.go`; `input` now lives in `components_input.go`; `image` now lives in `components_image.go`; `select-list` now lives in `components_select_list.go`; `settings-list` now lives in `components_settings_list.go`; `editor` now lives in `components_editor.go`; `markdown` now lives in `components_markdown.go` plus goldmark-specific compatibility helpers.
- Pi `fuzzy.ts` is implemented inside Gi `autocomplete.go`.
- Pi `kill-ring.ts` and `undo-stack.ts` are implemented in Gi `history.go`.
- Pi `terminal.ts`, `terminal-image.ts`, `keys.ts`, `keybindings.ts`, `stdin-buffer.ts`, `tui.ts`, and `utils.ts` map to same-purpose Gi files.
- Gi also has `virtual_terminal.go`, `internal/vtemu`, and shared `internal/width` helpers covering Pi's headless test terminal behavior and terminal-cell width rules.
- Pi exported symbols in `packages/tui/src` are now mapped one-by-one in
  `docs/pi-parity/tui-file-map.md` under `Exported Symbol Audit`; the remaining
  TUI parity work is behavioral/test-case coverage rather than public-surface
  ownership.

Open verification items:

- Pi test names still need a one-by-one behavioral coverage table against Gi
  test names. The current generated file-level inventory is the work queue for
  this remaining proof step.
- Markdown rendering needs continued fixture comparison because Pi uses `marked` while Gi uses Go rendering/goldmark-backed parsing.
- Virtual terminal behavior needs ongoing CSI/xterm parity checks against Pi fixtures.

### Coding Agent

Pi source roots inspected:

- `packages/coding-agent/src`
- `packages/coding-agent/test`

Gi source roots inspected:

- `gi-coding-agent`
- `cmd/gi`
- `protocol`

Observed direct coverage:

- Detailed source-directory mapping is tracked in
  `docs/pi-parity/coding-agent-file-map.md`.
- CLI/config/args/session/model/auth/resources/settings/tools map to same-purpose Gi files.
- Pi root `config.ts` and `migrations.ts` now have function-level mapping in
  `docs/pi-parity/coding-agent-file-map.md`: Gi exposes the same package/user
  path helper boundary with `.gi` naming and covers legacy auth/session/commands,
  keybindings, managed tools, and deprecated extension directory migrations with
  tests.
- Interactive mode is consolidated primarily into `cli_interactive_tui.go`, with editor/dialog/status/settings/model/auth/help host APIs split into `cli_interactive_editor_host.go`, `cli_interactive_dialog_host.go`, `cli_interactive_status.go`, `cli_interactive_settings.go`, `cli_interactive_model.go`, `cli_interactive_auth.go`, and `cli_interactive_help.go` plus focused component/helper files. Experimental first-time setup has its own immutable eligibility boundary and one component-owned mutable state projection in `startup_ui.go` and `first_time_setup.go`; only the CLI coordinator persists the submitted theme, analytics preference, and stable tracking ID.
- Pi `AssistantMessageComponent` / `UserMessageComponent` now map to Gi
  `message_components.go` and `cli_message_components.go`: both exported and
  internal paths share the same Pi-style Markdown/theme/OSC 133 rendering
  semantics, including hidden thinking labels and non-text tool-call handling.
- Pi `DaxnutsComponent` and `checkDaxnutsEasterEgg` now map to Gi
  `cli_message_components.go` / `cli_interactive_tui.go`: selecting
  `opencode/kimi-k2.5` mounts the same animated OpenCode Zen / daxnuts
  easter-egg surface, including the 32x32 truecolor half-block image,
  scanline reveal, final centered attribution text, and link.
- Print/RPC modes map to `cli_print_mode.go`, `rpc_mode.go`, `rpc_client.go`, `rpc_jsonl.go`, and `rpc_session_host.go`.
- Pi in-process TS extensions are intentionally replaced by Gi's protocol/runtime split: `protocol_extension_*`, `viewtree.go`, `inprocess_components.go`, `official_packages.go`, and `protocol/spec`.
- Built-in file/bash/search/edit tools now have a one-by-one
  `core/tools/*` file/function map in
  `docs/pi-parity/coding-agent-file-map.md`. Gi's `read` tool now also matches
  Pi's first-line-over-50KB guard instead of streaming the oversized line into
  model context.
- Pi `core/compaction/*` now has a one-by-one file/function map in
  `docs/pi-parity/coding-agent-file-map.md`. Gi's harness compaction matches
  Pi's default token reserves, strict trigger threshold, native
  `totalTokens` preference, hook-generated-compaction exclusion, read-only vs
  modified file-list semantics, exact structured history/turn-prefix prompt
  text, branch-summary prompt shape, empty-branch result, and XML
  file-operation summary tags.
- Pi `core/export-html/ansi-to-html.ts` now maps to Gi `export_html.go`:
  styled ANSI output from custom tool renderers is converted with Pi-style SGR
  foreground/background, bright/indexed/RGB colors, text styles, HTML escaping,
  empty-line `&nbsp;`, and ANSI-only spacing-line trimming.

Open verification items:

- Pi `modes/interactive/components/*` one-by-one mapping is tracked in
  `docs/pi-parity/coding-agent-interactive-components.md`; current component
  rows are direct, consolidated, or protocol-mapped, and exported component
  names are now pinned in the coding-agent exported-symbol audit.
- Pi extension loader/runner/types now have a function-level and exported-symbol
  map in `docs/pi-parity/coding-agent-file-map.md`; remaining work is test-case
  confirmation for package-provided process extensions against the protocol
  conformance suite.
- Pi package-manager source semantics now have a function-level and
  exported-symbol map in `docs/pi-parity/coding-agent-file-map.md`. Gi
  intentionally rejects `npm:`, installs/materializes official packages,
  validates local paths, clones git packages into scoped stores, resolves
  installed git/local/official resources through `gi.package.json` / `.gi.json`,
  and keeps package/top-level resource filters test-covered. Remaining work is
  broadening conformance fixtures for
  archive/lock metadata if those source backends are accepted into v1.
- Pi OAuth subscription flows now have provider-by-provider confirmation for
  OpenAI Codex, Anthropic, and GitHub Copilot: authorization URL construction,
  callback pages/paths, manual redirect parsing, token exchange/refresh request
  shape, credential storage, GitHub device polling, and Copilot base URL
  fallback are covered by mocked tests. Gi keeps Gi branding and `originator=gi`
  as product-identity differences.
- Pi export HTML templates/assets are now mapped in
  `docs/pi-parity/coding-agent-file-map.md`; the ANSI converter behavior is
  direct, while Pi's rich static viewer template/JS/vendor asset pipeline
  remains a documented gap or product-decision point.
- Pi web UI has no observed Gi equivalent.

## UI Parity Note

The current local Gi config at `~/.gi/agent/settings.json` contains
`hideThinkingBlock: true`, while the inspected Pi config does not. Pi and Gi
both default this setting to `false`. With the setting enabled, a final assistant
message containing a `thinking` block renders the hidden label (`Thinking...`);
that matches Pi component behavior rather than proving a component bug. The
remaining question is whether Gi provider/runtime is creating thinking blocks in
cases where Pi would not. The OpenAI Responses/Codex SSE parser now matches Pi's
summary-part guard for this path, and OpenAI-compatible Chat Completions now
uses the same lifecycle event shape as Pi for reasoning/tool streams. Codex
WebSocket reuse/cache/continuation and SSE request-compression parity are now
covered by separate transport-focused tests.

## Completion Criteria

The audit is complete only when each Pi source file has one of:

- a Gi file/function mapping with tests or runtime evidence;
- an intentional abstraction move documented here;
- an explicit gap with an implementation task or accepted scope exclusion.

For tests, each Pi test case must have one of:

- a Gi test with equivalent assertion scope;
- stronger coverage through a broader Gi conformance test;
- an explicit gap.
