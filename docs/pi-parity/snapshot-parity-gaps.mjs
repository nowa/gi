#!/usr/bin/env node

import os from "node:os";
import path from "node:path";
import process from "node:process";

import {
	collectCurrentGaps,
	gapSummary,
	readJson,
	verifyPiCheckout,
	writeJson,
} from "./parity-baseline-lib.mjs";

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
		out: "",
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
		} else if (arg === "--out") {
			args.out = argv[++index];
		} else if (arg === "--allow-dirty") {
			args.allowDirty = true;
		} else if (arg === "--help" || arg === "-h") {
			console.log(`Usage: node docs/pi-parity/snapshot-parity-gaps.mjs [options]

Options:
  --pi-root <path>   Exact Pi baseline checkout
  --gi-root <path>   Gi repository root
  --baseline <path>  Baseline JSON relative to Gi root
  --out <path>       Write deterministic gap snapshot to this path
  --allow-dirty      Permit an intentionally dirty Pi checkout`);
			process.exit(0);
		} else {
			throw new Error(`unknown argument: ${arg}`);
		}
	}
	args.giRoot = expandHome(args.giRoot);
	args.piRoot = expandHome(args.piRoot);
	args.baseline = path.isAbsolute(args.baseline)
		? args.baseline
		: path.join(args.giRoot, args.baseline);
	args.out = args.out ? expandHome(args.out) : "";
	return args;
}

function main() {
	const args = parseArgs(process.argv.slice(2));
	const baseline = readJson(args.baseline);
	const verification = verifyPiCheckout(baseline, args.piRoot, { allowDirty: args.allowDirty });
	if (!verification.ok) {
		throw new Error(`refusing to snapshot an invalid Pi checkout:\n${verification.errors.join("\n")}`);
	}
	const snapshot = collectCurrentGaps(args.giRoot, args.piRoot, baseline);
	if (args.out) {
		writeJson(args.out, snapshot);
	} else {
		console.log(JSON.stringify(snapshot, null, 2));
	}
	const summary = gapSummary(snapshot);
	console.error(
		`Captured ${summary.total} open gaps for ${baseline.upstream.tag}: ${JSON.stringify(summary.byModule)}`,
	);
}

try {
	main();
} catch (error) {
	console.error(error.message);
	process.exitCode = 1;
}
