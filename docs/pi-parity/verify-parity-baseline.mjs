#!/usr/bin/env node

import os from "node:os";
import path from "node:path";
import process from "node:process";

import {
	assertMatchingBaseline,
	collectCurrentGaps,
	compareGapSnapshots,
	gapSummary,
	readJson,
	verifyPiCheckout,
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
		knownGaps: "docs/pi-parity/v0.82.0-open-gaps.json",
		requireClosed: false,
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
		} else if (arg === "--known-gaps") {
			args.knownGaps = argv[++index];
		} else if (arg === "--require-closed") {
			args.requireClosed = true;
		} else if (arg === "--allow-dirty") {
			args.allowDirty = true;
		} else if (arg === "--format") {
			args.format = argv[++index];
		} else if (arg === "--help" || arg === "-h") {
			console.log(`Usage: node docs/pi-parity/verify-parity-baseline.mjs [options]

Options:
  --pi-root <path>    Exact Pi baseline checkout
  --gi-root <path>    Gi repository root
  --baseline <path>   Baseline JSON relative to Gi root
  --known-gaps <path> Accepted open-gap snapshot relative to Gi root
  --require-closed    Fail unless the current and recorded gap sets are empty
  --allow-dirty       Permit an intentionally dirty Pi checkout
  --format text|json  Output format`);
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
	for (const key of ["baseline", "knownGaps"]) {
		args[key] = path.isAbsolute(args[key]) ? args[key] : path.join(args.giRoot, args[key]);
	}
	return args;
}

function printEntries(label, entries) {
	if (entries.length === 0) {
		return;
	}
	console.error(`${label} (${entries.length}):`);
	for (const entry of entries.slice(0, 20)) {
		console.error(`  - [${entry.module}/${entry.kind}] ${entry.value}`);
	}
	if (entries.length > 20) {
		console.error(`  - ... ${entries.length - 20} more`);
	}
}

function main() {
	const args = parseArgs(process.argv.slice(2));
	const baseline = readJson(args.baseline);
	const known = readJson(args.knownGaps);
	assertMatchingBaseline(baseline, known, "known-gap snapshot");
	const checkout = verifyPiCheckout(baseline, args.piRoot, { allowDirty: args.allowDirty });
	if (!checkout.ok) {
		throw new Error(`invalid Pi checkout:\n${checkout.errors.join("\n")}`);
	}
	const current = collectCurrentGaps(args.giRoot, args.piRoot, baseline);
	const comparison = compareGapSnapshots(known, current);
	const summary = gapSummary(current);
	const closedFailure = args.requireClosed && (comparison.known !== 0 || comparison.current !== 0);
	const ok =
		comparison.unexpected.length === 0 &&
		comparison.resolved.length === 0 &&
		!closedFailure;
	const result = {
		ok,
		baseline: current.baseline,
		gaps: summary,
		comparison,
		requireClosed: args.requireClosed,
	};
	if (args.format === "json") {
		console.log(JSON.stringify(result, null, 2));
	} else if (ok) {
		console.log(
			`Pi parity debt matches ${baseline.upstream.tag}: ${summary.total} known open gaps (${JSON.stringify(summary.byModule)})`,
		);
	} else {
		printEntries("Unexpected gaps", comparison.unexpected);
		printEntries("Resolved but still recorded gaps", comparison.resolved);
		if (closedFailure) {
			console.error(
				`Release gate requires zero gaps; recorded=${comparison.known}, current=${comparison.current}`,
			);
		}
	}
	if (!ok) {
		process.exitCode = 1;
	}
}

try {
	main();
} catch (error) {
	console.error(error.message);
	process.exitCode = 1;
}
