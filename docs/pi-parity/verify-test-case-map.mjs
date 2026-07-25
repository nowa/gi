#!/usr/bin/env node

import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import process from "node:process";

const modules = [
	{
		name: "llm",
		label: "LLM provider",
		piTests: ["packages/ai/test"],
		giTests: ["gi-llm-provider"],
		excludedTests: {
			"packages/ai/test/reasoning-options.test.ts":
				"Exercises Pi's TypeScript-only models.dev source generator. Gi intentionally consumes Pi's published provider JSON, strictly decodes thinkingLevelMap, and preserves that verified metadata in its generated Go catalog.",
		},
		aliases: {
			"packages/ai/test/anthropic-oauth.test.ts": ["gi-llm-provider/oauth_test.go"],
			"packages/ai/test/azure-openai-base-url.test.ts": [
				"gi-llm-provider/config_test.go",
				"gi-llm-provider/azure_provider_test.go",
			],
			"packages/ai/test/bedrock-endpoint-resolution.test.ts": ["gi-llm-provider/config_test.go"],
			"packages/ai/test/cache-retention.test.ts": [
				"gi-llm-provider/anthropic_payload_test.go",
				"gi-llm-provider/openai_responses_payload_test.go",
				"gi-llm-provider/openai_completions_payload_test.go",
			],
			"packages/ai/test/empty.test.ts": [
				"gi-llm-provider/openai_completions_convert_test.go",
				"gi-llm-provider/provider_contracts_test.go",
			],
			"packages/ai/test/faux-provider.test.ts": ["gi-llm-provider/faux_test.go"],
			"packages/ai/test/fireworks-models.test.ts": ["gi-llm-provider/model_catalog_test.go"],
			"packages/ai/test/google-vertex-api-key-resolution.test.ts": [
				"gi-llm-provider/config_test.go",
				"gi-llm-provider/google_vertex_provider_test.go",
			],
			"packages/ai/test/image-tool-result.test.ts": [
				"gi-llm-provider/anthropic_payload_test.go",
				"gi-llm-provider/google_convert_test.go",
				"gi-llm-provider/openai_responses_stream_test.go",
			],
			"packages/ai/test/interleaved-thinking.test.ts": [
				"gi-llm-provider/anthropic_payload_test.go",
				"gi-llm-provider/bedrock_stream_test.go",
			],
			"packages/ai/test/node-http-proxy.test.ts": ["gi-llm-provider/config_test.go"],
			"packages/ai/test/openai-codex-oauth.test.ts": ["gi-llm-provider/oauth_test.go"],
			"packages/ai/test/openrouter-cache-write-repro.test.ts": [
				"gi-llm-provider/openai_completions_stream_test.go",
			],
			"packages/ai/test/responseid.test.ts": [
				"gi-llm-provider/anthropic_stream_test.go",
				"gi-llm-provider/google_provider_test.go",
				"gi-llm-provider/openai_completions_stream_test.go",
				"gi-llm-provider/openai_responses_stream_test.go",
			],
			"packages/ai/test/max-thinking.test.ts": ["gi-llm-provider/models_test.go"],
			"packages/ai/test/supports-xhigh.test.ts": ["gi-llm-provider/models_test.go"],
			"packages/ai/test/stream.test.ts": [
				"gi-llm-provider/stream_contract_test.go",
				"gi-llm-provider/anthropic_stream_test.go",
				"gi-llm-provider/bedrock_stream_test.go",
				"gi-llm-provider/event_stream_test.go",
				"gi-llm-provider/openai_completions_stream_test.go",
				"gi-llm-provider/openai_responses_stream_test.go",
			],
			"packages/ai/test/tokens.test.ts": [
				"gi-llm-provider/provider_contracts_test.go",
				"gi-llm-provider/openai_completions_stream_test.go",
				"gi-agent-core/harness/compaction_pi_parity_test.go",
			],
			"packages/ai/test/tool-call-id-normalization.test.ts": [
				"gi-llm-provider/message_transform_test.go",
				"gi-llm-provider/openai_responses_convert_test.go",
				"gi-llm-provider/cross_provider_handoff_test.go",
			],
			"packages/ai/test/tool-call-without-result.test.ts": [
				"gi-llm-provider/message_transform_test.go",
				"gi-llm-provider/provider_contracts_test.go",
			],
			"packages/ai/test/total-tokens.test.ts": [
				"gi-llm-provider/provider_contracts_test.go",
				"gi-llm-provider/openai_completions_stream_test.go",
				"gi-llm-provider/openai_responses_payload_test.go",
			],
			"packages/ai/test/unicode-surrogate.test.ts": ["gi-llm-provider/message_transform_test.go"],
			"packages/ai/test/xhigh.test.ts": [
				"gi-llm-provider/models_test.go",
				"gi-llm-provider/anthropic_payload_test.go",
				"gi-llm-provider/bedrock_payload_test.go",
			],
			"packages/ai/test/zen.test.ts": ["gi-llm-provider/model_catalog_test.go"],
		},
		caseAliases: {
			"packages/ai/test/bedrock-convert-messages.test.ts": {
				"replaces blank user string content with a placeholder": [
					"gi-llm-provider/bedrock_payload_test.go",
				],
				"replaces user content emptied by surrogate sanitization with a placeholder": [
					"gi-llm-provider/bedrock_payload_test.go",
				],
			},
			"packages/ai/test/github-copilot-oauth.test.ts": {
				"filters models to the authenticated account picker catalog": [
					"gi-llm-provider/builtin_providers_test.go",
				],
			},
			"packages/ai/test/openai-codex-stream.test.ts": {
				"fails immediately when a %i retry delay exceeds the limit": [
					"gi-llm-provider/openai_codex_retry_test.go",
				],
			},
			"packages/ai/test/openai-completions-tool-choice.test.ts": {
				"stores z.ai GLM-5.2 effort metadata": [
					"gi-llm-provider/model_catalog_test.go",
				],
				"uses Ant Ling compatibility metadata": [
					"gi-llm-provider/model_catalog_test.go",
				],
			},
			"packages/ai/test/openrouter-cache-control-models.test.ts": {
				"enables cache control for %s": [
					"gi-llm-provider/model_catalog_test.go",
				],
			},
			"packages/ai/test/xai-responses.test.ts": {
				"excludes retired and redundant models from the built-in catalog": [
					"gi-llm-provider/model_catalog_test.go",
				],
			},
			"packages/ai/test/models-runtime.test.ts": {
				"enumerates credential metadata without exposing secrets": [
					"gi-llm-provider/credential_store_test.go",
					"gi-coding-agent/auth_storage_test.go",
				],
				"resolves auth: stored credential owns the provider, ambient only when nothing stored": [
					"gi-llm-provider/auth_test.go",
				],
				"a stored credential without a matching handler blocks ambient fallback": [
					"gi-llm-provider/auth_test.go",
				],
				"refreshes expired oauth credentials and persists the rotated credential": [
					"gi-llm-provider/auth_test.go",
					"gi-coding-agent/auth_storage_test.go",
				],
				"rejects with code oauth when refresh fails, preserving the stored credential": [
					"gi-llm-provider/auth_test.go",
				],
				"serializes concurrent OAuth refreshes through store.modify (no double refresh)": [
					"gi-llm-provider/auth_test.go",
					"gi-coding-agent/auth_storage_test.go",
				],
				"valid oauth tokens resolve without touching modify": [
					"gi-llm-provider/auth_test.go",
				],
			},
		},
	},
	{
		name: "agent",
		label: "Agent core",
		piTests: ["packages/agent/test"],
		giTests: ["gi-agent-core"],
		excludedTests: {
			"packages/agent/test/harness/sqlite-migrations.test.ts":
				"Exercises the optional packages/storage/sqlite-node adapter, which baseline.json excludes from Gi's four-package parity scope.",
			"packages/agent/test/harness/sqlite-node.test.ts":
				"Exercises the optional packages/storage/sqlite-node adapter, which baseline.json excludes from Gi's four-package parity scope.",
		},
		aliases: {
			"packages/agent/test/harness/resource-formatting.test.ts": ["gi-agent-core/harness/format_test.go"],
			"packages/agent/test/harness/prompt-templates.test.ts": [
				"gi-agent-core/harness/prompt_templates_test.go",
				"gi-agent-core/harness/format_test.go",
			],
			"packages/agent/test/harness/system-prompt.test.ts": ["gi-agent-core/harness/format_test.go"],
			"packages/agent/test/harness/tools.test.ts": [
				"gi-agent-core/harness/tools/file_tools_test.go",
				"gi-agent-core/harness/tools/bash_test.go",
			],
		},
	},
	{
		name: "tui",
		label: "TUI",
		piTests: ["packages/tui/test"],
		giTests: ["gi-tui"],
		aliases: {
			"packages/tui/test/bug-regression-isimageline-startswith-bug.test.ts": ["gi-tui/terminal_image_test.go"],
			"packages/tui/test/editor.test.ts": ["gi-tui/components_test.go"],
			"packages/tui/test/fuzzy.test.ts": ["gi-tui/autocomplete_test.go"],
			"packages/tui/test/input.test.ts": ["gi-tui/components_test.go"],
			"packages/tui/test/markdown.test.ts": ["gi-tui/components_test.go"],
			"packages/tui/test/overlay-non-capturing.test.ts": ["gi-tui/tui_test.go"],
			"packages/tui/test/overlay-options.test.ts": ["gi-tui/tui_test.go"],
			"packages/tui/test/overlay-short-content.test.ts": ["gi-tui/tui_test.go"],
			"packages/tui/test/regression-overlay-cjk-boundary.test.ts": ["gi-tui/utils_test.go"],
			"packages/tui/test/regression-regional-indicator-width.test.ts": ["gi-tui/utils_test.go"],
			"packages/tui/test/select-list.test.ts": ["gi-tui/components_test.go"],
			"packages/tui/test/terminal-colors.test.ts": ["gi-tui/terminal_colors_test.go"],
			"packages/tui/test/truncate-to-width.test.ts": ["gi-tui/utils_test.go"],
			"packages/tui/test/truncated-text.test.ts": ["gi-tui/components_test.go"],
			"packages/tui/test/word-navigation.test.ts": ["gi-tui/word_navigation_test.go"],
			"packages/tui/test/wrap-ansi.test.ts": ["gi-tui/utils_test.go"],
		},
		caseAliases: {
			"packages/tui/test/regression-overlay-cjk-boundary.test.ts": {
				"excludes a wide grapheme from before when overlay starts inside it": ["gi-tui/utils_test.go"],
				"keeps ASCII before-segment behavior at the same boundary": ["gi-tui/utils_test.go"],
				"composites an overlay at the requested column when it starts inside a wide grapheme": [
					"gi-tui/utils_test.go",
				],
				"composites an overlay when it starts at a wide grapheme boundary": ["gi-tui/utils_test.go"],
			},
			"packages/tui/test/tab-width.test.ts": {
				"keeps tabs inside terminal control sequences byte-identical": ["gi-tui/utils_test.go"],
				"keeps tab-containing overlays on one physical terminal row": ["gi-tui/utils_test.go"],
			},
			"packages/tui/test/word-navigation.test.ts": {
				"basic words: hello world": ["gi-tui/word_navigation_test.go"],
				"dotted: foo.bar": ["gi-tui/word_navigation_test.go"],
				"colon: foo:bar": ["gi-tui/word_navigation_test.go"],
				"path: path/to/file": ["gi-tui/word_navigation_test.go"],
				"CJK mixed": ["gi-tui/word_navigation_test.go"],
				"whitespace at boundaries": ["gi-tui/word_navigation_test.go"],
				"punctuation run: foo...bar": ["gi-tui/word_navigation_test.go"],
				"cursor at 0 returns 0": ["gi-tui/word_navigation_test.go"],
				"cursor at end returns end": ["gi-tui/word_navigation_test.go"],
				"backward skips word then stops before atomic marker": ["gi-tui/word_navigation_test.go"],
				"backward skips whitespace then atomic marker as one unit": ["gi-tui/word_navigation_test.go"],
				"forward skips atomic marker as one unit": ["gi-tui/word_navigation_test.go"],
			},
		},
	},
	{
		name: "coding",
		label: "Coding agent",
		piTests: ["packages/coding-agent/test"],
		giTests: ["gi-coding-agent", "cmd/gi"],
		aliases: {
			"packages/coding-agent/test/ansi-utils.test.ts": ["gi-coding-agent/pi_coding_agent_case_names_test.go"],
			"packages/coding-agent/test/args.test.ts": ["gi-coding-agent/pi_coding_agent_case_names_test.go"],
			"packages/coding-agent/test/compaction-serialization.test.ts": [
				"gi-agent-core/harness/compaction_test.go",
				"gi-coding-agent/pi_coding_agent_case_names_test.go",
			],
			"packages/coding-agent/test/compaction-summary-reasoning.test.ts": [
				"gi-agent-core/harness/compaction_summary_reasoning_test.go",
				"gi-coding-agent/pi_coding_agent_case_names_test.go",
			],
			"packages/coding-agent/test/compaction.test.ts": [
				"gi-agent-core/harness/compaction_pi_parity_test.go",
				"gi-agent-core/harness/compaction_test.go",
				"gi-coding-agent/pi_coding_agent_case_names_test.go",
			],
			"packages/coding-agent/test/extensions-discovery.test.ts": [
				"gi-coding-agent/extension_discovery_test.go",
				"gi-coding-agent/protocol_extension_descriptor_test.go",
				"gi-coding-agent/pi_coding_agent_case_names_test.go",
			],
			"packages/coding-agent/test/frontmatter.test.ts": [
				"gi-coding-agent/utils_test.go",
				"gi-coding-agent/pi_coding_agent_case_names_test.go",
			],
			"packages/coding-agent/test/image-processing.test.ts": [
				"gi-coding-agent/image_resize_test.go",
				"gi-coding-agent/pi_coding_agent_case_names_test.go",
			],
			"packages/coding-agent/test/http-dispatcher.test.ts": [
				"gi-coding-agent/http_runtime_test.go",
				"gi-llm-provider/http_runtime_test.go",
			],
			"packages/coding-agent/test/interactive-mode-status.test.ts": [
				"gi-coding-agent/extension_discovery_test.go",
				"gi-coding-agent/cli_interactive_tui_test.go",
				"gi-coding-agent/pi_coding_agent_case_names_test.go",
			],
			"packages/coding-agent/test/llama-extension.test.ts": [
				"gi-coding-agent/llama_extension_test.go",
				"gi-coding-agent/internal/llama/llama_test.go",
			],
			"packages/coding-agent/test/max-thinking.test.ts": ["gi-coding-agent/max_thinking_test.go"],
			"packages/coding-agent/test/model-registry.test.ts": [
				"gi-coding-agent/model_registry_test.go",
				"gi-coding-agent/auth_storage_test.go",
				"gi-coding-agent/pi_coding_agent_case_names_test.go",
			],
			"packages/coding-agent/test/package-manager-ssh.test.ts": [
				"gi-coding-agent/package_manager_source_test.go",
				"gi-coding-agent/pi_coding_agent_case_names_test.go",
			],
			"packages/coding-agent/test/package-manager.test.ts": [
				"gi-coding-agent/resource_loader_test.go",
				"gi-coding-agent/protocol_package_resolver_test.go",
				"gi-coding-agent/package_manager_settings_test.go",
				"gi-coding-agent/pi_coding_agent_case_names_test.go",
			],
			"packages/coding-agent/test/path-utils.test.ts": [
				"gi-coding-agent/utils_test.go",
				"gi-coding-agent/pi_coding_agent_case_names_test.go",
			],
			"packages/coding-agent/test/paths.test.ts": [
				"gi-coding-agent/utils_test.go",
				"gi-coding-agent/pi_coding_agent_case_names_test.go",
			],
			"packages/coding-agent/test/plan-mode-utils.test.ts": [
				"gi-coding-agent/plan_mode_utils_test.go",
				"gi-coding-agent/pi_coding_agent_case_names_test.go",
			],
			"packages/coding-agent/test/prompt-templates.test.ts": [
				"gi-coding-agent/prompt_templates_test.go",
				"gi-coding-agent/pi_coding_agent_case_names_test.go",
			],
			"packages/coding-agent/test/rpc.test.ts": [
				"gi-coding-agent/rpc_client_test.go",
				"gi-coding-agent/rpc_process_transport_test.go",
				"gi-coding-agent/rpc_session_host_test.go",
				"gi-coding-agent/rpc_mode_cli_test.go",
				"gi-coding-agent/rpc_prompt_response_semantics_test.go",
			],
			"packages/coding-agent/test/rpc-client-process-exit.test.ts": [
				"gi-coding-agent/rpc_process_transport_test.go",
			],
			"packages/coding-agent/test/restore-sandbox-env.test.ts": ["gi-coding-agent/pi_coding_agent_case_names_test.go"],
			"packages/coding-agent/test/session-manager/labels.test.ts": [
				"gi-coding-agent/session_manager_migration_list_test.go",
				"gi-coding-agent/tree_selector_test.go",
			],
			"packages/coding-agent/test/session-manager/custom-session-id.test.ts": [
				"gi-coding-agent/session_manager_file_operations_test.go",
				"gi-coding-agent/session_manager_migration_list_test.go",
				"gi-coding-agent/pi_coding_agent_case_names_test.go",
			],
			"packages/coding-agent/test/session-manager/file-operations.test.ts": [
				"gi-coding-agent/session_manager_file_operations_test.go",
				"gi-coding-agent/session_manager_test.go",
				"gi-coding-agent/pi_coding_agent_case_names_test.go",
			],
			"packages/coding-agent/test/session-manager/migration.test.ts": [
				"gi-coding-agent/session_manager_migration_list_test.go",
				"gi-coding-agent/pi_coding_agent_case_names_test.go",
			],
			"packages/coding-agent/test/session-manager/save-entry.test.ts": ["gi-coding-agent/session_manager_tree_test.go"],
			"packages/coding-agent/test/session-manager/tree-traversal.test.ts": [
				"gi-coding-agent/session_manager_tree_test.go",
				"gi-coding-agent/session_manager_context_suite_test.go",
				"gi-coding-agent/pi_coding_agent_case_names_test.go",
			],
			"packages/coding-agent/test/sdk-stream-options.test.ts": [
				"gi-coding-agent/http_runtime_test.go",
				"gi-coding-agent/cli_print_mode_test.go",
			],
			"packages/coding-agent/test/skills.test.ts": [
				"gi-coding-agent/resource_loader_test.go",
				"gi-agent-core/harness/skills_test.go",
				"gi-coding-agent/pi_coding_agent_case_names_test.go",
			],
			"packages/coding-agent/test/suite/regressions/2835-tools-allowlist-filters-extension-tools.test.ts": [
				"gi-coding-agent/agent_session_dynamic_provider_tools_test.go",
				"gi-coding-agent/pi_coding_agent_case_names_test.go",
			],
			"packages/coding-agent/test/suite/regressions/3302-find-path-glob.test.ts": [
				"gi-coding-agent/tools_search_test.go",
				"gi-coding-agent/pi_coding_agent_case_names_test.go",
			],
			"packages/coding-agent/test/suite/regressions/2791-fswatch-error-crash.test.ts": [
				"gi-coding-agent/fs_watch_test.go",
			],
			"packages/coding-agent/test/suite/regressions/3303-find-nested-gitignore.test.ts": [
				"gi-coding-agent/tools_search_test.go",
			],
			"packages/coding-agent/test/suite/regressions/3592-no-builtin-tools-keeps-extension-tools.test.ts": [
				"gi-coding-agent/agent_session_dynamic_provider_tools_test.go",
				"gi-coding-agent/pi_coding_agent_case_names_test.go",
			],
			"packages/coding-agent/test/suite/regressions/6363-agent-settled-event.test.ts": [
				"gi-coding-agent/agent_session_concurrent_test.go",
				"gi-coding-agent/agent_session_retry_events_test.go",
			],
			"packages/coding-agent/test/tools.test.ts": [
				"gi-coding-agent/tools_read_test.go",
				"gi-coding-agent/tools_write_edit_test.go",
				"gi-coding-agent/tools_edit_test.go",
				"gi-coding-agent/tools_edit_fuzzy_test.go",
				"gi-coding-agent/tools_bash_test.go",
				"gi-coding-agent/tools_bash_advanced_test.go",
				"gi-coding-agent/tools_search_test.go",
				"gi-coding-agent/pi_coding_agent_case_names_test.go",
			],
			"packages/coding-agent/test/truncate-to-width.test.ts": [
				"gi-tui/utils_test.go",
				"gi-coding-agent/pi_coding_agent_case_names_test.go",
			],
		},
		caseAliases: {
			"packages/coding-agent/test/model-runtime-auth-options.test.ts": {
				"transforms fully assembled headers once without forwarding the transform": [
					"gi-llm-provider/models_runtime_test.go",
				],
			},
			"packages/coding-agent/test/runtime-credentials.test.ts": {
				"enumeration merges overrides without exposing keys": ["gi-coding-agent/auth_storage_test.go"],
				"delete clears both the override and persisted credential": ["gi-coding-agent/auth_storage_test.go"],
			},
		},
	},
];

