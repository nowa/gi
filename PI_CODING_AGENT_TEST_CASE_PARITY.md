<!-- markdownlint-disable MD013 MD033 MD060 -->

# Pi Coding Agent Test Case Parity

This file is the scope gate for the Gi `gi-coding-agent` port. It is generated from Pi `packages/coding-agent/test/*.test.ts` and maps every explicit `it(...)` / `test(...)` case to one of the allowed migration states.

Status meanings:

- `已有`: behavior already has Gi coverage in the listed Go tests.
- `待实现`: missing coding-agent host/runtime behavior; implement in Gi before marking covered.
- `需要协议 runtime`: the Pi case touches extensions, packages, dynamic registration, RPC, or custom TUI contribution surfaces; implement through `protocol/spec` boundaries instead of private core branches.
- `不适用`: intentionally excluded from the Go port. No cases are marked this way yet.

## Summary

- Pi coding-agent test files: `91`
- Pi explicit case definitions: `1037`
- `已有`: `876`
- `待实现`: `0`
- `需要协议 runtime`: `161`
- `不适用`: `0`

## Commit Plan

1. Commit this parity baseline by itself.
2. Commit host/runtime foundations in coherent batches: settings/auth/model registry, session runtime/event bus, tools/permissions, TUI shell.
3. Commit protocol-backed batches when package, extension, RPC, or custom TUI cases move from `需要协议 runtime` to `已有`.
4. Commit official packages after the Pi coding-agent parity gate is green.

## File-Level Scope

| Pi file | Cases | Status | Gi coverage / next step |
|---|---:|---|---|
| `agent-session-auto-compaction-queue.test.ts` | 6 | 已有 | gi-coding-agent/agent_session_auto_compaction_test.go |
| `agent-session-branching.test.ts` | 3 | 已有 | gi-coding-agent/agent_session_branching_test.go |
| `agent-session-compaction.test.ts` | 5 | 已有 | gi-coding-agent/agent_session_compaction_test.go |
| `agent-session-concurrent.test.ts` | 7 | 已有 | gi-coding-agent/agent_session_concurrent_test.go; gi-coding-agent/agent_session_concurrent_extension_test.go |
| `agent-session-dynamic-provider.test.ts` | 3 | 已有 | gi-coding-agent/agent_session_dynamic_provider_tools_test.go |
| `agent-session-dynamic-tools.test.ts` | 3 | 已有 | gi-coding-agent/agent_session_dynamic_provider_tools_test.go |
| `agent-session-retry.test.ts` | 5 | 已有 | gi-coding-agent/agent_session_retry_test.go |
| `agent-session-runtime-events.test.ts` | 4 | 已有 | gi-coding-agent/agent_session_runtime_events_test.go |
| `agent-session-stats.test.ts` | 3 | 已有 | gi-coding-agent/agent_session_stats_test.go |
| `agent-session-tree-navigation.test.ts` | 10 | 已有 | gi-coding-agent/agent_session_tree_navigation_test.go |
| `ansi-utils.test.ts` | 5 | 已有 | gi-coding-agent/utils_test.go |
| `args.test.ts` | 60 | 已有 | gi-coding-agent/args_test.go |
| `assistant-message.test.ts` | 2 | 已有 | gi-coding-agent/message_components_test.go |
| `auth-storage.test.ts` | 24 | 已有 | gi-coding-agent/auth_storage_test.go |
| `bash-close-hang-windows.test.ts` | 2 | 已有 | gi-coding-agent/bash_executor_test.go |
| `bash-execution-width.test.ts` | 2 | 已有 | gi-coding-agent/bash_execution_test.go |
| `block-images.test.ts` | 8 | 已有 | gi-coding-agent/block_images_test.go |
| `clipboard-image-bmp-conversion.test.ts` | 1 | 已有 | gi-coding-agent/clipboard_image_test.go |
| `clipboard-image.test.ts` | 5 | 已有 | gi-coding-agent/clipboard_image_test.go |
| `clipboard.test.ts` | 5 | 已有 | gi-coding-agent/clipboard_test.go |
| `compaction-extensions-example.test.ts` | 2 | 已有 | gi-coding-agent/agent_session_compaction_extensions_test.go |
| `compaction-extensions.test.ts` | 8 | 已有 | gi-coding-agent/agent_session_compaction_extensions_test.go |
| `compaction-serialization.test.ts` | 3 | 已有 | gi-agent-core/harness/compaction_test.go |
| `compaction-summary-reasoning.test.ts` | 4 | 已有 | gi-agent-core/harness/compaction_summary_reasoning_test.go |
| `compaction.test.ts` | 23 | 已有 | gi-agent-core/harness/compaction_test.go, gi-agent-core/harness/compaction_pi_parity_test.go |
| `config.test.ts` | 13 | 已有 | gi-coding-agent/config_test.go |
| `edit-tool-legacy-input.test.ts` | 8 | 已有 | gi-coding-agent/edit_tool_definition_test.go |
| `edit-tool-no-full-redraw.test.ts` | 3 | 已有 | gi-coding-agent/edit_tool_no_full_redraw_test.go |
| `export-html-skill-block.test.ts` | 4 | 已有 | gi-coding-agent/export_html_skill_block_test.go |
| `export-html-whitespace.test.ts` | 3 | 已有 | gi-coding-agent/export_html_whitespace_test.go |
| `export-html-xss.test.ts` | 8 | 已有 | gi-coding-agent/export_html_xss_test.go |
| `extensions-discovery.test.ts` | 27 | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| `extensions-input-event.test.ts` | 8 | 已有 | gi-coding-agent/protocol_input_event_test.go |
| `extensions-runner.test.ts` | 27 | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| `file-mutation-queue.test.ts` | 5 | 已有 | gi-coding-agent/file_mutation_queue_test.go |
| `footer-data-provider.test.ts` | 8 | 已有 | gi-coding-agent/footer_data_provider_test.go |
| `footer-width.test.ts` | 2 | 已有 | gi-coding-agent/footer_test.go |
| `frontmatter.test.ts` | 8 | 已有 | gi-coding-agent/utils_test.go |
| `git-ssh-url.test.ts` | 9 | 已有 | gi-coding-agent/git_test.go |
| `git-update.test.ts` | 11 | 已有 | gi-coding-agent/package_manager_git_update_test.go |
| `image-processing.test.ts` | 9 | 已有 | gi-coding-agent/image_resize_test.go |
| `image-resize-callers.test.ts` | 2 | 已有 | gi-coding-agent/image_resize_test.go |
| `initial-message.test.ts` | 3 | 已有 | gi-coding-agent/initial_message_test.go |
| `interactive-mode-anthropic-warning.test.ts` | 4 | 已有 | gi-coding-agent/anthropic_warning_test.go |
| `interactive-mode-clone-command.test.ts` | 2 | 已有 | gi-coding-agent/interactive_mode_test.go |
| `interactive-mode-compaction.test.ts` | 1 | 已有 | gi-coding-agent/interactive_mode_test.go |
| `interactive-mode-import-command.test.ts` | 6 | 已有 | gi-coding-agent/interactive_mode_test.go |
| `interactive-mode-status.test.ts` | 25 | 已有 | gi-coding-agent/interactive_status_test.go |
| `interactive-mode-suspend.test.ts` | 3 | 已有 | gi-coding-agent/interactive_mode_test.go |
| `keybindings-migration.test.ts` | 3 | 已有 | gi-coding-agent/keybindings_test.go |
| `model-registry.test.ts` | 64 | 已有 | gi-coding-agent/model_registry_test.go |
| `model-resolver.test.ts` | 31 | 已有 | gi-coding-agent/model_resolver_test.go |
| `oauth-selector.test.ts` | 6 | 已有 | gi-coding-agent/oauth_selector_test.go |
| `package-command-paths.test.ts` | 10 | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| `package-manager-ssh.test.ts` | 8 | 已有 | gi-coding-agent/package_manager_source_test.go |
| `package-manager.test.ts` | 102 | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| `path-utils.test.ts` | 11 | 已有 | gi-coding-agent/utils_test.go |
| `paths.test.ts` | 12 | 已有 | gi-coding-agent/utils_test.go |
| `pi-user-agent.test.ts` | 1 | 已有 | gi-coding-agent/utils_test.go |
| `plan-mode-utils.test.ts` | 33 | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| `print-mode.test.ts` | 3 | 已有 | gi-coding-agent/print_mode_test.go |
| `prompt-templates.test.ts` | 82 | 已有 | gi-coding-agent/prompt_templates_test.go |
| `resource-loader.test.ts` | 19 | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| `restore-sandbox-env.test.ts` | 3 | 已有 | gi-coding-agent/restore_sandbox_env_test.go |
| `rpc-client-clone.test.ts` | 1 | 已有 | gi-coding-agent/rpc_client_test.go |
| `rpc-jsonl.test.ts` | 4 | 已有 | gi-coding-agent/rpc_jsonl_test.go |
| `rpc-prompt-response-semantics.test.ts` | 3 | 已有 | gi-coding-agent/rpc_prompt_response_semantics_test.go |
| `rpc.test.ts` | 14 | 已有 | gi-coding-agent/rpc_session_host_test.go |
| `sdk-openrouter-attribution.test.ts` | 4 | 已有 | gi-coding-agent/sdk_attribution_test.go |
| `sdk-session-manager.test.ts` | 3 | 已有 | gi-coding-agent/sdk_session_test.go |
| `sdk-skills.test.ts` | 3 | 已有 | gi-coding-agent/sdk_session_test.go |
| `session-cwd.test.ts` | 3 | 已有 | gi-coding-agent/session_cwd_test.go |
| `session-info-modified-timestamp.test.ts` | 1 | 已有 | gi-coding-agent/session_manager_migration_list_test.go |
| `session-selector-path-delete.test.ts` | 7 | 已有 | gi-coding-agent/session_selector_path_delete_test.go |
| `session-selector-rename.test.ts` | 3 | 已有 | gi-coding-agent/session_selector_rename_test.go |
| `session-selector-search.test.ts` | 9 | 已有 | gi-coding-agent/session_selector_search_test.go |
| `settings-manager-bug.test.ts` | 4 | 已有 | gi-coding-agent/settings_manager_test.go |
| `settings-manager.test.ts` | 17 | 已有 | gi-coding-agent/settings_manager_test.go |
| `skills.test.ts` | 28 | 已有 | gi-agent-core/harness/skills_test.go |
| `stdout-cleanliness.test.ts` | 2 | 已有 | gi-coding-agent/stdout_cleanliness_test.go |
| `syntax-highlight.test.ts` | 5 | 已有 | gi-coding-agent/syntax_highlight_test.go |
| `system-prompt.test.ts` | 7 | 已有 | gi-coding-agent/system_prompt_test.go |
| `test-harness.test.ts` | 15 | 已有 | gi-coding-agent/test_harness_test.go |
| `theme-export.test.ts` | 2 | 已有 | gi-coding-agent/theme_export_test.go |
| `tool-execution-component.test.ts` | 16 | 已有 | gi-coding-agent/tool_execution_component_test.go |
| `tools.test.ts` | 68 | 已有 | gi-coding-agent/tools_read_test.go, gi-coding-agent/tools_bash_advanced_test.go, gi-coding-agent/tools_search_test.go, gi-coding-agent/tools_edit_fuzzy_test.go |
| `tree-selector.test.ts` | 15 | 已有 | gi-coding-agent/tree_selector_test.go |
| `trigger-compact-extension.test.ts` | 1 | 已有 | gi-coding-agent/trigger_compact_extension_test.go |
| `truncate-to-width.test.ts` | 6 | 已有 | gi-tui/utils_test.go |
| `user-message.test.ts` | 1 | 已有 | gi-coding-agent/message_components_test.go |
| `version-check.test.ts` | 5 | 已有 | gi-coding-agent/version_check_test.go |

## `agent-session-auto-compaction-queue.test.ts`

Pi cases: `6`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/agent_session_auto_compaction_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 100 | should resume after threshold compaction when only agent-level queued messages exist | 已有 | gi-coding-agent/agent_session_auto_compaction_test.go |
| 126 | should not compact repeatedly after overflow recovery already attempted | 已有 | gi-coding-agent/agent_session_auto_compaction_test.go |
| 181 | should ignore stale pre-compaction assistant usage on pre-prompt compaction checks | 已有 | gi-coding-agent/agent_session_auto_compaction_test.go |
| 238 | should trigger threshold compaction for error messages using last successful usage | 已有 | gi-coding-agent/agent_session_auto_compaction_test.go |
| 308 | should not trigger threshold compaction for error messages when no prior usage exists | 已有 | gi-coding-agent/agent_session_auto_compaction_test.go |
| 356 | should not trigger threshold compaction for error messages when only kept pre-compaction usage exists | 已有 | gi-coding-agent/agent_session_auto_compaction_test.go |

## `agent-session-branching.test.ts`

Pi cases: `3`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/agent_session_branching_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 90 | should allow forking from single message | 已有 | gi-coding-agent/agent_session_branching_test.go |
| 110 | should support in-memory forking in --no-session mode | 已有 | gi-coding-agent/agent_session_branching_test.go |
| 131 | should fork from middle of conversation | 已有 | gi-coding-agent/agent_session_branching_test.go |

## `agent-session-compaction.test.ts`

Pi cases: `5`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/agent_session_compaction_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 83 | should trigger manual compaction via compact() | 已有 | gi-coding-agent/agent_session_compaction_test.go |
| 109 | should maintain valid session state after compaction | 已有 | gi-coding-agent/agent_session_compaction_test.go |
| 134 | should persist compaction to session file | 已有 | gi-coding-agent/agent_session_compaction_test.go |
| 162 | should work with --no-session mode (in-memory only) | 已有 | gi-coding-agent/agent_session_compaction_test.go |
| 184 | should emit compaction events during manual compaction | 已有 | gi-coding-agent/agent_session_compaction_test.go |

## `agent-session-concurrent.test.ts`

Pi cases: `7`
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/agent_session_concurrent_test.go; gi-coding-agent/agent_session_concurrent_extension_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 130 | should throw when prompt() called while streaming | 已有 | gi-coding-agent/agent_session_concurrent_test.go |
| 152 | should allow steer() while streaming | 已有 | gi-coding-agent/agent_session_concurrent_test.go |
| 168 | should allow followUp() while streaming | 已有 | gi-coding-agent/agent_session_concurrent_test.go |
| 184 | should queue extension-origin steering messages while streaming | 已有 | gi-coding-agent/agent_session_concurrent_extension_test.go |
| 294 | should allow prompt() after previous completes | 已有 | gi-coding-agent/agent_session_concurrent_test.go |
| 339 | should wait for queued agent events before emitting tool_call | 已有 | gi-coding-agent/agent_session_concurrent_extension_test.go |
| 484 | should persist message_end events in order with slow extension handlers | 已有 | gi-coding-agent/agent_session_concurrent_extension_test.go |

## `agent-session-dynamic-provider.test.ts`

Pi cases: `3`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/agent_session_dynamic_provider_tools_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 67 | applies top-level registerProvider overrides to the active model | 已有 | gi-coding-agent/agent_session_dynamic_provider_tools_test.go |
| 80 | applies session_start registerProvider overrides to the active model | 已有 | gi-coding-agent/agent_session_dynamic_provider_tools_test.go |
| 97 | applies command-time registerProvider overrides without reload | 已有 | gi-coding-agent/agent_session_dynamic_provider_tools_test.go |

## `agent-session-dynamic-tools.test.ts`

Pi cases: `3`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/agent_session_dynamic_provider_tools_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 28 | refreshes tool registry when tools are registered after initialization | 已有 | gi-coding-agent/agent_session_dynamic_provider_tools_test.go |
| 94 | returns source metadata for SDK custom tools | 已有 | gi-coding-agent/agent_session_dynamic_provider_tools_test.go |
| 137 | keeps custom tools active but omits them from available tools when promptSnippet is not provided | 已有 | gi-coding-agent/agent_session_dynamic_provider_tools_test.go |

