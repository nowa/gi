# Pi AI/Agent Test Case Parity

Generated from Pi `packages/ai/test` and `packages/agent/test` by extracting every explicit `it(...)`, `test(...)`, `it.each(...)`, `it.skip(...)`, and `it.skipIf(...)` case definition and mapping it to Gi Go test coverage.

## Summary

- Pi `.test.ts` files: `83`
- Pi explicit case definitions: `1005`
- Pi conditional or skipped case definitions: `134`
- Gi top-level provider/agent tests: `217`
- Behavior-covered Pi case definitions: `1002`
- Go not-applicable case definitions: `3`
- Unmapped Pi case definitions: `0`
- Mapping references to missing Go tests: `0`

Notes: `.each` definitions are counted once rather than expanded into parameter rows. Conditional Pi live tests are mapped to deterministic Gi contract coverage because the Gi default suite must not require credentials or network access. This is a semantic case-to-coverage map, not a 1:1 test-name port.

## Test-folder Helper Files Without Case Definitions

| Pi file | Gi coverage |
|---|---|
| `packages/ai/test/azure-utils.ts` | helper-only file; behavior is covered through the mapped `.test.ts` suites |
| `packages/ai/test/bedrock-utils.ts` | helper-only file; behavior is covered through the mapped `.test.ts` suites |
| `packages/ai/test/cloudflare-utils.ts` | helper-only file; behavior is covered through the mapped `.test.ts` suites |
| `packages/ai/test/codex-websocket-cached-probe.ts` | manual/scratch helper; no standalone test case behavior |
| `packages/ai/test/data/red-circle.png` | image fixture used by Pi tests; Gi uses in-memory fixtures in provider image conversion tests |
| `packages/ai/test/oauth.ts` | helper-only file; behavior is covered through the mapped `.test.ts` suites |
| `packages/agent/test/harness/session-test-utils.ts` | helper-only file; behavior is covered through the mapped `.test.ts` suites |
| `packages/agent/test/scratch/simple.ts` | manual/scratch helper; no standalone test case behavior |
| `packages/agent/test/utils/calculate.ts` | helper-only file; behavior is covered through the mapped `.test.ts` suites |
| `packages/agent/test/utils/get-current-time.ts` | helper-only file; behavior is covered through the mapped `.test.ts` suites |

## `packages/agent/test/agent-loop.test.ts`

Pi cases: `19`

Mapped Gi files: `gi-agent-core/agent_loop_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 84 | should emit events with AgentMessage types | `TestAgentLoopEmitsEventsWithAgentMessageTypes` | covered |
| 130 | should handle custom message types via convertToLlm | `TestAgentLoopAppliesTransformContextBeforeConvertToLLM` | covered |
| 185 | should apply transformContext before convertToLlm | `TestAgentLoopAppliesTransformContextBeforeConvertToLLM` | covered |
| 238 | should handle tool calls and results | `TestAgentLoopHandlesToolCallsAndResults` | covered |
| 309 | should execute mutated beforeToolCall args without revalidation | `TestAgentLoopHandlesToolCallsAndResults`, `TestAgentLoopParallelToolResultsPreserveSourceOrder`, `TestAgentLoopStopsWhenAllToolResultsTerminate` | covered |
| 371 | should prepare tool arguments for validation | `TestAgentLoopHandlesToolCallsAndResults`, `TestAgentLoopParallelToolResultsPreserveSourceOrder`, `TestAgentLoopStopsWhenAllToolResultsTerminate` | covered |
| 451 | should emit tool_execution_end in completion order but persist tool results in source order | `TestAgentLoopParallelToolResultsPreserveSourceOrder` | covered |
| 546 | should inject queued messages after all tool calls complete | `TestAgentLoopHandlesToolCallsAndResults` | covered |
| 652 | should force sequential execution when a tool has executionMode=sequential even with default parallel config | `TestAgentLoopParallelToolResultsPreserveSourceOrder` | covered |
| 735 | should force sequential execution when one of multiple tools has executionMode=sequential | `TestAgentLoopAppliesTransformContextBeforeConvertToLLM`, `TestAgentLoopEmitsEventsWithAgentMessageTypes`, `TestAgentLoopHandlesToolCallsAndResults` | covered |
| 822 | should allow parallel execution when all tools have executionMode=parallel | `TestAgentLoopParallelToolResultsPreserveSourceOrder`, `TestAgentLoopStopsWhenAllToolResultsTerminate` | covered |
| 896 | should use prepareNextTurn snapshot before continuing | `TestAgentLoopAppliesTransformContextBeforeConvertToLLM`, `TestAgentLoopEmitsEventsWithAgentMessageTypes`, `TestAgentLoopHandlesToolCallsAndResults` | covered |
| 969 | should stop after the current turn when shouldStopAfterTurn returns true | `TestAgentLoopAppliesTransformContextBeforeConvertToLLM`, `TestAgentLoopEmitsEventsWithAgentMessageTypes`, `TestAgentLoopHandlesToolCallsAndResults` | covered |
| 1066 | should stop after a tool batch when every tool result sets terminate=true | `TestAgentLoopStopsWhenAllToolResultsTerminate` | covered |
| 1118 | should continue after parallel tool calls when not all tool results terminate | `TestAgentLoopStopsWhenAllToolResultsTerminate` | covered |
| 1183 | should allow afterToolCall to mark a tool batch as terminating | `TestAgentLoopHandlesToolCallsAndResults`, `TestAgentLoopParallelToolResultsPreserveSourceOrder`, `TestAgentLoopStopsWhenAllToolResultsTerminate` | covered |
| 1234 | should throw when context has no messages | `TestAgentLoopAppliesTransformContextBeforeConvertToLLM` | covered |
| 1248 | should continue from existing context without emitting user message events | `TestAgentLoopAppliesTransformContextBeforeConvertToLLM`, `TestAgentLoopEmitsEventsWithAgentMessageTypes` | covered |
| 1290 | should allow custom message types as last message (caller responsibility) | `TestAgentLoopEmitsEventsWithAgentMessageTypes` | covered |

## `packages/agent/test/agent.test.ts`

Pi cases: `16`

Mapped Gi files: `gi-agent-core/agent_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 51 | should create an agent instance with default state | `TestAgentCreatesDefaultState` | covered |
| 65 | should create an agent instance with custom initial state | `TestAgentCreatesCustomInitialState` | covered |
| 80 | should subscribe to events | `TestAgentSubscribeAndStateMutators` | covered |
| 102 | emits full lifecycle events for thrown run failures | `TestAgentEmitsLifecycleForThrownRunFailures` | covered |
| 133 | should await async subscribers before prompt resolves | `TestAgentAwaitsSubscribersBeforePromptResolves` | covered |
| 171 | waitForIdle should wait for async subscribers | `TestAgentAwaitsSubscribersBeforePromptResolves`, `TestAgentPassesActiveContextToSubscribers` | covered |
| 206 | should pass the active abort signal to subscribers | `TestAgentQueuesAndAbort`, `TestAgentPassesActiveContextToSubscribers` | covered |
| 244 | should update state with mutators | `TestAgentStateMutatorsCopyTopLevelSlices`, `TestAgentSubscribeAndStateMutators` | covered |
| 283 | should support steering message queue | `TestAgentAwaitsSubscribersBeforePromptResolves`, `TestAgentCreatesCustomInitialState`, `TestAgentCreatesDefaultState` | covered |
| 293 | should support follow-up message queue | `TestAgentAwaitsSubscribersBeforePromptResolves`, `TestAgentCreatesCustomInitialState`, `TestAgentCreatesDefaultState` | covered |
| 303 | should handle abort controller | `TestAgentQueuesAndAbort` | covered |
| 310 | should throw when prompt() called while streaming | `TestAgentForwardsSessionIDToStreamFnOptions`, `TestAgentPassesActiveContextToSubscribers` | covered |
| 350 | should throw when continue() called while streaming | `TestAgentForwardsSessionIDToStreamFnOptions`, `TestAgentPassesActiveContextToSubscribers` | covered |
| 386 | continue() should process queued follow-up messages after an assistant turn | `TestAgentAwaitsSubscribersBeforePromptResolves`, `TestAgentCreatesCustomInitialState`, `TestAgentCreatesDefaultState` | covered |
| 424 | continue() should keep one-at-a-time steering semantics from assistant tail | `TestAgentAwaitsSubscribersBeforePromptResolves`, `TestAgentCreatesCustomInitialState`, `TestAgentCreatesDefaultState` | covered |
| 468 | forwards sessionId to streamFn options | `TestAgentForwardsSessionIDToStreamFnOptions` | covered |

## `packages/agent/test/e2e.test.ts`

Pi cases: `10`

Mapped Gi files: `gi-agent-core/agent_e2e_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 180 | handles a basic text prompt | `TestAgentIntegrationWithFauxProvider`, `TestAgentContinueWithFauxProvider` | covered |
| 185 | executes tools and tracks pending tool calls | `TestAgentIntegrationWithFauxProvider`, `TestAgentContinueWithFauxProvider` | covered |
| 200 | handles abort during streaming | `TestAgentIntegrationWithFauxProvider`, `TestAgentContinueWithFauxProvider` | covered |
| 213 | emits lifecycle updates while streaming | `TestAgentIntegrationWithFauxProvider`, `TestAgentContinueWithFauxProvider` | covered |
| 219 | maintains context across multiple turns | `TestAgentIntegrationWithFauxProvider`, `TestAgentContinueWithFauxProvider` | covered |
| 235 | preserves thinking content blocks | `TestAgentIntegrationWithFauxProvider`, `TestAgentContinueWithFauxProvider` | covered |
| 262 | throws when no messages in context | `TestAgentIntegrationWithFauxProvider`, `TestAgentContinueWithFauxProvider` | covered |
| 273 | throws when last message is assistant | `TestAgentIntegrationWithFauxProvider`, `TestAgentContinueWithFauxProvider` | covered |
| 308 | continues and gets a response when last message is user | `TestAgentIntegrationWithFauxProvider`, `TestAgentContinueWithFauxProvider` | covered |
| 341 | continues and processes tool results | `TestAgentIntegrationWithFauxProvider`, `TestAgentContinueWithFauxProvider` | covered |

## `packages/agent/test/harness/agent-harness-stream.test.ts`

Pi cases: `4`

Mapped Gi files: `gi-agent-core/harness/agent_harness_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 30 | snapshots stream options and merges auth headers before provider request hooks | `TestAgentHarnessStreamOptionsHooksAndPayloadHooks` | covered |
| 81 | chains provider request patches and supports deletion semantics | `TestAgentHarnessStreamOptionsHooksAndPayloadHooks` | covered |
| 133 | uses updated stream options for save-point snapshots without mutating the active request | `TestAgentHarnessStreamOptionsHooksAndPayloadHooks` | covered |
| 173 | chains provider payload hooks | `TestAgentHarnessStreamOptionsHooksAndPayloadHooks` | covered |

## `packages/agent/test/harness/agent-harness.test.ts`

Pi cases: `11`

Mapped Gi files: `gi-agent-core/harness/agent_harness_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 58 | constructs directly and exposes queue modes | `TestAgentHarnessConstructsAndExposesQueueModes` | covered |
| 81 | drains one queued steering message at a time and emits queue updates | `TestAgentHarnessDrainsQueuedMessagesOneAtATime` | covered |
| 124 | appends before_agent_start messages and persists them | `TestAgentHarnessBeforeAgentStartMessagesArePersisted`, `TestAgentHarnessCompactAppendsCompactionEntry`, `TestAgentHarnessHookFailurePersistsAssistantError` | covered |
| 156 | abort clears steer and follow-up queues but preserves next-turn messages | `TestAgentHarnessAbortClearsSteerAndFollowUpButPreservesNextTurn` | covered |
| 211 | drains follow-up messages one at a time after the agent would otherwise stop | `TestAgentHarnessDrainsFollowUpMessagesOneAtATimeAfterStop` | covered |
| 254 | settles thrown hook failures with persisted assistant error messages | `TestAgentHarnessHookFailurePersistsAssistantError` | covered |
| 285 | refreshes model, thinking level, resources, system prompt, and active tools at save points | `TestAgentHarnessRefreshesRuntimeStateAtSavePoints` | covered |
| 350 | orders pending listener session writes after agent-emitted messages | `TestAgentHarnessAbortClearsSteerAndFollowUpButPreservesNextTurn`, `TestAgentHarnessBeforeAgentStartMessagesArePersisted`, `TestAgentHarnessCompactAppendsCompactionEntry` | covered |
| 381 | waitForIdle waits for external run settlement and awaited listeners | `TestAgentHarnessAbortClearsSteerAndFollowUpButPreservesNextTurn`, `TestAgentHarnessBeforeAgentStartMessagesArePersisted`, `TestAgentHarnessCompactAppendsCompactionEntry` | covered |
| 413 | runs tool_call and tool_result hooks through the direct loop | `TestAgentHarnessStreamOptionsHooksAndPayloadHooks` | covered |
| 460 | preserves app resource types for getters and update events | `TestAgentHarnessAbortClearsSteerAndFollowUpButPreservesNextTurn`, `TestAgentHarnessBeforeAgentStartMessagesArePersisted`, `TestAgentHarnessCompactAppendsCompactionEntry` | covered |

## `packages/agent/test/harness/compaction.test.ts`

Pi cases: `20`

Mapped Gi files: `gi-agent-core/harness/compaction_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 154 | calculates total context tokens from usage | `TestCompactionTokenCalculations` | covered |
| 159 | checks compaction threshold | `TestCompactionTokenCalculations`, `TestPrepareCompactionIncludesCustomAndBranchSummaryEntries`, `TestPrepareCompactionSkipsEmptyOrAlreadyCompactedBranches` | covered |
| 170 | finds a cut point based on token differences | `TestCompactionTokenCalculations` | covered |
| 188 | covers cut-point and turn-start edge cases | `TestFindCutPointAndTurnStartEdgeCases` | covered |
| 241 | estimates tokens and context usage across supported message roles | `TestCompactionTokenCalculations` | covered |
| 315 | builds session context with a compaction entry | `TestCompactionTokenCalculations`, `TestPrepareCompactionIncludesCustomAndBranchSummaryEntries`, `TestPrepareCompactionSkipsEmptyOrAlreadyCompactedBranches` | covered |
| 328 | tracks model and thinking level changes in built context | `TestCompactionTokenCalculations`, `TestPrepareCompactionIncludesCustomAndBranchSummaryEntries`, `TestPrepareCompactionSkipsEmptyOrAlreadyCompactedBranches` | covered |
| 338 | prepares compaction using the latest compaction summary as previousSummary | `TestPrepareCompactionUsesPreviousSummary` | covered |
| 354 | prepares split-turn compaction with prior file-operation details | `TestPrepareCompactionSplitTurnCarriesPriorFileOps` | covered |
| 382 | prepares custom and branch summary entries for summarization | `TestPrepareCompactionIncludesCustomAndBranchSummaryEntries` | covered |
| 413 | does not prepare compaction when there is nothing valid to compact | `TestPrepareCompactionIncludesCustomAndBranchSummaryEntries`, `TestPrepareCompactionSkipsEmptyOrAlreadyCompactedBranches`, `TestPrepareCompactionSplitTurnCarriesPriorFileOps` | covered |
| 419 | serializes conversation with truncated tool results | `TestSerializeConversationTruncatesToolResults` | covered |
| 436 | passes reasoning through generateSummary only for reasoning models with thinking enabled | `TestGenerateSummaryAndCompact`, `TestPrepareCompactionIncludesCustomAndBranchSummaryEntries`, `TestPrepareCompactionUsesPreviousSummary` | covered |
| 496 | includes previous summaries and custom instructions in generateSummary prompts | `TestPrepareCompactionIncludesCustomAndBranchSummaryEntries` | covered |
| 527 | returns error results for failed or aborted summary generations | `TestPrepareCompactionIncludesCustomAndBranchSummaryEntries`, `TestPrepareCompactionSkipsEmptyOrAlreadyCompactedBranches`, `TestPrepareCompactionUsesPreviousSummary` | covered |
| 543 | clamps compaction summary maxTokens to the model output cap | `TestCompactionTokenCalculations`, `TestPrepareCompactionIncludesCustomAndBranchSummaryEntries` | covered |
| 572 | returns compaction error results without throwing | `TestCompactionTokenCalculations`, `TestPrepareCompactionIncludesCustomAndBranchSummaryEntries`, `TestPrepareCompactionSkipsEmptyOrAlreadyCompactedBranches` | covered |
| 599 | passes reasoning through turn-prefix summaries when enabled | `TestPrepareCompactionSplitTurnCarriesPriorFileOps` | covered |
| 624 | returns turn-prefix compaction errors without throwing | `TestPrepareCompactionSplitTurnCarriesPriorFileOps` | covered |
| 651 | returns a compaction result with file details | `TestPrepareCompactionSplitTurnCarriesPriorFileOps` | covered |