function usage() {
	console.log(`Usage: node docs/pi-parity/verify-test-case-map.mjs [options]

Options:
  --pi-root <path>       Pi checkout path. Defaults to ~/Projects/agents/pi.
  --gi-root <path>       Gi checkout path. Defaults to cwd.
  --format <text|markdown|json>
                         Output format. Defaults to text.
  --out <path>           Write output to a file instead of stdout.
  --fail-on-undocumented Exit non-zero when any Pi test file is not mentioned in
                         known parity docs.

This extracts Pi test/it cases and Gi Go tests/subtests. It reports candidate
file-level mapping; it does not prove behavior parity by itself.`);
}

function parseArgs(argv) {
	const args = {
		piRoot: process.env.PI_REPO || path.join(os.homedir(), "Projects/agents/pi"),
		giRoot: process.cwd(),
		format: "text",
		out: "",
		failOnUndocumented: false,
	};
	for (let i = 0; i < argv.length; i += 1) {
		const arg = argv[i];
		if (arg === "--help" || arg === "-h") {
			usage();
			process.exit(0);
		}
		if (arg === "--pi-root") {
			args.piRoot = argv[++i];
			continue;
		}
		if (arg === "--gi-root") {
			args.giRoot = argv[++i];
			continue;
		}
		if (arg === "--format") {
			args.format = argv[++i];
			continue;
		}
		if (arg === "--out") {
			args.out = argv[++i];
			continue;
		}
		if (arg === "--fail-on-undocumented") {
			args.failOnUndocumented = true;
			continue;
		}
		throw new Error(`unknown argument: ${arg}`);
	}
	if (!["text", "markdown", "json"].includes(args.format)) {
		throw new Error(`unsupported --format value: ${args.format}`);
	}
	return {
		...args,
		piRoot: expandHome(args.piRoot),
		giRoot: expandHome(args.giRoot),
		out: args.out ? expandHome(args.out) : "",
	};
}

