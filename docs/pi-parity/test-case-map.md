# Pi Test-Case Parity Tracker

> These counts are generated against Pi v0.82.1 at
> `b4f293684bba718d59cc1157679bcf6157b3a7f5`. `baseline.json` declares scope
> and `v0.82.1-open-gaps.json` is the authoritative zero-gap drift snapshot.

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
| `packages/ai` / `gi-llm-provider` | 1208 | 457 | 1208/1208 | Provider/model/source surface has named case coverage for all 113 in-scope Pi AI test files; one TypeScript generator file with 3 cases is explicitly excluded. |
| `packages/agent` / `gi-agent-core` + harness | 212 | 144 | 212/212 | Core loop, proxy, session, compaction, and harness behavior have named case coverage for all 16 in-scope Pi agent-core test files; 2 optional SQLite-adapter files with 12 cases are explicitly excluded. |
| `packages/tui` / `gi-tui` | 700 | 501 | 700/700 | Component/editor/terminal parity has named case candidates for all 27 Pi TUI test files, with fixture-level Markdown/xterm checks validated by focused TUI tests. |
| `packages/coding-agent` / `gi-coding-agent` | 1650 | 1283 | 1650/1650 | Interactive, print/RPC, packages, extensions, tools, session, OAuth, and utility edge cases have named case coverage for all 181 in-scope Pi coding-agent test files; 2 product/runtime-specific files with 10 cases are explicitly excluded. |

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

## Pi v0.82.1 Incremental Test Delta

The `v0.82.0..v0.82.1` diff adds 34 `test`/`it` declarations under the
in-scope AI and coding-agent test trees. Each declaration has an explicit Go
regression below; table-driven subtests intentionally share their parent test.