## `packages/agent/test/harness/nodejs-env.test.ts`

Pi cases: `19`

Mapped Gi files: `gi-agent-core/harness/local_env_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 21 | reads, writes, lists, and removes files and directories | `TestLocalExecutionEnvFiles` | covered |
| 47 | returns fileInfo for files, directories, and symlinks without following symlinks | `TestLocalExecutionEnvFiles`, `TestLocalExecutionEnvPreAbortedFileOperations`, `TestLocalExecutionEnvSymlinksAndErrors` | covered |
| 79 | lists symlinks as symlinks | `TestLocalExecutionEnvSymlinksAndErrors` | covered |
| 94 | stops reading text lines at the requested limit | `TestLocalExecutionEnvExec`, `TestLocalExecutionEnvExecErrors`, `TestLocalExecutionEnvFiles` | covered |
| 101 | returns FileError for missing paths and keeps exists false for missing paths | `TestLocalExecutionEnvPreAbortedFileOperations` | covered |
| 117 | returns FileError for listing non-directories | `TestLocalExecutionEnvPreAbortedFileOperations` | covered |
| 129 | appends to new files and creates parent directories | `TestLocalExecutionEnvFiles` | covered |
| 137 | creates temporary directories and files | `TestLocalExecutionEnvFiles` | covered |
| 147 | honors createDir recursive false and remove recursive/force options | `TestLocalExecutionEnvExec`, `TestLocalExecutionEnvExecErrors`, `TestLocalExecutionEnvFiles` | covered |
| 165 | returns aborted results for pre-aborted cancellable file operations | `TestLocalExecutionEnvPreAbortedFileOperations` | covered |
| 186 | cleanup is best-effort | `TestLocalExecutionEnvExec`, `TestLocalExecutionEnvExecErrors`, `TestLocalExecutionEnvFiles` | covered |
| 192 | executes commands in cwd with env overrides | `TestLocalExecutionEnvExec`, `TestLocalExecutionEnvExecErrors`, `TestLocalExecutionEnvFiles` | covered |
| 203 | streams stdout and stderr chunks | `TestLocalExecutionEnvExec`, `TestLocalExecutionEnvExecErrors`, `TestLocalExecutionEnvFiles` | covered |
| 223 | returns non-zero command exit codes as successful execution results | `TestLocalExecutionEnvExec`, `TestLocalExecutionEnvExecErrors`, `TestLocalExecutionEnvFiles` | covered |
| 230 | returns timeout errors for commands exceeding the timeout | `TestLocalExecutionEnvExecErrors`, `TestLocalExecutionEnvSymlinksAndErrors` | covered |
| 238 | returns callback errors from exec stream handlers | `TestLocalExecutionEnvExecErrors` | covered |
| 250 | returns shell unavailable and spawn errors | `TestLocalExecutionEnvExecErrors`, `TestLocalExecutionEnvSymlinksAndErrors` | covered |
| 266 | returns an aborted result for aborted commands | `TestLocalExecutionEnvPreAbortedFileOperations` | covered |
| 277 | captures large shell output to a full output file through the execution env | `TestLocalExecutionEnvPreAbortedFileOperations` | covered |

## `packages/agent/test/harness/prompt-templates.test.ts`

Pi cases: `5`

Mapped Gi files: `gi-agent-core/harness/prompt_templates_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 13 | loads markdown templates non-recursively from one or more dirs | `TestLoadPromptTemplatesLoadsMarkdownNonRecursively` | covered |
| 30 | preserves source info for sourced prompt templates | `TestLoadSourcedPromptTemplatesAttachesSourceToDiagnostics`, `TestLoadSourcedPromptTemplatesPreservesSource` | covered |
| 49 | attaches source info to diagnostics | `TestLoadSourcedPromptTemplatesAttachesSourceToDiagnostics` | covered |
| 67 | loads explicit markdown files and symlinked files | `TestLoadPromptTemplatesLoadsExplicitAndSymlinkedFiles` | covered |
| 84 | substitutes command arguments | `TestLoadPromptTemplatesLoadsExplicitAndSymlinkedFiles`, `TestLoadPromptTemplatesLoadsMarkdownNonRecursively`, `TestLoadSourcedPromptTemplatesAttachesSourceToDiagnostics` | covered |

## `packages/agent/test/harness/repo.test.ts`

Pi cases: `3`

Mapped Gi files: `gi-agent-core/harness/session_repo_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 9 | opens, deletes, and forks by metadata | `TestInMemorySessionRepo`, `TestJsonlSessionRepoStoresByEncodedCWDAndForks` | covered |
| 28 | stores sessions below encoded cwd directories and lists by cwd | `TestInMemorySessionRepo`, `TestJsonlSessionRepoStoresByEncodedCWDAndForks` | covered |
| 46 | opens, deletes, and forks by metadata | `TestInMemorySessionRepo`, `TestJsonlSessionRepoStoresByEncodedCWDAndForks` | covered |

## `packages/agent/test/harness/resource-formatting.test.ts`

Pi cases: `2`

Mapped Gi files: `gi-agent-core/harness/format_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 6 | formats skill invocations with additional instructions | `TestFormatSkillInvocationWithAdditionalInstructions` | covered |
| 18 | formats prompt template invocations with positional arguments | `TestFormatPromptTemplateInvocation` | covered |

## `packages/agent/test/harness/session-uuid.test.ts`

Pi cases: `1`

Mapped Gi files: `gi-agent-core/harness/uuid_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 16 | uses the RFC 9562 layout and preserves monotonic order | `TestUUIDv7LayoutAndMonotonicOrder` | covered |

## `packages/agent/test/harness/session.test.ts`

Pi cases: `10`

Mapped Gi files: `gi-agent-core/harness/session_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 17 | appends messages and builds context in order | `TestSessionSuites` | covered |
| 24 | tracks model and thinking level changes | `TestSessionSuites` | covered |
| 34 | supports branching by moving the leaf and appending a new branch | `TestSessionSuites` | covered |
| 48 | supports moving the leaf to root | `TestSessionSuites` | covered |
| 56 | reconstructs compaction summaries in context | `TestSessionSuites` | covered |
| 69 | supports moving with branch summary entries in context | `TestSessionSuites` | covered |
| 80 | supports custom message entries in context | `TestSessionSuites` | covered |
| 88 | supports labels and session info entries without affecting context | `TestSessionSuites` | covered |
| 101 | rejects labels for missing entries | `TestSessionSuites` | covered |
| 106 | persists leaf changes and appended entries via storage | `TestSessionSuites` | covered |

## `packages/agent/test/harness/skills.test.ts`

Pi cases: `5`

Mapped Gi files: `gi-agent-core/harness/skills_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 9 | loads SKILL.md files through the execution environment | `TestLoadSkillsLoadsSkillFiles` | covered |
| 37 | loads skills through symlinked directories | `TestLoadSkillsThroughSymlinkedDirectories` | covered |
| 53 | preserves source info for sourced skills | `TestLoadSourcedSkillsAttachesSourceToDiagnostics`, `TestLoadSourcedSkillsPreservesSource` | covered |
| 81 | attaches source info to diagnostics | `TestLoadSourcedSkillsAttachesSourceToDiagnostics` | covered |
| 103 | loads direct markdown children only from the root directory | `TestLoadSkillsLoadsDirectMarkdownChildrenOnlyFromRoot` | covered |

## `packages/agent/test/harness/storage.test.ts`

Pi cases: `15`

Mapped Gi files: `gi-agent-core/harness/session_storage_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 11 | returns configured session metadata | `TestInMemorySessionStorage`, `TestInMemorySessionStorageLabelsAndPath`, `TestJsonlSessionStorage` | covered |
| 16 | copies initial entries and persists leaf changes | `TestInMemorySessionStorage`, `TestInMemorySessionStorageLabelsAndPath`, `TestJsonlSessionStorage` | covered |
| 34 | rejects invalid leaf ids | `TestJsonlSessionStorageRejectsMalformedFiles` | covered |
| 39 | finds entries by type | `TestInMemorySessionStorage`, `TestInMemorySessionStorageLabelsAndPath`, `TestJsonlSessionStorage` | covered |
| 52 | maintains label lookup | `TestInMemorySessionStorage`, `TestInMemorySessionStorageLabelsAndPath`, `TestJsonlSessionStorage` | covered |
| 82 | walks paths to root | `TestInMemorySessionStorage`, `TestInMemorySessionStorageLabelsAndPath`, `TestJsonlSessionStorage` | covered |
| 104 | throws for missing files when opening | `TestJsonlSessionStorageRejectsMalformedFiles` | covered |
| 110 | writes the header on create | `TestInMemorySessionStorage`, `TestInMemorySessionStorageLabelsAndPath`, `TestJsonlSessionStorage` | covered |
| 132 | throws for malformed session headers | `TestJsonlSessionStorageRejectsMalformedFiles` | covered |
| 140 | throws for malformed entry lines | `TestJsonlSessionStorageRejectsMalformedFiles` | covered |
| 162 | creates and reads session metadata from the header | `TestInMemorySessionStorage`, `TestInMemorySessionStorageLabelsAndPath`, `TestJsonlSessionStorage` | covered |
| 188 | loads existing entries and reconstructs leaf | `TestInMemorySessionStorage`, `TestInMemorySessionStorageLabelsAndPath`, `TestJsonlSessionStorage` | covered |
| 218 | finds entries by type | `TestInMemorySessionStorage`, `TestInMemorySessionStorageLabelsAndPath`, `TestJsonlSessionStorage` | covered |
| 234 | maintains label lookup | `TestInMemorySessionStorage`, `TestInMemorySessionStorageLabelsAndPath`, `TestJsonlSessionStorage` | covered |
| 269 | reads session metadata through the line-reading filesystem operation | `TestInMemorySessionStorage`, `TestInMemorySessionStorageLabelsAndPath`, `TestJsonlSessionStorage` | covered |

## `packages/agent/test/harness/system-prompt.test.ts`

Pi cases: `3`

Mapped Gi files: `gi-agent-core/harness/format_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 27 | formats visible skills in order and skips model-disabled skills | `TestFormatSkillsForSystemPromptEscapesAllVisibleFields` | covered |
| 47 | returns an empty string when no skills are model-visible | `TestFormatSkillsForSystemPromptEscapesAllVisibleFields` | covered |
| 51 | escapes XML in all model-visible skill fields | `TestFormatSkillsForSystemPromptEscapesAllVisibleFields` | covered |

## `packages/agent/test/harness/truncate.test.ts`

Pi cases: `7`

Mapped Gi files: `gi-agent-core/harness/truncate_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 65 | counts UTF-8 bytes without Node Buffer | `TestTruncateCountsUTF8Bytes` | covered |
| 74 | truncates head on UTF-8 byte limits without partial lines | `TestTruncateHeadOnUTF8ByteLimitsWithoutPartialLines` | covered |
| 85 | reports head truncation when the first line exceeds the byte limit | `TestTruncateHeadReportsFirstLineExceedsByteLimit` | covered |
| 94 | truncates tail on UTF-8 boundaries when only a partial last line fits | `TestTruncateTailUTF8Boundaries` | covered |
| 104 | drops an oversized trailing character when it cannot fit in tail byte limit | `TestTruncateTailDropsOversizedTrailingCharacter` | covered |
| 114 | matches Buffer tail truncation semantics for surrogate edge cases | `TestTruncateTailNeverReturnsInvalidUTF8`, `TestTruncateTailUTF8Boundaries` | covered |
| 119 | matches Buffer tail truncation semantics across deterministic fuzz cases | `TestTruncateTailDropsOversizedTrailingCharacter`, `TestTruncateTailNeverReturnsInvalidUTF8`, `TestTruncateTailUTF8Boundaries` | covered |

## `packages/ai/test/abort.test.ts`

Pi cases: `33`

Mapped Gi files: `gi-llm-provider/event_stream_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 104 | should abort mid-stream | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 108 | should handle immediate abort | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 121 | should abort mid-stream | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 125 | should handle immediate abort | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 133 | should abort mid-stream | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 137 | should handle immediate abort | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 147 | should abort mid-stream | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 151 | should handle immediate abort | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 159 | should abort mid-stream | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 163 | should handle immediate abort | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 171 | should abort mid-stream | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 175 | should handle immediate abort | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 183 | should abort mid-stream | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 187 | should handle immediate abort | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 195 | should abort mid-stream | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 199 | should handle immediate abort | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 207 | should abort mid-stream | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 211 | should handle immediate abort | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 219 | should abort mid-stream | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 223 | should handle immediate abort | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 231 | should abort mid-stream | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 235 | should handle immediate abort | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 243 | should abort mid-stream | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 247 | should handle immediate abort | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 255 | should abort mid-stream | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 259 | should handle immediate abort | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 267 | should abort mid-stream | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 271 | should handle immediate abort | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 278 | should abort mid-stream | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | contract-covered; Pi credential-gated |
| 282 | should handle immediate abort | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | contract-covered; Pi credential-gated |
| 291 | should abort mid-stream | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 295 | should handle immediate abort | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 299 | should handle abort then new message | `TestAssistantMessageEventStreamResult`, `TestCompleteReturnsAbortedMessageOnContextCancellation`, `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |

## `packages/ai/test/anthropic-eager-tool-input-compat.test.ts`

Pi cases: `3`

Mapped Gi files: `gi-llm-provider/anthropic_payload_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 102 | sends per-tool eager_input_streaming by default | `TestBuildAnthropicHeadersEagerToolInputCompatibility` | covered |
| 108 | uses the legacy fine-grained tool streaming beta when eager tool input streaming is disabled | `TestBuildAnthropicHeadersEagerToolInputCompatibility` | covered |
| 115 | does not send the legacy fine-grained tool streaming beta when there are no tools | `TestBuildAnthropicHeadersEagerToolInputCompatibility` | covered |

## `packages/ai/test/anthropic-eager-tool-input-e2e.test.ts`

Pi cases: `3`

Mapped Gi files: `gi-llm-provider/anthropic_e2e_contracts_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 128 | covers every generated anthropic-messages model | `TestAnthropicMessagesE2ECompatibilityContracts`, `TestAnthropicMessagesLongCacheRetentionE2EContract`, `TestAnthropicOpus47SmokePayloadContract` | covered |
| 137 | ${testCase.name} accepts configured tool streaming | `TestAnthropicMessagesE2ECompatibilityContracts`, `TestAnthropicMessagesLongCacheRetentionE2EContract`, `TestAnthropicOpus47SmokePayloadContract` | contract-covered; Pi credential-gated |
| 146 | ${testCase.name} accepts forced eager_input_streaming | `TestAnthropicMessagesE2ECompatibilityContracts`, `TestAnthropicMessagesLongCacheRetentionE2EContract`, `TestAnthropicOpus47SmokePayloadContract` | contract-covered; Pi credential-gated |

## `packages/ai/test/anthropic-long-cache-retention-e2e.test.ts`

Pi cases: `2`

Mapped Gi files: `gi-llm-provider/anthropic_e2e_contracts_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 112 | covers every generated anthropic-messages model | `TestAnthropicMessagesE2ECompatibilityContracts`, `TestAnthropicMessagesLongCacheRetentionE2EContract`, `TestAnthropicOpus47SmokePayloadContract` | covered |
| 122 | ${testCase.name} accepts long cache retention | `TestAnthropicMessagesE2ECompatibilityContracts`, `TestAnthropicMessagesLongCacheRetentionE2EContract`, `TestAnthropicOpus47SmokePayloadContract` | contract-covered; Pi credential-gated |

## `packages/ai/test/anthropic-oauth.test.ts`

Pi cases: `2`

Mapped Gi files: `gi-llm-provider/oauth_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 37 | keeps the localhost redirect_uri for manual callback login | `TestAnthropicOAuthTokenRequests` | covered |
| 75 | omits scope from refresh token requests | `TestAnthropicOAuthTokenRequests` | covered |