## `agent-session-retry.test.ts`

Pi cases: `5`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/agent_session_retry_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 132 | retries after a transient error and succeeds | 已有 | gi-coding-agent/agent_session_retry_test.go |
| 147 | exhausts max retries and emits failure | 已有 | gi-coding-agent/agent_session_retry_test.go |
| 164 | prompt waits for retry completion even when assistant message_end handling is delayed | 已有 | gi-coding-agent/agent_session_retry_test.go |
| 173 | retries provider network_error failures | 已有 | gi-coding-agent/agent_session_retry_test.go |
| 231 | prompt waits for full agent loop when retry produces tool calls | 已有 | gi-coding-agent/agent_session_retry_test.go |

## `agent-session-runtime-events.test.ts`

Pi cases: `4`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/agent_session_runtime_events_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 92 | emits session_before_switch and session_start for new and resume flows | 已有 | gi-coding-agent/agent_session_runtime_events_test.go |
| 136 | honors session_before_switch cancellation | 已有 | gi-coding-agent/agent_session_runtime_events_test.go |
| 160 | runs beforeSessionInvalidate after session_shutdown and before rebindSession | 已有 | gi-coding-agent/agent_session_runtime_events_test.go |
| 186 | emits session_before_fork and session_start and honors cancellation | 已有 | gi-coding-agent/agent_session_runtime_events_test.go |

## `agent-session-stats.test.ts`

Pi cases: `3`
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/agent_session_stats_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 81 | exposes the current context usage alongside token totals | 已有 | gi-coding-agent/agent_session_stats_test.go |
| 99 | reports unknown current context usage immediately after compaction | 已有 | gi-coding-agent/agent_session_stats_test.go |
| 121 | uses post-compaction usage for current context instead of stale kept usage | 已有 | gi-coding-agent/agent_session_stats_test.go |

## `agent-session-tree-navigation.test.ts`

Pi cases: `10`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/agent_session_tree_navigation_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 29 | should navigate to user message and put text in editor | 已有 | gi-coding-agent/agent_session_tree_navigation_test.go |
| 56 | should navigate to non-user message without editor text | 已有 | gi-coding-agent/agent_session_tree_navigation_test.go |
| 78 | should create branch summary when navigating with summarize=true | 已有 | gi-coding-agent/agent_session_tree_navigation_test.go |
| 108 | should attach summary to correct parent when navigating to nested user message | 已有 | gi-coding-agent/agent_session_tree_navigation_test.go |
| 147 | should attach summary to selected node when navigating to assistant message | 已有 | gi-coding-agent/agent_session_tree_navigation_test.go |
| 175 | should handle abort during summarization | 已有 | gi-coding-agent/agent_session_tree_navigation_test.go |
| 214 | should not create summary when navigating without summarize option | 已有 | gi-coding-agent/agent_session_tree_navigation_test.go |
| 238 | should handle navigation to same position (no-op) | 已有 | gi-coding-agent/agent_session_tree_navigation_test.go |
| 257 | should support custom summarization instructions | 已有 | gi-coding-agent/agent_session_tree_navigation_test.go |
| 292 | should navigate between branches correctly | 已有 | gi-coding-agent/agent_session_tree_navigation_test.go |

## `ansi-utils.test.ts`

Pi cases: `5`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/utils_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 77 | matches chalk strip-ansi for generated compatibility inputs | 已有 | gi-coding-agent/utils_test.go |
| 83 | throws the same TypeError as chalk strip-ansi for non-string values | 已有 | gi-coding-agent/utils_test.go |
| 93 | strips RIS without leaking the final byte | 已有 | gi-coding-agent/utils_test.go |
| 97 | strips single-byte ESC sequences without leaking final bytes | 已有 | gi-coding-agent/utils_test.go |
| 106 | strips common ANSI sequences used in tool output | 已有 | gi-coding-agent/utils_test.go |

## `args.test.ts`

Pi cases: `60`  
Status: `已有`  
Gi coverage / implementation target: `gi-coding-agent/args_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 6 | parses --version flag | 已有 | gi-coding-agent/args_test.go |
| 11 | parses -v shorthand | 已有 | gi-coding-agent/args_test.go |
| 16 | --version takes precedence over other args | 已有 | gi-coding-agent/args_test.go |
| 25 | parses --help flag | 已有 | gi-coding-agent/args_test.go |
| 30 | parses -h shorthand | 已有 | gi-coding-agent/args_test.go |
| 37 | parses --print flag | 已有 | gi-coding-agent/args_test.go |
| 42 | parses -p shorthand | 已有 | gi-coding-agent/args_test.go |
| 47 | parses prompt after -p even when it starts with YAML frontmatter | 已有 | gi-coding-agent/args_test.go |
| 55 | does not consume options after -p as prompts | 已有 | gi-coding-agent/args_test.go |
| 64 | parses --continue flag | 已有 | gi-coding-agent/args_test.go |
| 69 | parses -c shorthand | 已有 | gi-coding-agent/args_test.go |
| 76 | parses --resume flag | 已有 | gi-coding-agent/args_test.go |
| 81 | parses -r shorthand | 已有 | gi-coding-agent/args_test.go |
| 88 | parses --provider | 已有 | gi-coding-agent/args_test.go |
| 93 | parses --model | 已有 | gi-coding-agent/args_test.go |
| 98 | parses --api-key | 已有 | gi-coding-agent/args_test.go |
| 103 | parses --system-prompt | 已有 | gi-coding-agent/args_test.go |
| 108 | parses --append-system-prompt | 已有 | gi-coding-agent/args_test.go |
| 113 | parses multiple --append-system-prompt flags | 已有 | gi-coding-agent/args_test.go |
| 118 | parses --mode | 已有 | gi-coding-agent/args_test.go |
| 123 | parses --mode rpc | 已有 | gi-coding-agent/args_test.go |
| 128 | parses --session | 已有 | gi-coding-agent/args_test.go |
| 133 | parses --fork | 已有 | gi-coding-agent/args_test.go |
| 139 | parses --export | 已有 | gi-coding-agent/args_test.go |
| 144 | parses --thinking | 已有 | gi-coding-agent/args_test.go |
| 149 | parses --models as comma-separated list | 已有 | gi-coding-agent/args_test.go |
| 156 | parses --no-session flag | 已有 | gi-coding-agent/args_test.go |
| 163 | parses single --extension | 已有 | gi-coding-agent/args_test.go |
| 168 | parses -e shorthand | 已有 | gi-coding-agent/args_test.go |
| 173 | parses multiple --extension flags | 已有 | gi-coding-agent/args_test.go |
| 180 | parses --no-extensions flag | 已有 | gi-coding-agent/args_test.go |
| 185 | parses --no-extensions with explicit -e flags | 已有 | gi-coding-agent/args_test.go |
| 193 | parses single --skill | 已有 | gi-coding-agent/args_test.go |
| 198 | parses multiple --skill flags | 已有 | gi-coding-agent/args_test.go |
| 205 | parses single --prompt-template | 已有 | gi-coding-agent/args_test.go |
| 210 | parses multiple --prompt-template flags | 已有 | gi-coding-agent/args_test.go |
| 217 | parses single --theme | 已有 | gi-coding-agent/args_test.go |
| 222 | parses multiple --theme flags | 已有 | gi-coding-agent/args_test.go |
| 229 | parses --no-skills flag | 已有 | gi-coding-agent/args_test.go |
| 236 | parses --no-prompt-templates flag | 已有 | gi-coding-agent/args_test.go |
| 243 | parses --no-themes flag | 已有 | gi-coding-agent/args_test.go |
| 250 | parses --no-context-files flag | 已有 | gi-coding-agent/args_test.go |
| 255 | parses -nc shorthand | 已有 | gi-coding-agent/args_test.go |
| 262 | parses --verbose flag | 已有 | gi-coding-agent/args_test.go |
| 269 | parses --offline flag | 已有 | gi-coding-agent/args_test.go |
| 276 | parses --no-tools flag | 已有 | gi-coding-agent/args_test.go |
| 281 | parses -nt shorthand | 已有 | gi-coding-agent/args_test.go |
| 286 | parses --no-builtin-tools flag | 已有 | gi-coding-agent/args_test.go |
| 291 | parses -nbt shorthand | 已有 | gi-coding-agent/args_test.go |
| 296 | parses --tools flag | 已有 | gi-coding-agent/args_test.go |
| 301 | parses -t shorthand | 已有 | gi-coding-agent/args_test.go |
| 306 | parses --no-tools with explicit --tools flags | 已有 | gi-coding-agent/args_test.go |
| 312 | parses --no-builtin-tools with explicit --tools flags | 已有 | gi-coding-agent/args_test.go |
| 320 | parses plain text messages | 已有 | gi-coding-agent/args_test.go |
| 325 | parses @file arguments | 已有 | gi-coding-agent/args_test.go |
| 330 | parses mixed messages and file args | 已有 | gi-coding-agent/args_test.go |
| 336 | captures unknown long flags with string values | 已有 | gi-coding-agent/args_test.go |
| 342 | captures unknown boolean long flags | 已有 | gi-coding-agent/args_test.go |
| 347 | captures unknown long flags with equals syntax | 已有 | gi-coding-agent/args_test.go |
| 354 | parses multiple flags together | 已有 | gi-coding-agent/args_test.go |

## `assistant-message.test.ts`

Pi cases: `2`  
Status: `已有`  
Gi coverage / implementation target: `gi-coding-agent/message_components_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 31 | adds OSC 133 zone markers to assistant messages without tool calls | 已有 | gi-coding-agent/message_components_test.go |
| 42 | does not add OSC 133 zone markers when assistant message contains tool calls | 已有 | gi-coding-agent/message_components_test.go |

## `auth-storage.test.ts`

Pi cases: `24`  
Status: `已有`  
Gi coverage / implementation target: `gi-coding-agent/auth_storage_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 38 | literal API key is returned directly | 已有 | gi-coding-agent/auth_storage_test.go |
| 49 | apiKey with ! prefix executes command and uses stdout | 已有 | gi-coding-agent/auth_storage_test.go |
| 60 | apiKey with ! prefix trims whitespace from command output | 已有 | gi-coding-agent/auth_storage_test.go |
| 71 | apiKey with ! prefix handles multiline output (uses trimmed result) | 已有 | gi-coding-agent/auth_storage_test.go |
| 82 | apiKey with ! prefix returns undefined on command failure | 已有 | gi-coding-agent/auth_storage_test.go |
| 93 | apiKey with ! prefix returns undefined on nonexistent command | 已有 | gi-coding-agent/auth_storage_test.go |
| 104 | apiKey with ! prefix returns undefined on empty output | 已有 | gi-coding-agent/auth_storage_test.go |
| 115 | apiKey as environment variable name resolves to env value | 已有 | gi-coding-agent/auth_storage_test.go |
| 137 | apiKey as literal value is used directly when not an env var | 已有 | gi-coding-agent/auth_storage_test.go |
| 151 | apiKey command can use shell features like pipes | 已有 | gi-coding-agent/auth_storage_test.go |
| 163 | command is only executed once per process | 已有 | gi-coding-agent/auth_storage_test.go |
| 186 | cache persists across AuthStorage instances | 已有 | gi-coding-agent/auth_storage_test.go |
| 208 | clearConfigValueCache allows command to run again | 已有 | gi-coding-agent/auth_storage_test.go |
| 230 | different commands are cached separately | 已有 | gi-coding-agent/auth_storage_test.go |
| 245 | failed commands are cached (not retried) | 已有 | gi-coding-agent/auth_storage_test.go |
| 269 | environment variables are not cached (changes are picked up) | 已有 | gi-coding-agent/auth_storage_test.go |
| 302 | returns undefined on compromised lock and allows a later retry | 已有 | gi-coding-agent/auth_storage_test.go |
| 351 | set preserves unrelated external edits | 已有 | gi-coding-agent/auth_storage_test.go |
| 374 | remove preserves unrelated external edits | 已有 | gi-coding-agent/auth_storage_test.go |
| 397 | does not overwrite malformed auth file after load error | 已有 | gi-coding-agent/auth_storage_test.go |
| 412 | reload records parse errors and drainErrors clears buffer | 已有 | gi-coding-agent/auth_storage_test.go |
| 435 | does not expose stored API keys or OAuth tokens | 已有 | gi-coding-agent/auth_storage_test.go |
| 455 | runtime override takes priority over auth.json | 已有 | gi-coding-agent/auth_storage_test.go |
| 468 | removing runtime override falls back to auth.json | 已有 | gi-coding-agent/auth_storage_test.go |

## `bash-close-hang-windows.test.ts`

Pi cases: `2`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/bash_executor_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 84 | executeBash resolves after the shell exits even if inherited stdio handles stay open | 已有 | gi-coding-agent/bash_executor_test.go |
| 109 | bash tool resolves after the shell exits even if inherited stdio handles stay open | 已有 | gi-coding-agent/bash_executor_test.go |

## `bash-execution-width.test.ts`

Pi cases: `2`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/bash_execution_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 35 | collapsed preview lines respect render-time width, not construction-time width | 已有 | gi-coding-agent/bash_execution_test.go |
| 59 | re-computes lines when width changes between renders | 已有 | gi-coding-agent/bash_execution_test.go |

## `block-images.test.ts`

Pi cases: `8`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/block_images_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 15 | should default blockImages to false | 已有 | gi-coding-agent/block_images_test.go |
| 20 | should return true when blockImages is set to true | 已有 | gi-coding-agent/block_images_test.go |
| 25 | should persist blockImages setting via setBlockImages | 已有 | gi-coding-agent/block_images_test.go |
| 36 | should handle blockImages alongside autoResize | 已有 | gi-coding-agent/block_images_test.go |
| 57 | should always read images (filtering happens at convertToLlm layer) | 已有 | gi-coding-agent/block_images_test.go |
| 71 | should read text files normally | 已有 | gi-coding-agent/block_images_test.go |
| 98 | should always process images (filtering happens at convertToLlm layer) | 已有 | gi-coding-agent/block_images_test.go |
| 109 | should process text files normally | 已有 | gi-coding-agent/block_images_test.go |

## `clipboard-image-bmp-conversion.test.ts`

Pi cases: `1`
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/clipboard_image_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 70 | converts BMP to PNG on Wayland/WSLg | 已有 | gi-coding-agent/clipboard_image_test.go |

## `clipboard-image.test.ts`

Pi cases: `5`
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/clipboard_image_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 58 | Wayland: uses wl-paste and never calls clipboard | 已有 | gi-coding-agent/clipboard_image_test.go |
| 80 | Wayland: falls back to xclip when wl-paste is missing | 已有 | gi-coding-agent/clipboard_image_test.go |
| 111 | WSL: passes PowerShell path directly instead of through a custom env var | 已有 | gi-coding-agent/clipboard_image_test.go |
| 148 | Non-Wayland: uses clipboard | 已有 | gi-coding-agent/clipboard_image_test.go |
| 163 | Non-Wayland: returns null when clipboard has no image | 已有 | gi-coding-agent/clipboard_image_test.go |

## `clipboard.test.ts`

Pi cases: `5`
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/clipboard_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 90 | local native success skips OSC 52 and shell fallbacks | 已有 | gi-coding-agent/clipboard_test.go |
| 99 | remote native success emits OSC 52 after native write | 已有 | gi-coding-agent/clipboard_test.go |
| 114 | local shell fallback success skips OSC 52 | 已有 | gi-coding-agent/clipboard_test.go |
| 128 | uses OSC 52 fallback when native and shell tools fail | 已有 | gi-coding-agent/clipboard_test.go |
| 139 | does not emit oversized OSC 52 payloads | 已有 | gi-coding-agent/clipboard_test.go |

## `compaction-extensions-example.test.ts`

Pi cases: `2`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/agent_session_compaction_extensions_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 9 | custom compaction example should type-check correctly | 已有 | gi-coding-agent/agent_session_compaction_extensions_test.go |
| 50 | compact event should have correct fields | 已有 | gi-coding-agent/agent_session_compaction_extensions_test.go |

## `compaction-extensions.test.ts`

Pi cases: `8`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/agent_session_compaction_extensions_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 122 | should emit before_compact and compact events | 已有 | gi-coding-agent/agent_session_compaction_extensions_test.go |
| 158 | should allow extensions to cancel compaction | 已有 | gi-coding-agent/agent_session_compaction_extensions_test.go |
| 171 | should allow extensions to provide custom compaction | 已有 | gi-coding-agent/agent_session_compaction_extensions_test.go |
| 208 | should include entries in compact event after compaction is saved | 已有 | gi-coding-agent/agent_session_compaction_extensions_test.go |
| 229 | should continue with default compaction if extension throws error | 已有 | gi-coding-agent/agent_session_compaction_extensions_test.go |
| 276 | should call multiple extensions in order | 已有 | gi-coding-agent/agent_session_compaction_extensions_test.go |
| 351 | should pass correct data in before_compact event | 已有 | gi-coding-agent/agent_session_compaction_extensions_test.go |
| 390 | should use extension compaction even with different values | 已有 | gi-coding-agent/agent_session_compaction_extensions_test.go |

## `compaction-serialization.test.ts`

Pi cases: `3`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/export_html_whitespace_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 6 | should truncate long tool results | 已有 | gi-agent-core/harness/compaction_test.go |
| 28 | should not truncate short tool results | 已有 | gi-agent-core/harness/compaction_test.go |
| 47 | should not truncate assistant or user messages | 已有 | gi-agent-core/harness/compaction_test.go |

## `compaction-summary-reasoning.test.ts`

Pi cases: `4`  
Status: `已有`
Gi coverage / implementation target: `gi-agent-core/harness/compaction_summary_reasoning_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 59 | uses the provided thinking level for reasoning-capable models | 已有 | gi-agent-core/harness/compaction_summary_reasoning_test.go |
| 79 | does not set reasoning when thinking is off | 已有 | gi-agent-core/harness/compaction_summary_reasoning_test.go |
| 99 | does not set reasoning for non-reasoning models | 已有 | gi-agent-core/harness/compaction_summary_reasoning_test.go |
| 119 | clamps compaction summary maxTokens to the model output cap | 已有 | gi-agent-core/harness/compaction_summary_reasoning_test.go |

