# Pi Compatibility Matrix

This repository is a Go rebuild of the Pi `pi-ai` and `pi-agent-core` packages.

## Package Mapping

| Pi package | Go package | Status |
| --- | --- | --- |
| `@earendil-works/pi-ai` | `github.com/nowa/gi/gi-llm-provider` | Pi test-compatible: core types, model/image-model registry, provider/image-provider registries, event streams, env keys, validation, overflow, message transforms, unicode sanitization, Anthropic, OpenAI Completions/Responses, OpenAI Codex Responses, Azure OpenAI Responses, Google, Mistral, Bedrock, and OpenRouter Images conversion; live HTTP/SSE transport for Anthropic Messages, OpenAI-compatible Chat Completions, OpenAI Responses, OpenAI Codex Responses, Azure OpenAI Responses, Google Generative AI, Mistral Conversations, and OpenRouter Images; faux provider |
| `@earendil-works/pi-agent-core` | `github.com/nowa/gi/gi-agent-core` | Pi test-compatible: agent loop, stateful agent, tools, queues, lifecycle events |
| `pi-agent-core` harness/session | `github.com/nowa/gi/gi-agent-core/harness` | Pi test-compatible: AgentHarness turn orchestration, stream hooks, local execution env, queue lifecycle, skills, prompt templates, system prompt formatting, truncate, uuidv7, storage/session/repo, session compaction, branch tree navigation and branch-summary hook customization |

## Currently Ported Test Areas

