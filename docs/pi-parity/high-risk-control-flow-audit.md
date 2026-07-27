# Pi v0.82.1 High-Risk Control-Flow Audit

Baseline: Pi `v0.82.1` at
`b4f293684bba718d59cc1157679bcf6157b3a7f5`.

This is the branch-level audit for the utility paths that are not fully proven
by payload snapshots alone. `exact` means the public event/result or formatted
value matches Pi. `go-native` means the same contract is implemented through a
typed Go boundary instead of JavaScript object inspection or promises.

## Event stream

| Pi function or branch | Gi owner | Verdict and regression |
| --- | --- | --- |
| `EventStream.constructor`: empty queue, unresolved result, supplied completion/result functions | `internal/eventstream.NewEventStream` | exact; constructor owns one mutex/condition, one logical queue, and one dispatcher |
| `push`: ignore events after completion | `EventStream.Push` | exact; terminal state is checked and changed under the same mutex |
| `push`: terminal event resolves the final result and is still delivered | `EventStream.Push`, `dispatch` | exact; `TestEventStreamPushDoesNotWaitForConsumer` |
| `push`: waiting consumer or unbounded queued delivery | `EventStream.Push`, `dispatch` | go-native; producer state is an unbounded slice queue and only the dispatcher writes/closes the channel |
| `end`: retain already queued events, reject later pushes, resolve an explicit result | `EventStream.End` | exact; `TestEventStreamEndDrainsQueuedEventsAndIgnoresLaterPushes` |
| concurrent `push`/`end` and one terminal winner | mutex-owned state plus single channel closer | stronger Go guarantee; `TestEventStreamConcurrentCompletionIsSafe` under `-race` |
| `result()` waits independently from event consumption | `EventStream.Result(context.Context)` | go-native; terminal result closes independently of dispatcher backpressure and cancellation returns `ctx.Err()` |
| assistant `done`/`error` result extraction | `NewAssistantMessageEventStream` | exact; `TestAssistantMessageEventStreamResult` |

## Assistant and provider retry

| Pi function or branch | Gi owner | Verdict and regression |
| --- | --- | --- |
| non-retryable quota/billing patterns take precedence over retryable status text | `IsRetryableAssistantError` | exact; constant-pattern gate and `quota wins over status` |
| transient provider/network/DNS/WebSocket/early-EOF patterns | `IsRetryableAssistantError` | exact; constant-pattern gate plus table-driven retry tests |
| disabled policy, immediate success, aborted initial result | `RetryAssistantCall` | exact |
| retryable error budget and exponential delay | `RetryAssistantCall`, `exponentialRetryDelay` | exact observable order; initial call is not counted as a retry |
| scheduled, attempt-start, and one final callback | `RetryCallbacks` projection | go-native synchronous callback boundary with Pi-equivalent ordering |
| cancellation during backoff returns an aborted assistant message without an error string | `providerRetrySleep`, `RetryAssistantCall` | exact; cancellation regression |
| provider error shape validation | typed `ProviderError` plus `errors.As` | go-native; arbitrary Go errors are not silently retried |
| `x-should-retry` true/false before status policy | `IsRetryableProviderError` | exact, including Pi's case-sensitive directive values |
| undefined status, 408, 409, 429, and 5xx policy | `IsRetryableProviderError` | exact; `TestIsRetryableProviderErrorMatchesPiPolicy` |
| cancellation observed after a request error | `RetryProviderRequest` | exact; context cancellation wins before retry classification/budget |
| `retry-after-ms`, then `retry-after`, then exponential jitter | `providerRetryDelay` | exact precedence |
| JavaScript `parseFloat` numeric-prefix behavior | `parseProviderHeaderFloat` | exact for finite provider header values; focused prefix cases include suffixes and hexadecimal-looking input |
| numeric/date/invalid `retry-after` handling | `providerRetryDelay` | exact observable sleep: parsed duration, HTTP date, or immediate retry for invalid date text |
| server delay cap and zero-disabled cap | `validateProviderRetryDelay` | exact, including Pi's `Server requested ...` message casing |
| cancellation during provider delay | `providerRetrySleep` | go-native context cancellation; no second request |

## Provider error body

| Pi function or branch | Gi owner | Verdict and regression |
| --- | --- | --- |
| status/body/message normalized into one stable value | `ProviderError`, `NormalizedProviderError` | go-native typed boundary |
| body absent or already embedded in message | `NormalizeProviderError` | exact; avoids duplicate body text |
| SDK response body is an unread stream | `HTTPStatusCode()` inspection only | go-native; stream internals are neither consumed nor serialized |
| trim then cap at 4000 JavaScript characters | `readProviderErrorBody`, `TruncateProviderErrorText` | UTF-16 count and suffix match Pi; an otherwise unrepresentable split surrogate becomes U+FFFD at Go's valid UTF-8 boundary |
| count the complete omitted tail without retaining it | streaming rune reader plus bounded UTF-16 prefix | stronger Go memory behavior; exact truncation count without response-sized allocation |
| no prefix, prefix with status, and message-carries-body formatting | `FormatProviderError` | exact Pi strings, including omission of the former extra `HTTP` token |
| raw body is normalized once | `bodyNormalized` transport state | exact; double-truncation regression |
| non-`Error` JavaScript throws | ordinary Go `error` contract | go-native; Go callers cannot pass an arbitrary thrown value |

## Stream terminal and EOF behavior

| Protocol | Pi terminal rule | Gi regression/gate | Verdict |
| --- | --- | --- | --- |
| Anthropic Messages | `message_start` without `message_stop` is an error | processor EOF check and differential stream fixture | exact |
| OpenAI Chat Completions | missing `finish_reason` is an error | processor result check | exact |
| OpenAI Responses and Azure | completed/incomplete are terminal; failed/API error are errors; any other EOF is an error | four fixed differential failure/EOF cases and processor tests | exact |
| OpenAI Codex Responses SSE | early EOF is a Responses error; failed/API error use Codex-specific messages | three Codex differential terminal cases and focused regressions shared with WebSocket decoding | exact |
| Pi Messages | only `done`/`error` is terminal; trailing EOF record is flushed | Pi Messages differential fixture | exact |
| Google Generative AI and Vertex | stream exhaustion finalizes current block and default stop even without `finishReason` | shared Google processor | exact |
| Mistral Conversations | stream exhaustion finalizes current/tool blocks and emits the current stop state | Mistral processor | exact |
| Bedrock ConverseStream | event-stream exhaustion emits the accumulated result unless an explicit error/abort occurred | native event differential fixture | exact |

The fixed differential matrix contains 31 cases: 10 payload, 16 stream, and 5
cost cases. Together with 512 generated cost cases, the current gate compares
543 Gi outputs to the live exact Pi source.