## `compaction.test.ts`

Pi cases: `23`  
Status: `已有`
Gi coverage / implementation target: `gi-agent-core/harness/compaction_test.go, gi-agent-core/harness/compaction_pi_parity_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 178 | should calculate total context tokens from usage | 已有 | gi-agent-core/harness/compaction_pi_parity_test.go |
| 183 | should handle zero values | 已有 | gi-agent-core/harness/compaction_pi_parity_test.go |
| 190 | should find the last non-aborted assistant message usage | 已有 | gi-agent-core/harness/compaction_pi_parity_test.go |
| 203 | should skip aborted messages | 已有 | gi-agent-core/harness/compaction_pi_parity_test.go |
| 221 | should return undefined if no assistant messages | 已有 | gi-agent-core/harness/compaction_pi_parity_test.go |
| 228 | should return true when context exceeds threshold | 已有 | gi-agent-core/harness/compaction_pi_parity_test.go |
| 239 | should return false when disabled | 已有 | gi-agent-core/harness/compaction_pi_parity_test.go |
| 251 | should find cut point based on actual token differences | 已有 | gi-agent-core/harness/compaction_pi_parity_test.go |
| 271 | should return startIndex if no valid cut points in range | 已有 | gi-agent-core/harness/compaction_pi_parity_test.go |
| 277 | should keep everything if all messages fit within budget | 已有 | gi-agent-core/harness/compaction_pi_parity_test.go |
| 289 | should indicate split turn when cutting at assistant message | 已有 | gi-agent-core/harness/compaction_pi_parity_test.go |
| 313 | should load all messages when no compaction | 已有 | gi-agent-core/harness/compaction_pi_parity_test.go |
| 327 | should handle single compaction | 已有 | gi-agent-core/harness/compaction_pi_parity_test.go |
| 346 | should handle multiple compactions (only latest matters) | 已有 | gi-agent-core/harness/compaction_pi_parity_test.go |
| 369 | should keep all messages when firstKeptEntryId is first entry | 已有 | gi-agent-core/harness/compaction_pi_parity_test.go |
| 383 | should track model and thinking level changes | 已有 | gi-agent-core/harness/compaction_pi_parity_test.go |
| 399 | should preserve kept messages across repeated compactions when they still fit | 已有 | gi-agent-core/harness/compaction_pi_parity_test.go |
| 436 | should re-summarize previously kept messages when the recent window moves past them | 已有 | gi-agent-core/harness/compaction_pi_parity_test.go |
| 467 | should parse the large session | 已有 | gi-agent-core/harness/compaction_pi_parity_test.go |
| 475 | should find cut point in large session | 已有 | gi-agent-core/harness/compaction_pi_parity_test.go |
| 485 | should load session correctly | 已有 | gi-agent-core/harness/compaction_pi_parity_test.go |
| 499 | should generate a compaction result for the large session | 已有 | gi-agent-core/harness/compaction_pi_parity_test.go |
| 519 | should produce valid session after compaction | 已有 | gi-agent-core/harness/compaction_pi_parity_test.go |

## `config.test.ts`

Pi cases: `13`  
Status: `已有`  
Gi coverage / implementation target: `gi-coding-agent/config_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 143 | detects pnpm from Windows .pnpm install paths | 已有 | gi-coding-agent/config_test.go |
| 154 | does not self-update unknown wrapper installs | 已有 | gi-coding-agent/config_test.go |
| 164 | self-updates npm installs from custom prefixes | 已有 | gi-coding-agent/config_test.go |
| 177 | self-updates renamed packages from the current install prefix | 已有 | gi-coding-agent/config_test.go |
| 201 | self-update respects configured npmCommand | 已有 | gi-coding-agent/config_test.go |
| 213 | self-update treats empty npmCommand as unset | 已有 | gi-coding-agent/config_test.go |
| 221 | quotes npm self-update display paths | 已有 | gi-coding-agent/config_test.go |
| 229 | does not infer Windows npm custom prefixes from package paths | 已有 | gi-coding-agent/config_test.go |
| 240 | self-updates bun global installs from bun pm bin | 已有 | gi-coding-agent/config_test.go |
| 253 | self-updates renamed pnpm global installs by removing the old package first | 已有 | gi-coding-agent/config_test.go |
| 278 | self-updates renamed yarn global installs by removing the old package first | 已有 | gi-coding-agent/config_test.go |
| 303 | self-updates renamed bun global installs by removing the old package first | 已有 | gi-coding-agent/config_test.go |
| 328 | does not self-update when npm install path is not writable | 已有 | gi-coding-agent/config_test.go |

## `edit-tool-legacy-input.test.ts`

Pi cases: `8`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/edit_tool_definition_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 21 | keeps legacy fields out of the public schema | 已有 | gi-coding-agent/edit_tool_definition_test.go |
| 27 | folds top-level oldText/newText into edits | 已有 | gi-coding-agent/edit_tool_definition_test.go |
| 40 | appends legacy replacement to existing edits | 已有 | gi-coding-agent/edit_tool_definition_test.go |
| 57 | passes through valid input unchanged | 已有 | gi-coding-agent/edit_tool_definition_test.go |
| 67 | passes through non-object input unchanged | 已有 | gi-coding-agent/edit_tool_definition_test.go |
| 74 | prepared args execute correctly | 已有 | gi-coding-agent/edit_tool_definition_test.go |
| 93 | parses edits from a JSON string | 已有 | gi-coding-agent/edit_tool_definition_test.go |
| 105 | leaves edits alone when the string is not valid JSON | 已有 | gi-coding-agent/edit_tool_definition_test.go |

## `edit-tool-no-full-redraw.test.ts`

Pi cases: `3`
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/edit_tool_no_full_redraw_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 79 | renders the large diff in the call preview and does not full-redraw when the result settles | 已有 | gi-coding-agent/edit_tool_no_full_redraw_test.go |
| 152 | reconstructs the boxed preview from a settled result without argsComplete | 已有 | gi-coding-agent/edit_tool_no_full_redraw_test.go |
| 201 | shows a preflight error without rendering a diff when the edits do not apply | 已有 | gi-coding-agent/edit_tool_no_full_redraw_test.go |

## `export-html-skill-block.test.ts`

Pi cases: `4`  
Status: `待实现`  
Gi coverage / implementation target: `补 Gi coding-agent 本体 host/runtime 后添加 Go 对应测试`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 7 | strips skill wrapper XML from user message rendering | 已有 | gi-coding-agent/export_html_skill_block_test.go |
| 16 | renders skill invocation and user message as separate sibling blocks | 已有 | gi-coding-agent/export_html_skill_block_test.go |
| 29 | renders skill content as markdown, not raw text | 已有 | gi-coding-agent/export_html_skill_block_test.go |
| 35 | shows skill name and user message in the sidebar tree | 已有 | gi-coding-agent/export_html_skill_block_test.go |

## `export-html-whitespace.test.ts`

Pi cases: `3`  
Status: `待实现`  
Gi coverage / implementation target: `补 Gi coding-agent 本体 host/runtime 后添加 Go 对应测试`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 10 | preserves whitespace for plain-text tool output lines without preserving template whitespace | 已有 | gi-coding-agent/export_html_whitespace_test.go |
| 20 | does not insert source whitespace between ANSI-rendered lines | 已有 | gi-coding-agent/export_html_whitespace_test.go |
| 24 | trims TUI spacing lines from custom tool result HTML | 已有 | gi-coding-agent/export_html_whitespace_test.go |

## `export-html-xss.test.ts`

Pi cases: `8`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/export_html_xss_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 7 | overrides the marked link renderer to block javascript: protocol | 已有 | gi-coding-agent/export_html_xss_test.go |
| 14 | overrides the marked image renderer to block javascript: protocol | 已有 | gi-coding-agent/export_html_xss_test.go |
| 18 | escapes href attributes in the custom link renderer | 已有 | gi-coding-agent/export_html_xss_test.go |
| 23 | escapes image mimeType attributes | 已有 | gi-coding-agent/export_html_xss_test.go |
| 29 | escapes image data attributes | 已有 | gi-coding-agent/export_html_xss_test.go |
| 35 | escapes entry IDs before inserting them into attributes | 已有 | gi-coding-agent/export_html_xss_test.go |
| 43 | escapes tree metadata rendered from session fields | 已有 | gi-coding-agent/export_html_xss_test.go |
| 57 | escapes model names in the exported header | 已有 | gi-coding-agent/export_html_xss_test.go |

## `extensions-discovery.test.ts`

Pi cases: `27`  
Status: `需要协议 runtime`  
Gi coverage / implementation target: `按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 43 | discovers direct .ts files in extensions/ | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 54 | discovers direct .js files in extensions/ | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 64 | discovers subdirectory with index.ts | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 77 | discovers subdirectory with index.js | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 89 | prefers index.ts over index.js | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 102 | discovers subdirectory with package.json pi field | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 126 | package.json can declare multiple extensions | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 147 | package.json with pi field takes precedence over index.ts | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 172 | ignores package.json without pi field, falls back to index.ts | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 191 | ignores subdirectory without index or package.json | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 203 | does not recurse beyond one level | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 217 | handles mixed direct files and subdirectories | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 238 | skips non-existent paths declared in package.json | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 258 | loads extensions and registers commands | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 268 | loads extensions and registers tools | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 278 | reports errors for invalid extension code | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 288 | handles explicitly configured paths | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 300 | resolves dependencies from extension's own node_modules | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 313 | registers message renderers | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 330 | reports error when extension throws during initialization | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 345 | reports error when extension has no default export | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 360 | allows multiple extensions to register different tools | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 379 | loads extension with event handlers | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 398 | loads extension with shortcuts | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 416 | loads extension with flags | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 434 | loadExtensions only loads explicit paths without discovery | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 452 | loadExtensions with no paths loads nothing | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |

## `extensions-input-event.test.ts`

Pi cases: `8`
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/protocol_input_event_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 36 | returns continue when no handlers, undefined return, or explicit continue | 已有 | gi-coding-agent/protocol_input_event_test.go |
| 47 | transforms text and preserves images when omitted | 已有 | gi-coding-agent/protocol_input_event_test.go |
| 56 | transforms and replaces images when provided | 已有 | gi-coding-agent/protocol_input_event_test.go |
| 68 | chains transforms across multiple handlers | 已有 | gi-coding-agent/protocol_input_event_test.go |
| 77 | short-circuits on handled and skips subsequent handlers | 已有 | gi-coding-agent/protocol_input_event_test.go |
| 87 | passes source correctly for all source types | 已有 | gi-coding-agent/protocol_input_event_test.go |
| 97 | catches handler errors and continues | 已有 | gi-coding-agent/protocol_input_event_test.go |
| 106 | hasHandlers returns correct value | 已有 | gi-coding-agent/protocol_input_event_test.go |

## `extensions-runner.test.ts`

Pi cases: `27`  
Status: `需要协议 runtime`  
Gi coverage / implementation target: `按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 84 | warns when extension shortcut conflicts with built-in | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 107 | allows a shortcut when the reserved set no longer contains the default key | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 131 | warns but allows when extension uses non-reserved built-in shortcut | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 159 | blocks shortcuts for reserved actions even when rebound | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 183 | blocks shortcuts when reserved key is also bound to non-reserved actions | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 206 | blocks shortcuts when reserved action has multiple keys | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 230 | warns but allows when non-reserved action has multiple keys | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 256 | warns when two extensions register same shortcut | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 292 | collects tools from multiple extensions | 已有 | gi-coding-agent/protocol_extension_runner_test.go |
| 316 | keeps first tool when two extensions register the same name | 已有 | gi-coding-agent/protocol_extension_runner_test.go |
| 354 | collects commands from multiple extensions | 已有 | gi-coding-agent/protocol_extension_runner_test.go |
| 375 | gets command by invocation name | 已有 | gi-coding-agent/protocol_extension_runner_test.go |
| 399 | suffixes duplicate extension commands in insertion order | 已有 | gi-coding-agent/protocol_extension_runner_test.go |
| 427 | exposes the current abort signal on ExtensionContext | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 447 | calls error listeners when handler throws | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 475 | gets message renderer by type | 已有 | gi-coding-agent/protocol_extension_runner_test.go |
| 495 | collects flags from extensions | 已有 | gi-coding-agent/protocol_extension_runner_test.go |
| 513 | keeps first flag when two extensions register the same name | 已有 | gi-coding-agent/protocol_extension_runner_test.go |
| 543 | can set flag values | 已有 | gi-coding-agent/protocol_extension_runner_test.go |
| 566 | keeps ctx.getSystemPrompt() in sync with chained system prompt updates | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 610 | chains content modifications across handlers | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 657 | preserves previous modifications when later handlers return partial patches | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 702 | bindCore ignores invalid queued registrations and reports extension error | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 725 | pre-bind unregister removes all queued registrations for a provider | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 749 | post-bind register and unregister take effect immediately | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 766 | passes fork options through to the bound handler | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 790 | returns true when handlers exist for event type | 已有 | gi-coding-agent/protocol_extension_runner_test.go |

## `file-mutation-queue.test.ts`

Pi cases: `5`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/file_mutation_queue_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 26 | serializes operations for the same file | 已有 | gi-coding-agent/file_mutation_queue_test.go |
| 44 | allows different files to proceed in parallel | 已有 | gi-coding-agent/file_mutation_queue_test.go |
| 65 | uses the same queue for symlink aliases | 已有 | gi-coding-agent/file_mutation_queue_test.go |
| 90 | preserves both parallel edits on the same file | 已有 | gi-coding-agent/file_mutation_queue_test.go |
| 119 | shares the queue between edit and write | 已有 | gi-coding-agent/file_mutation_queue_test.go |

## `footer-data-provider.test.ts`