| Pi file and added case | Gi regression |
| --- | --- |
| `anthropic-auth-token.test.ts`: resolves `ANTHROPIC_AUTH_TOKEN` as a bearer Authorization header | `TestAnthropicAmbientAuthReachesRequestShape/uses_bearer_authorization_without_OAuth_shaping` |
| `anthropic-auth-token.test.ts`: preserves `ANTHROPIC_OAUTH_TOKEN` as OAuth-shaped API auth | `TestAnthropicAmbientAuthReachesRequestShape/preserves_OAuth_shaping_for_OAuth_token` |
| `anthropic-auth-token.test.ts`: uses Authorization headers without OAuth-mode request shaping | `TestAnthropicAmbientAuthReachesRequestShape/uses_bearer_authorization_without_OAuth_shaping` |
| `anthropic-auth-token.test.ts`: threads auth-context `ANTHROPIC_AUTH_TOKEN` through request headers | `TestAnthropicAmbientAuthReachesRequestShape/uses_bearer_authorization_without_OAuth_shaping` |
| `anthropic-auth-token.test.ts`: preserves OAuth request shaping for `ANTHROPIC_OAUTH_TOKEN` | `TestAnthropicAmbientAuthReachesRequestShape/preserves_OAuth_shaping_for_OAuth_token` |
| `anthropic-auth-token.test.ts`: lets explicit request headers override `ANTHROPIC_AUTH_TOKEN` | `TestAnthropicAmbientAuthReachesRequestShape/explicit_request_header_overrides_ambient_bearer` |
| `bedrock-models.test.ts`: exposes Claude Opus 5 through an inference profile only | `TestBedrockCatalogExposesOpus5ThroughInferenceProfileOnly` |
| `bedrock-thinking-payload.test.ts`: uses adaptive thinking for Claude Opus 5 | `TestBuildBedrockAdditionalModelRequestFieldsThinking/adaptive_opus_5_high` |
| `bedrock-thinking-payload.test.ts`: maps Claude Opus 5 `xhigh` reasoning to `effort=xhigh` | `TestBuildBedrockAdditionalModelRequestFieldsThinking/adaptive_opus_5_xhigh` |
| `env-api-keys.test.ts`: reports `ANTHROPIC_AUTH_TOKEN` but preserves OAuth token API-key lookup | `TestEnvironmentAPIKeysSeparatesAnthropicBearerFromAPIKeys` |
| `env-api-keys.test.ts`: does not return `ANTHROPIC_AUTH_TOKEN` as an API key | `TestEnvironmentAPIKeysSeparatesAnthropicBearerFromAPIKeys` |
| `env-api-keys.test.ts`: preserves `ANTHROPIC_OAUTH_TOKEN` as an API key | `TestEnvironmentAPIKeysSeparatesAnthropicBearerFromAPIKeys` |
| `env-api-keys.test.ts`: falls back to `ANTHROPIC_API_KEY` for API-key lookup | `TestEnvironmentAPIKeysSeparatesAnthropicBearerFromAPIKeys` |
| `error-body.test.ts`: ignores a Bedrock response stream instead of serializing internals | `TestNormalizeProviderErrorIgnoresBedrockResponseStreamInternals` |
| `models-runtime.test.ts`: keeps the underlying reason in wrapped OAuth refresh errors | `TestModelsRuntime/rejects_with_code_oauth_when_refresh_fails_preserving_the_stored_credential` |
| `provider-error-body-regression.test.ts`: preserves the Bedrock SDK validation message for a stream body | `TestBedrockPreservesSDKValidationMessageWhenResponseBodyIsStream` |
| `providers.test.ts`: resolves Anthropic bearer auth with auth-token precedence | `TestBuiltinProviderPiContracts/resolves_Anthropic_bearer_auth_from_env_with_auth_token_precedence` |
| `providers.test.ts`: preserves Anthropic OAuth-token precedence over the API key | `TestBuiltinProviderPiContracts/preserves_Anthropic_OAuth_token_precedence_over_API_key` |
| `radius-oauth.test.ts`: uses gateway endpoints directly for device login | `TestRadiusOAuthDeviceCodeLogin` |
| `radius-oauth.test.ts`: refreshes directly through the gateway without discovery | `TestRadiusOAuthRefreshPreservesProviderMetadata` |
| `radius-oauth.test.ts`: discovers only the interactive browser authorization endpoint | `TestRadiusOAuthBrowserLogin` |
| `supports-xhigh.test.ts`: includes `xhigh` and `max` for Anthropic Opus 5 | `TestSupportedThinkingLevelsPiCaseNames/includes_xhigh_and_max_for_Anthropic_Opus_5_on_anthropic-messages_API` |
| `supports-xhigh.test.ts`: includes `xhigh` and `max` for Bedrock Claude Opus 5 | `TestSupportedThinkingLevelsPiCaseNames/includes_xhigh_and_max_for_Bedrock_Claude_Opus_5` |
| `custom-message.test.ts`: provides output padding to custom renderers and updates it | `TestCLICustomMessageComponentProvidesOutputPaddingToRenderersAndUpdatesIt` |
| `llama-extension.test.ts`: persists and restores loaded models for cache-only startup refreshes | `TestLlamaProviderPersistsAndRestoresLoadedModelsForCacheOnlyStartupRefreshes` |
| `remote-catalog-provider.test.ts`: revalidates a stored catalog with ETag and keeps the overlay on 304 | `TestWithRemoteCatalogETagLifecycle/revalidates_a_stored_catalog_with_its_etag_and_keeps_the_overlay_on_304` |
| `remote-catalog-provider.test.ts`: drops a stale ETag when the overlay becomes unavailable | `TestWithRemoteCatalogETagLifecycle/drops_a_stale_etag_when_the_overlay_becomes_unavailable` |
| `remote-catalog-provider.test.ts`: keeps the ETag and overlay after a transient failure | `TestWithRemoteCatalogETagLifecycle/keeps_the_etag_and_overlay_after_a_transient_failure` |
| `resource-loader.test.ts`: ignores context-file candidates that are directories | `TestDefaultResourceLoaderPiBasics/ignores_context_file_candidates_that_are_directories` |
| `agent-session-compaction.test.ts`: manually compacts with provider-resolved bearer auth | `TestAgentSessionCompactManualWithProviderResolvedBearerAuth` |
| `6949-unavailable-scoped-model.test.ts`: shows and removes an enabled model without a catalog entry | `TestScopedModelsSelectorShowsAndRemovesEnabledModelWithoutCatalogEntry` |
| `6949-unavailable-scoped-model.test.ts`: passes unmatched settings patterns with one combined resolution | `TestScopedModelsSelectorCarriesUnmatchedSettingsAndUnavailableSessionScope` |
| `6949-unavailable-scoped-model.test.ts`: opens when only a session-scoped model is unavailable | `TestScopedModelsSelectorCarriesUnmatchedSettingsAndUnavailableSessionScope` |
| `6949-unavailable-scoped-model.test.ts`: does not clear a partial scope when an enabled model is unavailable | `TestScopedModelsSelectorDoesNotClearPartialScopeWhenEnabledModelIsUnavailable` |

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
| `status-indicator.test.ts` keeps idle height stable, reserves the bottom status region, and stops retry updates after disposal | `TestStatusIndicatorsPiCases` exact subtests, `TestCLIInteractiveLayoutReservesBottomRegionForTransientStatusPiStyle` | covered |
| Summarization retry swaps retry, branch-summary, and compaction status without stale-owner clears | `TestCLIInteractiveTUIHostSummarizationRetryStatusFlowPiStyle` | covered |
| Assistant response followed by next user message has Pi-style blank spacer | `TestCLIInteractiveTUIHostSeparatesAssistantAndNextUserMessagePiStyle` | covered |
| `thinking_level_changed` refreshes footer and editor border color immediately | `TestCLIInteractiveTUIHostThinkingLevelEventUpdatesEditorBorderPiStyle` | covered |
| Model resolver accepts Pi `model:thinking` patterns and rejects invalid thinking suffixes | `TestParseModelPatternCompatibilityCases`, `TestResolveCLIModelCompatibilityCases` | covered |
| `/model` provider-prefixed search ranks a direct provider model before a proxy-provider ID | `TestModelSearchTextPiContract`, `TestModelSelectorSearchRanksExactProviderBeforeProxyIDPiStyle`, `TestCLIInteractiveTUIHostModelSlashArgumentAutocompletePiStyle` | covered |
| `ModelSelectorComponent` renders the cached runtime snapshot before an asynchronous catalog refresh, republishes refreshed definitions without losing scoped thinking levels, reports refresh outcomes, closes its refresh lifecycle before selection callbacks, and retains static catalogs when an optional runtime is a typed nil | `TestModelSelectorTreatsTypedNilRuntimeAsStaticCatalog`, `TestModelSelectorLoadsCachedSnapshotThenPublishesRefresh`, `TestModelSelectorReportsRefreshOutcomes`, `TestModelSelectorTimesOutAndKeepsCachedModels`, `TestModelSelectorSelectionClosesRefreshBeforeCallback` | covered through the narrow `ModelSelectorRuntime` boundary and race-tested component state |
| DeepSeek V4 supports only `off/high/max`, so unsupported intermediate levels clamp upward/downward through the canonical order | `TestGetSupportedThinkingLevels`, `TestSupportedThinkingLevelsPiCaseNames`, `TestOfficialGrokAndDeepSeekModelCatalog` | covered |
| The official Pi v0.82.1 generated model catalog and compat flags are registered in Gi, including 1,109 models, Claude Opus 5, Fireworks Kimi K2.6 Turbo, GPT-5.6 pricing tiers, and `max` thinking maps | `TestFireworksModelCatalog`, `TestSupportedThinkingLevelsPiCaseNames`, `TestMaxThinkingLevelPiCaseNames`, `TestOfficialClaudeOpus5ModelCatalog`, generator tests | covered |
| `packages/ai/test/anthropic-auth-token.test.ts` bearer-token precedence, OAuth shaping, request-scoped environment, and explicit-header override | `TestAnthropicAmbientAuthReachesRequestShape`, `TestAnthropicAPIKeyLookupExcludesBearerToken`, `TestAnthropicMessagesProviderUsesOAuthAuthorizationHeadersPiStyle` | covered through provider-owned `ModelAuth` resolution |
| `packages/ai/test/radius-oauth.test.ts` gateway-direct device/token/refresh endpoints and browser-only discovery | `TestRadiusOAuthDeviceCodeLogin`, `TestRadiusOAuthRefreshPreservesProviderMetadata`, `TestRadiusOAuthBrowserLogin` | covered through the split Radius HTTP and orchestration boundaries |
| `packages/ai/test/error-body.test.ts` and `provider-error-body-regression.test.ts` do not consume or serialize Bedrock response streams while retaining SDK validation details | `TestNormalizeProviderErrorIgnoresBedrockResponseStreamInternals`, `TestBedrockPreservesSDKValidationMessageWhenResponseBodyIsStream` | covered through typed status-only SDK error inspection |
| `lazy-module-load.test.ts` root import/direct Anthropic wrapper/root dispatch contracts map to Gi's Go-native provider linking boundary | `TestLazyProviderModuleLoadingPiParity` | covered |
| `packages/ai/test/models-runtime.test.ts` provider lifecycle, dynamic refresh/cache restore, credential selection, OAuth serialization, login/logout, header/env assembly, and lazy stream errors | `TestModelsRuntime`, `TestInMemoryModelsStoreClonesEntries`, `TestResolveProviderAuth*`, `TestAuthStorageCredentialStore*` | covered |
| `packages/ai/test/pi-messages.test.ts` request/debug/error/rewrite/SSE conversion and built-in registration | `TestPiMessagesPiContracts`, focused pi-messages transport tests | covered |
| `packages/ai/test/max-thinking.test.ts` opt-in `max`, holes in thinking-level maps, GPT-5.6 catalog metadata, and Codex payload forwarding | `TestMaxThinkingLevelPiCaseNames` | covered |
| `packages/ai/test/oauth-device-code.test.ts` immediate/optional-delayed polling, RFC 8628 slow-down intervals, and cancellation | `TestOAuthDeviceCodePollingPiContracts` exact subcases | covered |
| `packages/ai/test/oauth-auth.test.ts` shared OAuth credential derivation and refresh-storage boundary | `TestResolveProviderAuth*`, `TestAuthStorageCredentialStore*`, `TestRadiusOAuth*`, `TestKimiCodingOAuth*`, `TestOpenRouterOAuth*`, `TestXAIOAuth*`, existing provider OAuth tests | covered for the provider-owned Radius, Kimi, OpenRouter, and xAI flows |
| `packages/ai/test/images-models.test.ts` ordered provider/model lifecycle, auth and option merging, unconfigured dispatch, coalesced refresh, typed targeted failures, and built-in OpenRouter assembly | `TestImagesModels*`, `TestImagesProviderRefreshCoalescesAndPreservesLastSnapshot`, `TestBuiltinImagesModelsSharesOpenRouterCredentialsWithTextModels` | covered |
| `packages/ai/test/openrouter-oauth.test.ts` one-shot PKCE callback, permanent key, exchange failures, cancellation, callback host, and shared text/image auth | `TestOpenRouterOAuth*`, `TestBuiltinOpenRouterProviderUsesDefaultOAuth`, `TestBuiltinImagesModelsSharesOpenRouterCredentialsWithTextModels` | covered |
| `packages/coding-agent/test/models-store.test.ts` provider-isolated JSON persistence and deletion | `TestFileModelsStorePiExactCaseNames` | covered |
| `packages/coding-agent/test/remote-catalog-provider.test.ts` keyed catalog parsing, attribution, TTL/force policy, ETag revalidation/304/unavailable/transient policy, generated-vs-remote recency, and unavailable routes | `TestWithRemoteCatalogRefreshLifecycle`, `TestWithRemoteCatalogETagLifecycle`, and `TestInMemoryModelsStoreClonesEntries` exact subcases | covered through the reusable provider-owned remote overlay |
| `packages/coding-agent/test/custom-message.test.ts` exposes width, expanded state, and mutable output padding to custom renderers | `TestCLICustomMessageComponentProvidesOutputPaddingToRenderersAndUpdatesIt` | covered with mutex-owned presentation state |
| `packages/coding-agent/test/suite/regressions/6949-unavailable-scoped-model.test.ts` displays, removes, persists, and opens scopes containing unavailable models without clearing partial scopes | `TestScopedModelsSelectorShowsAndRemovesEnabledModelWithoutCatalogEntry`, `TestScopedModelsSelectorCarriesUnmatchedSettingsAndUnavailableSessionScope`, `TestScopedModelsSelectorDoesNotClearPartialScopeWhenEnabledModelIsUnavailable` | covered through canonical full model IDs and membership-based scope state |
| Manual compaction accepts provider-resolved header-only bearer authentication | `TestAgentSessionCompactManualWithProviderResolvedBearerAuth` | covered through `ModelRuntime.CompleteSimple` request assembly |
| Resource context discovery skips directory candidates and continues to the next supported filename | `TestDefaultResourceLoaderPiBasics/ignores_context_file_candidates_that_are_directories` | covered through regular-file checks before reads |
| `packages/coding-agent/test/model-runtime-cloudflare-compat.test.ts` Cloudflare endpoint and header materialization through canonical runtime auth | `TestModelRuntimeUsesProviderOwnedAuthProjectionPiStyle` | covered without a compatibility-only request bypass |
| `packages/coding-agent/test/model-runtime-modify-models-compat.test.ts` native providers, topmost user overrides, extension refresh publication, and OAuth model projection | `TestModelRuntimeNativeProviderLifecyclePiStyle`, `TestModelRuntimeComposesRemoteModelsConfigAndExtensionLayers`, `TestModelRuntimeExtensionRefreshValidatesBeforePublish`, `TestModelRuntimeOAuthModelProjectionUsesRefreshCredential` | covered with synchronized provider-owned model state |
| `packages/coding-agent/test/model-runtime-auth-options.test.ts` injected `CredentialStore`, provider-owned auth methods/status, scoped availability, extension API-key/OAuth method construction, request-scoped env, and final header assembly | `TestModelRuntimeAcceptsPiAICredentialStore`, `TestModelRuntimeDefaultCredentialStoreSurvivesLoginRefresh`, `TestModelRuntimeOwnsProviderRequestAssemblyPiStyle`, `TestModelRuntimeUsesProviderOwnedAuthProjectionPiStyle`, `TestModelRuntimeAvailabilityRefreshHonorsContext`, and focused provider-composer tests | covered through one runtime-owned credential overlay and provider runtime |
| `packages/coding-agent/test/radius.test.ts` legacy/offline restore, authenticated refresh/persistence, network opt-in, auth isolation, and custom `models.json` gateways | `TestCodingAgentPiRadiusExactCaseNames` | covered |
| `packages/coding-agent/test/runtime-credentials.test.ts` runtime-key read/list overlay, persistent mutation delegation, and dual-layer delete | `TestAuthStorageCredentialStoreRuntimeOverlay` | covered through the Go-native consolidated `AuthStorage` |
| `packages/coding-agent/test/resolve-config-value.test.ts` literals, environment templates, scoped overrides, escapes, command success/failure caching, uncached execution, and cross-platform shell selection | `TestResolveConfigValue*`, `TestAuthStorageCredentialStoreResolvesStoredAPIKey`, existing model-registry request-time tests | covered; Gi retains bare environment-variable names as a backward-compatible extension |
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
| TUI v0.82 word navigation preserves ASCII punctuation boundaries, Unicode/CJK movement, whitespace runs, and atomic paste markers | `TestFindWordBackwardPiMatrix`, `TestFindWordForwardPiMatrix`, `TestFindWordNavigationTreatsCustomAtomicSegmentsAsUnits`, plus shared `Input`/`Editor` integration tests | covered |
| TUI v0.82 terminal input negotiates Kitty flags through a DA sentinel, buffers split replies, falls back to modifyOtherKeys, and normalizes Apple Terminal Shift+Enter | `TestParseKeyboardProtocolNegotiationSequence`, `TestProcessTerminalReassemblesSplitKeyboardNegotiation`, `TestProcessTerminalFallsBackImmediatelyForZeroKittyFlags`, `TestNormalizeAppleTerminalInput` | covered |
| TUI v0.82 terminal color parsing and queries consume OSC 11 / color-scheme reports before normal input, including invalid and late FIFO replies | `TestParseOSC11BackgroundColorPiMatrix`, `TestTUIQueryTerminalBackgroundColorUsesFIFOInputBoundary`, `TestTUIBackgroundQueryLeavesUnrelatedInputAndConsumesLateReply`, `TestTUIQueryTerminalColorSchemeAndNotifications` | covered |
| TUI v0.82 overlay focus restoration lets active base replacements finish, follows mounted replacement focus, supports explicit unfocus targets, and safely terminates cyclic ancestry | `TestTUIOverlayPiFocusRestoreStateMachine` | covered |
| TUI v0.82 multi-row Kitty images reserve and pre-clear their physical rows, expand differential ranges, delete stale placements, and use a full redraw when pre-clear would scroll | `TestParseKittyImageHeaderAndRows`, `TestTUIGetKittyImageReservedRows`, `TestTUIDiffRenderPreclearsKittyReservedRows`, `TestTUIFallsBackToFullRedrawWhenKittyPreclearWouldScroll` | covered |
| TUI v0.82 excludes CJK graphemes crossing overlay boundaries and expands visible tabs without changing tabs embedded in terminal control strings | `TestExtractSegmentsExcludesWideGraphemeCrossingOverlayBoundary`, `TestCompositeLineExcludesWideGraphemeAtOverlayBoundary`, `TestTabWidthAccountingMatchesSlicesAndOverlaySegments`, `TestTUITabContainingOverlayStaysOnOnePhysicalRow` | covered |
| Coding-agent request header transforms see the fully assembled auth/provider/model/explicit header set exactly once and are not forwarded downstream | `gi-llm-provider/models_runtime_test.go` `adds model headers only for model auth and transforms assembled headers once` | covered through the shared Go model runtime |
| Inline extension factories receive stable `<inline:N>` / `<inline:name>` identities and preserve hidden presentation state | `TestInlineExtensionNamingPiRegression` | covered through canonical `ProtocolExtensionFactory` to `ProtocolExtensionSource` normalization |
| Resource-loader reload clears descriptor cache before rebuilding the extension runtime | `TestDefaultResourceLoaderPiBasics/clears_the_cache_on_resource_loader_reload` | covered |
| OAuth prompt/manual submissions remain immutable when the next prompt becomes active | `TestLoginDialogComponentKeepsSubmittedInputStable` exact subtests | covered through append-only dialog presentation state |
| Re-entrant interactive shutdown is a no-op and runtime disposal executes once | `TestCLIInteractiveSignalShutdownExtensionCleanupPiRegression/re-entrant_shutdown_is_a_no-op` | covered through `sync.Once` stop state and one `RunContext` cleanup owner |
| Async reload keeps the blocker focused until completion | `TestCLIInteractiveTUIHostShowsReloadBoxWhileReloadingPiStyle` | covered |
| Fenced diff blocks and Pi default highlight scopes use the shared CLI theme | `TestSyntaxHighlightRenderer` diff and default-scope subtests | covered |
| `packages/coding-agent/test/sdk-openrouter-attribution.test.ts` OpenRouter, NVIDIA NIM, Cloudflare, and OpenCode session attribution with telemetry and override precedence | `TestSDKAttribution*` in `sdk_attribution_test.go`, `TestAgentSessionPrintModeProviderHeadersIncludeSessionState` | covered with Gi product-specific attribution values |
| `packages/coding-agent/test/http-dispatcher.test.ts` applies `httpProxy` to both uppercase proxy variables, preserves existing variables, and ignores blank settings | `TestApplyHTTPProxySettingsPiContract` exact subcases in `gi-coding-agent/http_runtime_test.go` | covered; environment projection is limited to CLI/application composition while provider transports remain explicitly owned |
| `packages/coding-agent/test/sdk-stream-options.test.ts` forwards `httpIdleTimeoutMs` and presence-aware `websocketConnectTimeoutMs`, including explicit zero | `TestSDKStreamOptionsForwardsWebSocketConnectTimeoutFromSettings`, `TestProviderRequestSettingsSnapshotAndPrecedence`, `TestAgentSessionPrintModeProviderResponderUsesProviderRetrySettingsPiStyle` | covered through the immutable provider-request snapshot |
| Experimental startup gating, first-time theme/analytics setup, stable tracking IDs, terminal theme detection, automatic setting resolution, interactive theme ownership, transactional single/automatic theme configuration, reserved custom names, and max-thinking parsing/settings/model/theme flow | `TestAreExperimentalFeaturesEnabledPiCases`, `TestShouldRunFirstTimeSetupPiCases`, `TestSettingsManagerAnalyticsPiCases`, `TestFirstTimeSetupComponentOwnsOneStateProjection`, `TestShowFirstTimeSetupPersistsSubmittedState`, `TestParseAutoThemeSettingMatchesPi`, `TestResolveThemeSettingMatchesPi`, `TestDetectTerminalBackgroundFromEnvPiCases`, `TestDetectTerminalBackgroundThemePiCases`, `TestDetectTerminalThemeForAutoUsesColorSchemeFirst`, `TestTerminalThemeLuminanceHelpersMatchPi`, `TestSettingsManagerSeparatesRawAndFixedThemeSettings`, `TestInteractiveThemeControllerAutomaticStateFlow`, `TestInteractiveThemeControllerFallsBackAndReportsErrors`, `TestInteractiveThemeControllerPersistsHighConfidenceDetection`, `TestInteractiveThemeControllerPreviewKeepsCommittedState`, `TestInteractiveThemeControllerInMemoryThemeDisablesAutomaticSync`, `TestInteractiveThemeControllerSupersedesInFlightDetection`, `TestInteractiveThemeControllerDisposeStopsTerminalUpdates`, `TestSettingsThemeSelectionMatchesPiStateRules`, `TestCLIThemeSubmenuAppliesFixedTheme`, `TestCLIThemeSubmenuConfiguresAndAppliesAutomaticPair`, `TestCLIThemeSubmenuSwitchesToActiveAutomaticThemeAndCancels`, `TestSettingsThemeChangeFlowsThroughController`, `TestMaxThinkingLevelIsAcceptedByCLIAndSettings`, `TestMaxThinkingLevelFallsBackToThinkingXHighForLegacyThemes`, `TestTUIThemeMaxThinkingColorMatchesPiAndFallsBackForLegacyThemes`, `TestTUIThemeRejectsSlashNamesReservedForAutomaticSettings`, `TestResolvedThemeCSSColorsFallsBackFromThinkingMaxToXhigh`, `TestThemeExportRejectsSlashNameReservedForAutomaticSettings`, `TestTUIThemeUsesTerminalCapabilities` | covered through a canonical typed thinking enum, provider capability clamping, immutable eligibility/settings projections, transactional selector state, one interactive theme owner, shared theme validation/fallback helpers, and revisioned serialized palette transitions |
| `packages/coding-agent/test/external-editor.test.ts` private workspace, failed-edit retention, empty edits, plus configured/environment/platform command precedence | `TestEditInExternalEditorPiCases`, `TestSettingsManagerExternalEditorPiCases`, `TestCLIInteractiveTUIHostCtrlGUsesExternalEditorPiStyle`, `TestCLIInteractiveTUIHostCtrlGReportsExternalEditorFailurePiStyle`, `TestCLIEditorDialogUsesEffectiveExternalEditorKeybindingPiStyle` | covered through one process boundary shared by the main and extension editors |

## Ongoing Maintenance

- Keep the generated inventory and zero-gap snapshot synchronized whenever Pi
  or Gi changes.
- Keep product-scope exclusions explicit. The v0.82.1 coding-agent exclusions
  are Pi's optional TypeScript git-merge example and Node
  `proper-lockfile`/`signal-exit` listener re-send behavior; Gi uses protocol
  packages and Go `os/signal` plus `sync.Once`.
- Continue fixture-level checks for Markdown, virtual terminal, and live
  provider behavior, where textual case mapping is evidence but not a
  substitute for runtime validation.