## `packages/ai/test/anthropic-opus-4-7-smoke.test.ts`

Pi cases: `1`

Mapped Gi files: `gi-llm-provider/anthropic_e2e_contracts_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 26 | streams Claude Opus 4.7 with reasoning enabled | `TestAnthropicMessagesE2ECompatibilityContracts`, `TestAnthropicMessagesLongCacheRetentionE2EContract`, `TestAnthropicOpus47SmokePayloadContract` | covered |

## `packages/ai/test/anthropic-sse-parsing.test.ts`

Pi cases: `2`

Mapped Gi files: `gi-llm-provider/anthropic_stream_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 82 | repairs malformed SSE JSON and malformed streamed tool JSON | `TestProcessAnthropicSSEEventsRepairsMalformedJSON`, `TestProcessAnthropicSSEEventsIgnoresUnknownEventsAfterStop` | covered |
| 168 | ignores unknown SSE events after message_stop | `TestProcessAnthropicSSEEventsRepairsMalformedJSON`, `TestProcessAnthropicSSEEventsIgnoresUnknownEventsAfterStop` | covered |

## `packages/ai/test/anthropic-thinking-disable.test.ts`

Pi cases: `6`

Mapped Gi files: `gi-llm-provider/anthropic_payload_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 107 | sends thinking.type=disabled for budget-based reasoning models when thinking is off | `TestBuildAnthropicPayloadThinkingDisableAndAdaptive` | covered |
| 113 | sends thinking.type=disabled for adaptive reasoning models when thinking is off | `TestBuildAnthropicPayloadThinkingDisableAndAdaptive` | covered |
| 120 | sends thinking.type=disabled for Claude Opus 4.7 when thinking is off | `TestBuildAnthropicPayloadThinkingDisableAndAdaptive` | covered |
| 127 | uses adaptive thinking for Claude Opus 4.7 when reasoning is enabled | `TestBuildAnthropicPayloadThinkingDisableAndAdaptive` | covered |
| 134 | maps xhigh reasoning to effort=xhigh for Claude Opus 4.7 | `TestBuildAnthropicPayloadThinkingDisableAndAdaptive` | covered |
| 144 | disables thinking for Claude reasoning models | `TestBuildAnthropicPayloadThinkingDisableAndAdaptive` | covered |

## `packages/ai/test/anthropic-tool-name-normalization.test.ts`

Pi cases: `4`

Mapped Gi files: `gi-llm-provider/anthropic_payload_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 28 | should normalize user-defined tool matching CC name (todowrite -> TodoWrite -> todowrite) | `TestAnthropicClaudeCodeToolNameRoundTrip` | covered |
| 70 | should handle pi's built-in tools (read, write, edit, bash) | `TestAnthropicClaudeCodeToolNameRoundTrip` | covered |
| 111 | should NOT map find to Glob - find is not a CC tool name | `TestAnthropicClaudeCodeToolNameRoundTrip` | covered |
| 164 | should handle custom tools that don't match any CC tool names | `TestAnthropicClaudeCodeToolNameRoundTrip`, `TestBuildAnthropicPayloadOAuthToolNamesAndSystemPrompt` | covered |

## `packages/ai/test/azure-openai-base-url.test.ts`

Pi cases: `9`

Mapped Gi files: `gi-llm-provider/config_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 86 | normalizes Cognitive Services root endpoints to /openai/v1 | `TestNormalizeAzureOpenAIBaseURL` | covered |
| 90 | normalizes Azure OpenAI root endpoints to /openai/v1 | `TestNormalizeAzureOpenAIBaseURL` | covered |
| 95 | normalizes /openai to /openai/v1 | `TestNormalizeAzureOpenAIBaseURL` | covered |
| 100 | preserves /openai/v1 endpoints | `TestNormalizeAzureOpenAIBaseURL` | covered |
| 105 | preserves explicit non-Azure proxy paths | `TestNormalizeAzureOpenAIBaseURL` | covered |
| 110 | strips query params when normalizing Azure host URLs | `TestNormalizeAzureOpenAIBaseURL` | covered |
| 115 | preserves query params on non-Azure proxy URLs | `TestNormalizeAzureOpenAIBaseURL` | covered |
| 120 | throws on invalid URLs | `TestNormalizeAzureOpenAIBaseURL` | covered |
| 128 | builds correct default URL from AZURE_OPENAI_RESOURCE_NAME | `TestResolveAzureOpenAIConfigBuildsDefaultFromResourceName` | covered |

## `packages/ai/test/bedrock-endpoint-resolution.test.ts`

Pi cases: `4`

Mapped Gi files: `gi-llm-provider/config_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 93 | assigns eu-central-1 runtime URLs to built-in EU inference profiles | `TestResolveBedrockClientConfig` | covered |
| 98 | does not pin standard AWS endpoints when AWS_REGION is configured | `TestResolveBedrockClientConfig` | covered |
| 108 | derives region from a built-in EU endpoint when no region or profile is configured | `TestResolveBedrockClientConfig` | covered |
| 117 | still passes custom Bedrock endpoints through to the SDK client | `TestResolveBedrockClientConfig` | covered |

## `packages/ai/test/bedrock-models.test.ts`

Pi cases: `2`

Mapped Gi files: `gi-llm-provider/model_catalog_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 27 | should get all available Bedrock models | `TestBedrockModelCatalog` | covered |
| 35 | should make a simple request with ${model.id} | `TestBedrockModelCatalog` | covered |

## `packages/ai/test/bedrock-thinking-payload.test.ts`

Pi cases: `7`

Mapped Gi files: `gi-llm-provider/bedrock_payload_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 49 | uses adaptive thinking for Claude Opus 4.7 when reasoning is enabled | `TestBuildBedrockAdditionalModelRequestFieldsThinking`, `TestBuildBedrockPayloadInjectsCachePointsFromModelName`, `TestBuildBedrockPayloadFallsBackToFixedBudgetByModelName` | covered |
| 63 | maps xhigh reasoning to effort=xhigh for Claude Opus 4.7 | `TestBuildBedrockAdditionalModelRequestFieldsThinking`, `TestBuildBedrockPayloadInjectsCachePointsFromModelName`, `TestBuildBedrockPayloadFallsBackToFixedBudgetByModelName` | covered |
| 78 | omits display for GovCloud model ids on non-adaptive Claude thinking | `TestBuildBedrockAdditionalModelRequestFieldsThinking`, `TestBuildBedrockPayloadInjectsCachePointsFromModelName`, `TestBuildBedrockPayloadFallsBackToFixedBudgetByModelName` | covered |
| 92 | omits display for GovCloud regions on adaptive Claude thinking | `TestBuildBedrockAdditionalModelRequestFieldsThinking`, `TestBuildBedrockPayloadInjectsCachePointsFromModelName`, `TestBuildBedrockPayloadFallsBackToFixedBudgetByModelName` | covered |
| 110 | uses adaptive thinking when model.name contains the model name but ARN does not | `TestBuildBedrockAdditionalModelRequestFieldsThinking`, `TestBuildBedrockPayloadInjectsCachePointsFromModelName`, `TestBuildBedrockPayloadFallsBackToFixedBudgetByModelName` | covered |
| 123 | injects cache points when model.name identifies a supported Claude model | `TestBuildBedrockAdditionalModelRequestFieldsThinking`, `TestBuildBedrockPayloadInjectsCachePointsFromModelName`, `TestBuildBedrockPayloadFallsBackToFixedBudgetByModelName` | covered |
| 161 | falls back to fixed-budget thinking for non-adaptive Claude via model.name | `TestBuildBedrockAdditionalModelRequestFieldsThinking`, `TestBuildBedrockPayloadInjectsCachePointsFromModelName`, `TestBuildBedrockPayloadFallsBackToFixedBudgetByModelName` | covered |

## `packages/ai/test/cache-retention.test.ts`

Pi cases: `15`

Mapped Gi files: `gi-llm-provider/anthropic_payload_test.go`, `gi-llm-provider/openai_completions_payload_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 27 | should use default cache TTL (no ttl field) when PI_CACHE_RETENTION is not set | `TestBuildAnthropicPayloadCacheRetention`, `TestBuildOpenAICompletionsPayloadPromptCacheRetention` | contract-covered; Pi credential-gated |
| 50 | should use 1h cache TTL when PI_CACHE_RETENTION=long | `TestBuildAnthropicPayloadCacheRetention`, `TestBuildOpenAICompletionsPayloadPromptCacheRetention` | contract-covered; Pi credential-gated |
| 72 | should add ttl for non-api.anthropic.com baseUrl by default | `TestBuildAnthropicPayloadCacheRetention` | covered |
| 112 | should omit ttl when supportsLongCacheRetention is false | `TestBuildAnthropicPayloadCacheRetention`, `TestBuildOpenAICompletionsPayloadPromptCacheRetention` | covered |
| 143 | should omit cache_control when cacheRetention is none | `TestBuildAnthropicPayloadCacheRetention`, `TestBuildOpenAICompletionsPayloadAppliesAnthropicCacheControl`, `TestBuildOpenAICompletionsPayloadPromptCacheRetention` | covered |
| 169 | should add cache_control to string user messages | `TestBuildAnthropicPayloadCacheRetention`, `TestBuildOpenAICompletionsPayloadAppliesAnthropicCacheControl`, `TestBuildOpenAICompletionsPayloadPromptCacheRetention` | covered |
| 197 | should set 1h cache TTL when cacheRetention is long | `TestBuildAnthropicPayloadCacheRetention`, `TestBuildOpenAICompletionsPayloadPromptCacheRetention` | covered |
| 226 | should not set prompt_cache_retention when PI_CACHE_RETENTION is not set | `TestBuildOpenAICompletionsPayloadPromptCacheRetention` | contract-covered; Pi credential-gated |
| 247 | should set prompt_cache_retention to 24h when PI_CACHE_RETENTION=long | `TestBuildOpenAICompletionsPayloadPromptCacheRetention` | contract-covered; Pi credential-gated |
| 270 | should set prompt_cache_retention for non-api.openai.com baseUrl by default | `TestBuildOpenAICompletionsPayloadPromptCacheRetention` | covered |
| 304 | should omit prompt_cache_retention when supportsLongCacheRetention is false | `TestBuildOpenAICompletionsPayloadPromptCacheRetention` | covered |
| 334 | should omit prompt_cache_key when cacheRetention is none | `TestBuildOpenAICompletionsPayloadPromptCacheRetention` | covered |
| 362 | should set prompt_cache_retention when cacheRetention is long | `TestBuildOpenAICompletionsPayloadPromptCacheRetention` | covered |
| 408 | should set prompt_cache_retention for non-api.openai.com baseUrl by default | `TestBuildOpenAICompletionsPayloadPromptCacheRetention` | covered |
| 434 | should omit prompt_cache_retention when supportsLongCacheRetention is false | `TestBuildOpenAICompletionsPayloadPromptCacheRetention` | covered |

## `packages/ai/test/context-overflow.test.ts`

Pi cases: `32`

Mapped Gi files: `gi-llm-provider/overflow_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 98 | claude-haiku-4-5 - should detect overflow via isContextOverflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 110 | claude-sonnet-4 - should detect overflow via isContextOverflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 128 | gpt-4o - should detect overflow via isContextOverflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | contract-covered; Pi credential-gated |
| 143 | claude-sonnet-4 - should detect overflow via isContextOverflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | contract-covered; Pi credential-gated |
| 164 | gpt-4o-mini - should detect overflow via isContextOverflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 177 | gpt-4o - should detect overflow via isContextOverflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 189 | gpt-4o-mini - should detect overflow via isContextOverflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 206 | gemini-2.0-flash - should detect overflow via isContextOverflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 230 | gpt-5.2-codex - should detect overflow via isContextOverflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | contract-covered; Pi credential-gated |
| 250 | claude-sonnet-4-5 - should detect overflow via isContextOverflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 266 | grok-3-fast - should detect overflow via isContextOverflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 283 | llama-3.3-70b-versatile - should detect overflow via isContextOverflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 300 | qwen-3-235b - should detect overflow via isContextOverflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 318 | Kimi-K2.5 - should detect overflow via isContextOverflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 334 | Kimi-K2.6 - should detect overflow via isContextOverflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 351 | glm-4.5-air - should detect overflow via isContextOverflow when z.ai reports it | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 381 | devstral-medium-latest - should detect overflow via isContextOverflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 398 | MiniMax-M2.7 - should detect overflow via isContextOverflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 416 | mimo-v2.5-pro - should detect overflow via isContextOverflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 428 | mimo-v2.5-pro - should detect overflow via isContextOverflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 440 | mimo-v2.5-pro - should detect overflow via isContextOverflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 452 | mimo-v2.5-pro - should detect overflow via isContextOverflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 468 | kimi-k2-thinking - should detect overflow via isContextOverflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 483 | google/gemini-2.5-flash via AI Gateway - should detect overflow via isContextOverflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 500 | anthropic/claude-sonnet-4 via OpenRouter - should detect overflow via isContextOverflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 511 | deepseek/deepseek-v3.2 via OpenRouter - should detect overflow via isContextOverflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 522 | mistralai/mistral-large-2512 via OpenRouter - should detect overflow via isContextOverflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 533 | google/gemini-2.5-flash via OpenRouter - should detect overflow via isContextOverflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 544 | meta-llama/llama-4-scout via OpenRouter - should detect overflow via isContextOverflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 631 | gpt-oss:20b - should detect overflow via isContextOverflow (ollama silently truncates) | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 665 | should detect overflow via isContextOverflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 706 | should detect overflow via isContextOverflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |

## `packages/ai/test/cross-provider-handoff.test.ts`

Pi cases: `2`

