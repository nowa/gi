#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { pathToFileURL } from "node:url";

const SCHEMA_VERSION = 1;
const PAYLOAD_MODULES = {
	"anthropic-messages": "anthropic-messages.ts",
	"azure-openai-responses": "azure-openai-responses.ts",
	"bedrock-converse-stream": "bedrock-converse-stream.ts",
	"google-generative-ai": "google-generative-ai.ts",
	"google-vertex": "google-vertex.ts",
	"mistral-conversations": "mistral-conversations.ts",
	"openai-codex-responses": "openai-codex-responses.ts",
	"openai-completions": "openai-completions.ts",
	"openai-responses": "openai-responses.ts",
	"pi-messages": "pi-messages.ts",
};

class PayloadCapturedError extends Error {
	constructor() {
		super("pi parity payload captured");
		this.name = "PayloadCapturedError";
	}
}

function parseArgs(argv) {
	const args = {
		piRoot: process.env.PI_REPO,
		input: undefined,
	};
	for (let index = 0; index < argv.length; index += 1) {
		const arg = argv[index];
		if (arg === "--pi-root") {
			args.piRoot = argv[++index];
		} else if (arg === "--input") {
			args.input = argv[++index];
		} else if (arg === "--help" || arg === "-h") {
			console.log(`Usage: node --import /path/to/tsx/loader.mjs \\
  docs/pi-parity/differential/pi-oracle.mjs --pi-root /exact/pi [--input cases.jsonl]`);
			process.exit(0);
		} else {
			throw new Error(`unknown argument: ${arg}`);
		}
	}
	if (!args.piRoot) {
		throw new Error("--pi-root or PI_REPO is required");
	}
	args.piRoot = path.resolve(args.piRoot);
	if (args.input) {
		args.input = path.resolve(args.input);
	}
	return args;
}

async function readStdin() {
	const chunks = [];
	for await (const chunk of process.stdin) {
		chunks.push(chunk);
	}
	return Buffer.concat(chunks).toString("utf8");
}

function parseJSONLines(text) {
	return text
		.split(/\r?\n/u)
		.map((line) => line.trim())
		.filter((line) => line && !line.startsWith("#"))
		.map((line) => JSON.parse(line));
}

function jsonValue(value) {
	return JSON.parse(JSON.stringify(value));
}

function fakeAPIKey(api) {
	if (api === "openai-codex-responses") {
		return "e30.eyJodHRwczovL2FwaS5vcGVuYWkuY29tL2F1dGgiOnsiY2hhdGdwdF9hY2NvdW50X2lkIjoiYWNjdC1waS1wYXJpdHkifX0.eA";
	}
	return "pi-parity-key";
}

function snakeMistralPayload(value) {
	const renamed = new Map([
		["imageUrl", "image_url"],
		["maxTokens", "max_tokens"],
		["promptCacheKey", "prompt_cache_key"],
		["promptMode", "prompt_mode"],
		["reasoningEffort", "reasoning_effort"],
		["toolCallId", "tool_call_id"],
		["toolCalls", "tool_calls"],
		["toolChoice", "tool_choice"],
	]);
	const visit = (entry) => {
		if (Array.isArray(entry)) {
			return entry.map(visit);
		}
		if (entry === null || typeof entry !== "object") {
			return entry;
		}
		return Object.fromEntries(
			Object.entries(entry).map(([key, child]) => [renamed.get(key) ?? key, visit(child)]),
		);
	};
	return visit(value);
}

function canonicalGooglePayload(payload) {
	const config = { ...(payload.config ?? {}) };
	const systemInstruction = config.systemInstruction;
	const tools = config.tools;
	const toolConfig = config.toolConfig;
	delete config.systemInstruction;
	delete config.tools;
	delete config.toolConfig;

	const result = {
		model: payload.model,
		contents: payload.contents,
		config,
	};
	if (systemInstruction !== undefined) {
		result.systemInstruction =
			typeof systemInstruction === "string"
				? { parts: [{ text: systemInstruction }] }
				: systemInstruction;
	}
	if (tools !== undefined) {
		result.tools = tools;
	}
	if (toolConfig !== undefined) {
		result.toolConfig = toolConfig;
	}
	return result;
}

