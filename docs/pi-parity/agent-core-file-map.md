# Agent Core File Map

This document tracks Pi `packages/agent/src` against Gi `gi-agent-core` and
`gi-agent-core/harness`. It is a working file/function parity audit, not a
completion claim.

## Status Legend

- `direct`: Gi has a same-purpose file/function implementation.
- `split`: Pi's file is represented by multiple Go files.
- `consolidated`: Pi's small file is folded into a larger Go file.
- `go-native`: Pi's Node/TS-specific surface is represented by Go-native code.
- `partial`: Gi has some equivalent behavior, but the file/function audit is not
  complete or a Pi feature is missing.
- `gap`: no current Gi equivalent.

## Root Source Files

| Pi file | Pi exported surface / major functions | Gi equivalent | Status | Notes |
| --- | --- | --- | --- | --- |
| `agent-loop.ts` | `agentLoop`, `agentLoopContinue`, `runAgentLoop`, `runAgentLoopContinue`, assistant stream event handling, tool execution, steering/follow-up turn scheduling | `agent_loop.go` `AgentLoop`, `AgentLoopContinue`, `RunAgentLoop`, `RunAgentLoopContinue`, stream/tool helpers | direct | Gi mirrors Pi's loop shape: validate continuation role, stream assistant events, execute tool calls, append tool results, consume steering before follow-up, and stop when no tool calls or queued work remain. |
| `agent.ts` | `Agent`, state, subscriptions, `prompt`, `steer`, `followUp`, queue handling | `agent.go` | direct | Gi preserves the same stateful agent boundary with Go channels/mutexes instead of TS promises. |
| `index.ts` | public package exports | Go package exports | consolidated | Go exports identifiers directly from `giagentcore` and `harness`; no barrel file is needed. |
| `node.ts` | Node execution environment re-export | `harness/env/local_env.go`, `harness/local_env.go` facade | go-native | Local filesystem/process execution is Go-native and does not need Node module re-export wiring. |
| `proxy.ts` | `streamProxy`, proxy event reconstruction, `/api/stream` fetch client | `proxy.go` `StreamProxy`, `NewProxyStreamFn`, `processProxyEvent` | direct | Gi now exposes the same server-managed-auth proxy stream pattern: POST to `/api/stream`, send only serializable stream options, reconstruct partial assistant messages from stripped proxy events, and emit Pi-shaped assistant message events. |
| `stream-fn.ts` | `setDefaultStreamFn`, `getDefaultStreamFn` | `stream_fn.go` `SetDefaultStreamFn`, `GetDefaultStreamFn`; `gi-coding-agent/sdk_stream_default.go` host registration | go-native | A mutex protects the process-wide fallback. Agent core returns an explicit Go error when no fallback is installed; the provider-aware coding-agent host installs `llm.StreamSimple`. |
| `types.ts` | agent event/message/tool/state/queue type contracts | `types.go`, `agent.go` structs | direct | Queue modes, tool execution mode, lifecycle events, agent state, and tool contracts are represented as Go structs/types. |

## Harness Source Files

