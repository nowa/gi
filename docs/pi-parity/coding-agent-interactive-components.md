# Coding Agent Interactive Component Parity

This is the one-by-one mapping for Pi
`packages/coding-agent/src/modes/interactive/components/*` against Gi. It is a
working audit, not a completion claim.

## Status Legend

- `direct`: Gi has a same-purpose component with matching runtime behavior.
- `consolidated`: Pi's standalone component is folded into a larger Go UI file.
- `protocol`: Pi's in-process extension component is represented by Gi's RPC or
  ViewTree protocol boundary.
- `partial`: Gi has the user-facing behavior, but the implementation shape or
  coverage still needs closer parity checks.
- `gap`: no current equivalent.

## Component Map

All Pi component names in this table are exact files under
`packages/coding-agent/src/modes/interactive/components/`.

| Pi component | Main Pi surface | Gi equivalent | Status | Notes |
| --- | --- | --- | --- | --- |
| `armin.ts` | Easter egg component | `gi-coding-agent/cli_message_components.go` `newCLIArminComponent` | direct | Bitmap text component is represented in Go and mounted by the interactive host. |
| `assistant-message.ts` | Assistant markdown/thinking/OSC133 render | `gi-coding-agent/message_components.go`, `gi-coding-agent/cli_message_components.go` | direct | Shared exported and CLI paths render text/thinking/tool-call suppression with Pi-style OSC 133 wrapping. |
| `bash-execution.ts` | Bash tool execution surface | `gi-coding-agent/bash_execution.go` | direct | Running/completed/cancelled/error status and collapsed output behavior are implemented as a Go component. |
| `bordered-loader.ts` | Loader wrapped by border | `gi-coding-agent/cli_message_components.go` `BorderedLoaderComponent`, `gi-tui` loader | direct | Standalone component mirrors Pi's default-cancellable bordered loader: dynamic top/bottom borders, accent/muted loader styling, cancel key hint, signal/onAbort wiring, input forwarding, and dispose behavior. `/share` uses this component instead of host-owned loader composition. |
| `branch-summary-message.ts` | Collapsible branch summary | `gi-coding-agent/cli_message_components.go` `newCLICollapsibleMarkdownMessage`, `gi-agent-core/harness/messages.go` | direct | Branch summary role and collapsible render are represented. |
| `compaction-summary-message.ts` | Collapsible compaction summary | `gi-coding-agent/cli_message_components.go`, `gi-agent-core/harness/messages.go` | direct | Token-before label and collapsed/expanded body are represented. |
| `config-selector.ts` | Package resource configuration selector | `gi-coding-agent/cli_config.go`, `gi-coding-agent/resource_loader.go`, `gi-coding-agent/package_manager.go` | direct | Gi now mirrors Pi's grouped resource selector shape: source group headers, resource-type subgroups, first-item selection, filter-with-parent-headers behavior, Space/Enter toggles, Esc/Ctrl-C close, and package/top-level filter persistence. |
| `countdown-timer.ts` | Timeout countdown display | `gi-coding-agent/status_indicator.go` `CountdownTimer`; dialog-specific timeout runners | direct | Retry status owns the reusable second-granularity timer and synchronously waits for disposal; dialog runners retain their dialog-local expiry actions. |
| `custom-editor.ts` | Extension-provided editor surface | `gi-coding-agent/inprocess_components.go`, `gi-coding-agent/cli_interactive_editor_host.go`, `gi-coding-agent/cli_interactive_tui.go` custom editor wiring | protocol | Trusted in-process components and out-of-process ViewTree/editor flows replace Pi's TS extension component boundary. |
| `custom-message.ts` | Extension-rendered custom message | `gi-coding-agent/cli_interactive_tui.go` `addRenderedCustomMessage`, `viewtree.go` | protocol | Message renderers are registered through Gi's extension protocol. |
| `daxnuts.ts` | OpenCode Zen easter egg | `gi-coding-agent/cli_message_components.go` `newCLIDaxnutsComponent` | direct | Exact DAX image data, truecolor half-block render, scanline reveal, final attribution text, and link are represented. |
| `diff.ts` | Diff rendering | `gi-coding-agent/diff_render.go`, `gi-coding-agent/file_tools.go`, `gi-coding-agent/tool_execution_component.go`, theme keys | direct | Edit diff rendering now matches Pi's line coloring, tab replacement, single-line intra-line inverse highlighting, multi-line replacement fallback, and tool-call/result integration. |
| `dynamic-border.ts` | Dynamic horizontal border | `gi-coding-agent/cli_message_components.go` `newCLIDynamicBorder` | direct | Host uses the dynamic border for Pi-style editor/status separation. |
| `earendil-announcement.ts` | Announcement/easter egg | `gi-coding-agent/cli_message_components.go` `newCLIEarendilAnnouncementComponent` | direct | Announcement surface is represented in Go. |
| `extension-editor.ts` | Extension text editor prompt | `gi-coding-agent/cli_tui_dialog.go`, `gi-coding-agent/cli_interactive_dialog_host.go` `newCLIEditorDialog`, `runTUIDialogRequest` | protocol | Out-of-process extensions request editor dialogs through host actions. |
| `extension-input.ts` | Extension input prompt | `gi-coding-agent/cli_tui_dialog.go`, `gi-coding-agent/cli_interactive_dialog_host.go` `newCLIInputDialog`, `runTUIDialogRequest` | protocol | Out-of-process extensions request input dialogs through host actions. |
| `extension-selector.ts` | Extension option selector | `gi-coding-agent/extension_selector.go`, `gi-coding-agent/cli_interactive_dialog_host.go` timeout handling | direct | Selector and timeout semantics are implemented in Go. |
| `first-time-setup.ts` | Theme and analytics onboarding state machine | `gi-coding-agent/first_time_setup.go`, `startup_ui.go`, `settings_manager.go` | direct | A typed component state drives the two steps and previews; the startup coordinator is the sole callback consumer and applies theme, analytics preference, and stable tracking ID as one settings transition before runtime services are created. |
| `footer.ts` | Footer with cwd/model/usage | `gi-coding-agent/footer.go`, `gi-coding-agent/footer_data_provider.go`, `gi-coding-agent/usage_totals.go` | direct | Footer consumes one canonical `llm.Usage` snapshot and the latest assistant cache-hit rate; width, model/provider, subscription, cumulative usage, context, and branch data are represented. |
| `index.ts` | Component re-exports | Go package exports and constructors | consolidated | Gi does not need a re-export barrel; public constructors are in package files. |
| `keybinding-hints.ts` | Hotkey hint display | `gi-coding-agent/keybindings.go`, `gi-coding-agent/cli_interactive_tui.go`, selector components | consolidated | Hints are rendered where each host surface needs them. |
| `login-dialog.ts` | Login flow dialog | `gi-coding-agent/login_dialog.go`, `gi-coding-agent/cli_provider_auth_interaction.go`, `gi-coding-agent/cli_interactive_auth.go` | direct | Typed provider prompts/events drive one dialog for URL, manual code, text/secret input, device code, info, and progress. Provider protocol tests stay in `gi-llm-provider`; virtual-terminal tests cover the generic runtime login path for API keys, Bedrock, Anthropic, OpenAI Codex, and GitHub Copilot. |
| `model-selector.ts` | Model picker | `gi-coding-agent/model_selector_component.go`, `gi-coding-agent/model_search.go`, `gi-coding-agent/cli_interactive_model.go` | direct | Provider/model filtering, slash-command selection, and thinking info are represented. `/model` uses the selector-specific search projection so exact provider-prefixed models rank before proxy IDs. |
| `oauth-selector.ts` | OAuth provider selector and `formatAuthSelectorProviderType` | `gi-coding-agent/oauth_selector.go` `formatAuthSelectorProviderType`, `OAuthSelectorComponent`; `gi-coding-agent/cli_interactive_auth.go`, `gi-coding-agent/cli_provider_auth_interaction.go` | direct | Login choices are derived from runtime `ProviderAuth`; mixed lists label subscription versus API key, and the same selector handles provider-owned method prompts before restoring the login dialog. |
| `scoped-models-selector.ts` | Scoped model picker | `gi-coding-agent/model_selector_component.go`, `gi-coding-agent/model_search.go`, `gi-coding-agent/cli_interactive_model.go`, `model_registry.go` | direct | Scoped model order, interactive enable/disable flow, canonical provider/ID/name search, and thinking-level preservation are covered by Gi model tests. |
| `session-selector-search.ts` | Search query renderer for sessions | `gi-coding-agent/session_selector_search.go` | direct | Session search tokenization and display are represented. |
| `session-selector.ts` | Session picker | `gi-coding-agent/session_selector.go` | direct | Session list, branch metadata, and key hints are represented. |
| `settings-selector.ts` | Settings menu | `gi-coding-agent/cli_interactive_settings.go`, `cli_interactive_theme_settings.go`, `settings_manager.go`, `interactive_theme_controller.go` | consolidated | Settings are built into a focused interactive settings host rather than a standalone package. Item order, image-control gating, select submenus, single/automatic light-dark theme configuration, controller-owned theme preview/cancel, and callback side effects are represented; transport settings flow into provider stream options, while cache-miss notice changes rebuild the transcript from session entries without persisting presentation state. |
| `show-images-selector.ts` | Image-display setting selector | `gi-coding-agent/cli_interactive_tui.go`, `tool_execution_component.go` `SetShowImages` | consolidated | User-facing setting exists; standalone selector structure is folded into settings. |
| `skill-invocation-message.ts` | Collapsible skill invocation | `gi-coding-agent/cli_interactive_tui.go` `addSkillInvocationMessage`, `export_html_skill_block.go` | direct | Skill block parsing and collapsible display are represented. |
| `status-indicator.ts` | Working, retry, compaction, branch-summary, and idle status components | `gi-coding-agent/status_indicator.go`, `cli_interactive_status.go` | direct | Typed status kinds share one host-owned slot. Replacement and typed clearing dispose the previous loader/countdown outside the host lock; clear-on-shrink installs a fixed-height idle component. |
| `theme-selector.ts` | Theme picker | `gi-coding-agent/cli_interactive_theme_settings.go` settings theme submenu, `interactive_theme_controller.go`, `theme_export.go`, theme tests | consolidated | Pi's standalone component is exported but not mounted directly by interactive mode; the mounted SettingsSelector theme submenu delegates preview and restore to the same state owner used by terminal auto-sync, including current selection, automatic light/dark configuration, preview on selection change, restore on cancel, and Enter/Esc key flow. |
| `thinking-selector.ts` | Thinking level picker | `gi-coding-agent/model_selector_component.go`, `cli_interactive_model.go`, `model_registry.go` | consolidated | Thinking level selection is integrated with model/scoped-model flows and footer state. |
| `tool-execution.ts` | Tool call/result UI | `gi-coding-agent/tool_execution_component.go` | direct | Tool arguments, result, expansion, images, and errors are represented. |
| `tree-selector.ts` | Tree/session branch selector | `gi-coding-agent/tree_selector.go` | direct | Tree navigation and selection behavior are represented. |
| `user-message-selector.ts` | User message picker | `gi-coding-agent/user_message_selector.go` | direct | User message list selection is represented. |
| `user-message.ts` | User message boxed render | `gi-coding-agent/message_components.go`, `gi-coding-agent/cli_message_components.go` | direct | Box padding, user message background/text theme, and OSC 133 wrapping are represented. |
| `visual-truncate.ts` | Visual-width truncation helper | `gi-coding-agent/bash_execution.go` `TruncateToVisualLines`, `gi-tui` text wrapping | direct | Gi mirrors Pi's helper shape: render through a temporary text component with caller-provided horizontal padding, keep the last N visual lines, and report skipped visual-line count. |