Pi cases: `8`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/footer_data_provider_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 109 | uses HEAD directly in a regular repo from a nested directory | 已有 | gi-coding-agent/footer_data_provider_test.go |
| 124 | resolves the branch via git when HEAD is .invalid in a reftable repo | 已有 | gi-coding-agent/footer_data_provider_test.go |
| 145 | resolves the branch via git in a reftable-backed worktree | 已有 | gi-coding-agent/footer_data_provider_test.go |
| 157 | treats an unresolved .invalid reftable HEAD as detached | 已有 | gi-coding-agent/footer_data_provider_test.go |
| 170 | does not notify listeners when reftable updates keep the same branch | 已有 | gi-coding-agent/footer_data_provider_test.go |
| 193 | debounces rapid reftable updates into a single async refresh | 已有 | gi-coding-agent/footer_data_provider_test.go |
| 214 | updates the cached branch when the reftable directory changes | 已有 | gi-coding-agent/footer_data_provider_test.go |
| 237 | retries git watchers 5 seconds after an async fs.watch error | 已有 | gi-coding-agent/footer_data_provider_test.go |

## `footer-width.test.ts`

Pi cases: `2`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/footer_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 81 | keeps all lines within width for wide session names | 已有 | gi-coding-agent/footer_test.go |
| 92 | keeps stats line within width for wide model and provider names | 已有 | gi-coding-agent/footer_test.go |

## `frontmatter.test.ts`

Pi cases: `8`  
Status: `已有`  
Gi coverage / implementation target: `gi-coding-agent/utils_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 5 | parses keys, strips quotes, and returns body | 已有 | gi-coding-agent/utils_test.go |
| 14 | normalizes newlines and handles CRLF | 已有 | gi-coding-agent/utils_test.go |
| 20 | throws on invalid YAML frontmatter | 已有 | gi-coding-agent/utils_test.go |
| 25 | parses \| multiline yaml syntax | 已有 | gi-coding-agent/utils_test.go |
| 32 | returns original content when frontmatter is missing or unterminated | 已有 | gi-coding-agent/utils_test.go |
| 43 | returns empty object for empty or comment-only frontmatter | 已有 | gi-coding-agent/utils_test.go |
| 51 | removes frontmatter and trims body | 已有 | gi-coding-agent/utils_test.go |
| 56 | returns body when no frontmatter present | 已有 | gi-coding-agent/utils_test.go |

## `git-ssh-url.test.ts`

Pi cases: `9`  
Status: `已有`  
Gi coverage / implementation target: `gi-coding-agent/git_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 6 | should parse HTTPS URL | 已有 | gi-coding-agent/git_test.go |
| 15 | should parse ssh:// URL | 已有 | gi-coding-agent/git_test.go |
| 24 | should parse protocol URL with ref | 已有 | gi-coding-agent/git_test.go |
| 36 | should parse git@host:path with git: prefix | 已有 | gi-coding-agent/git_test.go |
| 45 | should parse host/path shorthand with git: prefix | 已有 | gi-coding-agent/git_test.go |
| 54 | should parse shorthand with ref and git: prefix | 已有 | gi-coding-agent/git_test.go |
| 66 | should reject git@host:path without git: prefix | 已有 | gi-coding-agent/git_test.go |
| 70 | should reject host/path shorthand without git: prefix | 已有 | gi-coding-agent/git_test.go |
| 74 | should reject user/repo shorthand | 已有 | gi-coding-agent/git_test.go |

## `git-update.test.ts`

Pi cases: `11`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/package_manager_git_update_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 114 | should skip reset, clean, and install when already up to date | 已有 | gi-coding-agent/package_manager_git_update_test.go |
| 154 | should update to latest commit when remote has new commits | 已有 | gi-coding-agent/package_manager_git_update_test.go |
| 169 | should handle multiple commits ahead | 已有 | gi-coding-agent/package_manager_git_update_test.go |
| 183 | should update even when local checkout has no upstream | 已有 | gi-coding-agent/package_manager_git_update_test.go |
| 217 | should recover when remote history is rewritten | 已有 | gi-coding-agent/package_manager_git_update_test.go |
| 239 | should recover when local commit no longer exists in remote | 已有 | gi-coding-agent/package_manager_git_update_test.go |
| 261 | should handle complete history rewrite | 已有 | gi-coding-agent/package_manager_git_update_test.go |
| 285 | should not update pinned git sources (with @ref) | 已有 | gi-coding-agent/package_manager_git_update_test.go |
| 314 | should refresh cached temporary git sources when resolving | 已有 | gi-coding-agent/package_manager_git_update_test.go |
| 361 | should not refresh pinned temporary git sources | 已有 | gi-coding-agent/package_manager_git_update_test.go |
| 392 | should not install locally when source is only registered globally | 已有 | gi-coding-agent/package_manager_git_update_test.go |

## `image-processing.test.ts`

Pi cases: `9`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/image_resize_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 26 | should return original data for PNG input | 已有 | gi-coding-agent/image_resize_test.go |
| 33 | should convert JPEG to PNG | 已有 | gi-coding-agent/image_resize_test.go |
| 49 | should return original image if within limits | 已有 | gi-coding-agent/image_resize_test.go |
| 64 | should resize image exceeding dimension limits | 已有 | gi-coding-agent/image_resize_test.go |
| 78 | should resize image exceeding byte limit | 已有 | gi-coding-agent/image_resize_test.go |
| 95 | should return null when image cannot be resized below maxBytes | 已有 | gi-coding-agent/image_resize_test.go |
| 104 | should handle JPEG input | 已有 | gi-coding-agent/image_resize_test.go |
| 118 | should return undefined for non-resized images | 已有 | gi-coding-agent/image_resize_test.go |
| 131 | should return formatted note for resized images | 已有 | gi-coding-agent/image_resize_test.go |

## `image-resize-callers.test.ts`

Pi cases: `2`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/image_resize_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 32 | read tool returns text-only output when auto-resize cannot produce a safe image | 已有 | gi-coding-agent/image_resize_test.go |
| 44 | file processor omits image attachments when auto-resize cannot produce a safe image | 已有 | gi-coding-agent/image_resize_test.go |

## `initial-message.test.ts`

Pi cases: `3`  
Status: `已有`  
Gi coverage / implementation target: `gi-coding-agent/initial_message_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 15 | merges piped stdin with the first CLI message into one prompt | 已有 | gi-coding-agent/initial_message_test.go |
| 26 | uses stdin as the initial prompt when no CLI message is present | 已有 | gi-coding-agent/initial_message_test.go |
| 37 | combines stdin, file text, and first CLI message in one prompt | 已有 | gi-coding-agent/initial_message_test.go |

## `interactive-mode-anthropic-warning.test.ts`

Pi cases: `4`
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/anthropic_warning_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 11 | warns once when Anthropic subscription auth is detected | 已有 | gi-coding-agent/anthropic_warning_test.go |
| 37 | warns when Anthropic OAuth is stored even if token refresh lookup would fail | 已有 | gi-coding-agent/anthropic_warning_test.go |
| 60 | does not warn for non-Anthropic models | 已有 | gi-coding-agent/anthropic_warning_test.go |
| 83 | does not warn when Anthropic extra usage warning is disabled | 已有 | gi-coding-agent/anthropic_warning_test.go |

## `interactive-mode-clone-command.test.ts`

Pi cases: `2`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/interactive_mode_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 23 | clones the current leaf into a new session | 已有 | gi-coding-agent/interactive_mode_test.go |
| 51 | shows a status message when there is nothing to clone | 已有 | gi-coding-agent/interactive_mode_test.go |

## `interactive-mode-compaction.test.ts`

Pi cases: `1`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/interactive_mode_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 5 | rebuilds chat and appends a synthetic compaction summary at the bottom | 已有 | gi-coding-agent/interactive_mode_test.go |

## `interactive-mode-import-command.test.ts`

Pi cases: `6`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/interactive_mode_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 29 | strips quotes from /import path arguments | 已有 | gi-coding-agent/interactive_mode_test.go |
| 38 | preserves apostrophes in unquoted /import path arguments | 已有 | gi-coding-agent/interactive_mode_test.go |
| 44 | enforces command token boundaries | 已有 | gi-coding-agent/interactive_mode_test.go |
| 54 | passes unquoted path to runtimeHost.importFromJsonl | 已有 | gi-coding-agent/interactive_mode_test.go |
| 86 | passes unquoted apostrophe path to runtimeHost.importFromJsonl unchanged | 已有 | gi-coding-agent/interactive_mode_test.go |
| 114 | shows a non-fatal error when /import path does not exist | 已有 | gi-coding-agent/interactive_mode_test.go |

## `interactive-mode-status.test.ts`

Pi cases: `25`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/interactive_status_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 41 | coalesces immediately-sequential status messages | 已有 | gi-coding-agent/interactive_status_test.go |
| 60 | appends a new status line if something else was added in between | 已有 | gi-coding-agent/interactive_status_test.go |
| 83 | applies expansion state to the active header and chat entries | 已有 | gi-coding-agent/interactive_status_test.go |
| 104 | persists theme changes to settings manager | 已有 | gi-coding-agent/interactive_status_test.go |
| 129 | does not persist invalid theme names | 已有 | gi-coding-agent/interactive_status_test.go |
| 152 | stores wrapper factories and rebuilds autocomplete immediately | 已有 | gi-coding-agent/interactive_status_test.go |
| 168 | stacks wrapper factories over a fresh base provider | 已有 | gi-coding-agent/interactive_status_test.go |
| 407 | shows a compact resource listing by default | 已有 | gi-coding-agent/interactive_status_test.go |
| 423 | shows full resource listing when expanded | 已有 | gi-coding-agent/interactive_status_test.go |
| 440 | shows full resource listing on verbose startup even when tool output is collapsed | 已有 | gi-coding-agent/interactive_status_test.go |
| 458 | abbreviates extensions in compact listing | 已有 | gi-coding-agent/interactive_status_test.go |
| 474 | captures mixed extension layouts in compact output | 已有 | gi-coding-agent/interactive_status_test.go |
| 490 | adds more parent folders until local extension labels are unique | 已有 | gi-coding-agent/interactive_status_test.go |
| 536 | strips index.ts from local extension label, showing parent dir | 已有 | gi-coding-agent/interactive_status_test.go |
| 564 | strips index.js from local extension label, showing parent dir | 已有 | gi-coding-agent/interactive_status_test.go |
| 592 | mixed single-file and subdirectory index.ts extensions strip index.ts | 已有 | gi-coding-agent/interactive_status_test.go |
| 629 | multiple index.ts with unique parent dirs need no disambiguation | 已有 | gi-coding-agent/interactive_status_test.go |
| 666 | multiple index.ts with same parent dir name disambiguated with grandparent | 已有 | gi-coding-agent/interactive_status_test.go |
| 703 | non-index file in subdirectory stays as filename | 已有 | gi-coding-agent/interactive_status_test.go |
| 731 | package extensions still strip index.ts correctly (regression guard) | 已有 | gi-coding-agent/interactive_status_test.go |
| 758 | captures mixed extension layouts in expanded output | 已有 | gi-coding-agent/interactive_status_test.go |
| 788 | shows context paths relative to cwd while preserving full external paths | 已有 | gi-coding-agent/interactive_status_test.go |
| 807 | shows full context paths when expanded | 已有 | gi-coding-agent/interactive_status_test.go |
| 828 | does not show verbose listing on quiet startup during reload | 已有 | gi-coding-agent/interactive_status_test.go |
| 843 | still shows diagnostics on quiet startup when requested | 已有 | gi-coding-agent/interactive_status_test.go |

## `interactive-mode-suspend.test.ts`

Pi cases: `3`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/interactive_mode_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 31 | shows a status message and skips suspend on Windows | 已有 | gi-coding-agent/interactive_mode_test.go |
| 65 | keeps the process alive while suspended and restores the TUI on SIGCONT | 已有 | gi-coding-agent/interactive_mode_test.go |
| 115 | cleans up the temporary handlers if suspension fails | 已有 | gi-coding-agent/interactive_mode_test.go |

## `keybindings-migration.test.ts`

Pi cases: `3`  
Status: `已有`  
Gi coverage / implementation target: `gi-coding-agent/keybindings_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 25 | rewrites old key names to namespaced ids | 已有 | gi-coding-agent/keybindings_test.go |
| 49 | keeps the namespaced value when old and new names both exist | 已有 | gi-coding-agent/keybindings_test.go |
| 72 | loads old key names in memory before the file is rewritten | 已有 | gi-coding-agent/keybindings_test.go |

## `model-registry.test.ts`

