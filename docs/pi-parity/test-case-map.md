# Pi Test-Case Parity Tracker

This document tracks test-case-level parity between the local Pi checkout at
`~/Projects/agents/pi` and Gi. It is intentionally stricter than file-level
mapping: a Pi test file only counts as covered when the user-visible contract is
represented by named Gi tests or by an explicit product-scope decision.

## Current Counts

Extracted with:

```sh
node docs/pi-parity/verify-test-case-map.mjs \
  --pi-root /Users/nowa/Projects/agents/pi \
  --gi-root /Users/nowa/Projects/agents/gi
```

The generated file-level inventory is in
`docs/pi-parity/test-case-inventory.md`.

| Area | Pi `test`/`it` cases | Gi top-level Go tests | Case-name candidates | Current status |
| --- | ---: | ---: | ---: | --- |
| `packages/ai` / `gi-llm-provider` | 788 | 195 | 788/788 | Provider/model/source surface has named case coverage for every extracted Pi AI case; all 67 Pi test files have explicit Gi candidates. The former lazy-module-loading weak spot is represented by a Go-native provider dependency/dispatch guard. |
| `packages/agent` / `gi-agent-core` + harness | 150 | 97 | 150/150 | Core loop, proxy, session, compaction, and harness behavior have named case coverage for every extracted Pi agent-core case; all 16 Pi test files have explicit Gi candidate files. |
| `packages/tui` / `gi-tui` | 592 | 462 | 592/592 | Component/editor/terminal parity has named case candidates for every extracted Pi TUI case; all 22 Pi test files have explicit Gi candidate files, with fixture-level Markdown/xterm checks still validated by focused TUI tests. |
| `packages/coding-agent` / `gi-coding-agent` | 1225 | 947 | 1225/1225 | Interactive, print/RPC, packages, extensions, tools, session, OAuth, and utility edge cases now have named case coverage for every extracted Pi coding-agent case; all 121 Pi test files have explicit Gi candidates. |

These counts are not expected to match one-for-one because Pi uses nested
Vitest cases while Gi uses Go top-level tests with table-driven subtests. The
coverage requirement is behavioral, not count equality.

## Current File-Level Weak Spots

The generated inventory identifies Pi test files without obvious filename-token
Gi candidates, and Pi cases without obvious Go test/subtest name candidates.
These are not automatically gaps, but each needs one of: a named Gi test
mapping, a new Gi implementation/test, or an explicit scope decision.

| Area | Weak files |
| --- | --- |
| LLM provider | none at file-candidate level; Pi `lazy-module-load.test.ts` now maps to `gi-llm-provider/lazy_module_load_test.go`, which guards the Go-native equivalent provider dependency/dispatch boundary. |
| Agent core | none at file-candidate level |
| TUI | none at file-candidate level; continue fixture-level checks for consolidated component files |
| Coding agent | none at file-candidate level |

## Current Case-Level Work Queue

The case-name heuristic gives a stronger work queue than file-level mapping,
but remains a weak textual signal. A no-case-candidate row can still be covered
by a broader table-driven Go test; it means the coverage is not yet obvious from
test names alone.

| Area | Pi cases without Go test/subtest name candidates |
| --- | ---: |
| LLM provider | 0 |
| Agent core | 0 |
| TUI | 0 |
| Coding agent | 0 |

## Confirmed High-Risk UI/Logic Cases