## Current UI Finding

Pi and Gi both default `hideThinkingBlock` to `false`. When that setting is
`true` and the assistant message contains a real `thinking` block, Pi's
`AssistantMessageComponent` renders the hidden label (`Thinking...`) before the
visible text. Gi now follows that component behavior, and
`TestCLIInteractiveTUIHostDefaultThinkingVisibilityMatchesPi` locks the inverse
case: with default settings, Gi shows the reasoning text itself and does not
invent a `Thinking...` hidden-label row.

If `Thinking...` appears in Gi but not in a Pi run for the same prompt, the
first thing to compare is not the message component; it is the session/provider
payload:

- whether the local setting differs (`hideThinkingBlock`);
- whether the provider emitted a `thinking`/reasoning block;
- whether the model/API preserved reasoning replay signatures differently.

The OpenAI Responses stream path now matches Pi's reasoning item behavior:
reasoning items emit `thinking_start`, `thinking_delta`, and `thinking_end`;
completed reasoning items store the JSON reasoning item signature for replay;
completed text items store the Pi-style text signature for replay. Gi also now
matches Pi's summary-part state machine: bare
`response.reasoning_summary_text.delta` events are ignored unless a preceding
`response.reasoning_summary_part.added` opened a summary part, which prevents
provider noise from surfacing as a hidden `Thinking...` label in the TUI.