Pi cases: `64`
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/model_registry_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 92 | overriding baseUrl keeps all built-in models | 已有 | gi-coding-agent/model_registry_test.go |
| 105 | overriding baseUrl changes URL on all built-in models | 已有 | gi-coding-agent/model_registry_test.go |
| 119 | overriding headers resolves at request time | 已有 | gi-coding-agent/model_registry_test.go |
| 138 | headers-only override resolves at request time | 已有 | gi-coding-agent/model_registry_test.go |
| 160 | baseUrl-only override does not affect other providers | 已有 | gi-coding-agent/model_registry_test.go |
| 173 | can mix baseUrl override and models merge | 已有 | gi-coding-agent/model_registry_test.go |
| 198 | refresh() picks up baseUrl override changes | 已有 | gi-coding-agent/model_registry_test.go |
| 217 | built-in provider custom models inherit api and baseUrl without explicit fields | 已有 | gi-coding-agent/model_registry_test.go |
| 242 | non-built-in provider custom models still require baseUrl and apiKey | 已有 | gi-coding-agent/model_registry_test.go |
| 260 | custom provider with same name as built-in merges with built-in models | 已有 | gi-coding-agent/model_registry_test.go |
| 273 | custom model with same id replaces built-in model by id | 已有 | gi-coding-agent/model_registry_test.go |
| 290 | custom provider with same name as built-in does not affect other built-in providers | 已有 | gi-coding-agent/model_registry_test.go |
| 301 | provider-level baseUrl applies to both built-in and custom models | 已有 | gi-coding-agent/model_registry_test.go |
| 314 | provider-level compat applies to custom models | 已有 | gi-coding-agent/model_registry_test.go |
| 344 | model-level compat overrides provider-level compat for custom models | 已有 | gi-coding-agent/model_registry_test.go |
| 378 | provider-level compat applies to built-in models | 已有 | gi-coding-agent/model_registry_test.go |
| 399 | model schema accepts thinkingLevelMap and compat schema accepts supportsStrictMode and cacheControlFormat | 已有 | gi-coding-agent/model_registry_test.go |
| 436 | compat schema accepts Anthropic eager tool input streaming flag | 已有 | gi-coding-agent/model_registry_test.go |
| 465 | compat schema accepts long cache retention flag | 已有 | gi-coding-agent/model_registry_test.go |
| 494 | model-level baseUrl overrides provider-level baseUrl for custom models | 已有 | gi-coding-agent/model_registry_test.go |
| 531 | modelOverrides still apply when provider also defines models | 已有 | gi-coding-agent/model_registry_test.go |
| 565 | refresh() reloads merged custom models from disk | 已有 | gi-coding-agent/model_registry_test.go |
| 584 | removing custom models from models.json keeps built-in provider models | 已有 | gi-coding-agent/model_registry_test.go |
| 602 | model override applies to a single built-in model | 已有 | gi-coding-agent/model_registry_test.go |
| 624 | model override with compat.openRouterRouting | 已有 | gi-coding-agent/model_registry_test.go |
| 645 | model override deep merges compat settings | 已有 | gi-coding-agent/model_registry_test.go |
| 667 | multiple model overrides on same provider | 已有 | gi-coding-agent/model_registry_test.go |
| 693 | model override combined with baseUrl override | 已有 | gi-coding-agent/model_registry_test.go |
| 719 | model override for non-existent model ID is ignored | 已有 | gi-coding-agent/model_registry_test.go |
| 739 | model override can change cost fields partially | 已有 | gi-coding-agent/model_registry_test.go |
| 760 | model override can add headers at request time | 已有 | gi-coding-agent/model_registry_test.go |
| 783 | refresh() picks up model override changes | 已有 | gi-coding-agent/model_registry_test.go |
| 816 | removing model override restores built-in values | 已有 | gi-coding-agent/model_registry_test.go |
| 845 | getProviderDisplayName resolves registered, OAuth, built-in, and fallback names | 已有 | gi-coding-agent/model_registry_test.go |
| 895 | failed registerProvider does not persist invalid streamSimple config | 已有 | gi-coding-agent/model_registry_test.go |
| 909 | failed registerProvider does not remove existing provider models | 已有 | gi-coding-agent/model_registry_test.go |
| 954 | unregisterProvider removes custom OAuth provider and restores built-in OAuth provider | 已有 | gi-coding-agent/model_registry_test.go |
| 977 | unregisterProvider removes custom streamSimple override and restores built-in API stream handler | 已有 | gi-coding-agent/model_registry_test.go |
| 1008 | baseUrl-only override keeps built-in provider models after refresh | 已有 | gi-coding-agent/model_registry_test.go |
| 1019 | models-only override replaces built-in provider models after refresh | 已有 | gi-coding-agent/model_registry_test.go |
| 1032 | models plus baseUrl override replaces built-in provider models after refresh | 已有 | gi-coding-agent/model_registry_test.go |
| 1046 | models-only custom provider registration survives refresh | 已有 | gi-coding-agent/model_registry_test.go |
| 1061 | baseUrl-only override keeps custom provider models after refresh | 已有 | gi-coding-agent/model_registry_test.go |
| 1082 | headers-only override keeps custom provider models after refresh | 已有 | gi-coding-agent/model_registry_test.go |
| 1124 | apiKey with ! prefix executes command and uses stdout | 已有 | gi-coding-agent/model_registry_test.go |
| 1135 | apiKey with ! prefix trims whitespace from command output | 已有 | gi-coding-agent/model_registry_test.go |
| 1146 | apiKey with ! prefix handles multiline output (uses trimmed result) | 已有 | gi-coding-agent/model_registry_test.go |
| 1157 | apiKey with ! prefix returns undefined on command failure | 已有 | gi-coding-agent/model_registry_test.go |
| 1168 | apiKey with ! prefix returns undefined on nonexistent command | 已有 | gi-coding-agent/model_registry_test.go |
| 1179 | apiKey with ! prefix returns undefined on empty output | 已有 | gi-coding-agent/model_registry_test.go |
| 1190 | apiKey as environment variable name resolves to env value | 已有 | gi-coding-agent/model_registry_test.go |
| 1212 | apiKey as literal value is used directly when not an env var | 已有 | gi-coding-agent/model_registry_test.go |
| 1226 | apiKey command can use shell features like pipes | 已有 | gi-coding-agent/model_registry_test.go |
| 1238 | command is executed on every provider lookup | 已有 | gi-coding-agent/model_registry_test.go |
| 1257 | commands are re-executed across registry instances | 已有 | gi-coding-agent/model_registry_test.go |
| 1277 | different commands resolve independently | 已有 | gi-coding-agent/model_registry_test.go |
| 1292 | failed commands are retried | 已有 | gi-coding-agent/model_registry_test.go |
| 1313 | provider auth status reports apiKey environment variables from models.json | 已有 | gi-coding-agent/model_registry_test.go |
| 1340 | provider auth status reports non-env apiKey values from models.json as a config key | 已有 | gi-coding-agent/model_registry_test.go |
| 1353 | provider auth status reports command apiKey values from models.json without executing them | 已有 | gi-coding-agent/model_registry_test.go |
| 1371 | environment variables are not cached (changes are picked up) | 已有 | gi-coding-agent/model_registry_test.go |
| 1400 | getAvailable does not execute command-backed apiKey resolution | 已有 | gi-coding-agent/model_registry_test.go |
| 1418 | getApiKeyAndHeaders resolves authHeader on every request | 已有 | gi-coding-agent/model_registry_test.go |
| 1451 | getApiKeyAndHeaders returns an error for failed authHeader resolution | 已有 | gi-coding-agent/model_registry_test.go |

## `model-resolver.test.ts`

Pi cases: `31`
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/model_resolver_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 70 | exact match returns model with undefined thinking level | 已有 | gi-coding-agent/model_resolver_test.go |
| 77 | partial match returns best model with undefined thinking level | 已有 | gi-coding-agent/model_resolver_test.go |
| 84 | no match returns undefined model and thinking level | 已有 | gi-coding-agent/model_resolver_test.go |
| 93 | sonnet:high returns sonnet with high thinking level | 已有 | gi-coding-agent/model_resolver_test.go |
| 100 | gpt-4o:medium returns gpt-4o with medium thinking level | 已有 | gi-coding-agent/model_resolver_test.go |
| 107 | all valid thinking levels work | 已有 | gi-coding-agent/model_resolver_test.go |
| 118 | sonnet:random returns sonnet with undefined thinking level and warning | 已有 | gi-coding-agent/model_resolver_test.go |
| 126 | gpt-4o:invalid returns gpt-4o with undefined thinking level and warning | 已有 | gi-coding-agent/model_resolver_test.go |
| 135 | qwen3-coder:exacto matches the model with undefined thinking level | 已有 | gi-coding-agent/model_resolver_test.go |
| 142 | openrouter/qwen/qwen3-coder:exacto matches with provider prefix | 已有 | gi-coding-agent/model_resolver_test.go |
| 150 | qwen3-coder:exacto:high matches model with high thinking level | 已有 | gi-coding-agent/model_resolver_test.go |
| 157 | openrouter/qwen/qwen3-coder:exacto:high matches with provider and thinking level | 已有 | gi-coding-agent/model_resolver_test.go |
| 165 | gpt-4o:extended matches the extended model with undefined thinking level | 已有 | gi-coding-agent/model_resolver_test.go |
| 174 | qwen3-coder:exacto:random returns model with undefined thinking level and warning | 已有 | gi-coding-agent/model_resolver_test.go |
| 182 | qwen3-coder:exacto:high:random returns model with undefined thinking level and warning | 已有 | gi-coding-agent/model_resolver_test.go |
| 192 | empty pattern matches via partial matching | 已有 | gi-coding-agent/model_resolver_test.go |
| 199 | pattern ending with colon treats empty suffix as invalid | 已有 | gi-coding-agent/model_resolver_test.go |
| 210 | resolves --model provider/id without --provider | 已有 | gi-coding-agent/model_resolver_test.go |
| 225 | resolves fuzzy patterns within an explicit provider | 已有 | gi-coding-agent/model_resolver_test.go |
| 241 | supports --model <pattern>:<thinking> (without explicit --thinking) | 已有 | gi-coding-agent/model_resolver_test.go |
| 256 | prefers exact model id match over provider inference (OpenRouter-style ids) | 已有 | gi-coding-agent/model_resolver_test.go |
| 271 | does not strip invalid :suffix as thinking level in --model (treat as raw id) | 已有 | gi-coding-agent/model_resolver_test.go |
| 287 | allows custom model ids for explicit providers without double prefixing | 已有 | gi-coding-agent/model_resolver_test.go |
| 303 | returns a clear error when there are no models | 已有 | gi-coding-agent/model_resolver_test.go |
| 318 | prefers provider/model split over gateway model with matching id | 已有 | gi-coding-agent/model_resolver_test.go |
| 359 | resolves provider-prefixed fuzzy patterns (openrouter/qwen -> openrouter model) | 已有 | gi-coding-agent/model_resolver_test.go |
| 376 | openai defaults track current models | 已有 | gi-coding-agent/model_resolver_test.go |
| 381 | zai, minimax, and cerebras defaults track current models | 已有 | gi-coding-agent/model_resolver_test.go |
| 388 | ai-gateway default tracks current model | 已有 | gi-coding-agent/model_resolver_test.go |
| 392 | findInitialModel accepts explicit provider custom model ids | 已有 | gi-coding-agent/model_resolver_test.go |
| 409 | findInitialModel selects ai-gateway default when available | 已有 | gi-coding-agent/model_resolver_test.go |

## `oauth-selector.test.ts`

Pi cases: `6`
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/oauth_selector_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 30 | keeps built-in API key providers separate from OAuth-only providers | 已有 | gi-coding-agent/oauth_selector_test.go |
| 43 | shows stored OAuth auth distinctly in the API key selector | 已有 | gi-coding-agent/oauth_selector_test.go |
| 66 | shows environment API key auth as configured | 已有 | gi-coding-agent/oauth_selector_test.go |
| 84 | shows custom provider environment API key auth from status resolver | 已有 | gi-coding-agent/oauth_selector_test.go |
| 102 | shows models.json API key auth as configured | 已有 | gi-coding-agent/oauth_selector_test.go |
| 120 | shows models.json command auth as configured | 已有 | gi-coding-agent/oauth_selector_test.go |

## `package-command-paths.test.ts`

Pi cases: `10`  
Status: `需要协议 runtime`  
Gi coverage / implementation target: `按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 61 | should persist global relative local package paths relative to settings.json | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 75 | should remove local packages using a path with a trailing slash | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 88 | shows install subcommand help | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 106 | shows a friendly error for unknown install options | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 121 | shows a friendly error for missing install source | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 137 | uses global npmCommand and current package name for forced self updates without checking the api | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 187 | uses the current package name when the update check omits packageName | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 229 | installs the active package name from the update check during self-update | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 280 | fails self-update when renamed npm package installation fails | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 336 | suggests the configured source when update input omits the npm prefix | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |

## `package-manager-ssh.test.ts`

Pi cases: `8`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/package_manager_source_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 33 | should parse https:// URL | 已有 | gi-coding-agent/package_manager_source_test.go |
| 40 | should parse ssh:// URL | 已有 | gi-coding-agent/package_manager_source_test.go |
| 50 | should parse git@host:path format | 已有 | gi-coding-agent/package_manager_source_test.go |
| 59 | should parse host/path shorthand | 已有 | gi-coding-agent/package_manager_source_test.go |
| 66 | should parse shorthand with ref | 已有 | gi-coding-agent/package_manager_source_test.go |
| 75 | should treat git@host:path as local without git: prefix | 已有 | gi-coding-agent/package_manager_source_test.go |
| 80 | should treat host/path shorthand as local without git: prefix | 已有 | gi-coding-agent/package_manager_source_test.go |
| 87 | should normalize protocol and shorthand-prefixed URLs to same identity | 已有 | gi-coding-agent/package_manager_source_test.go |

## `package-manager.test.ts`

Pi cases: `102`  
Status: `需要协议 runtime`  
Gi coverage / implementation target: `按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 81 | should return no package-sourced paths when no sources configured | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 91 | should resolve local extension paths from settings | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 102 | should resolve skill paths from settings | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 122 | should auto-discover root markdown skills from .pi skill dirs | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 138 | should resolve project paths relative to .pi | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 150 | should auto-discover user prompts with overrides | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 162 | should resolve symlinked user and project resources once | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 230 | should auto-discover project prompts with overrides | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 242 | should resolve directory with package.json pi.extensions in extensions setting | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 278 | should use the agent dir as baseDir for user .pi/agent skills | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 291 | should use the project .pi dir as baseDir for project .pi skills | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 305 | should use ~/.agents as baseDir for user .agents skills | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 330 | should use each project .agents dir as baseDir for project .agents skills | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 366 | should scan .agents/skills from cwd up to git repo root | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 396 | should scan .agents/skills up to filesystem root when not in a git repo | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 420 | should ignore root markdown files in .agents/skills | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 440 | should keep ~/.agents/skills user-scoped when cwd is under home in a non-git directory | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 476 | should dedupe user skill entries when ~/.pi/agent/skills is a symlink to ~/.agents/skills | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 507 | should respect .gitignore in skill directories | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 527 | should not apply parent .gitignore to .pi auto-discovery | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 541 | should resolve local paths | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 549 | should handle directories with pi manifest | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 578 | should handle directories with auto-discovery layout | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 590 | should stop recursing when a package skill directory contains SKILL.md | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 605 | should emit progress events | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 621 | should avoid the shell for git so Windows paths with spaces stay single arguments | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 632 | should use npmCommand argv for npm installs | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 653 | should install git package dependencies with --omit=dev | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 671 | should use plain install for git package dependencies when npmCommand is configured | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 698 | should update git package dependencies with --omit=dev | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 725 | should use plain install through npmCommand argv when updating git package dependencies | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 763 | should use npmCommand argv for npm root lookup and invalidate cached root when npmCommand changes | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 802 | should resolve pnpm global package paths from pnpm list output | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 860 | should resolve wrapped pnpm global package paths from pnpm list output | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 886 | should fail when pnpm global package list is malformed | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 903 | should emit progress events on install attempt | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 920 | should recognize github URLs without git: prefix | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 945 | should parse package source types from docs examples | 已有 | gi-coding-agent/package_manager_source_test.go |
| 959 | should never parse dot-relative paths as git | 已有 | gi-coding-agent/package_manager_source_test.go |
| 971 | should store global local packages relative to agent settings base | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 985 | should store project local packages relative to .pi settings base | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 999 | should remove local package entries using equivalent path forms | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1012 | should parse HTTPS GitHub URLs correctly | 已有 | gi-coding-agent/package_manager_source_test.go |
| 1020 | should parse HTTPS URLs with git: prefix | 已有 | gi-coding-agent/package_manager_source_test.go |
| 1027 | should parse HTTPS URLs with ref | 已有 | gi-coding-agent/package_manager_source_test.go |
| 1036 | should parse host/path shorthand only with git: prefix | 已有 | gi-coding-agent/package_manager_source_test.go |
| 1043 | should treat host/path shorthand as local without git: prefix | 已有 | gi-coding-agent/package_manager_source_test.go |
| 1048 | should parse HTTPS URLs with .git suffix | 已有 | gi-coding-agent/package_manager_source_test.go |
| 1055 | should parse GitLab HTTPS URLs | 已有 | gi-coding-agent/package_manager_source_test.go |
| 1062 | should parse Bitbucket HTTPS URLs | 已有 | gi-coding-agent/package_manager_source_test.go |
| 1069 | should parse Codeberg HTTPS URLs | 已有 | gi-coding-agent/package_manager_source_test.go |
| 1076 | should generate correct package identity for protocol and git:-prefixed URLs | 已有 | gi-coding-agent/package_manager_source_test.go |
| 1089 | should deduplicate git URLs with different supported formats | 已有 | gi-coding-agent/package_manager_source_test.go |
| 1112 | should handle HTTPS URLs with refs in resolve | 已有 | gi-coding-agent/package_manager_source_test.go |
| 1124 | should exclude extensions with ! pattern | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1137 | should filter themes with glob patterns | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1152 | should filter prompts with exclusion pattern | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1165 | should filter skills with exclusion pattern | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1185 | should work without patterns (backward compatible) | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1199 | should support glob patterns in manifest extensions | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1222 | should support glob patterns in manifest skills | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1249 | should expand positive glob manifest entries before collecting skills | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1278 | should apply user filters on top of manifest filters (not replace) | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1316 | should exclude extensions from package with ! pattern | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1339 | should filter themes from package | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1360 | should combine include and exclude patterns | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1383 | should work with direct paths (no patterns) | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1406 | should force-include extensions with + pattern after exclusion | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1422 | should force-include overrides exclude in package filters | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1445 | should force-include multiple resources | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1470 | should force-include after specific exclusion | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1484 | should handle force-include in manifest patterns | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1506 | should force-include themes | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1521 | should force-include prompts | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1538 | should force-exclude top-level resources | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1551 | should force-exclude in package filters | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1574 | should dedupe same local package in global and project (project wins) | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1596 | should keep both if different packages | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1612 | should dedupe SSH and HTTPS URLs for same repo | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1626 | should dedupe SSH and HTTPS with refs | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1639 | should dedupe SSH URL with ssh:// protocol and git@ format | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1652 | should dedupe all supported URL formats for same repo | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1671 | should keep different repos separate (HTTPS vs SSH) | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1686 | should only load index.ts from subdirectories, not helper modules | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1716 | should respect package.json pi.extensions manifest in subdirectories | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1741 | should handle mixed top-level files and subdirectories | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1770 | should skip subdirectories without index.ts or manifest | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1790 | should update project npm packages using @latest when newer version is available | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1813 | should skip project npm update when installed version matches latest | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1832 | should batch npm updates per scope and run git updates in parallel while skipping pinned and current packages | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1941 | should suggest npm source prefixes for update lookups | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1949 | should suggest git source prefixes for update lookups | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1957 | should skip installing missing package sources when offline | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1969 | should skip refreshing temporary git sources when offline | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1985 | should not run npm view during resolve for installed unpinned packages | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 1999 | should reinstall pinned npm packages when installed version does not match | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 2013 | should not check package updates when offline | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 2022 | should report updates for installed unpinned npm packages | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 2041 | should skip pinned packages when checking for updates | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 2060 | should use npm view to fetch latest version | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 2073 | should use npmCommand argv for npm update checks | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 2094 | should wait for close before resolving captured stdout | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |

