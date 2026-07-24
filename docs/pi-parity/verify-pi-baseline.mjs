#!/usr/bin/env node

import os from "node:os";
import path from "node:path";
import process from "node:process";

import { readJson, verifyPiCheckout } from "./parity-baseline-lib.mjs";

function expandHome(value) {
	if (value === "~") {
		return os.homedir();
	}
	if (value.startsWith("~/")) {
		return path.join(os.homedir(), value.slice(2));
	}
	return path.resolve(value);
}

function parseArgs(argv) {
	const args = {
		giRoot: process.cwd(),
		piRoot: process.env.PI_REPO || path.join(os.homedir(), "Projects/agents/pi"),
		baseline: "docs/pi-parity/baseline.json",
		format: "text",
		allowDirty: false,
	};
	for (let index = 0; index < argv.length; index += 1) {
		const arg = argv[index];
		if (arg === "--gi-root") {
			args.giRoot = argv[++index];
		} else if (arg === "--pi-root") {
			args.piRoot = argv[++index];
		} else if (arg === "--baseline") {
			args.baseline = argv[++index];
		} else if (arg === "--format") {
			args.format = argv[++index];
		} else if (arg === "--allow-dirty") {
			args.allowDirty = true;
		} else if (arg === "--help" || arg === "-h") {
			console.log(`Usage: node docs/pi-parity/verify-pi-baseline.mjs [options]

Options:
  --pi-root <path>   Pi checkout to verify
  --gi-root <path>   Gi repository root
  --baseline <path>  Baseline JSON relative to Gi root
  --allow-dirty      Permit uncommitted paths in the Pi checkout
  --format text|json Output format`);
			process.exit(0);
		} else {
			throw new Error(`unknown argument: ${arg}`);
		}
	}
	if (!["text", "json"].includes(args.format)) {
		throw new Error(`unsupported --format value: ${args.format}`);
	}
	args.giRoot = expandHome(args.giRoot);
	args.piRoot = expandHome(args.piRoot);
	args.baseline = path.isAbsolute(args.baseline)
		? args.baseline
		: path.join(args.giRoot, args.baseline);
	return args;
}

function main() {
	const args = parseArgs(process.argv.slice(2));
	const baseline = readJson(args.baseline);
	const result = verifyPiCheckout(baseline, args.piRoot, { allowDirty: args.allowDirty });
	if (args.format === "json") {
		console.log(JSON.stringify(result, null, 2));
	} else if (result.ok) {
		console.log(
			`Pi baseline verified: ${result.expected.tag}@${result.expected.commit} (${result.packages.length} packages at ${result.expected.releaseVersion})`,
		);
	} else {
		console.error(
			`Pi baseline verification failed for ${result.expected.tag}@${result.expected.commit}:`,
		);
		for (const error of result.errors) {
			console.error(`  - ${error}`);
		}
	}
	if (!result.ok) {
		process.exitCode = 1;
	}
}

try {
	main();
} catch (error) {
	console.error(error.message);
	process.exitCode = 1;
}