| Pi file | Pi exported surface / major functions | Gi equivalent | Status | Notes |
| --- | --- | --- | --- | --- |
| `harness/agent-harness.ts` | `AgentHarness`, prompt/continue/fork/compact/session orchestration, per-turn stream options | `harness/agent_harness.go` | direct | Gi mirrors the harness-level orchestration with Go error returns and typed options. |
| `harness/compaction/branch-summarization.ts` | branch summary planning/generation helpers | `harness/branch_summary.go` | direct | Branch summary prompts, cut points, and summary message conversion are represented. |
| `harness/compaction/compaction.ts` | token estimation, compaction thresholding, cut point selection, summary generation, compacted session creation | `harness/compaction.go` | direct | Gi has the same responsibilities, including context token estimation and summary-message insertion. |
| `harness/compaction/utils.ts` | file operation extraction/formatting shared by summarizers | `harness/compaction.go`, `harness/branch_summary.go` | split | Shared helpers are folded into the Go compaction and branch summary files. |
| `harness/env/nodejs.ts` | local filesystem and shell execution environment | `harness/env/local_env.go`, `harness/local_env.go` facade | go-native | Gi implements local read/write/list/exec/capture directly in Go, including shell output sanitization. |
| `harness/messages.ts` | bash/custom/branch/compaction message text, context projection, `convertToLLM` | `harness/messages.go` | direct | Gi includes bash execution text, summary wrappers, custom-message projection, and LLM conversion for non-standard session roles. |
| `harness/prompt-templates.ts` | prompt template lookup/argument expansion | `harness/prompt_templates.go` | direct | Template invocation and placeholder formatting are represented. |
| `harness/session/jsonl-repo.ts` | JSONL session repository | `harness/session_repo.go`, `harness/session_storage.go` | direct | Gi splits repository and storage responsibilities across Go files. |
| `harness/session/jsonl-storage.ts` | JSONL storage implementation | `harness/session_storage.go` | direct | File-backed session storage is represented. |
| `harness/session/memory-repo.ts` | in-memory session repository | `harness/session_repo.go` | direct | In-memory repo behavior is represented for tests and embedded use. |
| `harness/session/memory-storage.ts` | in-memory storage implementation | `harness/session_storage.go` | direct | In-memory storage behavior is represented. |
| `harness/session/repo-utils.ts` | session IDs/timestamps, entry conversion, fork-entry selection | `harness/session.go`, `harness/session_repo.go`, `harness/uuid.go` | split | Go keeps these helpers next to session manager/repository logic. |
| `harness/session/session.ts` | session entry tree, append/fork/context construction | `harness/session.go` | direct | Session entries, parent links, context projection, labels, and thinking/model changes are represented. |
| `harness/session/uuid.ts` | time-sortable UUID/session ID helpers | `harness/uuid.go` | direct | UUID/session ID helpers are represented. |
| `harness/skills.ts` | skill parsing/loading/filtering | `harness/skills.go`, `harness/frontmatter.go` | direct | Skill metadata/frontmatter and model-visible filtering are represented. |
| `harness/system-prompt.ts` | system prompt formatting, skill/resource prompt blocks | `harness/format.go` | direct | Gi includes Pi-style skill formatting and prompt assembly helpers. |
| `harness/tools/bash.ts` | reusable bash tool, timeout validation, streaming updates, truncation, full-output persistence | `harness/tools/bash.go`, `harness/utils/shell_output.go` | direct | Command preparation is context-aware; updates are coalesced before materializing snapshots, and timeout/error paths retain captured output. |
| `harness/tools/edit-diff.ts` | fuzzy matching, atomic multi-edit application, line-ending preservation, display diff and unified patch generation | `harness/tools/edit_diff.go` | direct | Go matches every replacement against one immutable original snapshot, rejects duplicate or overlapping targets, then applies sorted ranges atomically. |
| `harness/tools/edit.ts` | edit argument preparation, validation, mutation serialization, and typed access errors | `harness/tools/edit.go` | direct | The tool executes through the per-turn execution environment and canonical mutation queue. |
| `harness/tools/file-mutation-queue.ts` | canonical-path mutation state and serialization | `harness/tools/mutation_queue.go` | direct | `FileMutationQueue` owns keyed locks and holds them until cancelled writes settle. |
| `harness/tools/image.ts` | supported image detection and base64 encoding | `harness/tools/image.go` | direct | Content signatures, animated PNG rejection, and BMP header validation are represented without extension-based guessing. |
| `harness/tools/index.ts` | public tool exports | `harness/tools` package exports | consolidated | Go exports package identifiers directly and does not need a barrel file. |
| `harness/tools/path-utils.ts` | tool path normalization and read-path recovery | `harness/tools/path.go` | direct | Go resolves paths through the injected execution environment and preserves Pi's Unicode-space and smart-apostrophe recovery variants. |
| `harness/tools/read.ts` | text/image reads, offsets, limits, truncation, and injected image processing | `harness/tools/read.go` | direct | One result contract carries either text plus truncation details or an image part plus optional processor note. |
| `harness/tools/tool-context.ts` | per-turn execution environment and mutation state | `harness/tool.go`, `harness/tools/context.go` | go-native | `AgentHarnessToolContextSource` snapshots context once per turn; `ExecutionToolContext` groups the environment with its file mutation queue. |
| `harness/tools/write.ts` | parent creation and serialized file writes | `harness/tools/write.go` | direct | Writes use the same execution context and canonical mutation queue as edits. |
| `harness/types.ts` | harness resources/options, filesystem/execution/session/compaction error types, TS `Result` helpers | `harness/types.go`, `harness/env/local_env.go`, `harness/local_env.go`, `harness/agent_harness.go` | go-native | Domain structs and stable error-code wrappers are represented: `FileError`, `ExecutionError`, `SessionError`, `CompactionError`, `BranchSummaryError`, and `AgentHarnessError`. TS-only `ok`/`err`/`getOrThrow` helpers map to Go's native `(value, error)` returns and are intentionally not exported. |
| `harness/utils/shell-output.ts` | binary/control-character sanitization and captured shell output | `harness/utils/shell_output.go` `SanitizeBinaryOutput`, `ExecuteShellWithCapture`; legacy local-env facade | direct | Gi incrementally decodes UTF-8 across process chunks, ignores callbacks after settlement, and applies Pi-style cleanup before bounded-tail/full-output capture. |
| `harness/utils/truncate.ts` | truncation helpers | `harness/truncate.go` | direct | Truncation behavior is represented and covered by Go tests. |

## Function-Level Checkpoints Completed In This Pass

- Pi `agentLoop` / `runAgentLoop` map to Gi `AgentLoop` / `RunAgentLoop`.
- Pi `agentLoopContinue` / `runAgentLoopContinue` map to Gi
  `AgentLoopContinue` / `RunAgentLoopContinue`, including the same "last
  message must not be assistant" continuation guard.
- Pi `message_start`, `message_update`, `message_end`, `tool_execution_*`,
  `turn_*`, and `agent_*` lifecycle event types are represented by Gi agent
  events and coding-agent runtime events.
- Pi `harness/messages.ts` non-standard session roles now have Gi equivalents:
  `bashExecution`, `custom`, `branchSummary`, and `compactionSummary` project
  into model context or UI text in `harness/messages.go`.
- Pi's Node `sanitizeBinaryOutput` behavior maps to Gi
  `SanitizeBinaryOutput`; captured local command output is sanitized before
  truncation or full-output spillover.

## Exported Symbol Audit

Every Pi exported function/type/constant in `packages/agent/src` is listed here
by name and mapped to a Go symbol, consolidated implementation, or intentional
Go-native difference.

### Root Agent And Loop Symbols