## `path-utils.test.ts`

Pi cases: `11`  
Status: `已有`  
Gi coverage / implementation target: `gi-coding-agent/utils_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 9 | should expand ~ to home directory | 已有 | gi-coding-agent/utils_test.go |
| 14 | should expand ~/path to home directory | 已有 | gi-coding-agent/utils_test.go |
| 19 | should normalize Unicode spaces | 已有 | gi-coding-agent/utils_test.go |
| 28 | should resolve absolute paths as-is | 已有 | gi-coding-agent/utils_test.go |
| 33 | should resolve relative paths against cwd | 已有 | gi-coding-agent/utils_test.go |
| 59 | should resolve existing file path | 已有 | gi-coding-agent/utils_test.go |
| 67 | should handle NFC vs NFD Unicode normalization (macOS filenames with accents) | 已有 | gi-coding-agent/utils_test.go |
| 95 | should handle curly quotes vs straight quotes (macOS filenames) | 已有 | gi-coding-agent/utils_test.go |
| 115 | should handle combined NFC + curly quote (French macOS screenshots) | 已有 | gi-coding-agent/utils_test.go |
| 132 | should handle macOS screenshot AM/PM variant with narrow no-break space | 已有 | gi-coding-agent/utils_test.go |
| 147 | should handle macOS screenshot lowercase am/pm variant (en_AU locale) | 已有 | gi-coding-agent/utils_test.go |

## `paths.test.ts`

Pi cases: `12`  
Status: `已有`  
Gi coverage / implementation target: `gi-coding-agent/utils_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 22 | returns the real path for a regular file | 已有 | gi-coding-agent/utils_test.go |
| 29 | resolves symlinks to their targets | 已有 | gi-coding-agent/utils_test.go |
| 38 | resolves directory symlinks | 已有 | gi-coding-agent/utils_test.go |
| 47 | falls back to the raw path when the target does not exist | 已有 | gi-coding-agent/utils_test.go |
| 53 | falls back to the raw path for a dangling symlink | 已有 | gi-coding-agent/utils_test.go |
| 65 | keeps cwd-relative names that start with dots | 已有 | gi-coding-agent/utils_test.go |
| 70 | rejects parent-directory traversals | 已有 | gi-coding-agent/utils_test.go |
| 77 | returns true for bare names | 已有 | gi-coding-agent/utils_test.go |
| 81 | returns true for relative paths | 已有 | gi-coding-agent/utils_test.go |
| 85 | returns false for npm: protocol | 已有 | gi-coding-agent/utils_test.go |
| 89 | returns false for git: protocol | 已有 | gi-coding-agent/utils_test.go |
| 93 | returns false for https: protocol | 已有 | gi-coding-agent/utils_test.go |

## `pi-user-agent.test.ts`

Pi cases: `1`  
Status: `已有`  
Gi coverage / implementation target: `gi-coding-agent/utils_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 5 | formats the user agent expected by pi.dev | 已有 | gi-coding-agent/utils_test.go |

## `plan-mode-utils.test.ts`

Pi cases: `33`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/plan_mode_utils_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 13 | allows basic read commands | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 22 | allows git read commands | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 29 | allows npm/yarn read commands | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 35 | allows other safe commands | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 45 | blocks file modification commands | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 54 | blocks git write commands | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 62 | blocks package manager installs | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 69 | blocks redirects | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 75 | blocks dangerous commands | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 81 | blocks editors | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 89 | requires command to be in safe list (not just non-destructive) | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 94 | handles commands with leading whitespace | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 102 | removes markdown bold/italic | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 107 | removes markdown code | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 112 | removes leading action words | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 118 | capitalizes first letter | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 122 | truncates long text | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 129 | normalizes whitespace | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 135 | extracts numbered items after Plan: header | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 150 | handles bold Plan header | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 158 | handles parenthesis-style numbering | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 167 | returns empty array without Plan header | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 176 | filters out short items | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 186 | filters out code-like items | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 197 | extracts single DONE marker | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 202 | extracts multiple DONE markers | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 207 | handles case insensitivity | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 212 | returns empty array with no markers | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 217 | ignores malformed markers | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 224 | marks matching items as completed | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 239 | returns count of completed items | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 246 | ignores markers for non-existent steps | 已有 | gi-coding-agent/plan_mode_utils_test.go |
| 255 | doesn't double-complete already completed items | 已有 | gi-coding-agent/plan_mode_utils_test.go |

## `print-mode.test.ts`

Pi cases: `3`  
Status: `待实现`  
Gi coverage / implementation target: `补 Gi coding-agent 本体 host/runtime 后添加 Go 对应测试`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 94 | emits session_shutdown in text mode | 已有 | gi-coding-agent/print_mode_test.go |
| 111 | emits session_shutdown in json mode | 已有 | gi-coding-agent/print_mode_test.go |
| 126 | emits session_shutdown and returns non-zero on assistant error | 已有 | gi-coding-agent/print_mode_test.go |

## `prompt-templates.test.ts`

Pi cases: `82`  
Status: `已有`  
Gi coverage / implementation target: `gi-coding-agent/prompt_templates_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 28 | should replace $ARGUMENTS with all args joined | 已有 | gi-coding-agent/prompt_templates_test.go |
| 32 | should replace $@ with all args joined | 已有 | gi-coding-agent/prompt_templates_test.go |
| 36 | should replace $@ and $ARGUMENTS identically | 已有 | gi-coding-agent/prompt_templates_test.go |
| 42 | should NOT recursively substitute patterns in argument values | 已有 | gi-coding-agent/prompt_templates_test.go |
| 48 | should support mixed $1, $2, and $ARGUMENTS | 已有 | gi-coding-agent/prompt_templates_test.go |
| 52 | should support mixed $1, $2, and $@ | 已有 | gi-coding-agent/prompt_templates_test.go |
| 56 | should handle empty arguments array with $ARGUMENTS | 已有 | gi-coding-agent/prompt_templates_test.go |
| 60 | should handle empty arguments array with $@ | 已有 | gi-coding-agent/prompt_templates_test.go |
| 64 | should handle empty arguments array with $1 | 已有 | gi-coding-agent/prompt_templates_test.go |
| 68 | should handle multiple occurrences of $ARGUMENTS | 已有 | gi-coding-agent/prompt_templates_test.go |
| 72 | should handle multiple occurrences of $@ | 已有 | gi-coding-agent/prompt_templates_test.go |
| 76 | should handle mixed occurrences of $@ and $ARGUMENTS | 已有 | gi-coding-agent/prompt_templates_test.go |
| 80 | should handle special characters in arguments | 已有 | gi-coding-agent/prompt_templates_test.go |
| 85 | should handle out-of-range numbered placeholders | 已有 | gi-coding-agent/prompt_templates_test.go |
| 90 | should handle unicode characters | 已有 | gi-coding-agent/prompt_templates_test.go |
| 94 | should preserve newlines and tabs in argument values | 已有 | gi-coding-agent/prompt_templates_test.go |
| 98 | should handle consecutive dollar patterns | 已有 | gi-coding-agent/prompt_templates_test.go |
| 102 | should handle quoted arguments with spaces | 已有 | gi-coding-agent/prompt_templates_test.go |
| 106 | should handle single argument with $ARGUMENTS | 已有 | gi-coding-agent/prompt_templates_test.go |
| 110 | should handle single argument with $@ | 已有 | gi-coding-agent/prompt_templates_test.go |
| 114 | should handle $0 (zero index) | 已有 | gi-coding-agent/prompt_templates_test.go |
| 118 | should handle decimal number in pattern (only integer part matches) | 已有 | gi-coding-agent/prompt_templates_test.go |
| 122 | should handle $ARGUMENTS as part of word | 已有 | gi-coding-agent/prompt_templates_test.go |
| 126 | should handle $@ as part of word | 已有 | gi-coding-agent/prompt_templates_test.go |
| 130 | should handle empty arguments in middle of list | 已有 | gi-coding-agent/prompt_templates_test.go |
| 134 | should handle trailing and leading spaces in arguments | 已有 | gi-coding-agent/prompt_templates_test.go |
| 138 | should handle argument containing pattern partially | 已有 | gi-coding-agent/prompt_templates_test.go |
| 142 | should handle non-matching patterns | 已有 | gi-coding-agent/prompt_templates_test.go |
| 146 | should handle case variations (case-sensitive) | 已有 | gi-coding-agent/prompt_templates_test.go |
| 150 | should handle both syntaxes in same command with same result | 已有 | gi-coding-agent/prompt_templates_test.go |
| 158 | should handle very long argument lists | 已有 | gi-coding-agent/prompt_templates_test.go |
| 164 | should handle numbered placeholders with single digit | 已有 | gi-coding-agent/prompt_templates_test.go |
| 168 | should handle numbered placeholders with multiple digits | 已有 | gi-coding-agent/prompt_templates_test.go |
| 173 | should handle escaped dollar signs (literal backslash preserved) | 已有 | gi-coding-agent/prompt_templates_test.go |
| 178 | should handle mixed numbered and wildcard placeholders | 已有 | gi-coding-agent/prompt_templates_test.go |
| 184 | should handle command with no placeholders | 已有 | gi-coding-agent/prompt_templates_test.go |
| 188 | should handle command with only placeholders | 已有 | gi-coding-agent/prompt_templates_test.go |
| 198 | should slice from index (\\${@:N}) | 已有 | gi-coding-agent/prompt_templates_test.go |
| 204 | should slice with length (\\${@:N:L}) | 已有 | gi-coding-agent/prompt_templates_test.go |
| 211 | should handle out of range slices | 已有 | gi-coding-agent/prompt_templates_test.go |
| 217 | should handle zero-length slices | 已有 | gi-coding-agent/prompt_templates_test.go |
| 222 | should handle length exceeding array | 已有 | gi-coding-agent/prompt_templates_test.go |
| 227 | should process slice before simple $@ | 已有 | gi-coding-agent/prompt_templates_test.go |
| 232 | should not recursively substitute slice patterns in args | 已有 | gi-coding-agent/prompt_templates_test.go |
| 237 | should handle mixed usage with positional args | 已有 | gi-coding-agent/prompt_templates_test.go |
| 242 | should treat \\${@:0} as all args | 已有 | gi-coding-agent/prompt_templates_test.go |
| 246 | should handle empty args array | 已有 | gi-coding-agent/prompt_templates_test.go |
| 251 | should handle single arg array | 已有 | gi-coding-agent/prompt_templates_test.go |
| 256 | should handle slice in middle of text | 已有 | gi-coding-agent/prompt_templates_test.go |
| 262 | should handle multiple slices in one template | 已有 | gi-coding-agent/prompt_templates_test.go |
| 267 | should handle quoted arguments in slices | 已有 | gi-coding-agent/prompt_templates_test.go |
| 271 | should handle special characters in sliced args | 已有 | gi-coding-agent/prompt_templates_test.go |
| 275 | should handle unicode in sliced args | 已有 | gi-coding-agent/prompt_templates_test.go |
| 279 | should combine positional, slice, and wildcard placeholders | 已有 | gi-coding-agent/prompt_templates_test.go |
| 287 | should handle slice with no spacing | 已有 | gi-coding-agent/prompt_templates_test.go |
| 291 | should handle large slice lengths gracefully | 已有 | gi-coding-agent/prompt_templates_test.go |
| 302 | should parse simple space-separated arguments | 已有 | gi-coding-agent/prompt_templates_test.go |
| 306 | should parse quoted arguments with spaces | 已有 | gi-coding-agent/prompt_templates_test.go |
| 310 | should parse single-quoted arguments | 已有 | gi-coding-agent/prompt_templates_test.go |
| 314 | should parse mixed quote styles | 已有 | gi-coding-agent/prompt_templates_test.go |
| 318 | should handle empty string | 已有 | gi-coding-agent/prompt_templates_test.go |
| 322 | should handle extra spaces | 已有 | gi-coding-agent/prompt_templates_test.go |
| 326 | should handle tabs as separators | 已有 | gi-coding-agent/prompt_templates_test.go |
| 330 | should handle quoted empty string | 已有 | gi-coding-agent/prompt_templates_test.go |
| 335 | should handle arguments with special characters | 已有 | gi-coding-agent/prompt_templates_test.go |
| 339 | should handle unicode characters | 已有 | gi-coding-agent/prompt_templates_test.go |
| 343 | should handle newlines in quoted arguments | 已有 | gi-coding-agent/prompt_templates_test.go |
| 347 | should treat unquoted newlines as separators | 已有 | gi-coding-agent/prompt_templates_test.go |
| 358 | should collapse mixed unquoted whitespace | 已有 | gi-coding-agent/prompt_templates_test.go |
| 362 | should handle escaped quotes inside quoted strings | 已有 | gi-coding-agent/prompt_templates_test.go |
| 367 | should handle trailing spaces | 已有 | gi-coding-agent/prompt_templates_test.go |
| 371 | should handle leading spaces | 已有 | gi-coding-agent/prompt_templates_test.go |
| 381 | should split template arguments on unquoted newlines | 已有 | gi-coding-agent/prompt_templates_test.go |
| 395 | should support template command separated from args by newline | 已有 | gi-coding-agent/prompt_templates_test.go |
| 415 | should parse and substitute together correctly | 已有 | gi-coding-agent/prompt_templates_test.go |
| 423 | should handle the example from README | 已有 | gi-coding-agent/prompt_templates_test.go |
| 433 | should produce same result with $@ and $ARGUMENTS | 已有 | gi-coding-agent/prompt_templates_test.go |
| 453 | should parse required argument-hint from frontmatter | 已有 | gi-coding-agent/prompt_templates_test.go |
| 476 | should parse optional argument-hint from frontmatter | 已有 | gi-coding-agent/prompt_templates_test.go |
| 499 | should leave argumentHint undefined when not specified | 已有 | gi-coding-agent/prompt_templates_test.go |
| 520 | should ignore empty argument-hint | 已有 | gi-coding-agent/prompt_templates_test.go |
| 542 | should preserve argument-hint with special characters | 已有 | gi-coding-agent/prompt_templates_test.go |

## `resource-loader.test.ts`