| Pi test case or contract | Gi coverage | Status |
| --- | --- | --- |
| `assistant-message.test.ts` adds OSC 133 zone markers to assistant messages without tool calls | `TestAssistantMessageComponentOSC133Markers` | covered |
| `assistant-message.test.ts` does not add OSC 133 markers when assistant message contains tool calls | `TestAssistantMessageComponentOSC133Markers` | covered |
| Hidden thinking block renders Pi-style label only when a real thinking block exists | `TestCLIInteractiveTUIHostDefaultThinkingVisibilityMatchesPi`, `TestCLIInteractiveTUIHostHiddenThinkingFinalMessageStaysInAssistantFlowPiStyle`, `TestAssistantMessageComponentOSC133Markers` subtests | covered |
| Streaming hidden thinking partial is replaced in-place by the final assistant message | `TestCLIInteractiveTUIHostReplacesHiddenThinkingPartialPiStyle`, `TestCLIInteractiveTUIHostReplacesHiddenThinkingPartialFromSessionEventsPiStyle` | covered |
| Live user message renders above working status, with `Working...` outside chat transcript | `TestCLIInteractiveTUIHostKeepsLiveUserPromptAboveWorkingStatusPiStyle`, `TestCLIInteractiveTUIHostAgentStartShowsWorkingLoaderPiStyle` | covered |
| Manual/auto compaction status is temporary status UI, not durable transcript content | `TestCLIInteractiveTUIHostCancelsCompactionPiStyle`, `TestCLIInteractiveTUIHostQueuesInputDuringCompactionPiStyle` | covered |
| Auto-retry status is temporary status UI and clears on success/cancel | `TestCLIInteractiveTUIHostShowsAndCancelsAutoRetryPiStyle`, `TestCLIInteractiveTUIHostAutoRetrySuccessClearsStatusPiStyle` | covered |
| Assistant response followed by next user message has Pi-style blank spacer | `TestCLIInteractiveTUIHostSeparatesAssistantAndNextUserMessagePiStyle` | covered |
| `thinking_level_changed` refreshes footer and editor border color immediately | `TestCLIInteractiveTUIHostThinkingLevelEventUpdatesEditorBorderPiStyle` | covered |
| Model resolver accepts Pi `model:thinking` patterns and rejects invalid thinking suffixes | `TestParseModelPatternCompatibilityCases`, `TestResolveCLIModelCompatibilityCases` | covered |
| DeepSeek V4 Pro supports only `off/high/xhigh`, so default `medium` clamps to `high` like Pi | `TestGetSupportedThinkingLevels`, `TestOfficialGrokAndDeepSeekModelCatalog`, `TestResolveCLIPrintModeModelClampsThinkingToModelCapabilities` | covered |
| Current Pi generated model catalog entries and compat flags are registered in Gi, including Fireworks Kimi K2.6, Kimi K2.5 Turbo, and MiniMax M2.1 | `TestFireworksModelCatalog`, `TestFireworksAnthropicHeadersAndToolCompat` | covered |
| `lazy-module-load.test.ts` root import/direct Anthropic wrapper/root dispatch contracts map to Gi's Go-native provider linking boundary | `TestLazyProviderModuleLoadingPiParity` | covered |
| Agent session runtime restores destination model/thinking state on session switch | `TestAgentSessionRuntimeSwitchUsesDestinationCWDAndModelState` | covered |
| Retry event lifecycle, event order, and streaming deltas match Pi suite contracts | `TestAgentSessionRetryEventsPiParity*` tests | covered |
| Default coding-agent provider path consumes real provider streaming events instead of waiting for complete responses before synthesizing updates | `TestAgentSessionStreamResponderPiParityUsesProviderStreamingEvents`, `TestAgentSessionMessageUpdateExtensionEventPiParity` | covered |
| TUI fuzzy matching preserves Pi order/case/boundary/exact-prefix scoring cases | `TestFuzzyMatchPiCaseParity`, `TestFuzzyFilterPiCaseParity` | covered |
| TUI `@` file suggestions use Pi-style `fd` substring/full-path scoring instead of over-broad fuzzy path matches | `TestCombinedAutocompleteFileSuggestionPiCaseParity`, `TestCombinedAutocompleteDotSlashPiCaseParity` | covered |
| TUI parses legacy double-bracket pageUp sequence | `TestKeysLegacySequencesAndKittyAltGate/should_parse_double_bracket_pageUp` | covered |
| TUI empty stdin input emits an empty data event | `TestStdinBufferPiEdgeCases/should_handle_empty_input` | covered |
| TUI image-line detection catches embedded image escape sequences even when the old prefix-only implementation would miss them | `TestIsImageLineBugRegressionPiCaseNames/old_implementation_would_return_false_causing_crash` | covered |
| TUI terminal image capabilities match Pi for unknown terminals, Ghostty, cmux+Ghostty, and VSCode hyperlink behavior | `TestDetectCapabilitiesPiCaseNames` subtests | covered |
| TUI Kitty image rendering honors `maxHeightCells` by reducing rendered width | `TestRenderImagePiCaseNames/honors_maxHeightCells_by_reducing_rendered_width` | covered |
| TUI OSC 8 hyperlinks work with `file://` URIs | `TestHyperlinkPiOSC8ExactRendering/works_with_file://_URIs` | covered |

## Remaining Work

- Expand this tracker from file-level candidates to case-level assertions for
  each extracted Pi case using the generated inventory as the work queue.
- Keep product-scope exclusions explicit, especially Pi npm package source
  tests and in-process TypeScript extension APIs that Gi intentionally replaces
  with protocol packages / ViewTree / process RPC.
- Continue fixture-level checks for Markdown and virtual terminal behavior,
  where public API parity is already mapped but renderer/emulator semantics can
  drift.