| Pi file | Pi symbols | Gi equivalent | Status | Notes |
| --- | --- | --- | --- | --- |
| `agent-loop.ts` | `AgentEventSink`, `agentLoop`, `agentLoopContinue`, `runAgentLoop`, `runAgentLoopContinue`, `failToolCallsFromTruncatedMessage` | `agent_loop.go` `AgentEventSink`, `AgentLoop`, `AgentLoopContinue`, `RunAgentLoop`, `RunAgentLoopContinue`, `failToolCallsFromTruncatedMessage` | direct | Go-style capitalization and explicit context/error returns; output-length truncation produces error tool results without executing possibly incomplete arguments. |
| `agent.ts` | `Agent`, `AgentOptions` | `agent.go` `Agent`, `AgentOptions`, `AgentOption` helpers | direct | Stateful agent boundary and queue handling are represented. |
| `stream-fn.ts` | `setDefaultStreamFn`, `getDefaultStreamFn` | `stream_fn.go` `SetDefaultStreamFn`, `GetDefaultStreamFn` | go-native | Go returns `(StreamFn, error)` instead of throwing when the fallback is absent. |
| `types.ts` | `AgentMessage`, `AgentToolCall`, `StreamFn`, `AgentTool`, `AgentToolResult`, `AgentToolUpdateCallback` | `types.go` same names or aliases | direct | LLM message/content aliases preserve the contract. |
| `types.ts` | `BeforeToolCallContext`, `BeforeToolCallResult`, `AfterToolCallContext`, `AfterToolCallResult`, `ShouldStopAfterTurnContext`, `PrepareNextTurnContext` | `types.go` same names | direct | Tool hook and turn hook contexts are represented. |
| `types.ts` | `AgentLoopConfig`, `AgentLoopTurnUpdate`, `AgentContext`, `AgentEvent`, `AgentState`, `QueueMode`, `ThinkingLevel`, `ToolExecutionMode`, `CustomAgentMessages` | `types.go` and `agent.go` structs/constants | direct | TS literal unions map to Go string constants and structs. |

### Harness Agent, Event, And Resource Symbols

| Pi file | Pi symbols | Gi equivalent | Status | Notes |
| --- | --- | --- | --- | --- |
| `harness/agent-harness.ts` | `AgentHarness` | `harness/agent_harness.go` `AgentHarness` | direct | Prompt/continue/fork/compact/navigation orchestration is represented. |
| `harness/types.ts` | `AgentHarnessOptions`, `AgentHarnessResources`, `AgentHarnessStreamOptions`, `AgentHarnessStreamOptionsPatch`, `AgentHarnessEvent`, `AgentHarnessOwnEvent`, `AgentHarnessEventResultMap`, `AgentHarnessPhase`, `AgentHarnessPromptOptions` | `harness/agent_harness.go` same purpose structs/constants | direct | Pi event/result maps are represented by typed Go events and hook results. |
| `harness/types.ts` | `BeforeAgentStartEvent`, `BeforeAgentStartResult`, `BeforeProviderPayloadEvent`, `BeforeProviderPayloadResult`, `BeforeProviderRequestEvent`, `BeforeProviderRequestResult`, `AfterProviderResponseEvent` | `harness/agent_harness.go` `AgentHarnessEvent`, `AgentHarnessHookResult`, stream option/payload/status fields | consolidated | Go folds hook-specific event/result interfaces into one event/result struct. |
| `harness/types.ts` | `AbortEvent`, `AbortResult`, `ContextEvent`, `ContextResult`, `QueueUpdateEvent`, `ResourcesUpdateEvent`, `ModelSelectEvent`, `ThinkingLevelSelectEvent`, `SavePointEvent`, `SettledEvent` | `harness/agent_harness.go` `AgentHarnessEvent` fields and phase constants | consolidated | Event names are preserved as `Type` values; event payloads are typed fields. |
| `harness/types.ts` | `ToolCallEvent`, `ToolResultEvent`, `ToolResultPatch`, `ToolCallResult` | `harness/agent_harness.go` `AgentHarnessEvent` tool fields, `AgentHarnessHookResult`; core `AgentToolResult` | split | Tool call/result hook payloads span harness and core. |
| `harness/types.ts` | `SessionBeforeCompactEvent`, `SessionBeforeCompactResult`, `SessionCompactEvent`, `CompactResult`, `SessionBeforeTreeEvent`, `SessionBeforeTreeResult`, `SessionTreeEvent`, `TreePreparation`, `NavigateTreeResult` | `harness/agent_harness.go` compaction/tree fields, `NavigateTreeResult`; compaction structs | direct | Session lifecycle hooks are represented. |

### Harness Session And Storage Symbols

| Pi file | Pi symbols | Gi equivalent | Status | Notes |
| --- | --- | --- | --- | --- |
| `harness/session/session.ts` | `Session`, `deriveSessionContextState`, `defaultContextEntryTransform`, `buildContextEntries`, `sessionEntryToContextMessages`, `buildSessionContext` | `harness/session.go` `Session`, state derivation, `DefaultContextEntryTransform`, `BuildContextEntries`, `SessionEntryToContextMessages`, `BuildSessionContext` | direct | One append-only `Entry` representation feeds independently composable state derivation, entry transforms, custom projectors, and message projection. |
| `harness/session/repo-utils.ts` | `createSessionId`, `createTimestamp`, `getEntriesToFork`, `getFileSystemResultOrThrow`, `toSession` | `harness/uuid.go` `UUIDv7`, `session.go`/`session_repo.go` helpers | consolidated | ID/timestamp/session conversion helpers are implemented next to repository/session logic; Go uses errors instead of `Result`. |
| `harness/session/uuid.ts` | `uuidv7` | `harness/uuid.go` `UUIDv7`, `UUIDv7With` | direct | Time-sortable UUID behavior is represented. |
| `harness/session/jsonl-repo.ts` | `JsonlSessionRepo` | `harness/session_repo.go` `JsonlSessionRepo`, `NewJsonlSessionRepo` | direct | JSONL repository is represented. |
| `harness/session/memory-repo.ts` | `InMemorySessionRepo` | `harness/session_repo.go` `InMemorySessionRepo`, `NewInMemorySessionRepo` | direct | In-memory repository is represented. |
| `harness/session/jsonl-storage.ts` | `JsonlSessionStorage`, `loadJsonlSessionMetadata` | `harness/session_storage.go` `JsonlSessionStorage`, `LoadJsonlSessionMetadata` | direct | Go-style capitalization. |
| `harness/session/memory-storage.ts` | `InMemorySessionStorage` | `harness/session_storage.go` `InMemorySessionStorage`, `NewInMemorySessionStorage` | direct | In-memory storage is represented. |
| `harness/types.ts` | `SessionMetadata`, `SessionContext`, `SessionCreateOptions`, `SessionForkOptions`, `SessionRepo`, `SessionStorage`, `JsonlSessionRepoApi`, `JsonlSessionCreateOptions`, `JsonlSessionListOptions`, `JsonlSessionMetadata`, `PendingSessionWrite` | `harness/types.go`, `session.go`, `session_repo.go`, `session_storage.go` | split | Go storage/repo interfaces and metadata structs preserve the same ownership boundary. |
| `harness/types.ts` | `SessionTreeEntry`, `SessionTreeEntryBase`, `LeafEntry`, `MessageEntry`, `ModelChangeEntry`, `ThinkingLevelChangeEntry`, `ActiveToolsChangeEntry`, `CompactionEntry`, `BranchSummaryEntry`, `CustomEntry`, `CustomMessageEntry`, `LabelEntry`, `SessionInfoEntry`, `SessionStats`, `SessionEntryCursorOptions` | `harness/types.go` `Entry`, `SessionStats`, `SessionEntryCursorOptions` | consolidated | Pi discriminated entry interfaces map to one JSON-compatible Go `Entry` struct; active-tool state, retained compaction tails, custom data, stats, and cursor fields remain explicit. |