Pi cases: `19`  
Status: `需要协议 runtime`  
Gi coverage / implementation target: `按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 32 | should initialize with empty results before reload | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 41 | should discover skills from agentDir | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 60 | should ignore extra markdown files in auto-discovered skill dirs | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 81 | should discover prompts from agentDir | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 99 | should prefer project resources over user on name collisions | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 159 | should load symlinked user and project extensions once | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 189 | should keep both extensions loaded when command names collide | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 255 | should honor overrides for auto-discovered resources | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 299 | should discover AGENTS.md context files | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 309 | should skip AGENTS.md and CLAUDE.md discovery when noContextFiles is true | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 320 | should discover SYSTEM.md from cwd/.pi | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 331 | should discover APPEND_SYSTEM.md | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 344 | should load skills and prompts with extension metadata | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 411 | should skip skill discovery when noSkills is true | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 430 | should still load additional skill paths when noSkills is true | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 456 | should apply skillsOverride | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 480 | should apply systemPromptOverride | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 493 | should detect tool conflicts between extensions | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |
| 537 | should prefer explicit CLI extensions over discovered extensions when commands and tools conflict | 需要协议 runtime | 按 protocol/spec 的 host actions、registry、capability、ViewTree 或 package resolver 落地 |

## `restore-sandbox-env.test.ts`

Pi cases: `3`
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/restore_sandbox_env_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 12 | does nothing when not running under bun | 已有 | gi-coding-agent/restore_sandbox_env_test.go |
| 28 | does nothing when process.env already has entries | 已有 | gi-coding-agent/restore_sandbox_env_test.go |
| 46 | restores environment from /proc/self/environ when bun env is empty | 已有 | gi-coding-agent/restore_sandbox_env_test.go |

## `rpc-client-clone.test.ts`

Pi cases: `1`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/rpc_client_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 10 | sends the clone RPC command | 已有 | gi-coding-agent/rpc_client_test.go |

## `rpc-jsonl.test.ts`

Pi cases: `4`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/rpc_jsonl_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 6 | serializes strict JSONL records without escaping Unicode separators | 已有 | gi-coding-agent/rpc_jsonl_test.go |
| 14 | splits on LF only and preserves U+2028/U+2029 inside payloads | 已有 | gi-coding-agent/rpc_jsonl_test.go |
| 32 | handles CRLF-delimited input | 已有 | gi-coding-agent/rpc_jsonl_test.go |
| 49 | emits a final line without trailing LF | 已有 | gi-coding-agent/rpc_jsonl_test.go |

## `rpc-prompt-response-semantics.test.ts`

Pi cases: `3`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/rpc_prompt_response_semantics_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 191 | emits one failure response when prompt preflight rejects | 已有 | gi-coding-agent/rpc_prompt_response_semantics_test.go |
| 230 | emits one success response when prompt preflight succeeds | 已有 | gi-coding-agent/rpc_prompt_response_semantics_test.go |
| 251 | emits one success response when prompt is queued during streaming | 已有 | gi-coding-agent/rpc_prompt_response_semantics_test.go |

## `rpc.test.ts`

Pi cases: `14`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/rpc_session_host_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 36 | should get state | 已有 | gi-coding-agent/rpc_session_host_test.go |
| 47 | should save messages to session file | 已有 | gi-coding-agent/rpc_session_host_test.go |
| 89 | should handle manual compaction | 已有 | gi-coding-agent/rpc_session_host_test.go |
| 119 | should execute bash command | 已有 | gi-coding-agent/rpc_session_host_test.go |
| 128 | should add bash output to context | 已有 | gi-coding-agent/rpc_session_host_test.go |
| 160 | should include bash output in LLM context | 已有 | gi-coding-agent/rpc_session_host_test.go |
| 184 | should set and get thinking level | 已有 | gi-coding-agent/rpc_session_host_test.go |
| 195 | should cycle thinking level | 已有 | gi-coding-agent/rpc_session_host_test.go |
| 212 | should get available models | 已有 | gi-coding-agent/rpc_session_host_test.go |
| 227 | should get session stats | 已有 | gi-coding-agent/rpc_session_host_test.go |
| 240 | should create new session | 已有 | gi-coding-agent/rpc_session_host_test.go |
| 258 | should export to HTML | 已有 | gi-coding-agent/rpc_session_host_test.go |
| 271 | should get last assistant text | 已有 | gi-coding-agent/rpc_session_host_test.go |
| 286 | should set and get session name | 已有 | gi-coding-agent/rpc_session_host_test.go |

## `sdk-openrouter-attribution.test.ts`

Pi cases: `4`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/sdk_attribution_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 140 | adds default attribution headers for OpenRouter models | 已有 | gi-coding-agent/sdk_attribution_test.go |
| 148 | does not add attribution headers when telemetry is disabled | 已有 | gi-coding-agent/sdk_attribution_test.go |
| 158 | adds attribution headers for custom providers routed through OpenRouter | 已有 | gi-coding-agent/sdk_attribution_test.go |
| 166 | lets provider and request headers override the defaults | 已有 | gi-coding-agent/sdk_attribution_test.go |

## `sdk-session-manager.test.ts`

Pi cases: `3`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/sdk_session_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 28 | uses agentDir for the default persisted session path | 已有 | gi-coding-agent/sdk_session_test.go |
| 49 | keeps an explicit sessionManager override | 已有 | gi-coding-agent/sdk_session_test.go |
| 67 | derives cwd from an explicit sessionManager when cwd is omitted | 已有 | gi-coding-agent/sdk_session_test.go |

## `sdk-skills.test.ts`

Pi cases: `3`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/sdk_session_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 41 | should discover skills by default and expose them on session.skills | 已有 | gi-coding-agent/sdk_session_test.go |
| 53 | should have empty skills when resource loader returns none (--no-skills) | 已有 | gi-coding-agent/sdk_session_test.go |
| 77 | should use provided skills when resource loader supplies them | 已有 | gi-coding-agent/sdk_session_test.go |

## `session-cwd.test.ts`

Pi cases: `3`
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/session_cwd_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 37 | detects missing session cwd from persisted sessions | 已有 | gi-coding-agent/session_cwd_test.go |
| 54 | supports overriding the effective cwd when opening a session | 已有 | gi-coding-agent/session_cwd_test.go |
| 67 | throws a controlled error before runtime creation when the stored cwd is missing | 已有 | gi-coding-agent/session_cwd_test.go |

## `session-info-modified-timestamp.test.ts`

Pi cases: `1`  
Status: `已有`  
Gi coverage / implementation target: `gi-coding-agent/session_manager_migration_list_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 49 | uses last user/assistant message timestamp instead of file mtime | 已有 | gi-coding-agent/session_manager_migration_list_test.go |

## `session-selector-path-delete.test.ts`

Pi cases: `7`
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/session_selector_path_delete_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 108 | does not treat Ctrl+Backspace as delete when search query is non-empty | 已有 | gi-coding-agent/session_selector_path_delete_test.go |
| 132 | enters confirmation mode on Ctrl+D even with a non-empty search query | 已有 | gi-coding-agent/session_selector_path_delete_test.go |
| 156 | enters confirmation mode on Ctrl+Backspace when search query is empty | 已有 | gi-coding-agent/session_selector_path_delete_test.go |
| 187 | does not switch scope back to All when All load resolves after toggling back to Current | 已有 | gi-coding-agent/session_selector_path_delete_test.go |
| 219 | does not start redundant All loads when toggling scopes while All is already loading | 已有 | gi-coding-agent/session_selector_path_delete_test.go |
| 249 | threads sessions when parent and child paths use different symlink aliases | 已有 | gi-coding-agent/session_selector_path_delete_test.go |
| 285 | treats the current session as active across symlink aliases | 已有 | gi-coding-agent/session_selector_path_delete_test.go |

## `session-selector-rename.test.ts`

Pi cases: `3`
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/session_selector_rename_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 41 | shows rename hint in interactive /resume picker configuration | 已有 | gi-coding-agent/session_selector_rename_test.go |
| 60 | does not show rename hint in --resume picker configuration | 已有 | gi-coding-agent/session_selector_rename_test.go |
| 79 | enters rename mode on Ctrl+R and submits with Enter | 已有 | gi-coding-agent/session_selector_rename_test.go |

## `session-selector-search.test.ts`

Pi cases: `9`
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/session_selector_search_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 22 | filters by quoted phrase with whitespace normalization | 已有 | gi-coding-agent/session_selector_search_test.go |
| 40 | filters by regex (re:) and is case-insensitive | 已有 | gi-coding-agent/session_selector_search_test.go |
| 58 | recent sort preserves input order | 已有 | gi-coding-agent/session_selector_search_test.go |
| 81 | relevance sort orders by score and tie-breaks by modified desc | 已有 | gi-coding-agent/session_selector_search_test.go |
| 115 | returns empty list for invalid regex | 已有 | gi-coding-agent/session_selector_search_test.go |
| 154 | returns all sessions when nameFilter is 'all' | 已有 | gi-coding-agent/session_selector_search_test.go |
| 159 | returns only named sessions when nameFilter is 'named' | 已有 | gi-coding-agent/session_selector_search_test.go |
| 164 | applies name filter before search query | 已有 | gi-coding-agent/session_selector_search_test.go |
| 169 | excludes whitespace-only names from named filter | 已有 | gi-coding-agent/session_selector_search_test.go |

## `settings-manager-bug.test.ts`

Pi cases: `4`  
Status: `已有`  
Gi coverage / implementation target: `gi-coding-agent/settings_manager_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 37 | should preserve file changes to packages array when changing unrelated setting | 已有 | gi-coding-agent/settings_manager_test.go |
| 74 | should preserve file changes to extensions array when changing unrelated setting | 已有 | gi-coding-agent/settings_manager_test.go |
| 102 | should preserve external project settings changes when updating unrelated project field | 已有 | gi-coding-agent/settings_manager_test.go |
| 126 | should let in-memory project changes override external changes for the same project field | 已有 | gi-coding-agent/settings_manager_test.go |

## `settings-manager.test.ts`

Pi cases: `17`  
Status: `已有`  
Gi coverage / implementation target: `gi-coding-agent/settings_manager_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 28 | should preserve enabledModels when changing thinking level | 已有 | gi-coding-agent/settings_manager_test.go |
| 59 | should preserve custom settings when changing theme | 已有 | gi-coding-agent/settings_manager_test.go |
| 87 | should let in-memory changes override file changes for same key | 已有 | gi-coding-agent/settings_manager_test.go |
| 114 | should keep local-only extensions in extensions array | 已有 | gi-coding-agent/settings_manager_test.go |
| 129 | should handle packages with filtering objects | 已有 | gi-coding-agent/settings_manager_test.go |
| 159 | should reload global settings from disk | 已有 | gi-coding-agent/settings_manager_test.go |
| 187 | should keep previous settings when file is invalid | 已有 | gi-coding-agent/settings_manager_test.go |
| 201 | should collect and clear load errors via drainErrors | 已有 | gi-coding-agent/settings_manager_test.go |
| 217 | should not create .pi folder when only reading project settings | 已有 | gi-coding-agent/settings_manager_test.go |
| 235 | should create .pi folder when writing project settings | 已有 | gi-coding-agent/settings_manager_test.go |
| 261 | should load shellCommandPrefix from settings | 已有 | gi-coding-agent/settings_manager_test.go |
| 270 | should return undefined when shellCommandPrefix is not set | 已有 | gi-coding-agent/settings_manager_test.go |
| 279 | should preserve shellCommandPrefix when saving unrelated settings | 已有 | gi-coding-agent/settings_manager_test.go |
| 294 | should return undefined when not set | 已有 | gi-coding-agent/settings_manager_test.go |
| 300 | should return global sessionDir | 已有 | gi-coding-agent/settings_manager_test.go |
| 306 | should return project sessionDir, overriding global | 已有 | gi-coding-agent/settings_manager_test.go |
| 313 | should expand ~ in sessionDir | 已有 | gi-coding-agent/settings_manager_test.go |

## `skills.test.ts`

Pi cases: `28`  
Status: `已有`  
Gi coverage / implementation target: `gi-agent-core/harness/skills_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 31 | should load a valid skill | 已有 | gi-agent-core/harness/skills_test.go |
| 44 | should allow names that don't match parent directory | 已有 | gi-agent-core/harness/skills_test.go |
| 57 | should warn when name contains invalid characters | 已有 | gi-agent-core/harness/skills_test.go |
| 67 | should warn when name exceeds 64 characters | 已有 | gi-agent-core/harness/skills_test.go |
| 77 | should warn and skip skill when description is missing | 已有 | gi-agent-core/harness/skills_test.go |
| 87 | should ignore unknown frontmatter fields | 已有 | gi-agent-core/harness/skills_test.go |
| 97 | should load nested skills recursively | 已有 | gi-agent-core/harness/skills_test.go |
| 108 | should prefer a directory's root SKILL.md over nested SKILL.md files | 已有 | gi-agent-core/harness/skills_test.go |
| 120 | should skip files without frontmatter | 已有 | gi-agent-core/harness/skills_test.go |
| 131 | should warn and skip skill when YAML frontmatter is invalid | 已有 | gi-agent-core/harness/skills_test.go |
| 141 | should preserve multiline descriptions from YAML | 已有 | gi-agent-core/harness/skills_test.go |
| 153 | should warn when name contains consecutive hyphens | 已有 | gi-agent-core/harness/skills_test.go |
| 163 | should load all skills from fixture directory | 已有 | gi-agent-core/harness/skills_test.go |
| 175 | should return empty for non-existent directory | 已有 | gi-agent-core/harness/skills_test.go |
| 185 | should use parent directory name when name not in frontmatter | 已有 | gi-agent-core/harness/skills_test.go |
| 198 | should parse disable-model-invocation frontmatter field | 已有 | gi-agent-core/harness/skills_test.go |
| 213 | should default disableModelInvocation to false when not specified | 已有 | gi-agent-core/harness/skills_test.go |
| 225 | should return empty string for no skills | 已有 | gi-agent-core/harness/skills_test.go |
| 230 | should format skills as XML | 已有 | gi-agent-core/harness/skills_test.go |
| 250 | should include intro text before XML | 已有 | gi-agent-core/harness/skills_test.go |
| 268 | should escape XML special characters | 已有 | gi-agent-core/harness/skills_test.go |
| 285 | should format multiple skills | 已有 | gi-agent-core/harness/skills_test.go |
| 308 | should exclude skills with disableModelInvocation from prompt | 已有 | gi-agent-core/harness/skills_test.go |
| 332 | should return empty string when all skills have disableModelInvocation | 已有 | gi-agent-core/harness/skills_test.go |
| 352 | should load from explicit skillPaths | 已有 | gi-agent-core/harness/skills_test.go |
| 364 | should warn when skill path does not exist | 已有 | gi-agent-core/harness/skills_test.go |
| 375 | should expand ~ in skillPaths | 已有 | gi-agent-core/harness/skills_test.go |
| 394 | should detect name collisions and keep first skill | 已有 | gi-agent-core/harness/skills_test.go |

## `stdout-cleanliness.test.ts`

Pi cases: `2`
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/stdout_cleanliness_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 84 | keeps stdout empty for --mode json --help while routing startup chatter to stderr | 已有 | gi-coding-agent/stdout_cleanliness_test.go |
| 94 | keeps stdout empty for -p --help while routing startup chatter to stderr | 已有 | gi-coding-agent/stdout_cleanliness_test.go |

## `syntax-highlight.test.ts`

Pi cases: `5`  
Status: `已有`  
Gi coverage / implementation target: `gi-coding-agent/syntax_highlight_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 5 | renders highlighted spans with the provided theme | 已有 | gi-coding-agent/syntax_highlight_test.go |
| 12 | decodes HTML entities emitted by highlight.js | 已有 | gi-coding-agent/syntax_highlight_test.go |
| 17 | inherits parent formatting for unmapped nested scopes | 已有 | gi-coding-agent/syntax_highlight_test.go |
| 28 | keeps parent formatting across unscoped nested spans | 已有 | gi-coding-agent/syntax_highlight_test.go |
| 35 | highlights code through highlight.js | 已有 | gi-coding-agent/syntax_highlight_test.go |

## `system-prompt.test.ts`

Pi cases: `7`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/system_prompt_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 6 | shows (none) for empty tools list | 已有 | gi-coding-agent/system_prompt_test.go |
| 17 | shows file paths guideline even with no tools | 已有 | gi-coding-agent/system_prompt_test.go |
| 30 | includes all default tools when snippets are provided | 已有 | gi-coding-agent/system_prompt_test.go |
| 51 | includes custom tools in available tools section when promptSnippet is provided | 已有 | gi-coding-agent/system_prompt_test.go |
| 65 | omits custom tools from available tools section when promptSnippet is not provided | 已有 | gi-coding-agent/system_prompt_test.go |
| 78 | appends promptGuidelines to default guidelines | 已有 | gi-coding-agent/system_prompt_test.go |
| 90 | deduplicates and trims promptGuidelines | 已有 | gi-coding-agent/system_prompt_test.go |