Mapped Gi files: `gi-llm-provider/cross_provider_handoff_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 370 | should have at least 2 fixtures to test handoffs | `TestCrossProviderHandoffConvertersAcceptMixedHistory` | contract-covered; Pi credential-gated |
| 374 | should handle cross-provider handoffs for each target | `TestCrossProviderHandoffConvertersAcceptMixedHistory` | contract-covered; Pi credential-gated |

## `packages/ai/test/empty.test.ts`

Pi cases: `104`

Mapped Gi files: `gi-llm-provider/provider_contracts_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 150 | should handle empty content array | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 154 | should handle empty string content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 158 | should handle whitespace-only content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 162 | should handle empty assistant message in conversation | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 170 | should handle empty content array | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 174 | should handle empty string content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 178 | should handle whitespace-only content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 182 | should handle empty assistant message in conversation | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 190 | should handle empty content array | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 194 | should handle empty string content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 198 | should handle whitespace-only content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 202 | should handle empty assistant message in conversation | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 212 | should handle empty content array | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 216 | should handle empty string content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 220 | should handle whitespace-only content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 224 | should handle empty assistant message in conversation | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 232 | should handle empty content array | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 236 | should handle empty string content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 240 | should handle whitespace-only content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 244 | should handle empty assistant message in conversation | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 252 | should handle empty content array | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 256 | should handle empty string content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 260 | should handle whitespace-only content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 264 | should handle empty assistant message in conversation | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 272 | should handle empty content array | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 276 | should handle empty string content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 280 | should handle whitespace-only content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 284 | should handle empty assistant message in conversation | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 292 | should handle empty content array | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 296 | should handle empty string content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 300 | should handle whitespace-only content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 304 | should handle empty assistant message in conversation | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 312 | should handle empty content array | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 316 | should handle empty string content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 320 | should handle whitespace-only content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 324 | should handle empty assistant message in conversation | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 332 | should handle empty content array | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 336 | should handle empty string content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 340 | should handle whitespace-only content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 344 | should handle empty assistant message in conversation | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 352 | should handle empty content array | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 356 | should handle empty string content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 360 | should handle whitespace-only content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 364 | should handle empty assistant message in conversation | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 372 | should handle empty content array | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 376 | should handle empty string content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 380 | should handle whitespace-only content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 384 | should handle empty assistant message in conversation | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 392 | should handle empty content array | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 396 | should handle empty string content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 400 | should handle whitespace-only content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 404 | should handle empty assistant message in conversation | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 412 | should handle empty content array | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 416 | should handle empty string content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 420 | should handle whitespace-only content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 424 | should handle empty assistant message in conversation | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 432 | should handle empty content array | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 436 | should handle empty string content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 440 | should handle whitespace-only content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 444 | should handle empty assistant message in conversation | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 452 | should handle empty content array | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 456 | should handle empty string content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 460 | should handle whitespace-only content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 464 | should handle empty assistant message in conversation | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 474 | should handle empty content array | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 478 | should handle empty string content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 482 | should handle whitespace-only content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 486 | should handle empty assistant message in conversation | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 497 | should handle empty content array | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 501 | should handle empty string content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 505 | should handle whitespace-only content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 509 | should handle empty assistant message in conversation | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 520 | should handle empty content array | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 524 | should handle empty string content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 528 | should handle whitespace-only content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 532 | should handle empty assistant message in conversation | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 541 | should handle empty content array | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 545 | should handle empty string content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 549 | should handle whitespace-only content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 553 | should handle empty assistant message in conversation | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 561 | should handle empty content array | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 565 | should handle empty string content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 569 | should handle whitespace-only content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 573 | should handle empty assistant message in conversation | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 581 | should handle empty content array | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 585 | should handle empty string content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 589 | should handle whitespace-only content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 593 | should handle empty assistant message in conversation | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 605 | should handle empty content array | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | contract-covered; Pi credential-gated |
| 609 | should handle empty string content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | contract-covered; Pi credential-gated |
| 613 | should handle whitespace-only content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | contract-covered; Pi credential-gated |
| 621 | should handle empty assistant message in conversation | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | contract-covered; Pi credential-gated |
| 632 | gpt-4o - should handle empty content array | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | contract-covered; Pi credential-gated |
| 640 | gpt-4o - should handle empty string content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | contract-covered; Pi credential-gated |
| 649 | gpt-4o - should handle whitespace-only content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | contract-covered; Pi credential-gated |
| 658 | gpt-4o - should handle empty assistant message in conversation | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | contract-covered; Pi credential-gated |
| 667 | claude-sonnet-4 - should handle empty content array | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | contract-covered; Pi credential-gated |
| 676 | claude-sonnet-4 - should handle empty string content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | contract-covered; Pi credential-gated |
| 685 | claude-sonnet-4 - should handle whitespace-only content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | contract-covered; Pi credential-gated |
| 694 | claude-sonnet-4 - should handle empty assistant message in conversation | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | contract-covered; Pi credential-gated |
| 706 | gpt-5.2-codex - should handle empty content array | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | contract-covered; Pi credential-gated |
| 714 | gpt-5.2-codex - should handle empty string content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | contract-covered; Pi credential-gated |
| 723 | gpt-5.2-codex - should handle whitespace-only content | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | contract-covered; Pi credential-gated |
| 732 | gpt-5.2-codex - should handle empty assistant message in conversation | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | contract-covered; Pi credential-gated |

## `packages/ai/test/env-api-keys.test.ts`

Pi cases: `2`

Mapped Gi files: `gi-llm-provider/env_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 29 | does not treat generic GitHub tokens as GitHub Copilot credentials | `TestEnvironmentAPIKeysDoesNotTreatGenericGitHubTokensAsCopilotCredentials`, `TestEnvironmentAPIKeysResolvesCopilotToken` | covered |
| 37 | resolves GitHub Copilot credentials from COPILOT_GITHUB_TOKEN | `TestEnvironmentAPIKeysDoesNotTreatGenericGitHubTokensAsCopilotCredentials`, `TestEnvironmentAPIKeysResolvesCopilotToken` | covered |

## `packages/ai/test/faux-provider.test.ts`

Pi cases: `22`

Mapped Gi files: `gi-llm-provider/faux_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 31 | registers a custom provider and estimates usage | `TestFauxProviderRegistersCustomProviderAndEstimatesUsage` | covered |
| 48 | supports helper blocks for text, thinking, and tool calls | `TestFauxProviderFactoryErrorsBecomeTerminalErrorEvents`, `TestFauxProviderHelpersAndMultipleModels`, `TestFauxProviderPromptCaching` | covered |
| 69 | supports multiple models with per-model reasoning and model-aware factories | `TestFauxProviderHelpersAndMultipleModels` | covered |
| 98 | rewrites api, provider, and model on returned messages | `TestFauxProviderRewritesAPIProviderModel` | covered |
| 116 | consumes queued responses in order and errors when exhausted | `TestFauxProviderQueuedResponses` | covered |
| 137 | can replace and append queued responses | `TestFauxProviderQueuedResponses` | covered |
| 160 | supports async response factories | `TestFauxProviderFactoryErrorsBecomeTerminalErrorEvents`, `TestFauxProviderHelpersAndMultipleModels`, `TestFauxProviderPromptCaching` | covered |
| 174 | emits an error when a response factory throws | `TestFauxProviderFactoryErrorsBecomeTerminalErrorEvents` | covered |
| 195 | estimates prompt and output tokens from serialized context | `TestFauxProviderRegistersCustomProviderAndEstimatesUsage` | covered |
| 247 | does not share cache across sessions or requests without sessionId | `TestFauxProviderFactoryErrorsBecomeTerminalErrorEvents`, `TestFauxProviderHelpersAndMultipleModels`, `TestFauxProviderPromptCaching` | covered |
| 280 | simulates prompt caching per sessionId | `TestFauxProviderPromptCaching` | covered |
| 308 | does not simulate caching when cacheRetention is none | `TestFauxProviderPromptCaching` | covered |
| 328 | streams thinking, text, and partial tool call deltas | `TestFauxProviderStreamsContentEvents` | covered |
| 363 | streams an exact event order for fixed-size chunks | `TestFauxProviderStreamsContentEvents` | covered |
| 391 | streams multiple tool calls in one message | `TestFauxProviderStreamsContentEvents` | covered |
| 412 | streams an explicit assistant error message as a terminal error | `TestFauxProviderStreamsContentEvents`, `TestFauxProviderFactoryErrorsBecomeTerminalErrorEvents` | covered |
| 437 | streams an explicit assistant aborted message as a terminal error | `TestFauxProviderStreamsContentEvents`, `TestFauxProviderFactoryErrorsBecomeTerminalErrorEvents` | covered |
| 462 | supports aborting before the first chunk | `TestFauxProviderFactoryErrorsBecomeTerminalErrorEvents`, `TestFauxProviderHelpersAndMultipleModels`, `TestFauxProviderPromptCaching` | covered |
| 485 | supports aborting mid-text stream when paced | `TestFauxProviderStreamsContentEvents` | covered |
| 513 | supports aborting mid-thinking stream when paced | `TestFauxProviderStreamsContentEvents` | covered |
| 546 | supports aborting mid-toolcall stream when paced | `TestFauxProviderStreamsContentEvents` | covered |
| 587 | unregisters the provider | `TestFauxProviderFactoryErrorsBecomeTerminalErrorEvents`, `TestFauxProviderHelpersAndMultipleModels`, `TestFauxProviderPromptCaching` | covered |

## `packages/ai/test/fireworks-models.test.ts`

Pi cases: `11`

Mapped Gi files: `gi-llm-provider/model_catalog_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 21 | registers the default Kimi K2.6 model via Anthropic-compatible Messages API | `TestFireworksAnthropicHeadersAndToolCompat` | covered |
| 39 | registers the Fire Pass turbo router model | `TestFireworksAndTogetherEnvKeys`, `TestFireworksAnthropicHeadersAndToolCompat`, `TestFireworksModelCatalog` | covered |
| 48 | resolves FIREWORKS_API_KEY from the environment | `TestFireworksAndTogetherEnvKeys` | covered |
| 55 | sets Fireworks-specific compat for session affinity and unsupported tool fields | `TestFireworksAnthropicHeadersAndToolCompat` | covered |
| 183 | sends x-session-affinity header for Fireworks models | `TestFireworksAnthropicHeadersAndToolCompat` | covered |
| 192 | omits x-session-affinity header for native Anthropic models | `TestFireworksAnthropicHeadersAndToolCompat` | covered |
| 201 | omits x-session-affinity header when cacheRetention is none | `TestFireworksAnthropicHeadersAndToolCompat` | covered |
| 211 | omits cache_control on tools for Fireworks models | `TestFireworksAndTogetherEnvKeys`, `TestFireworksAnthropicHeadersAndToolCompat`, `TestFireworksModelCatalog` | covered |
| 220 | omits eager_input_streaming on tools for Fireworks models | `TestFireworksAndTogetherEnvKeys`, `TestFireworksAnthropicHeadersAndToolCompat`, `TestFireworksModelCatalog` | covered |
| 230 | sends cache_control on tools for native Anthropic models | `TestFireworksAnthropicHeadersAndToolCompat` | covered |
| 240 | sends eager_input_streaming on tools for native Anthropic models | `TestFireworksAnthropicHeadersAndToolCompat` | covered |

## `packages/ai/test/github-copilot-anthropic.test.ts`

Pi cases: `2`

Mapped Gi files: `gi-llm-provider/github_copilot_headers_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 56 | uses Bearer auth, Copilot headers, and valid Anthropic Messages payload | `TestCopilotClaudeAnthropicHeadersAndPayload`, `TestCopilotDynamicHeadersAgentAndVision`, `TestCopilotAdaptiveThinkingOmitsInterleavedBeta` | covered |
| 93 | omits interleaved-thinking beta for adaptive-thinking models | `TestCopilotClaudeAnthropicHeadersAndPayload`, `TestCopilotDynamicHeadersAgentAndVision`, `TestCopilotAdaptiveThinkingOmitsInterleavedBeta` | covered |

## `packages/ai/test/github-copilot-oauth.test.ts`

Pi cases: `2`

Mapped Gi files: `gi-llm-provider/oauth_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 31 | waits before the first poll and increases the safety margin after slow_down | `TestGitHubCopilotPollScheduleFinalPollBeforeSlowDownTimeout`, `TestGitHubCopilotPollScheduleSlowDown` | covered |
| 130 | uses the remaining lifetime for a final poll before timing out after repeated slow_down responses | `TestGitHubCopilotPollScheduleFinalPollBeforeSlowDownTimeout` | covered |

## `packages/ai/test/google-shared-convert-tools.test.ts`

Pi cases: `7`

Mapped Gi files: `gi-llm-provider/google_convert_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 14 | strips JSON Schema meta keys from parameters when useParameters=true | `TestConvertGoogleToolsReturnsNilForEmptyAndUsesParametersMode` | covered |
| 51 | recursively strips nested JSON Schema meta keys | `TestConvertGoogleToolsReturnsNilForEmptyAndUsesParametersMode`, `TestSanitizeSchemaForOpenAPIStripsMetaKeysRecursively` | covered |
| 80 | preserves $ref while stripping meta keys | `TestConvertGoogleToolsReturnsNilForEmptyAndUsesParametersMode` | covered |
| 109 | does not mutate the original Tool.parameters object | `TestConvertGoogleToolsReturnsNilForEmptyAndUsesParametersMode` | covered |
| 132 | preserves $schema in parametersJsonSchema when useParameters=false | `TestConvertGoogleToolsReturnsNilForEmptyAndUsesParametersMode` | covered |
| 158 | handles tools without $schema gracefully | `TestConvertGoogleToolsReturnsNilForEmptyAndUsesParametersMode` | covered |
| 182 | returns undefined for empty tool list | `TestConvertGoogleToolsReturnsNilForEmptyAndUsesParametersMode` | covered |

## `packages/ai/test/google-shared-gemini3-unsigned-tool-call.test.ts`

Pi cases: `4`

Mapped Gi files: `gi-llm-provider/google_convert_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 65 | does not add skip_thought_signature_validator for unsigned Google Gen AI tool calls | `TestConvertGoogleMessagesGemini3UnsignedToolCalls` | covered |
| 82 | does not add skip_thought_signature_validator for unsigned Vertex tool calls | `TestConvertGoogleMessagesGemini3UnsignedToolCalls` | covered |
| 94 | preserves valid thoughtSignature when present for the same provider and model | `TestConvertGoogleMessagesPreservesValidThoughtSignatureForSameModel` | covered |
| 106 | does not add a thoughtSignature for non-Gemini-3 models | `TestConvertGoogleMessagesGemini3UnsignedToolCalls` | covered |

## `packages/ai/test/google-shared-image-tool-result-routing.test.ts`

Pi cases: `2`

Mapped Gi files: `gi-llm-provider/google_convert_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 79 | keeps separate synthetic image turn for Gemini 2.x Google API models | `TestConvertGoogleMessagesImageToolResultRouting` | covered |
| 89 | nests image tool results for Gemini 3 Google API models | `TestConvertGoogleMessagesImageToolResultRouting` | covered |

## `packages/ai/test/google-thinking-disable.test.ts`

Pi cases: `9`

Mapped Gi files: `gi-llm-provider/google_convert_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 94 | disables thinking for budget-based reasoning models | `TestGoogleThinkingConfigDisableAndBudgets` | covered |
| 99 | disables thinking for adaptive reasoning models | `TestGoogleThinkingConfigDisableAndBudgets` | covered |
| 108 | disables thinking for Gemini 2.5 | `TestGoogleThinkingConfigDisableAndBudgets` | covered |
| 111 | disables thinking for Gemini 3.x | `TestGoogleThinkingConfigDisableAndBudgets` | covered |
| 115 | does not error when thinking is off for Gemini 3.1 Pro | `TestGoogleThinkingConfigDisableAndBudgets` | covered |
| 133 | disables thinking for Gemini 2.5 | `TestGoogleThinkingConfigDisableAndBudgets` | contract-covered; Pi credential-gated |
| 139 | disables thinking for Gemini 3.x | `TestGoogleThinkingConfigDisableAndBudgets` | contract-covered; Pi credential-gated |
| 148 | disables thinking for Responses reasoning models | `TestGoogleThinkingConfigDisableAndBudgets` | covered |
| 156 | disables thinking for Qwen 3.5 reasoning models | `TestGoogleThinkingConfigDisableAndBudgets` | covered |

## `packages/ai/test/google-thinking-signature.test.ts`

Pi cases: `5`

Mapped Gi files: `gi-llm-provider/google_convert_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 5 | treats part.thought === true as thinking | `TestGoogleThinkingDetectionAndSignatureRetention` | covered |
| 9 | does not treat thoughtSignature alone as thinking | `TestGoogleThinkingDetectionAndSignatureRetention` | covered |
| 17 | does not treat empty/missing signatures as thinking if thought is not set | `TestGoogleThinkingDetectionAndSignatureRetention` | covered |
| 22 | preserves the existing signature when subsequent deltas omit thoughtSignature | `TestGoogleThinkingDetectionAndSignatureRetention` | covered |
| 33 | updates the signature when a new non-empty signature arrives | `TestGoogleThinkingDetectionAndSignatureRetention` | covered |

## `packages/ai/test/google-vertex-api-key-resolution.test.ts`

Pi cases: `8`