The interactive host also has a live-session regression test for Pi-style
stream replacement: a hidden `thinking_delta` partial must be updated in the
same assistant component and removed when a later final assistant text message
arrives. It must not remain as a separate stale `Thinking...` row above the
final answer.

Live prompt/status layout is also pinned: on `agent_start` followed by a live
user `message_start`, Gi renders the user message in the chat transcript above
the working status line, while `Working...` stays in the separate status
container below chat and above the editor. This matches Pi's
`chatContainer`/`statusContainer` split and prevents the status spinner from
appearing as a stale transcript row.

Working, manual/auto compaction, branch summarization, auto-retry, and
summarization-retry status use one typed state slot:

```text
session events / host.tui.working
              |
              v
   activeStatusIndicator (one owner)
              |
              v
        statusContainer

replace / typed clear / shutdown
              |
              v
 dispose loader + owned CountdownTimer
```

The temporary `Working...`, `Compacting context...`, `Auto-compacting...`,
`Summarizing branch...`, and `Retrying...` components never become durable
transcript entries. Summary retry events replace the active summary indicator
with a retry countdown and restore the appropriate branch-summary or
compaction indicator when the next attempt begins. A type-filtered completion
event cannot accidentally clear a newer status owner.

Thinking-level UI updates now match Pi's event path: `thinking_level_changed`
refreshes the footer and immediately reapplies the editor border color for the
current reasoning level instead of waiting for a later full render/build.