## `test-harness.test.ts`

Pi cases: `15`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/test_harness_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 19 | simple text response | 已有 | gi-coding-agent/test_harness_test.go |
| 34 | response sequence | 已有 | gi-coding-agent/test_harness_test.go |
| 50 | tool call response triggers tool execution | 已有 | gi-coding-agent/test_harness_test.go |
| 78 | error response | 已有 | gi-coding-agent/test_harness_test.go |
| 91 | retry on transient error | 已有 | gi-coding-agent/test_harness_test.go |
| 109 | custom usage numbers | 已有 | gi-coding-agent/test_harness_test.go |
| 121 | event capture | 已有 | gi-coding-agent/test_harness_test.go |
| 136 | context capture | 已有 | gi-coding-agent/test_harness_test.go |
| 147 | wraps around when more calls than responses | 已有 | gi-coding-agent/test_harness_test.go |
| 163 | streams text deltas | 已有 | gi-coding-agent/test_harness_test.go |
| 177 | streams thinking deltas | 已有 | gi-coding-agent/test_harness_test.go |
| 197 | streams tool call deltas | 已有 | gi-coding-agent/test_harness_test.go |
| 224 | streams thinking then text then tool call in order | 已有 | gi-coding-agent/test_harness_test.go |
| 260 | loads inline extension factories and disambiguates duplicate commands | 已有 | gi-coding-agent/test_harness_test.go |
| 312 | session persistence works | 已有 | gi-coding-agent/test_harness_test.go |

## `theme-export.test.ts`

Pi cases: `2`  
Status: `已有`  
Gi coverage / implementation target: `gi-coding-agent/theme_export_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 38 | resolves export variable references using the same syntax as colors | 已有 | gi-coding-agent/theme_export_test.go |
| 72 | resolves recursive vars and converts 256-color export values to hex | 已有 | gi-coding-agent/theme_export_test.go |

## `tool-execution-component.test.ts`

Pi cases: `16`
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/tool_execution_component_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 38 | stacks custom call and result renderers like the old implementation | 已有 | gi-coding-agent/tool_execution_component_test.go |
| 70 | uses built-in rendering for built-in overrides without custom renderers | 已有 | gi-coding-agent/tool_execution_component_test.go |
| 91 | preserves legacy file_path rendering compatibility for built-in tools | 已有 | gi-coding-agent/tool_execution_component_test.go |
| 106 | bash execute emits an initial empty partial update before output arrives | 已有 | gi-coding-agent/tool_execution_component_test.go |
| 126 | does not duplicate built-in headers when passed the active built-in definition | 已有 | gi-coding-agent/tool_execution_component_test.go |
| 141 | inherits missing built-in result renderer slot from the built-in tool | 已有 | gi-coding-agent/tool_execution_component_test.go |
| 162 | inherits missing built-in call renderer slot from the built-in tool | 已有 | gi-coding-agent/tool_execution_component_test.go |
| 184 | uses custom renderers for built-in overrides that reuse built-in definition parameters | 已有 | gi-coding-agent/tool_execution_component_test.go |
| 206 | uses custom renderers for built-in overrides that reuse wrapped built-in tool parameters | 已有 | gi-coding-agent/tool_execution_component_test.go |
| 228 | shares renderer state across custom call and result slots | 已有 | gi-coding-agent/tool_execution_component_test.go |
| 256 | exposes args in render result context | 已有 | gi-coding-agent/tool_execution_component_test.go |
| 278 | falls back when custom renderers are absent | 已有 | gi-coding-agent/tool_execution_component_test.go |
| 298 | trims trailing blank display lines from write previews | 已有 | gi-coding-agent/tool_execution_component_test.go |
| 314 | trims trailing blank display lines from read results | 已有 | gi-coding-agent/tool_execution_component_test.go |
| 368 | renders ${scenario.title} read results compactly until expanded | 已有 | gi-coding-agent/tool_execution_component_test.go |
| 400 | shows the read line range in compact ${scenario.title} reads before the expand hint | 已有 | gi-coding-agent/tool_execution_component_test.go |

## `tools.test.ts`

Pi cases: `68`  
Status: `已有`  
Gi coverage / implementation target: `gi-coding-agent/tools_read_test.go, gi-coding-agent/tools_bash_advanced_test.go, gi-coding-agent/tools_search_test.go, gi-coding-agent/tools_edit_fuzzy_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 51 | should read file contents that fit within limits | 已有 | gi-coding-agent/tools_read_test.go |
| 64 | should handle non-existent files | 已有 | gi-coding-agent/tools_read_test.go |
| 70 | should truncate files exceeding line limit | 已有 | gi-coding-agent/tools_read_test.go |
| 84 | should truncate when byte limit exceeded | 已有 | gi-coding-agent/tools_read_test.go |
| 98 | should handle offset parameter | 已有 | gi-coding-agent/tools_read_test.go |
| 113 | should handle limit parameter | 已有 | gi-coding-agent/tools_read_test.go |
| 127 | should handle offset + limit together | 已有 | gi-coding-agent/tools_read_test.go |
| 146 | should show error when offset is beyond file length | 已有 | gi-coding-agent/tools_read_test.go |
| 155 | should include truncation details when truncated | 已有 | gi-coding-agent/tools_read_test.go |
| 170 | should detect image MIME type from file magic (not extension) | 已有 | gi-coding-agent/tools_read_test.go |
| 192 | should treat files with image extension but non-image content as text | 已有 | gi-coding-agent/tools_read_test.go |
| 205 | should write file contents | 已有 | gi-coding-agent/tools_write_edit_test.go |
| 216 | should create parent directories | 已有 | gi-coding-agent/tools_write_edit_test.go |
| 227 | should replace text in file | 已有 | gi-coding-agent/tools_edit_test.go |
| 244 | should fail if text not found | 已有 | gi-coding-agent/tools_edit_test.go |
| 257 | should include ENOENT when the edit target does not exist | 已有 | gi-coding-agent/tools_edit_test.go |
| 268 | should fail if text appears multiple times | 已有 | gi-coding-agent/tools_edit_test.go |
| 281 | should replace multiple disjoint regions in one call | 已有 | gi-coding-agent/tools_edit_test.go |
| 299 | should collapse large unchanged gaps in multi-edit diffs | 已有 | gi-coding-agent/tools_edit_test.go |
| 322 | should match edits against the original file, not incrementally | 已有 | gi-coding-agent/tools_edit_test.go |
| 337 | should fail when edits is empty | 已有 | gi-coding-agent/tools_write_edit_test.go |
| 349 | should fail when multi-edit regions overlap | 已有 | gi-coding-agent/tools_edit_test.go |
| 364 | should not partially apply edits when one edit fails | 已有 | gi-coding-agent/tools_edit_test.go |
| 382 | should include EACCES for read-only files | 已有 | gi-coding-agent/tools_edit_errors_test.go |
| 395 | should include the original error message for unknown edit access errors | 已有 | gi-coding-agent/tools_edit_errors_test.go |
| 414 | should include ENOENT in diff preview for missing files | 已有 | gi-coding-agent/tools_edit_errors_test.go |
| 421 | should include EACCES in diff preview for unreadable files | 已有 | gi-coding-agent/tools_edit_errors_test.go |
| 433 | should execute simple commands | 已有 | gi-coding-agent/tools_bash_test.go |
| 440 | should handle command errors | 已有 | gi-coding-agent/tools_bash_test.go |
| 446 | should respect timeout | 已有 | gi-coding-agent/tools_bash_test.go |
| 452 | should include full output path for truncated timeout and abort errors | 已有 | gi-coding-agent/tools_bash_advanced_test.go |
| 488 | should throw error when cwd does not exist | 已有 | gi-coding-agent/tools_bash_test.go |
| 498 | should handle process spawn errors | 已有 | gi-coding-agent/tools_bash_advanced_test.go |
| 509 | should pass shellPath through to shell resolution | 已有 | gi-coding-agent/tools_bash_advanced_test.go |
| 531 | should prepend command prefix when configured | 已有 | gi-coding-agent/tools_bash_test.go |
| 540 | should include output from both prefix and command | 已有 | gi-coding-agent/tools_bash_test.go |
| 549 | should work without command prefix | 已有 | gi-coding-agent/tools_bash_test.go |
| 556 | should coalesce streaming updates for chatty output | 已有 | gi-coding-agent/tools_bash_advanced_test.go |
| 576 | should decode UTF-8 characters split across output chunks | 已有 | gi-coding-agent/tools_bash_advanced_test.go |
| 592 | should expose local bash operations for extension reuse | 已有 | gi-coding-agent/tools_bash_advanced_test.go |
| 605 | should preserve executeBash sanitization when using local bash operations | 已有 | gi-coding-agent/tools_bash_advanced_test.go |
| 616 | should persist full output when truncation happens by line count only | 已有 | gi-coding-agent/tools_bash_advanced_test.go |
| 639 | executeBash should persist full output when truncation happens by line count only | 已有 | gi-coding-agent/tools_bash_advanced_test.go |
| 659 | should include filename when searching a single file | 已有 | gi-coding-agent/tools_search_test.go |
| 672 | should respect global limit and include context lines | 已有 | gi-coding-agent/tools_search_test.go |
| 693 | should treat flag-like patterns as search text | 已有 | gi-coding-agent/tools_search_test.go |
| 712 | should include hidden files that are not gitignored | 已有 | gi-coding-agent/tools_search_test.go |
| 732 | should respect .gitignore | 已有 | gi-coding-agent/tools_search_test.go |
| 747 | should surface fd glob parse errors | 已有 | gi-coding-agent/tools_search_test.go |
| 756 | should treat flag-like patterns as search text | 已有 | gi-coding-agent/tools_search_test.go |
| 767 | should list dotfiles and directories | 已有 | gi-coding-agent/tools_search_test.go |
| 792 | should match text with trailing whitespace stripped | 已有 | gi-coding-agent/tools_edit_fuzzy_test.go |
| 808 | should match fullwidth punctuation in Chinese text | 已有 | gi-coding-agent/tools_edit_fuzzy_test.go |
| 822 | should match compatibility-equivalent Unicode forms | 已有 | gi-coding-agent/tools_edit_fuzzy_test.go |
| 836 | should match smart single quotes to ASCII quotes | 已有 | gi-coding-agent/tools_edit_fuzzy_test.go |
| 852 | should match smart double quotes to ASCII quotes | 已有 | gi-coding-agent/tools_edit_fuzzy_test.go |
| 868 | should match Unicode dashes to ASCII hyphen | 已有 | gi-coding-agent/tools_edit_fuzzy_test.go |
| 884 | should match non-breaking space to regular space | 已有 | gi-coding-agent/tools_edit_fuzzy_test.go |
| 900 | should prefer exact match over fuzzy match | 已有 | gi-coding-agent/tools_edit_fuzzy_test.go |
| 915 | should still fail when text is not found even with fuzzy matching | 已有 | gi-coding-agent/tools_edit_fuzzy_test.go |
| 927 | should detect duplicates after fuzzy normalization | 已有 | gi-coding-agent/tools_edit_fuzzy_test.go |
| 940 | should support fuzzy matching in multi-edit mode | 已有 | gi-coding-agent/tools_edit_fuzzy_test.go |
| 968 | should match LF oldText against CRLF file content | 已有 | gi-coding-agent/tools_edit_fuzzy_test.go |
| 981 | should preserve CRLF line endings after edit | 已有 | gi-coding-agent/tools_edit_fuzzy_test.go |
| 994 | should preserve LF line endings for LF files | 已有 | gi-coding-agent/tools_edit_fuzzy_test.go |
| 1007 | should detect duplicates across CRLF/LF variants | 已有 | gi-coding-agent/tools_edit_fuzzy_test.go |
| 1020 | should preserve UTF-8 BOM after edit | 已有 | gi-coding-agent/tools_edit_fuzzy_test.go |
| 1033 | should preserve CRLF line endings and BOM in multi-edit mode | 已有 | gi-coding-agent/tools_edit_fuzzy_test.go |

## `tree-selector.test.ts`

Pi cases: `15`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/tree_selector_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 129 | focuses nearest visible ancestor when currentLeafId is a model_change with sibling branch | 已有 | gi-coding-agent/tree_selector_test.go |
| 158 | focuses nearest visible ancestor when currentLeafId is a thinking_level_change entry | 已有 | gi-coding-agent/tree_selector_test.go |
| 189 | switches to nearest visible user message when changing to user-only filter | 已有 | gi-coding-agent/tree_selector_test.go |
| 218 | returns to nearest visible ancestor when switching back to default filter | 已有 | gi-coding-agent/tree_selector_test.go |
| 252 | toggles label timestamps for labeled nodes | 已有 | gi-coding-agent/tree_selector_test.go |
| 282 | preserves selection when switching to empty labeled filter and back | 已有 | gi-coding-agent/tree_selector_test.go |
| 316 | preserves selection through multiple empty filter switches | 已有 | gi-coding-agent/tree_selector_test.go |
| 392 | ctrl+right unfolds a folded node, then does segment jump when unfolded | 已有 | gi-coding-agent/tree_selector_test.go |
| 428 | alt+left/right are aliases for fold and unfold navigation | 已有 | gi-coding-agent/tree_selector_test.go |
| 452 | folding root hides entire subtree, nested fold preserved on unfold | 已有 | gi-coding-agent/tree_selector_test.go |
| 488 | fold and navigate on non-active branch | 已有 | gi-coding-agent/tree_selector_test.go |
| 523 | fold and navigate with multiple roots | 已有 | gi-coding-agent/tree_selector_test.go |
| 564 | folding root hides descendants even when intermediate nodes are filtered out | 已有 | gi-coding-agent/tree_selector_test.go |
| 592 | search resets fold state | 已有 | gi-coding-agent/tree_selector_test.go |
| 625 | filter mode change resets fold state | 已有 | gi-coding-agent/tree_selector_test.go |

## `trigger-compact-extension.test.ts`

Pi cases: `1`  
Status: `已有`
Gi coverage / implementation target: `gi-coding-agent/trigger_compact_extension_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 25 | only auto-compacts when context usage crosses the threshold | 已有 | gi-coding-agent/trigger_compact_extension_test.go |

## `truncate-to-width.test.ts`

Pi cases: `6`  
Status: `已有`  
Gi coverage / implementation target: `gi-tui/utils_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 11 | should truncate messages with Unicode characters correctly | 已有 | gi-tui/utils_test.go |
| 23 | should handle emoji characters | 已有 | gi-tui/utils_test.go |
| 34 | should handle mixed ASCII and wide characters | 已有 | gi-tui/utils_test.go |
| 45 | should not truncate messages that fit | 已有 | gi-tui/utils_test.go |
| 56 | should add ellipsis when truncating | 已有 | gi-tui/utils_test.go |
| 67 | should handle the exact crash case from issue report | 已有 | gi-tui/utils_test.go |

## `user-message.test.ts`

Pi cases: `1`  
Status: `已有`  
Gi coverage / implementation target: `gi-coding-agent/message_components_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 11 | keeps user message height stable while moving closing OSC markers off line end | 已有 | gi-coding-agent/message_components_test.go |

## `version-check.test.ts`

Pi cases: `5`  
Status: `已有`  
Gi coverage / implementation target: `gi-coding-agent/version_check_test.go`

| Pi line | Pi test case | Status | Gi coverage / next step |
|---:|---|---|---|
| 28 | compares package versions | 已有 | gi-coding-agent/version_check_test.go |
| 36 | returns only newer versions | 已有 | gi-coding-agent/version_check_test.go |
| 44 | uses the pi.dev version check api with a pi user agent | 已有 | gi-coding-agent/version_check_test.go |
| 60 | returns the active package name from the version check api | 已有 | gi-coding-agent/version_check_test.go |
| 67 | skips api calls when version checks are disabled | 已有 | gi-coding-agent/version_check_test.go |