### Harness Compaction And Branch Summary Symbols

| Pi file | Pi symbols | Gi equivalent | Status | Notes |
| --- | --- | --- | --- | --- |
| `harness/compaction/compaction.ts` | `CompactionSettings`, `DEFAULT_COMPACTION_SETTINGS`, `ContextUsageEstimate`, `CutPointResult`, `CompactionPreparation`, `CompactionDetails`, `CompactionResult` | `harness/compaction.go` `CompactionSettings`, `DefaultCompactionSettings`, `ContextTokenEstimate`, `CutPoint`, `CompactionPreparation`, file-op details, `CompactionResult` | direct | Naming differs slightly where Go uses shorter struct names. |
| `harness/compaction/compaction.ts` | `calculateContextTokens`, `estimateContextTokens`, `estimateTokens`, `findCutPoint`, `findTurnStartIndex`, `getLastAssistantUsage`, `prepareCompaction`, `shouldCompact`, `completeSimpleWithRetries`, `combineUsage`, `generateSummary`, `generateSummaryWithUsage`, `compact` | `CalculateContextTokens`, `EstimateContextTokens`, `EstimateTokens`, `FindCutPoint`, `FindTurnStartIndex`, `GetLastAssistantUsage`, `PrepareCompaction`, `ShouldCompact`, `CompleteSimpleWithRetries`, `combineUsage`, `GenerateSummary`, `GenerateSummaryWithUsage`, `Compact` / `CompactWithOptions` | direct | Summary calls use one isolated request snapshot, disable unusable cache retention, share bounded retry callbacks, and preserve or combine provider usage. |
| `harness/compaction/compaction.ts` | `SUMMARIZATION_SYSTEM_PROMPT` | `harness/compaction.go` summarization system prompt constant | direct | Prompt text is Pi-aligned in compaction tests. |
| `harness/compaction/branch-summarization.ts` | `CollectEntriesResult`, `BranchPreparation`, `GenerateBranchSummaryOptions`, `BranchSummaryDetails`, `BranchSummaryResult`, `collectEntriesForBranchSummary`, `prepareBranchEntries`, `generateBranchSummary` | `harness/branch_summary.go` `CollectEntriesResult`, `BranchPreparation`, `BranchSummaryOptions`, `BranchSummaryResult`, `CollectEntriesForBranchSummary`, `PrepareBranchEntries`, `GenerateBranchSummary` | direct | `BranchSummaryDetails` fields are represented inside `BranchSummaryResult.Details`. |
| `harness/compaction/utils.ts` | `FileOperations`, `createFileOps`, `extractFileOpsFromMessage`, `computeFileLists`, `formatFileOperations`, `serializeConversation` | `harness/compaction.go` `FileOps`, `SerializeConversation`, file-operation helpers | direct | Helper functions are internal where only compaction/branch summary need them. |

### Harness Resources, Messages, Env, And Utility Symbols