| Pi test file | Go coverage |
| --- | --- |
| `packages/agent/test/agent-loop.test.ts` | `gi-agent-core/agent_loop_test.go` |
| `packages/agent/test/agent.test.ts` | `gi-agent-core/agent_test.go` |
| `packages/agent/test/e2e.test.ts` | `gi-agent-core/agent_e2e_test.go` |
| `packages/agent/test/harness/agent-harness.test.ts` | `gi-agent-core/harness/agent_harness_test.go` |
| `packages/agent/test/harness/agent-harness-stream.test.ts` | `gi-agent-core/harness/agent_harness_test.go` |
| `packages/agent/test/harness/compaction.test.ts` | `gi-agent-core/harness/compaction_test.go` |
| `packages/agent/test/harness/nodejs-env.test.ts` | `gi-agent-core/harness/local_env_test.go` |
| `packages/agent/test/harness/prompt-templates.test.ts` | `gi-agent-core/harness/prompt_templates_test.go` |
| `packages/agent/test/harness/repo.test.ts` | `gi-agent-core/harness/session_repo_test.go` |
| `packages/agent/test/harness/resource-formatting.test.ts` | `gi-agent-core/harness/format_test.go` |
| `packages/agent/test/harness/session-uuid.test.ts` | `gi-agent-core/harness/uuid_test.go` |
| `packages/agent/test/harness/session.test.ts` | `gi-agent-core/harness/session_test.go` |
| `packages/agent/test/harness/skills.test.ts` | `gi-agent-core/harness/skills_test.go` |
| `packages/agent/test/harness/storage.test.ts` | `gi-agent-core/harness/session_storage_test.go` |
| `packages/agent/test/harness/system-prompt.test.ts` | `gi-agent-core/harness/format_test.go` |
| `packages/agent/test/harness/truncate.test.ts` | `gi-agent-core/harness/truncate_test.go` |
| `packages/ai/test/env-api-keys.test.ts` | `gi-llm-provider/env_test.go` |
| `packages/ai/test/faux-provider.test.ts` | `gi-llm-provider/faux_test.go` |
| `packages/ai/test/fireworks-models.test.ts` | `gi-llm-provider/model_catalog_test.go` |
| `packages/ai/test/images.test.ts` | `gi-llm-provider/openrouter_images_test.go` |
| `packages/ai/test/anthropic-eager-tool-input-compat.test.ts` | `gi-llm-provider/anthropic_payload_test.go` |
| `packages/ai/test/anthropic-eager-tool-input-e2e.test.ts` | `gi-llm-provider/anthropic_e2e_contracts_test.go` |
| `packages/ai/test/anthropic-long-cache-retention-e2e.test.ts` | `gi-llm-provider/anthropic_e2e_contracts_test.go` |
| `packages/ai/test/anthropic-oauth.test.ts` | `gi-llm-provider/oauth_test.go` |
| `packages/ai/test/anthropic-opus-4-7-smoke.test.ts` | `gi-llm-provider/anthropic_e2e_contracts_test.go` |
| `packages/ai/test/anthropic-sse-parsing.test.ts` | `gi-llm-provider/anthropic_stream_test.go` |
| `packages/ai/test/anthropic-thinking-disable.test.ts` | `gi-llm-provider/anthropic_payload_test.go` |
| `packages/ai/test/anthropic-tool-name-normalization.test.ts` | `gi-llm-provider/anthropic_payload_test.go` |
| `packages/ai/test/abort.test.ts` | `gi-llm-provider/event_stream_test.go` |
| `packages/ai/test/azure-openai-base-url.test.ts` | `gi-llm-provider/config_test.go` |
| `packages/ai/test/bedrock-endpoint-resolution.test.ts` | `gi-llm-provider/config_test.go` |
| `packages/ai/test/bedrock-models.test.ts` | `gi-llm-provider/model_catalog_test.go` |
| `packages/ai/test/cache-retention.test.ts` | `gi-llm-provider/anthropic_payload_test.go`, `gi-llm-provider/openai_completions_payload_test.go` |
| `packages/ai/test/context-overflow.test.ts` | `gi-llm-provider/overflow_test.go` |
| `packages/ai/test/cross-provider-handoff.test.ts` | `gi-llm-provider/cross_provider_handoff_test.go` |
| `packages/ai/test/empty.test.ts` | `gi-llm-provider/provider_contracts_test.go` |
| `packages/ai/test/google-shared-convert-tools.test.ts` | `gi-llm-provider/google_convert_test.go` |
| `packages/ai/test/google-shared-gemini3-unsigned-tool-call.test.ts` | `gi-llm-provider/google_convert_test.go` |
| `packages/ai/test/google-shared-image-tool-result-routing.test.ts` | `gi-llm-provider/google_convert_test.go` |
| `packages/ai/test/google-thinking-disable.test.ts` | `gi-llm-provider/google_convert_test.go` |
| `packages/ai/test/google-thinking-signature.test.ts` | `gi-llm-provider/google_convert_test.go` |
| `packages/ai/test/google-vertex-api-key-resolution.test.ts` | `gi-llm-provider/config_test.go` |
| `packages/ai/test/github-copilot-anthropic.test.ts` | `gi-llm-provider/github_copilot_headers_test.go` |
| `packages/ai/test/github-copilot-oauth.test.ts` | `gi-llm-provider/oauth_test.go` |
| `packages/ai/test/image-tool-result.test.ts` | `gi-llm-provider/anthropic_payload_test.go`, `gi-llm-provider/google_convert_test.go`, `gi-llm-provider/openai_completions_convert_test.go`, `gi-llm-provider/openai_responses_convert_test.go` |
| `packages/ai/test/interleaved-thinking.test.ts` | `gi-llm-provider/anthropic_payload_test.go`, `gi-llm-provider/bedrock_payload_test.go` |
| `packages/ai/test/bedrock-thinking-payload.test.ts` | `gi-llm-provider/bedrock_payload_test.go` |
| `packages/ai/test/mistral-reasoning-mode.test.ts` | `gi-llm-provider/mistral_payload_test.go` |
| `packages/ai/test/mistral-tool-schema.test.ts` | `gi-llm-provider/mistral_payload_test.go` |
| `packages/ai/test/node-http-proxy.test.ts` | `gi-llm-provider/config_test.go` |
| `packages/ai/test/openai-completions-cache-control-format.test.ts` | `gi-llm-provider/openai_completions_payload_test.go` |
| `packages/ai/test/openai-completions-empty-tools.test.ts` | `gi-llm-provider/openai_completions_convert_test.go` |
| `packages/ai/test/openai-completions-prompt-cache.test.ts` | `gi-llm-provider/openai_completions_payload_test.go` |
| `packages/ai/test/openai-completions-response-model.test.ts` | `gi-llm-provider/openai_completions_stream_test.go` |
| `packages/ai/test/openai-completions-thinking-as-text.test.ts` | `gi-llm-provider/openai_completions_payload_test.go` |
| `packages/ai/test/openai-completions-tool-result-images.test.ts` | `gi-llm-provider/openai_completions_convert_test.go` |
| `packages/ai/test/openai-completions-tool-choice.test.ts` | `gi-llm-provider/openai_completions_payload_test.go` |
| `packages/ai/test/openai-codex-oauth.test.ts` | `gi-llm-provider/oauth_test.go` |
| `packages/ai/test/openai-codex-cache-affinity-e2e.test.ts` | `gi-llm-provider/openai_codex_test.go` |
| `packages/ai/test/openai-codex-stream.test.ts` | `gi-llm-provider/openai_codex_test.go`, `gi-llm-provider/openai_responses_stream_test.go` |
| `packages/ai/test/openai-responses-copilot-provider.test.ts` | `gi-llm-provider/openai_responses_payload_test.go`, `gi-llm-provider/openai_responses_stream_test.go` |
| `packages/ai/test/openai-responses-cache-affinity-e2e.test.ts` | `gi-llm-provider/openai_responses_payload_test.go` |
| `packages/ai/test/openai-responses-foreign-toolcall-id.test.ts` | `gi-llm-provider/openai_responses_convert_test.go` |
| `packages/ai/test/openai-responses-partial-json-cleanup.test.ts` | `gi-llm-provider/openai_responses_stream_test.go` |
| `packages/ai/test/openai-responses-reasoning-replay-e2e.test.ts` | `gi-llm-provider/openai_responses_replay_test.go` |
| `packages/ai/test/openai-responses-tool-result-images.test.ts` | `gi-llm-provider/openai_responses_stream_test.go` |
| `packages/ai/test/openrouter-images.test.ts` | `gi-llm-provider/openrouter_images_test.go` |
| `packages/ai/test/openrouter-cache-write-repro.test.ts` | `gi-llm-provider/openai_completions_stream_test.go` |
| `packages/ai/test/responseid.test.ts` | `gi-llm-provider/openai_responses_stream_test.go` |
| `packages/ai/test/stream.test.ts` | `gi-llm-provider/stream_contract_test.go`, `gi-llm-provider/faux_test.go` |
| `packages/ai/test/tool-call-id-normalization.test.ts` | `gi-llm-provider/message_transform_test.go` |
| `packages/ai/test/tool-call-without-result.test.ts` | `gi-llm-provider/provider_contracts_test.go`, `gi-llm-provider/message_transform_test.go` |
| `packages/ai/test/total-tokens.test.ts` | `gi-llm-provider/provider_contracts_test.go` |
| `packages/ai/test/tokens.test.ts` | `gi-llm-provider/abort_usage_test.go`, `gi-llm-provider/event_stream_test.go` |
| `packages/ai/test/together-models.test.ts` | `gi-llm-provider/model_catalog_test.go` |
| `packages/ai/test/transform-messages-copilot-openai-to-anthropic.test.ts` | `gi-llm-provider/message_transform_test.go` |
| `packages/ai/test/unicode-surrogate.test.ts` | `gi-llm-provider/message_transform_test.go` |
| `packages/ai/test/overflow.test.ts` | `gi-llm-provider/overflow_test.go` |
| `packages/ai/test/supports-xhigh.test.ts` | `gi-llm-provider/models_test.go` |
| `packages/ai/test/xhigh.test.ts` | `gi-llm-provider/models_test.go` |
| `packages/ai/test/validation.test.ts` | `gi-llm-provider/validation_test.go` |
| `packages/ai/test/zen.test.ts` | `gi-llm-provider/model_catalog_test.go` |
| `packages/ai/test/lazy-module-load.test.ts` | N/A in Go: the package has no provider SDK module dependencies; providers are explicit registry entries |

## Outside Current Test Gate

- Credentialed live probes are intentionally not part of the local Go suite.
- Provider-specific live transports not exercised by the Pi tests should be validated before production use.
- Remote filesystems, sandbox service adapters, and WebSocket transports are extension points rather than requirements for the current `pi-agent-core` / `pi-ai` test compatibility gate.

## Local Verification

Run:

```sh
GOCACHE=/private/tmp/gi-gocache go test -timeout 30s ./...
```

Current local Go test count: 213.