Mapped Gi files: `gi-llm-provider/config_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 73 | falls back to ADC when options.apiKey is a placeholder marker | `TestResolveGoogleVertexClientConfig`, `TestResolveGoogleVertexCustomBaseURL` | covered |
| 91 | falls back to ADC when options.apiKey is the gcp-vertex-credentials marker | `TestResolveGoogleVertexClientConfig`, `TestResolveGoogleVertexCustomBaseURL` | covered |
| 110 | falls back to ADC when GOOGLE_CLOUD_API_KEY is a placeholder marker | `TestResolveGoogleVertexClientConfig`, `TestResolveGoogleVertexCustomBaseURL` | covered |
| 130 | still uses the API key client for real API keys | `TestResolveGoogleVertexClientConfig` | covered |
| 147 | does not forward generated Vertex base URL placeholders | `TestResolveGoogleVertexCustomBaseURL` | covered |
| 159 | forwards custom baseUrl to the ADC client | `TestResolveGoogleVertexCustomBaseURL` | covered |
| 181 | forwards custom baseUrl to the API key client | `TestResolveGoogleVertexCustomBaseURL` | covered |
| 201 | does not append apiVersion when custom baseUrl already includes one | `TestResolveGoogleVertexCustomBaseURL` | covered |

## `packages/ai/test/image-tool-result.test.ts`

Pi cases: `38`

Mapped Gi files: `gi-llm-provider/anthropic_payload_test.go`, `gi-llm-provider/google_convert_test.go`, `gi-llm-provider/openai_completions_convert_test.go`, `gi-llm-provider/openai_responses_convert_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 212 | should handle tool result with only image | `TestConvertGoogleMessagesImageToolResultRouting` | covered |
| 216 | should handle tool result with text and image | `TestConvertGoogleMessagesImageToolResultRouting` | covered |
| 229 | should handle tool result with only image | `TestConvertGoogleMessagesImageToolResultRouting` | covered |
| 233 | should handle tool result with text and image | `TestConvertGoogleMessagesImageToolResultRouting` | covered |
| 241 | should handle tool result with only image | `TestConvertGoogleMessagesImageToolResultRouting` | covered |
| 245 | should handle tool result with text and image | `TestConvertGoogleMessagesImageToolResultRouting` | covered |
| 255 | should handle tool result with only image | `TestConvertGoogleMessagesImageToolResultRouting` | covered |
| 259 | should handle tool result with text and image | `TestConvertGoogleMessagesImageToolResultRouting` | covered |
| 267 | should handle tool result with only image | `TestConvertGoogleMessagesImageToolResultRouting` | covered |
| 271 | should handle tool result with text and image | `TestConvertGoogleMessagesImageToolResultRouting` | covered |
| 279 | should handle tool result with only image | `TestConvertGoogleMessagesImageToolResultRouting` | covered |
| 283 | should handle tool result with text and image | `TestConvertGoogleMessagesImageToolResultRouting` | covered |
| 291 | should handle tool result with only image | `TestConvertGoogleMessagesImageToolResultRouting` | covered |
| 295 | should handle tool result with text and image | `TestConvertGoogleMessagesImageToolResultRouting` | covered |
| 304 | should handle tool result with only image | `TestConvertGoogleMessagesImageToolResultRouting` | covered |
| 308 | should handle tool result with text and image | `TestConvertGoogleMessagesImageToolResultRouting` | covered |
| 316 | should handle tool result with only image | `TestConvertGoogleMessagesImageToolResultRouting` | covered |
| 328 | should handle tool result with text and image | `TestConvertGoogleMessagesImageToolResultRouting` | contract-covered; Pi skipped |
| 337 | should handle tool result with only image | `TestConvertGoogleMessagesImageToolResultRouting` | covered |
| 344 | should handle tool result with text and image | `TestConvertGoogleMessagesImageToolResultRouting` | contract-covered; Pi skipped |
| 354 | should handle tool result with only image | `TestConvertGoogleMessagesImageToolResultRouting` | covered |
| 361 | should handle tool result with text and image | `TestConvertGoogleMessagesImageToolResultRouting` | contract-covered; Pi skipped |
| 371 | should handle tool result with only image | `TestConvertGoogleMessagesImageToolResultRouting` | covered |
| 378 | should handle tool result with text and image | `TestConvertGoogleMessagesImageToolResultRouting` | contract-covered; Pi skipped |
| 386 | should handle tool result with only image | `TestConvertGoogleMessagesImageToolResultRouting` | covered |
| 390 | should handle tool result with text and image | `TestConvertGoogleMessagesImageToolResultRouting` | covered |
| 398 | should handle tool result with only image | `TestConvertGoogleMessagesImageToolResultRouting` | covered |
| 402 | should handle tool result with text and image | `TestConvertGoogleMessagesImageToolResultRouting` | covered |
| 410 | should handle tool result with only image | `TestConvertGoogleMessagesImageToolResultRouting` | covered |
| 414 | should handle tool result with text and image | `TestConvertGoogleMessagesImageToolResultRouting` | covered |
| 426 | should handle tool result with only image | `TestConvertGoogleMessagesImageToolResultRouting` | contract-covered; Pi credential-gated |
| 434 | should handle tool result with text and image | `TestConvertGoogleMessagesImageToolResultRouting` | contract-covered; Pi credential-gated |
| 445 | gpt-4o - should handle tool result with only image | `TestConvertGoogleMessagesImageToolResultRouting` | contract-covered; Pi credential-gated |
| 453 | gpt-4o - should handle tool result with text and image | `TestConvertGoogleMessagesImageToolResultRouting` | contract-covered; Pi credential-gated |
| 462 | claude-sonnet-4 - should handle tool result with only image | `TestConvertGoogleMessagesImageToolResultRouting` | contract-covered; Pi credential-gated |
| 471 | claude-sonnet-4 - should handle tool result with text and image | `TestConvertGoogleMessagesImageToolResultRouting` | contract-covered; Pi credential-gated |
| 483 | gpt-5.2-codex - should handle tool result with only image | `TestConvertGoogleMessagesImageToolResultRouting` | contract-covered; Pi credential-gated |
| 491 | gpt-5.2-codex - should handle tool result with text and image | `TestConvertGoogleMessagesImageToolResultRouting` | contract-covered; Pi credential-gated |

## `packages/ai/test/images.test.ts`

Pi cases: `3`

Mapped Gi files: `gi-llm-provider/openrouter_images_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 76 | should generate a basic image | `TestBuildOpenRouterImagesPayloadImageOnlyOutput`, `TestGenerateImagesHandlesImmediateContextCancel` | covered |
| 80 | should handle text plus image output | `TestBuildOpenRouterImagesPayloadImageOnlyOutput` | covered |
| 84 | should handle image input | `TestBuildOpenRouterImagesPayloadImageOnlyOutput` | covered |

## `packages/ai/test/interleaved-thinking.test.ts`

Pi cases: `4`

Mapped Gi files: `gi-llm-provider/anthropic_payload_test.go`, `gi-llm-provider/bedrock_payload_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 124 | should do interleaved thinking on Claude Opus 4.5 | `TestBuildAnthropicHeadersInterleavedThinking` | covered |
| 128 | should do interleaved thinking on Claude Opus 4.6 | `TestBuildAnthropicHeadersInterleavedThinking` | covered |
| 136 | should do interleaved thinking on Claude Opus 4.5 | `TestBuildAnthropicHeadersInterleavedThinking` | covered |
| 140 | should do interleaved thinking on Claude Opus 4.6 | `TestBuildAnthropicHeadersInterleavedThinking` | covered |

## `packages/ai/test/lazy-module-load.test.ts`

Pi cases: `3`

N/A in Go: providers are explicit registry entries; there is no SDK module lazy-loading boundary.

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 67 | does not load provider SDKs when importing the root barrel | N/A in Go: providers are explicit registry entries; there is no SDK module lazy-loading boundary. | not applicable |
| 71 | loads only the Anthropic SDK when calling the root lazy wrapper | N/A in Go: providers are explicit registry entries; there is no SDK module lazy-loading boundary. | not applicable |
| 92 | loads only the Anthropic SDK when dispatching through streamSimple | N/A in Go: providers are explicit registry entries; there is no SDK module lazy-loading boundary. | not applicable |

## `packages/ai/test/mistral-reasoning-mode.test.ts`

Pi cases: `5`

Mapped Gi files: `gi-llm-provider/mistral_payload_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 46 | uses reasoning_effort for Mistral Small 4 | `TestBuildMistralPayloadReasoningModeSelection`, `TestConvertMistralToolsSerializesPlainJSONSchema` | covered |
| 52 | omits reasoning controls for Mistral Small 4 when thinking is off | `TestBuildMistralPayloadReasoningModeSelection`, `TestConvertMistralToolsSerializesPlainJSONSchema` | covered |
| 59 | uses prompt_mode for Magistral reasoning models | `TestBuildMistralPayloadReasoningModeSelection`, `TestConvertMistralToolsSerializesPlainJSONSchema` | covered |
| 66 | uses reasoning_effort for Mistral Medium 3.5 | `TestBuildMistralPayloadReasoningModeSelection`, `TestConvertMistralToolsSerializesPlainJSONSchema` | covered |
| 73 | omits reasoning controls for Mistral Medium 3.5 when thinking is off | `TestBuildMistralPayloadReasoningModeSelection`, `TestConvertMistralToolsSerializesPlainJSONSchema` | covered |

## `packages/ai/test/mistral-tool-schema.test.ts`

Pi cases: `1`

Mapped Gi files: `gi-llm-provider/mistral_payload_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 18 | strips TypeBox symbol keys before the SDK validates tool schemas | `TestBuildMistralPayloadReasoningModeSelection`, `TestConvertMistralToolsSerializesPlainJSONSchema` | covered |

## `packages/ai/test/node-http-proxy.test.ts`

Pi cases: `3`

Mapped Gi files: `gi-llm-provider/config_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 40 | respects NO_PROXY exclusions | `TestNormalizeAzureOpenAIBaseURL`, `TestResolveAzureOpenAIConfigBuildsDefaultFromResourceName`, `TestResolveBedrockClientConfig` | covered |
| 47 | resolves HTTP and HTTPS proxy URLs | `TestNormalizeAzureOpenAIBaseURL`, `TestResolveAzureOpenAIConfigBuildsDefaultFromResourceName`, `TestResolveBedrockClientConfig` | covered |
| 56 | rejects SOCKS and PAC proxy URLs explicitly | `TestNormalizeAzureOpenAIBaseURL`, `TestResolveAzureOpenAIConfigBuildsDefaultFromResourceName`, `TestResolveBedrockClientConfig` | covered |

## `packages/ai/test/openai-codex-cache-affinity-e2e.test.ts`

Pi cases: `1`

Mapped Gi files: `gi-llm-provider/openai_codex_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 10 | handles SSE requests with aligned cache-affinity identifiers | `TestOpenAICodexCacheAffinityE2EContract` | contract-covered; Pi credential-gated |

## `packages/ai/test/openai-codex-oauth.test.ts`

Pi cases: `1`

Mapped Gi files: `gi-llm-provider/oauth_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 9 | does not write token refresh failures to stderr | `TestOpenAICodexRefreshErrorMessage` | covered |

## `packages/ai/test/openai-codex-stream.test.ts`

Pi cases: `12`

Mapped Gi files: `gi-llm-provider/openai_codex_test.go`, `gi-llm-provider/openai_responses_stream_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 83 | streams SSE responses into AssistantMessageEventStream | `TestProcessOpenAICodexStreamEventsTextCompletedAndIncomplete` | covered |
| 193 | completes after response.completed even when the SSE body stays open | `TestProcessOpenAICodexStreamEventsTextCompletedAndIncomplete` | covered |
| 253 | maps response.incomplete to stopReason length even when the SSE body stays open | `TestProcessOpenAICodexStreamEventsTextCompletedAndIncomplete` | covered |
| 313 | sets session_id/x-client-request-id headers and prompt_cache_key when sessionId is provided | `TestBuildOpenAICodexPayloadAndHeaders`, `TestProcessOpenAICodexStreamEventsTextCompletedAndIncomplete` | covered |
| 413 | preserves gpt-5.5 xhigh reasoning effort from simple options | `TestProcessOpenAICodexStreamEventsTextCompletedAndIncomplete` | covered |
| 470 | clamps %s minimal reasoning effort to low | `TestProcessOpenAICodexStreamEventsTextCompletedAndIncomplete` | covered |
| 566 | uses the client-sent %s service tier for %s when Codex echoes default | `TestProcessOpenAICodexStreamEventsTextCompletedAndIncomplete`, `TestOpenAICodexServiceTierPricing` | covered |
| 661 | does not set session_id/x-client-request-id headers when sessionId is not provided | `TestBuildOpenAICodexPayloadAndHeaders`, `TestProcessOpenAICodexStreamEventsTextCompletedAndIncomplete` | covered |
| 757 | forwards auto transport from streamSimple options and uses cached websocket context | `TestProcessOpenAICodexStreamEventsTextCompletedAndIncomplete` | covered |
| 864 | sends only response input deltas in websocket-cached mode | `TestProcessOpenAICodexStreamEventsTextCompletedAndIncomplete` | covered |
| 1011 | uses %s for SSE retries | `TestProcessOpenAICodexStreamEventsTextCompletedAndIncomplete` | covered |
| 1081 | uses exponential backoff across repeated SSE retries without retry headers | `TestBuildOpenAICodexPayloadAndHeaders`, `TestProcessOpenAICodexStreamEventsTextCompletedAndIncomplete` | covered |

## `packages/ai/test/openai-completions-cache-control-format.test.ts`

Pi cases: `3`

Mapped Gi files: `gi-llm-provider/openai_completions_payload_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 128 | applies Anthropic-style cache markers when model compat enables them | `TestBuildOpenAICompletionsPayloadAppliesAnthropicCacheControl` | covered |
| 154 | preserves Anthropic-style cache markers for OpenRouter Anthropic models | `TestBuildOpenAICompletionsPayloadAppliesAnthropicCacheControl` | covered |
| 160 | omits Anthropic-style cache markers when cacheRetention is none | `TestBuildOpenAICompletionsPayloadAppliesAnthropicCacheControl` | covered |

## `packages/ai/test/openai-completions-empty-tools.test.ts`

Pi cases: `6`

Mapped Gi files: `gi-llm-provider/openai_completions_convert_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 62 | omits tools field when context.tools is an empty array | `TestConvertOpenAICompletionsMessagesOmitsEmptyAssistantMessages` | covered |
| 79 | omits tools field when context.tools is undefined | `TestConvertOpenAICompletionsMessagesOmitsEmptyAssistantMessages` | covered |
| 95 | uses conservative OpenAI-compatible fields for Cloudflare AI Gateway /compat models | `TestConvertOpenAICompletionsMessagesOmitsEmptyAssistantMessages` | covered |
| 131 | preserves inline upstream Authorization for Cloudflare AI Gateway BYOK requests | `TestConvertOpenAICompletionsMessagesOmitsEmptyAssistantMessages` | covered |
| 149 | sends session affinity headers for Workers AI through Cloudflare AI Gateway | `TestConvertOpenAICompletionsMessagesOmitsEmptyAssistantMessages` | covered |
| 168 | still emits tools: [] for Anthropic/LiteLLM proxy when conversation has tool history | `TestConvertOpenAICompletionsMessagesOmitsEmptyAssistantMessages` | covered |

## `packages/ai/test/openai-completions-prompt-cache.test.ts`

Pi cases: `8`

Mapped Gi files: `gi-llm-provider/openai_completions_payload_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 113 | sets prompt_cache_key for direct OpenAI requests when caching is enabled | `TestBuildOpenAICompletionsPayloadPromptCacheRetention` | covered |
| 120 | sets prompt_cache_retention to 24h for direct OpenAI requests when cacheRetention is long | `TestBuildOpenAICompletionsPayloadPromptCacheRetention` | covered |
| 127 | omits prompt cache fields when cacheRetention is none | `TestBuildOpenAICompletionsPayloadPromptCacheRetention` | covered |
| 134 | omits prompt cache fields for non-OpenAI base URLs without compatible long retention | `TestBuildOpenAICompletionsPayloadPromptCacheRetention` | covered |
| 145 | uses PI_CACHE_RETENTION for direct OpenAI requests | `TestBuildOpenAICompletionsPayloadPromptCacheRetention` | covered |
| 153 | sends known session-affinity headers when compat.sendSessionAffinityHeaders is enabled | `TestBuildOpenAICompletionsHeadersSessionAffinity` | covered |
| 165 | omits session-affinity headers when cacheRetention is none | `TestBuildOpenAICompletionsHeadersSessionAffinity` | covered |
| 177 | lets explicit headers override generated session-affinity headers | `TestBuildOpenAICompletionsHeadersSessionAffinity` | covered |

## `packages/ai/test/openai-completions-response-model.test.ts`

Pi cases: `3`

Mapped Gi files: `gi-llm-provider/openai_completions_stream_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 60 | surfaces routed chunk.model on responseModel without changing model | `TestProcessOpenAICompletionsChunksIgnoresNilAndCapturesResponseID` | covered |
| 88 | leaves responseModel undefined when chunks echo the requested id | `TestProcessOpenAICompletionsChunksIgnoresNilAndCapturesResponseID` | covered |
| 114 | ignores empty or missing chunk.model | `TestProcessOpenAICompletionsChunksIgnoresNilAndCapturesResponseID` | covered |