| Pi file | Pi symbols | Gi equivalent | Status | Notes |
| --- | --- | --- | --- | --- |
| `harness/messages.ts` | `BRANCH_SUMMARY_PREFIX`, `BRANCH_SUMMARY_SUFFIX`, `COMPACTION_SUMMARY_PREFIX`, `COMPACTION_SUMMARY_SUFFIX` | `harness/messages.go` `BranchSummaryPrefix`, `BranchSummarySuffix`, `CompactionSummaryPrefix`, `CompactionSummarySuffix` | direct | Go-style constant names. |
| `harness/messages.ts` | `BashExecutionMessage`, `BranchSummaryMessage`, `CompactionSummaryMessage`, `CustomMessage`, `bashExecutionToText`, `convertToLlm`, `createBranchSummaryMessage`, `createCompactionSummaryMessage`, `createCustomMessage` | `harness/messages.go` `BashExecutionText`, `ConvertToLLM`, summary/custom message conversion helpers | direct | TS message interfaces map to `llm.Message` plus typed custom roles/details. |
| `harness/prompt-templates.ts` | `PromptTemplate`, `PromptTemplateDiagnostic`, `PromptTemplateDiagnosticCode`, `formatPromptTemplateInvocation`, `loadPromptTemplates`, `loadSourcedPromptTemplates`, `parseCommandArgs`, `substituteArgs` | `harness/types.go` `PromptTemplate`, `harness/prompt_templates.go` diagnostics/loaders plus coding-agent prompt-template expansion helpers | split | Loading lives in core harness; command arg parsing/substitution is shared with coding-agent where CLI command semantics live. |
| `harness/skills.ts` | `SkillDiagnostic`, `SkillDiagnosticCode`, `formatSkillInvocation`, `loadSkills`, `loadSourcedSkills` | `harness/skills.go` `SkillDiagnostic`, `LoadSkills`, `LoadSourcedSkills`; `harness/format.go` `FormatSkillInvocation` | direct | Diagnostic codes are strings in Go. |
| `harness/system-prompt.ts` | `formatSkillsForSystemPrompt` | `harness/format.go` `FormatSkillsForSystemPrompt` | direct | Go-style capitalization. |
| `harness/env/nodejs.ts` | `NodeExecutionEnv` | `harness/env/local_env.go`, `harness/local_env.go` `LocalExecutionEnv`, `ExecutionEnv`, `FileSystem`, `Shell` facade | go-native | Node-specific class maps to Go-native local environment. |
| `harness/types.ts` | `ExecutionEnv`, `ExecutionEnvExecOptions`, `FileSystem`, `FileInfo`, `FileKind`, `FileOperations`, `Shell` | `harness/env/local_env.go` with `harness/local_env.go` same-purpose interfaces/struct facades | direct | File and execution environment contracts are represented. |
| `harness/utils/shell-output.ts` | `ShellCaptureOptions`, `ShellCaptureResult`, `executeShellWithCapture`, `sanitizeBinaryOutput` | `harness/env/local_env.go` `CapturedShellResult`, `ExecuteShellWithCapture`, `SanitizeBinaryOutput`, root facade | direct | Go uses an explicit inline byte limit argument instead of an options object. |
| `harness/utils/truncate.ts` | `DEFAULT_MAX_BYTES`, `DEFAULT_MAX_LINES`, `GREP_MAX_LINE_LENGTH`, `TruncationOptions`, `TruncationResult`, `formatSize`, `truncateHead`, `truncateTail`, `truncateLine` | `harness/truncate.go` `DefaultMaxBytes`, `DefaultMaxLines`, `GrepMaxLineLength`, `TruncationOptions`, `TruncationResult`, `FormatSize`, `TruncateHead`, `TruncateTail`, `TruncateLine` | direct | Go-style capitalization. |
| `harness/types.ts` | `AbortResult`, `AgentHarnessError`, `AgentHarnessErrorCode`, `BranchSummaryError`, `BranchSummaryErrorCode`, `CompactionError`, `CompactionErrorCode`, `ExecutionError`, `ExecutionErrorCode`, `FileError`, `FileErrorCode`, `SessionError`, `SessionErrorCode`, `Result`, `ok`, `err`, `getOrThrow`, `getOrUndefined`, `toError` | typed Go errors plus native `(value, error)` returns | go-native | Error code fields are preserved; TS `Result` helper functions are intentionally not ported. |

### Harness Tool Symbols

| Pi file | Pi symbols | Gi equivalent | Status | Notes |
| --- | --- | --- | --- | --- |
| `harness/tools/bash.ts` | `validateTimeout`, `createBashTool` | `harness/tools/bash.go` `validateBashTimeout`, `CreateBashTool` | direct | Timeout validation rejects non-finite, non-positive, and oversized values before execution. |
| `harness/tools/edit-diff.ts` | `detectLineEnding`, `normalizeToLF`, `restoreLineEndings`, `normalizeForFuzzyMatch`, `splitLinesWithEndings`, `getLineSpans`, `getReplacementLineRange`, `applyReplacements`, `applyReplacementsPreservingUnchangedLines`, `fuzzyFindText`, `stripBom`, `countOccurrences`, `getNotFoundError`, `getDuplicateError`, `getEmptyOldTextError`, `getNoChangeError`, `applyEditsToNormalizedContent`, `generateUnifiedPatch`, `generateDiffString` | `harness/tools/edit_diff.go` exported normalization/application/diff functions plus focused internal range, occurrence, and error helpers | consolidated | Go consolidates Pi's intermediate line-span helpers behind the atomic `ApplyEditsToNormalizedContent` boundary. |
| `harness/tools/edit.ts` | `prepareEditArguments`, `validateEditInput`, `editAccessError`, `createEditTool` | `harness/tools/edit.go` `PrepareEditArguments`, input parsing/validation, `editAccessError`, `CreateEditTool` | direct | Legacy single-edit arguments and the v0.82 multi-edit array converge on one validated representation. |
| `harness/tools/file-mutation-queue.ts` | `getState`, `getMutationQueueKey`, `withFileMutationQueue` | `harness/tools/mutation_queue.go` queue lookup, `mutationQueueKey`, `FileMutationQueue.With` | consolidated | State is encapsulated in a Go owner type instead of module-global weak maps. |
| `harness/tools/image.ts` | `detectSupportedImageMimeType`, `encodeBase64`, `isPng`, `isAnimatedPng`, `isBmp`, `readUint16LE`, `readUint32BE`, `readUint32LE`, `startsWith`, `startsWithAscii` | `harness/tools/image.go` `DetectSupportedImageMIMEType`, `EncodeBase64`, `isPNG`, `isAnimatedPNG`, `isBMP`, and `encoding/binary`/`bytes` helpers | consolidated | Standard-library byte readers replace one-off integer and prefix helpers. |
| `harness/tools/path-utils.ts` | `normalizeToolPath`, `resolveToolPath`, `resolveReadToolPath` | `harness/tools/path.go` `NormalizeToolPath`, `ResolveToolPath`, `ResolveReadToolPath` | direct | Paths remain environment-relative and testable. |
| `harness/tools/read.ts` | `createReadTool` | `harness/tools/read.go` `CreateReadTool` | direct | Text and image result variants share `AgentToolResult`. |
| `harness/tools/write.ts` | `createWriteTool` | `harness/tools/write.go` `CreateWriteTool` | direct | Parent creation and write settlement are serialized through `FileMutationQueue`. |
| `harness/agent-harness.ts` | `findDuplicateNames` | `harness/agent_harness.go` `findDuplicateNames` | direct | Active-tool validation preserves first-duplicate order, reports each duplicate once, and leaves harness state unchanged on failure. |