function expandHome(input) {
	if (input === "~") {
		return os.homedir();
	}
	if (input.startsWith("~/")) {
		return path.join(os.homedir(), input.slice(2));
	}
	return path.resolve(input);
}

function walk(dir, predicate) {
	if (!fs.existsSync(dir)) {
		return [];
	}
	const out = [];
	for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
		if (entry.name.startsWith(".")) {
			continue;
		}
		const fullPath = path.join(dir, entry.name);
		if (entry.isDirectory()) {
			out.push(...walk(fullPath, predicate));
			continue;
		}
		if (!predicate || predicate(fullPath)) {
			out.push(fullPath);
		}
	}
	return out.sort();
}

function isPiTestFile(file) {
	return /\.test\.tsx?$/.test(file) || /\.spec\.tsx?$/.test(file);
}

function isGoTestFile(file) {
	return file.endsWith("_test.go");
}

function rel(root, file) {
	return path.relative(root, file).split(path.sep).join("/");
}

function extractPiCases(file) {
	const source = fs.readFileSync(file, "utf8");
	const cases = [];
	const lines = source.split(/\n/);
	for (let i = 0; i < lines.length; i += 1) {
		const line = lines[i];
		let match = matchPiCaseTitle(line);
		if (match) {
			cases.push({ line: i + 1, name: unescapeTestName(match.name) });
			continue;
		}
		match = matchPiCaseTitle(line, true);
		if (match) {
			cases.push({ line: i + 1, name: unescapeTestName(match.name) });
			continue;
		}
		if (/^\s*(?:it|test)\s*\(/.test(line)) {
			const lookahead = matchPiCaseTitle(lines.slice(i, i + 5).join("\n"));
			cases.push({ line: i + 1, name: lookahead ? unescapeTestName(lookahead.name) : "<unparsed>" });
		}
	}
	return cases;
}

function matchPiCaseTitle(value, curried = false) {
	const pattern = curried
		? /^\s*(?:it|test)\.\w+\s*\([^)]*\)\s*\(\s*(["'`])((?:\\.|(?!\1)[\s\S])*)\1/
		: /^\s*(?:it|test)(?:\.\w+)?\s*\(\s*(["'`])((?:\\.|(?!\1)[\s\S])*)\1/;
	const match = value.match(pattern);
	return match ? { name: match[2] } : null;
}

function extractGoTests(file) {
	const source = fs.readFileSync(file, "utf8");
	const tests = [];
	const subtests = [];
	const lines = source.split(/\n/);
	for (let i = 0; i < lines.length; i += 1) {
		const line = lines[i];
		let match = line.match(/^func\s+(Test[A-Za-z0-9_]+)\s*\(/);
		if (match) {
			tests.push({ line: i + 1, name: match[1] });
		}
		match = line.match(/\.Run\(\s*(["'`])((?:\\.|(?!\1).)*)\1/);
		if (match) {
			subtests.push({ line: i + 1, name: unescapeTestName(match[2]) });
		}
	}
	return { tests, subtests };
}

function buildGiEntry(args, file) {
	const parsed = extractGoTests(file);
	const relative = rel(args.giRoot, file);
	return {
		file,
		relative,
		stemTokens: normalizeTokens(path.basename(file)),
		tests: parsed.tests,
		subtests: parsed.subtests,
	};
}

function unescapeTestName(value) {
	return value.replace(/\\(["'`\\])/g, "$1").replace(/\\n/g, " ");
}

function normalizeTokens(value) {
	const base = value
		.replace(/\.(test|spec)\.[jt]sx?$/g, "")
		.replace(/_test\.go$/g, "")
		.replace(/([A-Z]+)([A-Z][a-z])/g, "$1 $2")
		.replace(/([a-z0-9])([A-Z])/g, "$1 $2")
		.toLowerCase();
	return new Set(
		base
			.split(/[^a-z0-9]+/)
			.filter((token) => token.length >= 3)
			.map((token) => token.replace(/tests?$/g, "")),
	);
}

function normalizeCaseTokens(value) {
	const stopWords = new Set([
		"and",
		"are",
		"can",
		"does",
		"for",
		"from",
		"handle",
		"handles",
		"have",
		"should",
		"that",
		"the",
		"this",
		"use",
		"uses",
		"using",
		"when",
		"with",
	]);
	const tokens = normalizeTokens(value);
	for (const token of [...tokens]) {
		if (stopWords.has(token)) {
			tokens.delete(token);
		}
	}
	return tokens;
}

function tokenScore(left, right) {
	let score = 0;
	for (const token of left) {
		if (right.has(token)) {
			score += token.length;
		}
	}
	return score;
}

function caseCandidates(piCase, fileCandidates) {
	const caseTokens = normalizeCaseTokens(piCase.name);
	if (caseTokens.size === 0 || piCase.name === "<unparsed>") {
		return [];
	}
	const candidates = [];
	for (const fileCandidate of fileCandidates) {
		for (const test of fileCandidate.tests) {
			const score = tokenScore(caseTokens, normalizeCaseTokens(test.name));
			if (score > 0) {
				candidates.push({
					relative: fileCandidate.relative,
					name: test.name,
					kind: "test",
					score,
				});
			}
		}
		for (const subtest of fileCandidate.subtests) {
			const score = tokenScore(caseTokens, normalizeCaseTokens(subtest.name));
			if (score > 0) {
				candidates.push({
					relative: fileCandidate.relative,
					name: subtest.name,
					kind: "subtest",
					score,
				});
			}
		}
	}
	return candidates
		.sort((a, b) => b.score - a.score || a.relative.localeCompare(b.relative) || a.name.localeCompare(b.name))
		.slice(0, 5);
}

function knownParityDocText(giRoot) {
	const docs = [
		"PI_COMPATIBILITY.md",
		"PI_CODING_AGENT_SOURCE_AUDIT.md",
		"docs/pi-parity/test-case-map.md",
		"docs/pi-parity/test-case-inventory.md",
		"docs/pi-parity/llm-provider-file-map.md",
		"docs/pi-parity/agent-core-file-map.md",
		"docs/pi-parity/tui-file-map.md",
		"docs/pi-parity/coding-agent-file-map.md",
		"docs/pi-parity/coding-agent-interactive-components.md",
		"docs/pi-parity/module-audit.md",
	];
	return docs
		.map((doc) => {
			const file = path.join(giRoot, doc);
			return fs.existsSync(file) ? fs.readFileSync(file, "utf8") : "";
		})
		.join("\n");
}

function collectModule(args, module, docsText) {
	const discoveredPiFiles = module.piTests.flatMap((dir) => walk(path.join(args.piRoot, dir), isPiTestFile));
	const excludedFiles = discoveredPiFiles
		.map((file) => {
			const relative = rel(args.piRoot, file);
			const reason = module.excludedTests?.[relative];
			return reason
				? {
						relative,
						cases: extractPiCases(file),
						reason,
					}
				: undefined;
		})
		.filter(Boolean);
	const piFiles = discoveredPiFiles.filter((file) => !module.excludedTests?.[rel(args.piRoot, file)]);
	const giFiles = module.giTests.flatMap((dir) => walk(path.join(args.giRoot, dir), isGoTestFile));
	const giEntries = giFiles.map((file) => buildGiEntry(args, file));
	const giEntryByRelative = new Map(giEntries.map((entry) => [entry.relative, entry]));

	const files = piFiles.map((file) => {
		const relative = rel(args.piRoot, file);
		const cases = extractPiCases(file);
		const piTokens = normalizeTokens(path.basename(file));
		const explicitCaseAliases = module.caseAliases?.[relative] || {};
		const aliasCandidates = (module.aliases?.[relative] || [])
			.map((candidateRelative) => {
				const inModule = giEntryByRelative.get(candidateRelative);
				if (inModule) {
					return inModule;
				}
				const candidatePath = path.join(args.giRoot, candidateRelative);
				return fs.existsSync(candidatePath) && isGoTestFile(candidatePath) ? buildGiEntry(args, candidatePath) : undefined;
			})
			.filter(Boolean)
			.map((entry) => ({
				relative: entry.relative,
				score: Number.MAX_SAFE_INTEGER,
				tests: entry.tests,
				subtests: entry.subtests,
			}));
		const tokenCandidates = giEntries
			.map((entry) => ({
				relative: entry.relative,
				score: tokenScore(piTokens, entry.stemTokens),
				tests: entry.tests,
				subtests: entry.subtests,
			}))
			.filter((entry) => entry.score > 0)
			.sort((a, b) => b.score - a.score || a.relative.localeCompare(b.relative))
			.slice(0, 5);
		const explicitCandidatePaths = [...new Set(Object.values(explicitCaseAliases).flat())];
		const explicitFileCandidates = explicitCandidatePaths
			.map((candidateRelative) => {
				const inModule = giEntryByRelative.get(candidateRelative);
				if (inModule) {
					return inModule;
				}
				const candidatePath = path.join(args.giRoot, candidateRelative);
				return fs.existsSync(candidatePath) && isGoTestFile(candidatePath) ? buildGiEntry(args, candidatePath) : undefined;
			})
			.filter(Boolean)
			.map((entry) => ({
				relative: entry.relative,
				score: Number.MAX_SAFE_INTEGER,
				tests: entry.tests,
				subtests: entry.subtests,
			}));
		const seenCandidates = new Set();
		const candidates = [];
		for (const candidate of [...aliasCandidates, ...tokenCandidates, ...explicitFileCandidates]) {
			if (seenCandidates.has(candidate.relative)) {
				continue;
			}
			seenCandidates.add(candidate.relative);
			candidates.push(candidate);
		}
		const automaticCandidates = [...aliasCandidates, ...tokenCandidates];
		const mappedCases = cases.map((piCase) => {
			const mapped = caseCandidates(piCase, automaticCandidates);
			for (const candidateRelative of explicitCaseAliases[piCase.name] || []) {
				if (mapped.some((candidate) => candidate.relative === candidateRelative)) {
					continue;
				}
				mapped.push({
					relative: candidateRelative,
					name: piCase.name,
					kind: "explicit",
					score: Number.MAX_SAFE_INTEGER,
				});
			}
			return {
				...piCase,
				candidates: mapped,
			};
		});
		return {
			relative,
			cases: mappedCases,
			documented: docsText.includes(relative),
			candidates,
		};
	});

	const giTestCount = giEntries.reduce((sum, entry) => sum + entry.tests.length, 0);
	const giSubtestCount = giEntries.reduce((sum, entry) => sum + entry.subtests.length, 0);
	const piCaseCount = files.reduce((sum, file) => sum + file.cases.length, 0);
	const excludedPiCaseCount = excludedFiles.reduce((sum, file) => sum + file.cases.length, 0);
	const candidateCaseCount = files.reduce(
		(sum, file) => sum + file.cases.filter((piCase) => piCase.candidates.length > 0).length,
		0,
	);
	const noCandidateCases = files.flatMap((file) =>
		file.cases
			.filter((piCase) => piCase.candidates.length === 0)
			.map((piCase) => ({ file: file.relative, line: piCase.line, name: piCase.name })),
	);
	const documentedFiles = files.filter((file) => file.documented).length;
	const candidateFiles = files.filter((file) => file.candidates.length > 0).length;
	const noCandidateFiles = files.filter((file) => file.candidates.length === 0);
	const undocumentedFiles = files.filter((file) => !file.documented);
	return {
		name: module.name,
		label: module.label,
		piTestFiles: files.length,
		piCaseCount,
		excludedPiTestFiles: excludedFiles.length,
		excludedPiCaseCount,
		excludedFiles,
		giTestFiles: giEntries.length,
		giTestCount,
		giSubtestCount,
		candidateCaseCount,
		noCandidateCases,
		documentedFiles,
		candidateFiles,
		noCandidateFiles,
		undocumentedFiles,
		files,
	};
}

function buildReport(args) {
	if (!fs.existsSync(args.piRoot)) {
		throw new Error(`Pi root not found: ${args.piRoot}`);
	}
	if (!fs.existsSync(args.giRoot)) {
		throw new Error(`Gi root not found: ${args.giRoot}`);
	}
	const docsText = knownParityDocText(args.giRoot);
	const results = modules.map((module) => collectModule(args, module, docsText));
	return {
		piRoot: args.piRoot,
		giRoot: args.giRoot,
		results,
	};
}

function renderText(report) {
	const lines = [];
	lines.push(`Pi root: ${report.piRoot}`);
	lines.push(`Gi root: ${report.giRoot}`);
	for (const result of report.results) {
		lines.push(
			`${result.name}: piFiles=${result.piTestFiles} piCases=${result.piCaseCount} ` +
				`excludedPiFiles=${result.excludedPiTestFiles} excludedPiCases=${result.excludedPiCaseCount} ` +
				`giFiles=${result.giTestFiles} giTests=${result.giTestCount} giSubtests=${result.giSubtestCount} ` +
				`documentedFiles=${result.documentedFiles} candidateFiles=${result.candidateFiles} ` +
				`noCandidateFiles=${result.noCandidateFiles.length} candidateCases=${result.candidateCaseCount} ` +
				`noCandidateCases=${result.noCandidateCases.length}`,
		);
		if (result.noCandidateFiles.length > 0) {
			lines.push("  no candidate Gi test files:");
			for (const file of result.noCandidateFiles.slice(0, 20)) {
				lines.push(`    ${file.relative} (${file.cases.length} cases)`);
			}
			if (result.noCandidateFiles.length > 20) {
				lines.push(`    ... ${result.noCandidateFiles.length - 20} more`);
			}
		}
	}
	return `${lines.join("\n")}\n`;
}

function renderMarkdown(report) {
	const lines = [];
	lines.push("# Pi Test-Case Inventory");
	lines.push("");
	lines.push("Generated by `docs/pi-parity/verify-test-case-map.mjs`.");
	lines.push("");
	lines.push(`- Pi root: \`${report.piRoot}\``);
	lines.push(`- Gi root: \`${report.giRoot}\``);
	lines.push("");
	lines.push("| Area | In-scope Pi test files | In-scope Pi cases | Excluded Pi test files | Excluded Pi cases | Gi test files | Gi top-level tests | Gi subtests | Documented Pi test files | Candidate-mapped Pi test files | No-candidate Pi test files | Candidate-mapped Pi cases | No-candidate Pi cases |");
	lines.push("| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |");
	for (const result of report.results) {
		lines.push(
			`| ${result.label} | ${result.piTestFiles} | ${result.piCaseCount} | ${result.excludedPiTestFiles} | ${result.excludedPiCaseCount} | ${result.giTestFiles} | ` +
				`${result.giTestCount} | ${result.giSubtestCount} | ${result.documentedFiles} | ` +
				`${result.candidateFiles} | ${result.noCandidateFiles.length} | ${result.candidateCaseCount} | ${result.noCandidateCases.length} |`,
		);
	}
	lines.push("");
	lines.push("## Explicitly Excluded Cross-Package Tests");
	lines.push("");
	lines.push("These tests live under an in-scope package's test tree but exercise a package excluded by `baseline.json`. They remain visible here so scope decisions cannot silently hide test coverage.");
	for (const result of report.results) {
		if (result.excludedFiles.length === 0) {
			continue;
		}
		lines.push("");
		lines.push(`### ${result.label}`);
		lines.push("");
		lines.push("| Pi test file | Cases | Reason |");
		lines.push("| --- | ---: | --- |");
		for (const file of result.excludedFiles) {
			lines.push(`| \`${file.relative}\` | ${file.cases.length} | ${markdownCell(file.reason)} |`);
		}
	}
	lines.push("");
	lines.push("## No-Candidate Pi Test Files");
	lines.push("");
	lines.push("These files have no filename-token candidate in the corresponding Gi test package. That does not prove missing behavior, but each row needs explicit mapping, implementation, or an accepted scope decision.");
	for (const result of report.results) {
		lines.push("");
		lines.push(`### ${result.label}`);
		lines.push("");
		if (result.noCandidateFiles.length === 0) {
			lines.push("No files.");
			continue;
		}
		lines.push("| Pi test file | Cases | Documented in parity docs |");
		lines.push("| --- | ---: | --- |");
		for (const file of result.noCandidateFiles) {
			lines.push(`| \`${file.relative}\` | ${file.cases.length} | ${file.documented ? "yes" : "no"} |`);
		}
	}
	lines.push("");
	lines.push("## No-Case-Candidate Pi Cases");
	lines.push("");
	lines.push("These rows have a candidate Gi test file but no obvious Go test/subtest name match. This is a weak-name heuristic for prioritizing manual case-level review, not proof of a missing implementation.");
	for (const result of report.results) {
		lines.push("");
		lines.push(`### ${result.label}`);
		lines.push("");
		if (result.noCandidateCases.length === 0) {
			lines.push("No cases.");
			continue;
		}
		lines.push("| Pi test file | Line | Pi case |");
		lines.push("| --- | ---: | --- |");
		for (const piCase of result.noCandidateCases.slice(0, 80)) {
			lines.push(`| \`${piCase.file}\` | ${piCase.line} | ${markdownCell(piCase.name)} |`);
		}
		if (result.noCandidateCases.length > 80) {
			lines.push(`| ... | ... | ${result.noCandidateCases.length - 80} more cases omitted from this summary |`);
		}
	}
	lines.push("");
	lines.push("## Full File Inventory");
	for (const result of report.results) {
		lines.push("");
		lines.push(`### ${result.label}`);
		lines.push("");
		lines.push("| Pi test file | Cases | Documented | Candidate Gi test files |");
		lines.push("| --- | ---: | --- | --- |");
		for (const file of result.files) {
			const candidates =
				file.candidates.length === 0
					? ""
					: file.candidates.map((candidate) => `\`${candidate.relative}\``).join("<br>");
			lines.push(`| \`${file.relative}\` | ${file.cases.length} | ${file.documented ? "yes" : "no"} | ${candidates} |`);
		}
	}
	lines.push("");
	while (lines.at(-1) === "") {
		lines.pop();
	}
	return `${lines.join("\n")}\n`;
}

function markdownCell(value) {
	return String(value).replace(/\|/g, "\\|").replace(/\n/g, " ");
}

function main() {
	const args = parseArgs(process.argv.slice(2));
	const report = buildReport(args);
	let output;
	if (args.format === "json") {
		output = `${JSON.stringify(report, null, 2)}\n`;
	} else if (args.format === "markdown") {
		output = renderMarkdown(report);
	} else {
		output = renderText(report);
	}
	if (args.out) {
		fs.writeFileSync(args.out, output);
	} else {
		process.stdout.write(output);
	}
	const undocumented = report.results.reduce((sum, result) => sum + result.undocumentedFiles.length, 0);
	if (args.failOnUndocumented && undocumented > 0) {
		process.exit(1);
	}
}

main();