## `packages/ai/test/openai-completions-thinking-as-text.test.ts`

Pi cases: `3`

Mapped Gi files: `gi-llm-provider/openai_completions_payload_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 104 | serializes same-model thinking-plus-text replay as assistant text parts | `TestConvertOpenAICompletionsMessagesThinkingAsText` | covered |
| 125 | serializes same-model thinking-only replay as assistant text parts | `TestConvertOpenAICompletionsMessagesThinkingAsText` | covered |
| 138 | reaches the endpoint when replay contains both thinking and text | `TestConvertOpenAICompletionsMessagesThinkingAsText` | covered |

## `packages/ai/test/openai-completions-tool-choice.test.ts`

Pi cases: `18`

Mapped Gi files: `gi-llm-provider/openai_completions_payload_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 71 | forwards toolChoice from simple options to payload | `TestBuildOpenAICompletionsPayloadToolChoiceAndStrictMode` | covered |
| 112 | omits strict when compat disables strict mode | `TestBuildOpenAICompletionsPayloadToolChoiceAndStrictMode` | covered |
| 157 | maps groq qwen3 reasoning levels to default reasoning_effort | `TestBuildOpenAICompletionsPayloadReasoningAndZAIToolStream`, `TestBuildOpenAICompletionsPayloadToolChoiceAndStrictMode` | covered |
| 185 | keeps normal reasoning_effort for groq models without compat mapping | `TestBuildOpenAICompletionsPayloadReasoningAndZAIToolStream`, `TestBuildOpenAICompletionsPayloadToolChoiceAndStrictMode` | covered |
| 213 | enables tool_stream for supported z.ai models with tools | `TestBuildOpenAICompletionsPayloadReasoningAndZAIToolStream`, `TestOpenAICompletionsResolvedZAIToolStreamCompat` | covered |
| 250 | stores z.ai tool_stream support in model compat metadata | `TestOpenAICompletionsResolvedZAIToolStreamCompat` | covered |
| 258 | omits tool_stream for unsupported z.ai models | `TestBuildOpenAICompletionsPayloadReasoningAndZAIToolStream`, `TestOpenAICompletionsResolvedZAIToolStreamCompat` | covered |
| 295 | respects explicit z.ai tool_stream compat override | `TestOpenAICompletionsResolvedZAIToolStreamCompat` | covered |
| 339 | omits tool_stream when no tools are provided | `TestBuildOpenAICompletionsPayloadReasoningAndZAIToolStream`, `TestOpenAICompletionsResolvedZAIToolStreamCompat` | covered |
| 366 | maps non-standard provider finish_reason values to stopReason error | `TestBuildOpenAICompletionsPayloadToolChoiceAndStrictMode` | covered |
| 401 | ignores null stream chunks from openai-compatible providers | `TestBuildOpenAICompletionsPayloadReasoningAndZAIToolStream`, `TestOpenAICompletionsResolvedZAIToolStreamCompat` | covered |
| 443 | errors when a stream ends after only null finish_reason chunks | `TestBuildOpenAICompletionsPayloadReasoningAndZAIToolStream`, `TestOpenAICompletionsResolvedZAIToolStreamCompat` | covered |
| 475 | coalesces tool call deltas by stable index when provider mutates ids mid-stream | `TestBuildOpenAICompletionsPayloadReasoningAndZAIToolStream`, `TestOpenAICompletionsResolvedZAIToolStreamCompat` | covered |
| 586 | accumulates mixed content, reasoning, and parallel tool call deltas independently | `TestBuildOpenAICompletionsPayloadReasoningAndZAIToolStream` | covered |
| 818 | does not double-count reasoning tokens in completion usage | `TestBuildOpenAICompletionsPayloadReasoningAndZAIToolStream`, `TestBuildOpenAICompletionsPayloadToolChoiceAndStrictMode` | covered |
| 853 | preserves prompt_tokens_details cache read/write fields from chunk usage | `TestBuildOpenAICompletionsPayloadPromptCacheRetention`, `TestBuildOpenAICompletionsPayloadToolChoiceAndStrictMode` | covered |
| 894 | preserves prompt_tokens_details cache read/write fields from choice usage fallback | `TestBuildOpenAICompletionsPayloadPromptCacheRetention`, `TestBuildOpenAICompletionsPayloadToolChoiceAndStrictMode` | covered |
| 940 | uses OpenRouter reasoning object instead of reasoning_effort | `TestBuildOpenAICompletionsPayloadReasoningAndZAIToolStream`, `TestBuildOpenAICompletionsPayloadToolChoiceAndStrictMode` | covered |

## `packages/ai/test/openai-completions-tool-result-images.test.ts`

Pi cases: `1`

Mapped Gi files: `gi-llm-provider/openai_completions_convert_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 57 | batches tool-result images after consecutive tool results | `TestConvertOpenAICompletionsMessagesBatchesToolResultImages` | covered |

## `packages/ai/test/openai-responses-cache-affinity-e2e.test.ts`

Pi cases: `1`

Mapped Gi files: `gi-llm-provider/openai_responses_payload_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 7 | handles direct OpenAI Responses requests with aligned cache-affinity identifiers | `TestBuildOpenAIResponsesHeadersCacheAffinity` | covered |

## `packages/ai/test/openai-responses-copilot-provider.test.ts`

Pi cases: `9`

Mapped Gi files: `gi-llm-provider/openai_responses_payload_test.go`, `gi-llm-provider/openai_responses_stream_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 58 | omits reasoning when no reasoning is requested | `TestBuildOpenAIResponsesPayloadReasoningDefaults` | covered |
| 93 | sends none reasoning effort for OpenAI %s when no reasoning is requested | `TestBuildOpenAIResponsesPayloadReasoningDefaults` | covered |
| 130 | omits reasoning effort for OpenAI %s when off is unsupported | `TestBuildOpenAIResponsesPayloadReasoningDefaults` | covered |
| 167 | sets cache-affinity headers for official OpenAI Responses requests with a sessionId | `TestBuildOpenAIResponsesHeadersCacheAffinity` | covered |
| 173 | sets cache-affinity headers for proxy OpenAI Responses requests with a sessionId | `TestBuildOpenAIResponsesHeadersCacheAffinity` | covered |
| 184 | can omit the session_id header while preserving other cache-affinity headers | `TestBuildOpenAIResponsesHeadersCacheAffinity` | covered |
| 196 | lets explicit headers override the default OpenAI cache-affinity headers | `TestBuildOpenAIResponsesHeadersCacheAffinity` | covered |
| 208 | omits OpenAI cache-affinity headers when cacheRetention is none | `TestBuildOpenAIResponsesHeadersCacheAffinity` | covered |
| 214 | applies %s %s service-tier cost multiplier | `TestOpenAIResponsesServiceTierPricing` | covered |

## `packages/ai/test/openai-responses-foreign-toolcall-id.test.ts`

Pi cases: `1`

Mapped Gi files: `gi-llm-provider/openai_responses_convert_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 20 | hashes foreign Copilot tool item IDs into a bounded Codex-safe fc_<hash> shape | `TestConvertOpenAIResponsesMessagesHashesForeignToolItemID`, `TestConvertOpenAIResponsesMessagesHandlesEmptyAndImageToolOutputs` | covered |

## `packages/ai/test/openai-responses-partial-json-cleanup.test.ts`

Pi cases: `1`

Mapped Gi files: `gi-llm-provider/openai_responses_stream_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 63 | removes partialJson from persisted tool-call blocks at output_item.done | `TestProcessOpenAIResponsesStreamCleansToolCallScratchState`, `TestConvertOpenAIResponsesMessagesKeepsToolResultImagesInsideFunctionOutput`, `TestProcessOpenAIResponsesStreamCapturesResponseID` | covered |

## `packages/ai/test/openai-responses-reasoning-replay-e2e.test.ts`

Pi cases: `3`

Mapped Gi files: `gi-llm-provider/openai_responses_replay_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 20 | skips reasoning-only history after an aborted turn | `TestOpenAIResponsesReasoningReplaySkipsAbortedReasoningOnlyHistory`, `TestOpenAIResponsesReasoningReplaySameProviderDifferentModelHandoff`, `TestOpenAIResponsesReasoningReplayCrossProviderHandoff` | covered |
| 83 | handles same-provider different-model handoff with tool calls | `TestOpenAIResponsesReasoningReplaySkipsAbortedReasoningOnlyHistory`, `TestOpenAIResponsesReasoningReplaySameProviderDifferentModelHandoff`, `TestOpenAIResponsesReasoningReplayCrossProviderHandoff` | covered |
| 183 | handles cross-provider handoff from Anthropic to OpenAI Codex | `TestOpenAIResponsesReasoningReplaySkipsAbortedReasoningOnlyHistory`, `TestOpenAIResponsesReasoningReplaySameProviderDifferentModelHandoff`, `TestOpenAIResponsesReasoningReplayCrossProviderHandoff` | covered |

## `packages/ai/test/openai-responses-tool-result-images.test.ts`

Pi cases: `4`

Mapped Gi files: `gi-llm-provider/openai_responses_stream_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 155 | should send tool result images in function_call_output | `TestProcessOpenAIResponsesStreamCleansToolCallScratchState`, `TestConvertOpenAIResponsesMessagesKeepsToolResultImagesInsideFunctionOutput`, `TestProcessOpenAIResponsesStreamCapturesResponseID` | covered |
| 165 | should send tool result images in function_call_output | `TestProcessOpenAIResponsesStreamCleansToolCallScratchState`, `TestConvertOpenAIResponsesMessagesKeepsToolResultImagesInsideFunctionOutput`, `TestProcessOpenAIResponsesStreamCapturesResponseID` | covered |
| 173 | should send tool result images in function_call_output | `TestProcessOpenAIResponsesStreamCleansToolCallScratchState`, `TestConvertOpenAIResponsesMessagesKeepsToolResultImagesInsideFunctionOutput`, `TestProcessOpenAIResponsesStreamCapturesResponseID` | contract-covered; Pi credential-gated |
| 188 | should send tool result images in function_call_output | `TestProcessOpenAIResponsesStreamCleansToolCallScratchState`, `TestConvertOpenAIResponsesMessagesKeepsToolResultImagesInsideFunctionOutput`, `TestProcessOpenAIResponsesStreamCapturesResponseID` | contract-covered; Pi credential-gated |

## `packages/ai/test/openrouter-cache-write-repro.test.ts`

Pi cases: `1`

Mapped Gi files: `gi-llm-provider/openai_completions_stream_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 15 | regression: preserves cache_write_tokens on openai-completions stream path | `TestParseOpenAIChatUsagePreservesCacheReadWrite`, `TestProcessOpenAICompletionsChunksMixedContentThinkingToolsAndUsage` | covered |

## `packages/ai/test/openrouter-images.test.ts`

Pi cases: `3`

Mapped Gi files: `gi-llm-provider/openrouter_images_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 66 | returns text plus images in final output | `TestBuildOpenRouterImagesPayloadImageOnlyOutput` | covered |
| 98 | passes through abort signal and returns aborted result | `TestBuildOpenRouterImagesPayload`, `TestBuildOpenRouterImagesPayloadImageOnlyOutput`, `TestOpenRouterImagesProviderHandlesHTTPError` | covered |
| 121 | generateImages resolves the final assistant images result | `TestBuildOpenRouterImagesPayload`, `TestBuildOpenRouterImagesPayloadImageOnlyOutput`, `TestOpenRouterImagesProviderHandlesHTTPError` | covered |

## `packages/ai/test/overflow.test.ts`

Pi cases: `11`

Mapped Gi files: `gi-llm-provider/overflow_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 33 | detects explicit Ollama prompt-too-long errors | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 37 | detects Together AI context length errors | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 44 | detects LiteLLM-wrapped OpenAI maximum context length errors | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 51 | does not treat generic non-overflow Ollama errors as overflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 56 | does not treat Bedrock throttling 'Too many tokens' as overflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 63 | does not treat Bedrock service unavailable as overflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 68 | does not treat generic rate limit errors as overflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 73 | does not treat HTTP 429 style errors as overflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 98 | detects Xiaomi-style overflow (length stop with zero output and filled context) | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 103 | does not treat normal length stops with output as overflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |
| 108 | does not treat length stops far below context as overflow | `TestIsContextOverflowErrorPatterns`, `TestIsContextOverflowLengthStopSignals` | covered |

## `packages/ai/test/responseid.test.ts`

Pi cases: `11`