### Proxy Symbols

| Pi file | Pi symbols | Gi equivalent | Status | Notes |
| --- | --- | --- | --- | --- |
| `proxy.ts` | `ProxyAssistantMessageEvent`, `ProxyStreamOptions`, `streamProxy` | `proxy.go` `ProxyAssistantMessageEvent`, `ProxyStreamOptions`, `StreamProxy`, `NewProxyStreamFn` | direct | Proxy event reconstruction and `/api/stream` client contract are represented. |

## Internal Top-Level Implementation Checkpoints

The exported-symbol audit above proves public ownership. This section tracks
Pi's non-exported top-level helper functions/classes in `packages/agent/src` so
the implementation audit also covers private behavior.

| Pi file | Pi internal helpers | Gi equivalent | Status | Notes |
| --- | --- | --- | --- | --- |
| `agent-loop.ts` | `createAgentStream`, `runLoop`, `streamAssistantResponse`, `executeToolCalls`, `executeToolCallsSequential`, `executeToolCallsParallel`, `shouldTerminateToolBatch`, `prepareToolCallArguments`, `prepareToolCall`, `executePreparedToolCall`, `finalizeExecutedToolCall`, `createErrorToolResult`, `emitToolExecutionEnd`, `createToolResultMessage`, `emitToolResultMessage` | `agent_loop.go` `llm.NewEventStream` construction in `AgentLoop`/`AgentLoopContinue`, `runLoop`, `streamAssistantResponse`, `executeToolCalls`, `executeToolCallsSequential`, `executeToolCallsParallel`, `shouldTerminateToolBatch`, `prepareToolCall`, `executePreparedToolCall`, `finalizeExecutedToolCall`, `createErrorToolResult`, `emitToolExecutionEnd`, `createToolResultMessage`, `emitToolResultMessage` | direct | Go keeps the same helper split for turn scheduling, streaming partial/final assistant messages, sequential/parallel tool execution, hook handling, and tool-result message emission. `prepareToolCallArguments` is folded into `prepareToolCall` plus provider-level `ValidateToolArguments`. |
| `agent.ts` | `defaultConvertToLlm`, `createMutableAgentState`, `PendingMessageQueue` | `agent_loop.go` `defaultConvertToLLM`, `agent.go` state-copy methods and `pendingMessageQueue` | direct | Go uses mutex-protected state snapshots and a small queue type instead of TS mutable state helpers. |
| `harness/agent-harness.ts` | `createUserMessage`, `createFailureMessage`, `cloneStreamOptions`, `mergeHeaders`, `applyStreamOptionsPatch`, `normalizeHarnessError`, `normalizeHookError` | `harness/agent_harness.go` prompt/failure message helpers, stream-option cloning/patching, header merge, harness/hook error normalization | direct | Harness prompt construction, hook mutation, and typed error wrapping are represented in Go with explicit `(value, error)` returns. |
| `harness/compaction/branch-summarization.ts` | `getMessageFromEntry` | `harness/branch_summary.go`, `harness/messages.go` entry-to-message projection | direct | Branch-summary collection projects session entries through Go session/message helpers. |
| `harness/compaction/compaction.ts` | `safeJsonStringify`, `extractFileOperations`, `getMessageFromEntry`, `getMessageFromEntryForCompaction`, `getAssistantUsage`, `getLastAssistantUsageInfo`, `findValidCutPoints`, `estimateTextAndImageContentChars`, `generateTurnPrefixSummary` | `harness/compaction.go`, `harness/messages.go` file-operation extraction, message projection, assistant usage lookup, cut-point discovery, content-size estimation, turn-prefix summary generation | direct | Gi preserves Pi's compaction responsibilities while folding shared helpers into the Go compaction file; text uses UTF-16-compatible character counts and each image contributes the same 4,800-character estimate. |
| `harness/compaction/utils.ts` | `safeJsonStringify`, `truncateForSummary` | `harness/compaction.go`, `harness/branch_summary.go` summary serialization and truncation helpers | direct | Shared compaction utility behavior is consolidated where Go callers need it. |
| `harness/env/nodejs.ts` | `resolveTimeoutMs`, `resolvePath`, `fileKindFromStats`, `fileInfoFromStats`, `isNodeError`, `toFileError`, `abortResult`, `pathExists`, `runCommand`, `findBashOnPath`, `isLegacyWslBashPath`, `getBashShellConfig`, `getShellConfig`, `getShellEnv`, `killProcessTree`, `waitForChildProcess` | `harness/env/local_env.go` timeout normalization, path/file conversion, typed file/exec errors, context abort handling, shell configuration, and bounded child waiting | go-native | Go uses `time.Duration`, `exec.Cmd.WaitDelay`, and `context.Context`; legacy WSL `bash.exe` paths still switch from `-c argv` to `-s` stdin transport, and cwd existence is checked before spawn. |
| `harness/prompt-templates.ts` | `loadTemplatesFromDir`, `loadTemplateFromFile`, `resolveKind`, `parseFrontmatter`, `basenameEnvPath` | `harness/prompt_templates.go`, `harness/frontmatter.go` directory/file loaders, frontmatter parsing, path helpers | direct | Template kind/path logic is folded into Go loaders and shared frontmatter helpers. |
| `harness/session/jsonl-repo.ts` | `encodeCwd` | `harness/session_repo.go` cwd-scoped session path encoding | direct | JSONL repo cwd scoping is represented in Go repository helpers. |
| `harness/session/jsonl-storage.ts` | `updateLabelCache`, `buildLabelsById`, `generateEntryId`, `isRecord`, `invalidSession`, `invalidEntry`, `parseHeaderLine`, `parseEntryLine`, `leafIdAfterEntry`, `headerToSessionMetadata`, `loadJsonlStorage` | `harness/session_storage.go` JSONL load/save helpers, label cache, entry ID generation, header/entry parsing, leaf tracking, metadata conversion | direct | File-backed storage behavior is represented with typed Go parse and cache helpers. |
| `harness/session/memory-storage.ts` | `updateLabelCache`, `buildLabelsById`, `generateEntryId`, `leafIdAfterEntry` | `harness/session_storage.go` in-memory storage helpers | direct | The in-memory storage mirrors the same label/leaf/ID semantics as JSONL storage. |
| `harness/session/uuid.ts` | `fillRandomBytes`, `formatUuid` | `harness/uuid.go` random byte filling and UUIDv7 formatting helpers | direct | Time-sortable UUID behavior is represented in Go. |
| `harness/skills.ts` | `loadSkillsFromDirInternal`, `addIgnoreRules`, `prefixIgnorePattern`, `loadSkillFromFile`, `validateName`, `validateDescription`, `parseFrontmatter`, `resolveKind`, `joinEnvPath`, `dirnameEnvPath`, `basenameEnvPath`, `relativeEnvPath` | `harness/skills.go`, `harness/frontmatter.go` skill discovery, ignore handling, validation, frontmatter parsing, and path helpers | direct | Skill loading and diagnostics are represented with Go path operations and frontmatter parsing. |
| `harness/system-prompt.ts` | `escapeXml` | `harness/format.go` XML escaping for system-prompt resource blocks | direct | Prompt formatting preserves Pi's XML-safe skill/resource rendering. |
| `harness/utils/shell-output.ts` | `toExecutionError`, `trimToLastUtf8Bytes` | `harness/utils/shell_output.go` typed `ExecutionError` normalization and UTF-8-aware `trimTail` | direct | Shell capture errors are normalized into Go typed errors; the bounded tail starts only at complete UTF-8 runes. |
| `harness/utils/truncate.ts` | `utf8ByteLength`, `splitLinesForCounting`, `replaceUnpairedSurrogates`, `truncateStringToBytesFromEnd` | `harness/utils/truncate.go` UTF-8 byte accounting, line counting, and truncation helpers | direct | Go strings are valid UTF-8; trailing newlines do not create phantom counted lines, and byte-limited head/tail truncation preserves valid string boundaries. |
| `proxy.ts` | `ProxyMessageEventStream`, `buildProxyRequestOptions` | `proxy.go` `llm.AssistantMessageEventStream` reconstruction and `ProxyStreamOptions` JSON request construction | direct | Gi preserves Pi's server-managed-auth proxy request shape and stream reconstruction while using Go stream types. |