Theme data now follows one directional path:

```text
raw SettingsManager value
        |
        v
fixed name or immutable AutoThemeSetting
        |
        +---- terminal query/report ----+
        |                               |
        v                               v
 revisioned interactiveThemeController state
        |
        v
 serialized global palette transition
        |
        v
 invalidate -> editor border refresh -> render
```

Terminal queries never run while controller or palette-transition locks are
held. A newer selection advances the revision, so a delayed OSC response cannot
overwrite it. Preview changes only the palette; the committed name remains the
restore point. Shutdown cancels in-flight detection, unsubscribes the listener,
and disables terminal notifications. Terminal implementations explicitly
report whether they are headless: virtual terminals resolve from the immutable
environment snapshot and skip OSC queries, while process terminals retain
Pi-compatible active detection.

The settings submenu adds a separate transaction value above that runtime
owner. Its single/automatic mode and single/light/dark choices are copied into
one `settingsThemeSelection`; navigation emits preview-only projections, Apply
publishes one complete fixed or `light/dark` setting, and cancel restores the
original preview without mutating `SettingsManager`.

Theme loading now enforces the same boundary as that grammar: custom names
cannot contain `/`, including discovery and HTML export paths. Pi v0.82.0's
`thinkingMax` token is distinct in both built-ins; legacy custom themes that
omit it inherit `thinkingXhigh` consistently in terminal and CSS projections.

Edit diff rendering now matches Pi's `diff.ts` surface: Gi parses `+/-/ `
line-numbered diff rows, applies `toolDiffAdded` / `toolDiffRemoved` /
`toolDiffContext` colors, replaces tabs with three spaces, and applies inverse
highlighting only for one removed row followed by one added row.

## Open Follow-ups

- Finish Pi test-case to Gi test-case mapping for each component above.