Mapped Gi files: `gi-llm-provider/openai_responses_stream_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 29 | should expose responseId | `TestProcessOpenAIResponsesStreamCleansToolCallScratchState`, `TestConvertOpenAIResponsesMessagesKeepsToolResultImagesInsideFunctionOutput`, `TestProcessOpenAIResponsesStreamCapturesResponseID` | covered |
| 42 | should expose responseId with ADC | `TestProcessOpenAIResponsesStreamCleansToolCallScratchState`, `TestConvertOpenAIResponsesMessagesKeepsToolResultImagesInsideFunctionOutput`, `TestProcessOpenAIResponsesStreamCapturesResponseID` | contract-covered; Pi credential-gated |
| 46 | should expose responseId with API key | `TestProcessOpenAIResponsesStreamCleansToolCallScratchState`, `TestConvertOpenAIResponsesMessagesKeepsToolResultImagesInsideFunctionOutput`, `TestProcessOpenAIResponsesStreamCapturesResponseID` | contract-covered; Pi credential-gated |
| 59 | should expose responseId | `TestProcessOpenAIResponsesStreamCleansToolCallScratchState`, `TestConvertOpenAIResponsesMessagesKeepsToolResultImagesInsideFunctionOutput`, `TestProcessOpenAIResponsesStreamCapturesResponseID` | covered |
| 67 | should expose responseId | `TestProcessOpenAIResponsesStreamCleansToolCallScratchState`, `TestConvertOpenAIResponsesMessagesKeepsToolResultImagesInsideFunctionOutput`, `TestProcessOpenAIResponsesStreamCapturesResponseID` | covered |
| 75 | should expose responseId | `TestProcessOpenAIResponsesStreamCleansToolCallScratchState`, `TestConvertOpenAIResponsesMessagesKeepsToolResultImagesInsideFunctionOutput`, `TestProcessOpenAIResponsesStreamCapturesResponseID` | covered |
| 85 | should expose responseId | `TestProcessOpenAIResponsesStreamCleansToolCallScratchState`, `TestConvertOpenAIResponsesMessagesKeepsToolResultImagesInsideFunctionOutput`, `TestProcessOpenAIResponsesStreamCapturesResponseID` | covered |
| 93 | should expose responseId | `TestProcessOpenAIResponsesStreamCleansToolCallScratchState`, `TestConvertOpenAIResponsesMessagesKeepsToolResultImagesInsideFunctionOutput`, `TestProcessOpenAIResponsesStreamCapturesResponseID` | covered |
| 100 | OpenAI path should expose responseId | `TestProcessOpenAIResponsesStreamCleansToolCallScratchState`, `TestConvertOpenAIResponsesMessagesKeepsToolResultImagesInsideFunctionOutput`, `TestProcessOpenAIResponsesStreamCapturesResponseID` | contract-covered; Pi credential-gated |
| 104 | Anthropic path should expose responseId | `TestProcessOpenAIResponsesStreamCleansToolCallScratchState`, `TestConvertOpenAIResponsesMessagesKeepsToolResultImagesInsideFunctionOutput`, `TestProcessOpenAIResponsesStreamCapturesResponseID` | contract-covered; Pi credential-gated |
| 116 | should expose responseId | `TestProcessOpenAIResponsesStreamCleansToolCallScratchState`, `TestConvertOpenAIResponsesMessagesKeepsToolResultImagesInsideFunctionOutput`, `TestProcessOpenAIResponsesStreamCapturesResponseID` | contract-covered; Pi credential-gated |

## `packages/ai/test/stream.test.ts`

Pi cases: `205`

Mapped Gi files: `gi-llm-provider/stream_contract_test.go`, `gi-llm-provider/faux_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 353 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | covered |
| 357 | should handle tool calling | `TestStreamContractsWithFauxProvider` | covered |
| 361 | should handle streaming | `TestStreamContractsWithFauxProvider` | covered |
| 365 | should handle thinking | `TestStreamContractsWithFauxProvider` | covered |
| 369 | should handle multi-turn with thinking and tools | `TestStreamContractsWithFauxProvider` | covered |
| 373 | should handle image input | `TestStreamContractsWithFauxProvider` | covered |
| 386 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 390 | should complete basic text generation with Vertex API key | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 394 | should handle tool calling | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 398 | should handle thinking | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 406 | should handle streaming | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 410 | should handle multi-turn with thinking and tools | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 418 | should handle image input | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 431 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | covered |
| 435 | should handle tool calling | `TestStreamContractsWithFauxProvider` | covered |
| 439 | should handle streaming | `TestStreamContractsWithFauxProvider` | covered |
| 443 | should handle image input | `TestStreamContractsWithFauxProvider` | covered |
| 453 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | covered |
| 457 | should handle tool calling | `TestStreamContractsWithFauxProvider` | covered |
| 461 | should handle streaming | `TestStreamContractsWithFauxProvider` | covered |
| 465 | should handle thinking mode | `TestStreamContractsWithFauxProvider` | covered |
| 469 | should handle multi-turn with thinking and tools | `TestStreamContractsWithFauxProvider` | covered |
| 478 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | covered |
| 482 | should handle tool calling | `TestStreamContractsWithFauxProvider` | covered |
| 486 | should handle streaming | `TestStreamContractsWithFauxProvider` | covered |
| 490 | should handle thinking | `TestStreamContractsWithFauxProvider` | covered |
| 494 | should handle multi-turn with thinking and tools | `TestStreamContractsWithFauxProvider` | covered |
| 498 | should handle image input | `TestStreamContractsWithFauxProvider` | covered |
| 506 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | covered |
| 510 | should handle tool calling | `TestStreamContractsWithFauxProvider` | covered |
| 514 | should handle streaming | `TestStreamContractsWithFauxProvider` | covered |
| 518 | should handle image input | `TestStreamContractsWithFauxProvider` | covered |
| 528 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | covered |
| 532 | should handle tool calling | `TestStreamContractsWithFauxProvider` | covered |
| 536 | should handle streaming | `TestStreamContractsWithFauxProvider` | covered |
| 540 | should handle image input | `TestStreamContractsWithFauxProvider` | covered |
| 548 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | covered |
| 552 | should handle tool calling | `TestStreamContractsWithFauxProvider` | covered |
| 556 | should handle streaming | `TestStreamContractsWithFauxProvider` | covered |
| 560 | should handle thinking mode | `TestStreamContractsWithFauxProvider` | covered |
| 564 | should handle multi-turn with thinking and tools | `TestStreamContractsWithFauxProvider` | covered |
| 572 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | covered |
| 576 | should handle tool calling | `TestStreamContractsWithFauxProvider` | covered |
| 580 | should handle streaming | `TestStreamContractsWithFauxProvider` | covered |
| 584 | should handle thinking mode | `TestStreamContractsWithFauxProvider` | covered |
| 588 | should handle multi-turn with thinking and tools | `TestStreamContractsWithFauxProvider` | covered |
| 596 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | covered |
| 600 | should handle tool calling | `TestStreamContractsWithFauxProvider` | covered |
| 604 | should handle streaming | `TestStreamContractsWithFauxProvider` | covered |
| 608 | should handle thinking mode | `TestStreamContractsWithFauxProvider` | covered |
| 612 | should handle multi-turn with thinking and tools | `TestStreamContractsWithFauxProvider` | covered |
| 622 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | covered |
| 626 | should handle tool calling | `TestStreamContractsWithFauxProvider` | covered |
| 630 | should handle streaming | `TestStreamContractsWithFauxProvider` | covered |
| 634 | should handle thinking mode | `TestStreamContractsWithFauxProvider` | covered |
| 638 | should handle multi-turn with thinking and tools | `TestStreamContractsWithFauxProvider` | covered |
| 649 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | covered |
| 653 | should handle tool calling | `TestStreamContractsWithFauxProvider` | covered |
| 657 | should handle streaming | `TestStreamContractsWithFauxProvider` | covered |
| 661 | should handle thinking mode | `TestStreamContractsWithFauxProvider` | covered |
| 665 | should handle multi-turn with thinking and tools | `TestStreamContractsWithFauxProvider` | covered |
| 682 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | covered |
| 686 | should handle tool calling | `TestStreamContractsWithFauxProvider` | covered |
| 690 | should handle streaming | `TestStreamContractsWithFauxProvider` | covered |
| 694 | should handle thinking mode | `TestStreamContractsWithFauxProvider` | covered |
| 698 | should handle multi-turn with thinking and tools | `TestStreamContractsWithFauxProvider` | covered |
| 715 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | covered |
| 719 | should handle tool calling | `TestStreamContractsWithFauxProvider` | covered |
| 723 | should handle streaming | `TestStreamContractsWithFauxProvider` | covered |
| 727 | should handle thinking mode | `TestStreamContractsWithFauxProvider` | covered |
| 731 | should handle multi-turn with thinking and tools | `TestStreamContractsWithFauxProvider` | covered |
| 740 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | covered |
| 744 | should handle tool calling | `TestStreamContractsWithFauxProvider` | covered |
| 748 | should handle streaming | `TestStreamContractsWithFauxProvider` | covered |
| 752 | should handle thinking mode | `TestStreamContractsWithFauxProvider` | covered |
| 756 | should handle multi-turn with thinking and tools | `TestStreamContractsWithFauxProvider` | covered |
| 764 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | covered |
| 768 | should handle tool calling | `TestStreamContractsWithFauxProvider` | covered |
| 772 | should handle streaming | `TestStreamContractsWithFauxProvider` | covered |
| 776 | should handle thinking mode | `TestStreamContractsWithFauxProvider` | covered |
| 780 | should handle multi-turn with thinking and tools | `TestStreamContractsWithFauxProvider` | covered |
| 784 | should handle image input | `TestStreamContractsWithFauxProvider` | covered |
| 792 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | covered |
| 796 | should handle tool calling | `TestStreamContractsWithFauxProvider` | covered |
| 800 | should handle streaming | `TestStreamContractsWithFauxProvider` | covered |
| 804 | should handle thinking mode | `TestStreamContractsWithFauxProvider` | covered |
| 808 | should handle multi-turn with thinking and tools | `TestStreamContractsWithFauxProvider` | covered |
| 812 | should handle image input | `TestStreamContractsWithFauxProvider` | covered |
| 822 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | covered |
| 826 | should handle tool calling | `TestStreamContractsWithFauxProvider` | covered |
| 830 | should handle streaming | `TestStreamContractsWithFauxProvider` | covered |
| 834 | should handle image input | `TestStreamContractsWithFauxProvider` | covered |
| 838 | should handle multi-turn with tools | `TestStreamContractsWithFauxProvider` | covered |
| 849 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | covered |
| 853 | should handle tool calling | `TestStreamContractsWithFauxProvider` | covered |
| 857 | should handle streaming | `TestStreamContractsWithFauxProvider` | covered |
| 861 | should handle image input | `TestStreamContractsWithFauxProvider` | covered |
| 865 | should handle multi-turn with tools | `TestStreamContractsWithFauxProvider` | covered |
| 876 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | covered |
| 880 | should handle tool calling | `TestStreamContractsWithFauxProvider` | covered |
| 884 | should handle streaming | `TestStreamContractsWithFauxProvider` | covered |
| 888 | should handle image input | `TestStreamContractsWithFauxProvider` | covered |
| 892 | should handle multi-turn with tools | `TestStreamContractsWithFauxProvider` | covered |
| 901 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | covered |
| 905 | should handle tool calling | `TestStreamContractsWithFauxProvider` | covered |
| 909 | should handle streaming | `TestStreamContractsWithFauxProvider` | covered |
| 913 | should handle thinking mode | `TestStreamContractsWithFauxProvider` | covered |
| 917 | should handle multi-turn with thinking and tools | `TestStreamContractsWithFauxProvider` | covered |
| 921 | should handle image input | `TestStreamContractsWithFauxProvider` | covered |
| 929 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | covered |
| 933 | should handle tool calling | `TestStreamContractsWithFauxProvider` | covered |
| 937 | should handle streaming | `TestStreamContractsWithFauxProvider` | covered |
| 941 | should handle thinking mode | `TestStreamContractsWithFauxProvider` | covered |
| 946 | should handle multi-turn with thinking and tools | `TestStreamContractsWithFauxProvider` | covered |
| 955 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | covered |
| 959 | should handle tool calling | `TestStreamContractsWithFauxProvider` | covered |
| 963 | should handle streaming | `TestStreamContractsWithFauxProvider` | covered |
| 967 | should handle image input | `TestStreamContractsWithFauxProvider` | covered |
| 975 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | covered |
| 979 | should handle tool calling | `TestStreamContractsWithFauxProvider` | covered |
| 983 | should handle streaming | `TestStreamContractsWithFauxProvider` | covered |
| 987 | should handle thinking mode | `TestStreamContractsWithFauxProvider` | covered |
| 991 | should handle multi-turn with thinking and tools | `TestStreamContractsWithFauxProvider` | covered |
| 1001 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | covered |
| 1005 | should handle tool calling | `TestStreamContractsWithFauxProvider` | covered |
| 1009 | should handle streaming | `TestStreamContractsWithFauxProvider` | covered |
| 1013 | should handle thinking mode | `TestStreamContractsWithFauxProvider` | covered |
| 1017 | should handle multi-turn with thinking and tools | `TestStreamContractsWithFauxProvider` | covered |
| 1032 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | covered |
| 1036 | should handle tool calling | `TestStreamContractsWithFauxProvider` | covered |
| 1040 | should handle streaming | `TestStreamContractsWithFauxProvider` | covered |
| 1044 | should handle thinking mode | `TestStreamContractsWithFauxProvider` | covered |
| 1048 | should handle multi-turn with thinking and tools | `TestStreamContractsWithFauxProvider` | covered |
| 1063 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | covered |
| 1067 | should handle tool calling | `TestStreamContractsWithFauxProvider` | covered |
| 1071 | should handle streaming | `TestStreamContractsWithFauxProvider` | covered |
| 1075 | should handle thinking mode | `TestStreamContractsWithFauxProvider` | covered |
| 1079 | should handle multi-turn with thinking and tools | `TestStreamContractsWithFauxProvider` | covered |
| 1094 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | covered |
| 1098 | should handle tool calling | `TestStreamContractsWithFauxProvider` | covered |
| 1102 | should handle streaming | `TestStreamContractsWithFauxProvider` | covered |
| 1106 | should handle thinking mode | `TestStreamContractsWithFauxProvider` | covered |
| 1110 | should handle multi-turn with thinking and tools | `TestStreamContractsWithFauxProvider` | covered |
| 1125 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | covered |
| 1129 | should handle tool calling | `TestStreamContractsWithFauxProvider` | covered |
| 1133 | should handle streaming | `TestStreamContractsWithFauxProvider` | covered |
| 1137 | should handle thinking mode | `TestStreamContractsWithFauxProvider` | covered |
| 1141 | should handle multi-turn with thinking and tools | `TestStreamContractsWithFauxProvider` | covered |
| 1155 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1159 | should handle tool calling | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1163 | should handle streaming | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1167 | should handle thinking | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1171 | should handle multi-turn with thinking and tools | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1175 | should handle image input | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1183 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1187 | should handle tool calling | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1191 | should handle streaming | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1195 | should handle adaptive thinking with effort high | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1199 | should handle adaptive thinking with effort medium | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1203 | should handle multi-turn with adaptive thinking and tools | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1211 | should handle image input | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1219 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1223 | should handle tool calling | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1227 | should handle streaming | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1231 | should handle thinking | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1236 | should handle multi-turn with thinking and tools | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1241 | should handle image input | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1249 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1253 | should handle tool calling | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1257 | should handle streaming | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1261 | should handle thinking | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1265 | should handle multi-turn with thinking and tools | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1269 | should handle image input | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1277 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1281 | should handle tool calling | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1285 | should handle streaming | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1289 | should handle thinking | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1293 | should handle multi-turn with thinking and tools | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1297 | should handle image input | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1305 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1309 | should handle tool calling | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1313 | should handle streaming | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1317 | should handle thinking with reasoningEffort xhigh | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1321 | should handle multi-turn with thinking and tools | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1325 | should handle image input | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1334 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1338 | should handle tool calling | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1342 | should handle streaming | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1346 | should handle thinking with reasoningEffort xhigh | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1350 | should handle multi-turn with thinking and tools | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1354 | should handle image input | `TestStreamContractsWithFauxProvider` | contract-covered; Pi credential-gated |
| 1362 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | covered |
| 1366 | should handle tool calling | `TestStreamContractsWithFauxProvider` | covered |
| 1370 | should handle streaming | `TestStreamContractsWithFauxProvider` | covered |
| 1374 | should handle thinking | `TestStreamContractsWithFauxProvider` | covered |
| 1378 | should handle multi-turn with thinking and tools | `TestStreamContractsWithFauxProvider` | covered |
| 1382 | should handle image input | `TestStreamContractsWithFauxProvider` | covered |
| 1390 | should use adaptive thinking without anthropic_beta | `TestStreamContractsWithFauxProvider` | covered |
| 1433 | should pass requestMetadata to the SDK payload | `TestStreamContractsWithFauxProvider` | covered |
| 1461 | should omit requestMetadata from payload when not provided | `TestStreamContractsWithFauxProvider` | covered |
| 1567 | should complete basic text generation | `TestStreamContractsWithFauxProvider` | covered |
| 1571 | should handle tool calling | `TestStreamContractsWithFauxProvider` | covered |
| 1575 | should handle streaming | `TestStreamContractsWithFauxProvider` | covered |
| 1579 | should handle thinking mode | `TestStreamContractsWithFauxProvider` | covered |
| 1583 | should handle multi-turn with thinking and tools | `TestStreamContractsWithFauxProvider` | covered |

## `packages/ai/test/supports-xhigh.test.ts`

Pi cases: `8`

Mapped Gi files: `gi-llm-provider/models_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 5 | includes xhigh for Anthropic Opus 4.6 on anthropic-messages API | `TestGetSupportedThinkingLevels`, `TestValidateThinkingLevelSupportedForXHigh` | covered |
| 10 | includes xhigh for Anthropic Opus 4.7 on anthropic-messages API | `TestGetSupportedThinkingLevels`, `TestValidateThinkingLevelSupportedForXHigh` | covered |
| 16 | does not include xhigh for non-Opus Anthropic models | `TestGetSupportedThinkingLevels`, `TestValidateThinkingLevelSupportedForXHigh` | covered |
| 22 | includes xhigh for %s models | `TestGetSupportedThinkingLevels`, `TestValidateThinkingLevelSupportedForXHigh` | covered |
| 28 | includes only high/xhigh plus off for DeepSeek V4 Flash on the DeepSeek provider | `TestGetSupportedThinkingLevels`, `TestValidateThinkingLevelSupportedForXHigh` | covered |
| 34 | includes only high/xhigh plus off for DeepSeek V4 Flash on opencode-go | `TestGetSupportedThinkingLevels`, `TestValidateThinkingLevelSupportedForXHigh` | covered |
| 40 | includes only high/xhigh plus off for DeepSeek V4 Flash on OpenRouter | `TestGetSupportedThinkingLevels`, `TestValidateThinkingLevelSupportedForXHigh` | covered |
| 46 | includes xhigh for OpenRouter Opus 4.6 (openai-completions API) | `TestGetSupportedThinkingLevels`, `TestValidateThinkingLevelSupportedForXHigh` | covered |

## `packages/ai/test/together-models.test.ts`

Pi cases: `3`

Mapped Gi files: `gi-llm-provider/model_catalog_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 16 | registers the default Kimi K2.6 model via OpenAI-compatible Chat Completions API | `TestFireworksAndTogetherEnvKeys`, `TestTogetherModelCatalog`, `TestTogetherReasoningControls` | covered |
| 44 | models Together reasoning controls from the Together API surface | `TestTogetherReasoningControls` | covered |
| 71 | resolves TOGETHER_API_KEY from the environment | `TestFireworksAndTogetherEnvKeys` | covered |

## `packages/ai/test/tokens.test.ts`

Pi cases: `26`