## Internal Member-Level Checkpoints

These entries are tracked by
`docs/pi-parity/verify-source-map.mjs --scope members` so class and instance
methods are not skipped by the file/top-level audit.

| Pi file | Pi member symbols | Gi equivalent | Status | Notes |
| --- | --- | --- | --- | --- |
| `agent.ts` | `PendingMessageQueue.constructor`, `PendingMessageQueue.enqueue`, `PendingMessageQueue.hasItems`, `PendingMessageQueue.drain`, `PendingMessageQueue.clear` | `agent.go` `pendingMessageQueue` and queue methods | direct | Go keeps the same one-at-a-time/all drain semantics behind a mutex-protected queue. |
| `agent.ts` | `Agent.constructor`, `Agent.subscribe`, `Agent.state`, `Agent.steeringMode`, `Agent.followUpMode`, `Agent.steer`, `Agent.followUp`, `Agent.clearSteeringQueue`, `Agent.clearFollowUpQueue`, `Agent.clearAllQueues`, `Agent.hasQueuedMessages`, `Agent.signal`, `Agent.abort`, `Agent.waitForIdle`, `Agent.reset`, `Agent.prompt`, `Agent.continue`, `Agent.normalizePromptInput`, `Agent.runPromptMessages`, `Agent.runContinuation`, `Agent.createContextSnapshot`, `Agent.createLoopConfig`, `Agent.runWithLifecycle`, `Agent.handleRunFailure`, `Agent.finishRun`, `Agent.processEvents` | `agent.go` `NewAgent`, `Subscribe`, `State`, queue mode accessors, queue/run helpers, lifecycle helpers | direct | Go adds explicit context parameters and method capitalization. Pi's `signal` getter is represented by the active run `context.Context` passed to subscribers and loop callbacks. |
| `harness/env/nodejs.ts` | `NodeExecutionEnv.constructor`, `NodeExecutionEnv.absolutePath`, `NodeExecutionEnv.joinPath`, `NodeExecutionEnv.exec`, `NodeExecutionEnv.readTextFile`, `NodeExecutionEnv.readTextLines`, `NodeExecutionEnv.readBinaryFile`, `NodeExecutionEnv.writeFile`, `NodeExecutionEnv.appendFile`, `NodeExecutionEnv.fileInfo`, `NodeExecutionEnv.listDir`, `NodeExecutionEnv.canonicalPath`, `NodeExecutionEnv.exists`, `NodeExecutionEnv.createDir`, `NodeExecutionEnv.remove`, `NodeExecutionEnv.createTempDir`, `NodeExecutionEnv.createTempFile`, `NodeExecutionEnv.cleanup` | `harness/env/local_env.go` `NewLocalExecutionEnv`, `LocalExecutionEnv` filesystem/process methods, root facade | go-native | Node-specific implementation is replaced by Go filesystem/process calls while preserving the same environment contract. |
| `harness/session/jsonl-repo.ts` | `JsonlSessionRepo.constructor`, `JsonlSessionRepo.getSessionsRoot`, `JsonlSessionRepo.getSessionDir`, `JsonlSessionRepo.createSessionFilePath`, `JsonlSessionRepo.create`, `JsonlSessionRepo.open`, `JsonlSessionRepo.list`, `JsonlSessionRepo.delete`, `JsonlSessionRepo.fork`, `JsonlSessionRepo.listSessionDirs` | `harness/session_repo.go` `NewJsonlSessionRepo`, `JsonlSessionRepo` methods and path helpers | direct | Go keeps the same cwd-scoped JSONL repository responsibilities with path helpers internal to the repo. |
| `harness/session/jsonl-storage.ts` | `JsonlSessionStorage.constructor`, `JsonlSessionStorage.open`, `JsonlSessionStorage.create`, `JsonlSessionStorage.getMetadata`, `JsonlSessionStorage.getLeafId`, `JsonlSessionStorage.setLeafId`, `JsonlSessionStorage.createEntryId`, `JsonlSessionStorage.appendEntry`, `JsonlSessionStorage.getEntry`, `JsonlSessionStorage.findEntries`, `JsonlSessionStorage.getLabel`, `JsonlSessionStorage.getSessionName`, `JsonlSessionStorage.getSessionStats`, `JsonlSessionStorage.getPathToRootOrCompaction`, `JsonlSessionStorage.getEntries` | `harness/session_storage.go` `JsonlSessionStorage` and storage interface methods | direct | Go uses interface-style names such as `Metadata`, `LeafID`, `SessionName`, `SessionStats`, `PathToRootOrCompaction`, and cursor-aware `Entries`; JSONL header metadata is preserved across create/open/list flows. |
| `harness/session/memory-repo.ts` | `InMemorySessionRepo.create`, `InMemorySessionRepo.open`, `InMemorySessionRepo.list`, `InMemorySessionRepo.delete`, `InMemorySessionRepo.fork` | `harness/session_repo.go` `InMemorySessionRepo` methods | direct | In-memory session repo behavior is represented for tests and embedded callers. |
| `harness/session/session.ts` | `Session.constructor`, `Session.getMetadata`, `Session.getStorage`, `Session.getLeafId`, `Session.getEntry`, `Session.getEntries`, `Session.getBranch`, `Session.buildContextEntries`, `Session.buildContext`, `Session.mergeContextBuildOptions`, `Session.getLabel`, `Session.getSessionStats`, `Session.getSessionName`, `Session.appendTypedEntry`, `Session.appendMessage`, `Session.appendThinkingLevelChange`, `Session.appendModelChange`, `Session.appendActiveToolsChange`, `Session.appendCompaction`, `Session.appendCustomEntry`, `Session.appendCustomMessageEntry`, `Session.appendLabel`, `Session.appendSessionName`, `Session.moveTo` | `harness/session.go` `NewSession`, `Session` accessors, `BuildContextEntries`, `BuildContext`, option merge, stats, append helpers, branch/context/move helpers | direct | Constructor and per-call context options compose in order; projector keys override cleanly, while active tools and retained tails stay in the same append-only state flow. |
| `harness/types.ts` | `FileError.constructor`, `ExecutionError.constructor`, `CompactionError.constructor`, `BranchSummaryError.constructor`, `SessionError.constructor`, `AgentHarnessError.constructor` | typed Go error structs in `harness/env/local_env.go`, `harness/local_env.go`, `harness/types.go`, and `harness/agent_harness.go` | go-native | Go constructs typed errors as struct values and supports `errors.As`; there is no TS-style class constructor API. |
| `proxy.ts` | `ProxyMessageEventStream.constructor` | `proxy.go` `llm.NewAssistantMessageEventStream` inside `StreamProxy` | direct | Proxy stream reconstruction uses the shared Go assistant event stream rather than a proxy-specific subclass. |