function canonicalBedrockPayload(payload) {
	const result = {};
	for (const key of ["system", "messages", "toolConfig", "additionalModelRequestFields"]) {
		if (payload[key] !== undefined) {
			result[key] = payload[key];
		}
	}
	return result;
}

function canonicalPayload(api, payload) {
	const value = jsonValue(payload);
	switch (api) {
		case "bedrock-converse-stream":
			return canonicalBedrockPayload(value);
		case "google-generative-ai":
		case "google-vertex":
			return canonicalGooglePayload(value);
		case "mistral-conversations":
			return snakeMistralPayload(value);
		default:
			return value;
	}
}

function withTimeout(promise, milliseconds, label) {
	let timeout;
	const deadline = new Promise((_, reject) => {
		timeout = setTimeout(() => reject(new Error(`timed out waiting for ${label}`)), milliseconds);
		timeout.unref?.();
	});
	return Promise.race([promise, deadline]).finally(() => clearTimeout(timeout));
}

class PiOracle {
	constructor(piRoot) {
		this.piRoot = piRoot;
		this.modules = new Map();
	}

	async importSource(relative) {
		const source = path.join(this.piRoot, "packages/ai/src", relative);
		return import(pathToFileURL(source).href);
	}

	async module(relative) {
		if (!this.modules.has(relative)) {
			this.modules.set(relative, this.importSource(relative));
		}
		return this.modules.get(relative);
	}

	async cost(input) {
		const models = await this.module("models.ts");
		const usage = structuredClone(input.usage);
		return models.calculateCost(input.model, usage);
	}

	async payload(input) {
		const source = PAYLOAD_MODULES[input.api];
		if (!source) {
			throw new Error(`no Pi payload adapter for API ${JSON.stringify(input.api)}`);
		}
		const implementation = await this.module(path.join("api", source));
		if (typeof implementation.streamSimple !== "function") {
			throw new Error(`${source} does not export streamSimple`);
		}

		let resolveCapture;
		const captured = new Promise((resolve) => {
			resolveCapture = resolve;
		});
		const options = {
			...(input.options ?? {}),
			apiKey: fakeAPIKey(input.api),
			env:
				input.api === "bedrock-converse-stream"
					? { ...(input.options?.env ?? {}), AWS_BEDROCK_SKIP_AUTH: "1" }
					: input.options?.env,
			onPayload(payload) {
				resolveCapture(canonicalPayload(input.api, payload));
				throw new PayloadCapturedError();
			},
		};
		implementation.streamSimple(input.model, input.context, options);
		return withTimeout(captured, 5_000, `${input.api} payload`);
	}

	async execute(request) {
		const result = {
			schemaVersion: SCHEMA_VERSION,
			id: request.id,
			kind: request.kind,
		};
		try {
			if (request.schemaVersion !== SCHEMA_VERSION) {
				throw new Error(`unsupported schemaVersion ${request.schemaVersion}`);
			}
			switch (request.kind) {
				case "cost":
					result.output = await this.cost(request.input);
					break;
				case "payload":
					result.output = await this.payload(request.input);
					break;
				default:
					throw new Error(`unsupported conformance kind ${JSON.stringify(request.kind)}`);
			}
		} catch (error) {
			result.error = {
				name: error instanceof Error ? error.name : "Error",
				message: error instanceof Error ? error.message : String(error),
			};
		}
		return result;
	}
}

async function main() {
	const args = parseArgs(process.argv.slice(2));
	const input = args.input ? fs.readFileSync(args.input, "utf8") : await readStdin();
	const cases = parseJSONLines(input);
	const oracle = new PiOracle(args.piRoot);
	for (const testCase of cases) {
		const result = await oracle.execute(testCase);
		process.stdout.write(`${JSON.stringify(result)}\n`);
	}
}

main().catch((error) => {
	console.error(error.message);
	process.exitCode = 1;
});