Mapped Gi files: `gi-llm-provider/abort_usage_test.go`, `gi-llm-provider/event_stream_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 90 | should include token stats when aborted mid-stream | `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 103 | should include token stats when aborted mid-stream | `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 111 | should include token stats when aborted mid-stream | `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 121 | should include token stats when aborted mid-stream | `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 129 | should include token stats when aborted mid-stream | `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 137 | should include token stats when aborted mid-stream | `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 145 | should include token stats when aborted mid-stream | `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 153 | should include token stats when aborted mid-stream | `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 161 | should include token stats when aborted mid-stream | `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 169 | should include token stats when aborted mid-stream | `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 177 | should include token stats when aborted mid-stream | `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 185 | should include token stats when aborted mid-stream | `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 193 | should include token stats when aborted mid-stream | `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 201 | should include token stats when aborted mid-stream | `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 209 | should include token stats when aborted mid-stream | `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 217 | should include token stats when aborted mid-stream | `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 225 | should include token stats when aborted mid-stream | `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |
| 239 | should include token stats when aborted mid-stream | `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | contract-covered; Pi skipped |
| 249 | should include token stats when aborted mid-stream | `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | contract-covered; Pi skipped |
| 259 | should include token stats when aborted mid-stream | `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | contract-covered; Pi skipped |
| 269 | should include token stats when aborted mid-stream | `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | contract-covered; Pi skipped |
| 280 | should include token stats when aborted mid-stream | `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | contract-covered; Pi credential-gated |
| 291 | gpt-4o - should include token stats when aborted mid-stream | `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | contract-covered; Pi credential-gated |
| 299 | claude-sonnet-4 - should include token stats when aborted mid-stream | `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | contract-covered; Pi credential-gated |
| 311 | gpt-5.2-codex - should include token stats when aborted mid-stream | `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | contract-covered; Pi credential-gated |
| 323 | should include token stats when aborted mid-stream | `TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation` | covered |

## `packages/ai/test/tool-call-id-normalization.test.ts`

Pi cases: `4`

Mapped Gi files: `gi-llm-provider/message_transform_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 46 | github-copilot -> openrouter should normalize pipe-separated IDs | `TestNormalizeToolCallIDHelpers`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | contract-covered; Pi credential-gated |
| 115 | github-copilot -> openai-codex should normalize pipe-separated IDs | `TestNormalizeToolCallIDHelpers`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | contract-covered; Pi credential-gated |
| 239 | openrouter should handle prefilled context with long pipe-separated IDs | `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | contract-covered; Pi credential-gated |
| 265 | openai-codex should handle prefilled context with long pipe-separated IDs | `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | contract-covered; Pi credential-gated |

## `packages/ai/test/tool-call-without-result.test.ts`

Pi cases: `26`

Mapped Gi files: `gi-llm-provider/provider_contracts_test.go`, `gi-llm-provider/message_transform_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 101 | should filter out tool calls without corresponding tool results | `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 114 | should filter out tool calls without corresponding tool results | `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 122 | should filter out tool calls without corresponding tool results | `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 132 | should filter out tool calls without corresponding tool results | `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 140 | should filter out tool calls without corresponding tool results | `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 148 | should filter out tool calls without corresponding tool results | `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 156 | should filter out tool calls without corresponding tool results | `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 164 | should filter out tool calls without corresponding tool results | `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 172 | should filter out tool calls without corresponding tool results | `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 180 | should filter out tool calls without corresponding tool results | `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 188 | should filter out tool calls without corresponding tool results | `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 196 | should filter out tool calls without corresponding tool results | `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 204 | should filter out tool calls without corresponding tool results | `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 212 | should filter out tool calls without corresponding tool results | `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 220 | should filter out tool calls without corresponding tool results | `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 228 | should filter out tool calls without corresponding tool results | `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 236 | should filter out tool calls without corresponding tool results | `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 244 | should filter out tool calls without corresponding tool results | `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 252 | should filter out tool calls without corresponding tool results | `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 260 | should filter out tool calls without corresponding tool results | `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 268 | should filter out tool calls without corresponding tool results | `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 276 | should filter out tool calls without corresponding tool results | `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 288 | should filter out tool calls without corresponding tool results | `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | contract-covered; Pi credential-gated |
| 299 | gpt-4o - should filter out tool calls without corresponding tool results | `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | contract-covered; Pi credential-gated |
| 307 | claude-sonnet-4 - should filter out tool calls without corresponding tool results | `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | contract-covered; Pi credential-gated |
| 319 | gpt-5.2-codex - should filter out tool calls without corresponding tool results | `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | contract-covered; Pi credential-gated |

## `packages/ai/test/total-tokens.test.ts`

Pi cases: `31`

Mapped Gi files: `gi-llm-provider/provider_contracts_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 107 | claude-sonnet-4-5 - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 130 | claude-sonnet-4 - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | contract-covered; Pi credential-gated |
| 157 | gpt-4o-mini - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 181 | gpt-4o - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 196 | gpt-4o-mini - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 221 | gemini-2.0-flash - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 244 | grok-3-fast - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 267 | openai/gpt-oss-120b - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 290 | gpt-oss-120b - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 313 | @cf/moonshotai/kimi-k2.6 - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 338 | workers-ai/@cf/moonshotai/kimi-k2.6 - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 363 | Kimi-K2.5 - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 382 | Kimi-K2.6 - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 404 | glm-4.5-air - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 427 | devstral-medium-latest - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 450 | MiniMax-M2.7 - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 473 | mimo-v2.5-pro - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 496 | mimo-v2.5-pro - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 521 | mimo-v2.5-pro - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 546 | mimo-v2.5-pro - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 571 | kimi-k2-thinking - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 594 | google/gemini-2.5-flash - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 617 | anthropic/claude-sonnet-4 - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 633 | deepseek/deepseek-chat - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 650 | mistralai/mistral-small-3.2-24b-instruct - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 667 | google/gemini-2.0-flash-001 - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 684 | meta-llama/llama-4-scout - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 708 | gpt-4o - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | contract-covered; Pi credential-gated |
| 724 | claude-sonnet-4 - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | contract-covered; Pi credential-gated |
| 750 | claude-sonnet-4-5 - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | covered |
| 773 | gpt-5.2-codex - should return totalTokens equal to sum of components | `TestProviderConvertersHandleEmptyMessages`, `TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls`, `TestUsageTotalTokensEqualsComponentsAcrossProviders` | contract-covered; Pi credential-gated |

## `packages/ai/test/transform-messages-copilot-openai-to-anthropic.test.ts`

Pi cases: `4`

Mapped Gi files: `gi-llm-provider/message_transform_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 50 | converts thinking blocks to plain text when source model differs | `TestTransformMessagesKeepsSameModelThinkingAndDropsCrossModelOpaqueThinking` | covered |
| 89 | removes thoughtSignature from tool calls when migrating between models | `TestTransformMessagesNormalizesToolCallIDsAndToolResults`, `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls` | covered |
| 135 | adds synthetic tool results for trailing orphaned tool calls | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls` | covered |
| 161 | adds synthetic results only for trailing tool calls that are still missing results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |

## `packages/ai/test/unicode-surrogate.test.ts`

Pi cases: `78`

Mapped Gi files: `gi-llm-provider/message_transform_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 289 | should handle emoji in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 293 | should handle real-world LinkedIn comment data with emoji | `TestSanitizeSurrogatesKeepsValidUTF8AndDropsInvalidSequences` | covered |
| 297 | should handle unpaired high surrogate (0xD83D) in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 305 | should handle emoji in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 309 | should handle real-world LinkedIn comment data with emoji | `TestSanitizeSurrogatesKeepsValidUTF8AndDropsInvalidSequences` | covered |
| 313 | should handle unpaired high surrogate (0xD83D) in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 321 | should handle emoji in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 325 | should handle real-world LinkedIn comment data with emoji | `TestSanitizeSurrogatesKeepsValidUTF8AndDropsInvalidSequences` | covered |
| 329 | should handle unpaired high surrogate (0xD83D) in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 339 | should handle emoji in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 343 | should handle real-world LinkedIn comment data with emoji | `TestSanitizeSurrogatesKeepsValidUTF8AndDropsInvalidSequences` | covered |
| 347 | should handle unpaired high surrogate (0xD83D) in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 355 | should handle emoji in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 359 | should handle real-world LinkedIn comment data with emoji | `TestSanitizeSurrogatesKeepsValidUTF8AndDropsInvalidSequences` | covered |
| 363 | should handle unpaired high surrogate (0xD83D) in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 375 | should handle emoji in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | contract-covered; Pi credential-gated |
| 379 | should handle real-world LinkedIn comment data with emoji | `TestSanitizeSurrogatesKeepsValidUTF8AndDropsInvalidSequences` | contract-covered; Pi credential-gated |
| 387 | should handle unpaired high surrogate (0xD83D) in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | contract-covered; Pi credential-gated |
| 398 | gpt-4o - should handle emoji in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | contract-covered; Pi credential-gated |
| 406 | gpt-4o - should handle real-world LinkedIn comment data with emoji | `TestSanitizeSurrogatesKeepsValidUTF8AndDropsInvalidSequences` | contract-covered; Pi credential-gated |
| 415 | gpt-4o - should handle unpaired high surrogate (0xD83D) in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | contract-covered; Pi credential-gated |
| 424 | claude-sonnet-4 - should handle emoji in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | contract-covered; Pi credential-gated |
| 433 | claude-sonnet-4 - should handle real-world LinkedIn comment data with emoji | `TestSanitizeSurrogatesKeepsValidUTF8AndDropsInvalidSequences` | contract-covered; Pi credential-gated |
| 442 | claude-sonnet-4 - should handle unpaired high surrogate (0xD83D) in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | contract-covered; Pi credential-gated |
| 455 | should handle emoji in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 459 | should handle real-world LinkedIn comment data with emoji | `TestSanitizeSurrogatesKeepsValidUTF8AndDropsInvalidSequences` | covered |
| 463 | should handle unpaired high surrogate (0xD83D) in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 471 | should handle emoji in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 475 | should handle real-world LinkedIn comment data with emoji | `TestSanitizeSurrogatesKeepsValidUTF8AndDropsInvalidSequences` | covered |
| 479 | should handle unpaired high surrogate (0xD83D) in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 487 | should handle emoji in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 491 | should handle real-world LinkedIn comment data with emoji | `TestSanitizeSurrogatesKeepsValidUTF8AndDropsInvalidSequences` | covered |
| 495 | should handle unpaired high surrogate (0xD83D) in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 503 | should handle emoji in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 507 | should handle real-world LinkedIn comment data with emoji | `TestSanitizeSurrogatesKeepsValidUTF8AndDropsInvalidSequences` | covered |
| 511 | should handle unpaired high surrogate (0xD83D) in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 519 | should handle emoji in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 523 | should handle real-world LinkedIn comment data with emoji | `TestSanitizeSurrogatesKeepsValidUTF8AndDropsInvalidSequences` | covered |
| 527 | should handle unpaired high surrogate (0xD83D) in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 535 | should handle emoji in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 539 | should handle real-world LinkedIn comment data with emoji | `TestSanitizeSurrogatesKeepsValidUTF8AndDropsInvalidSequences` | covered |
| 543 | should handle unpaired high surrogate (0xD83D) in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 552 | should handle emoji in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 556 | should handle real-world LinkedIn comment data with emoji | `TestSanitizeSurrogatesKeepsValidUTF8AndDropsInvalidSequences` | covered |
| 560 | should handle unpaired high surrogate (0xD83D) in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 568 | should handle emoji in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 572 | should handle real-world LinkedIn comment data with emoji | `TestSanitizeSurrogatesKeepsValidUTF8AndDropsInvalidSequences` | covered |
| 576 | should handle unpaired high surrogate (0xD83D) in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 584 | should handle emoji in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 588 | should handle real-world LinkedIn comment data with emoji | `TestSanitizeSurrogatesKeepsValidUTF8AndDropsInvalidSequences` | covered |
| 592 | should handle unpaired high surrogate (0xD83D) in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 600 | should handle emoji in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 604 | should handle real-world LinkedIn comment data with emoji | `TestSanitizeSurrogatesKeepsValidUTF8AndDropsInvalidSequences` | covered |
| 608 | should handle unpaired high surrogate (0xD83D) in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 616 | should handle emoji in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 620 | should handle real-world LinkedIn comment data with emoji | `TestSanitizeSurrogatesKeepsValidUTF8AndDropsInvalidSequences` | covered |
| 624 | should handle unpaired high surrogate (0xD83D) in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 634 | should handle emoji in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 638 | should handle real-world LinkedIn comment data with emoji | `TestSanitizeSurrogatesKeepsValidUTF8AndDropsInvalidSequences` | covered |
| 642 | should handle unpaired high surrogate (0xD83D) in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 657 | should handle emoji in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 661 | should handle real-world LinkedIn comment data with emoji | `TestSanitizeSurrogatesKeepsValidUTF8AndDropsInvalidSequences` | covered |
| 665 | should handle unpaired high surrogate (0xD83D) in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 680 | should handle emoji in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 684 | should handle real-world LinkedIn comment data with emoji | `TestSanitizeSurrogatesKeepsValidUTF8AndDropsInvalidSequences` | covered |
| 688 | should handle unpaired high surrogate (0xD83D) in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 701 | should handle emoji in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 705 | should handle real-world LinkedIn comment data with emoji | `TestSanitizeSurrogatesKeepsValidUTF8AndDropsInvalidSequences` | covered |
| 709 | should handle unpaired high surrogate (0xD83D) in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 717 | should handle emoji in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 721 | should handle real-world LinkedIn comment data with emoji | `TestSanitizeSurrogatesKeepsValidUTF8AndDropsInvalidSequences` | covered |
| 725 | should handle unpaired high surrogate (0xD83D) in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 733 | should handle emoji in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 737 | should handle real-world LinkedIn comment data with emoji | `TestSanitizeSurrogatesKeepsValidUTF8AndDropsInvalidSequences` | covered |
| 741 | should handle unpaired high surrogate (0xD83D) in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | covered |
| 748 | gpt-5.2-codex - should handle emoji in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | contract-covered; Pi credential-gated |
| 756 | gpt-5.2-codex - should handle real-world LinkedIn comment data with emoji | `TestSanitizeSurrogatesKeepsValidUTF8AndDropsInvalidSequences` | contract-covered; Pi credential-gated |
| 765 | gpt-5.2-codex - should handle unpaired high surrogate (0xD83D) in tool results | `TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls`, `TestTransformMessagesNormalizesToolCallIDsAndToolResults` | contract-covered; Pi credential-gated |

## `packages/ai/test/validation.test.ts`

Pi cases: `3`

Mapped Gi files: `gi-llm-provider/validation_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 36 | still validates when Function constructor is unavailable | `TestValidateToolArgumentsCoercesPlainJSONSchemaPrimitiveRules`, `TestValidateToolArgumentsRejectsInvalidCoercions` | covered |
| 62 | coerces serialized plain JSON schemas with AJV-compatible primitive rules | `TestValidateToolArgumentsCoercesPlainJSONSchemaPrimitiveRules`, `TestValidateToolArgumentsRejectsInvalidCoercions` | covered |
| 99 | rejects invalid coercions for serialized plain JSON schemas | `TestValidateToolArgumentsCoercesPlainJSONSchemaPrimitiveRules`, `TestValidateToolArgumentsRejectsInvalidCoercions` | covered |

## `packages/ai/test/xhigh.test.ts`

Pi cases: `3`

Mapped Gi files: `gi-llm-provider/models_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 21 | should work with openai-responses | `TestGetSupportedThinkingLevels`, `TestValidateThinkingLevelSupportedForXHigh` | covered |
| 40 | should error with openai-responses when using xhigh | `TestGetSupportedThinkingLevels`, `TestValidateThinkingLevelSupportedForXHigh` | covered |
| 52 | should error with openai-completions when using xhigh | `TestGetSupportedThinkingLevels`, `TestValidateThinkingLevelSupportedForXHigh` | covered |

## `packages/ai/test/zen.test.ts`

Pi cases: `1`

Mapped Gi files: `gi-llm-provider/model_catalog_test.go`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 15 | ${label}: ${model.id} | `TestOpenCodeZenModelCatalog` | covered |