## Remaining Agent Core Gaps

- `proxy.ts` now maps to Gi `proxy.go`. The Go helper intentionally uses a
  typed `ProxyStreamOptions` / `StreamFn` adapter instead of Pi's TS
  `AbortSignal` shape, but preserves the `/api/stream` payload and event
  reconstruction contract.
- `harness/types.ts` TS `Result` helper functions are intentionally not ported;
  Go callers use `(value, error)` and typed errors. If SDK consumers need a
  result object over RPC, that should live in the RPC/protocol layer, not core.
- `harness/types.ts` typed error classes now map to Go typed errors and
  `errors.As` checks. This is intentionally Go-native rather than a TS `Result`
  object clone.

## Verification Evidence

- `GOCACHE=/private/tmp/gi-gocache go test ./gi-agent-core ./gi-agent-core/harness`
  is the focused validation gate for this map.
- `GOCACHE=/private/tmp/gi-gocache go test -run TestStreamProxy ./gi-agent-core`
  validates the Pi proxy stream event reconstruction and error path. It requires
  localhost binding for `httptest`.
- `GOCACHE=/private/tmp/gi-gocache go test -run 'TestCompactionErrorsCarryPiStyleCodes|TestGenerateBranchSummaryErrorsCarryPiStyleCodes' ./gi-agent-core/harness`
  validates the stable compaction and branch-summary error-code wrappers.
- `GOCACHE=/private/tmp/gi-gocache go test -timeout 30s ./gi-agent-core ./gi-agent-core/harness`
  passes after completing the exported-symbol audit. The `gi-agent-core`
  package requires localhost binding for proxy `httptest` cases.
