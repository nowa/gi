#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import process from "node:process";

import {
	compareResultSets,
	formatJSONLines,
	generateCostCases,
	parseJSONLines,
} from "./differential/lib.mjs";
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

function parseInteger(value, name) {
	const parsed = Number(value);
	if (!Number.isSafeInteger(parsed) || parsed < 0) {
		throw new Error(`${name} must be a non-negative safe integer, got ${JSON.stringify(value)}`);
	}
	return parsed;
}

function parseArgs(argv) {
	const args = {
		giRoot: process.cwd(),
		piRoot: process.env.PI_REPO || path.join(os.homedir(), "Projects/agents/pi"),
		piRuntimeRoot: process.env.PI_RUNTIME_REPO,
		tsxLoader: process.env.PI_TSX_LOADER,
		baseline: "docs/pi-parity/baseline.json",
		cases: "docs/pi-parity/differential/cases.jsonl",
		fixture: "docs/pi-parity/differential/pi-v0.82.1.jsonl",
		randomCases: 512,
		seed: 0x82_01_2026,
		format: "text",
		out: undefined,
		allowDirty: false,
	};

	for (let index = 0; index < argv.length; index += 1) {
		const arg = argv[index];
		if (arg === "--gi-root") {
			args.giRoot = argv[++index];
		} else if (arg === "--pi-root") {
			args.piRoot = argv[++index];
		} else if (arg === "--pi-runtime-root") {
			args.piRuntimeRoot = argv[++index];
		} else if (arg === "--tsx-loader") {
			args.tsxLoader = argv[++index];
		} else if (arg === "--baseline") {
			args.baseline = argv[++index];
		} else if (arg === "--cases") {
			args.cases = argv[++index];
		} else if (arg === "--fixture") {
			args.fixture = argv[++index];
		} else if (arg === "--random-cases") {
			args.randomCases = parseInteger(argv[++index], "--random-cases");
		} else if (arg === "--seed") {
			args.seed = parseInteger(argv[++index], "--seed");
		} else if (arg === "--format") {
			args.format = argv[++index];
		} else if (arg === "--out") {
			args.out = argv[++index];
		} else if (arg === "--allow-dirty") {
			args.allowDirty = true;
		} else if (arg === "--help" || arg === "-h") {
			console.log(`Usage: node docs/pi-parity/verify-differential-parity.mjs [options]

Options:
  --pi-root <path>          Exact Pi baseline checkout used as the source oracle
  --pi-runtime-root <path>  Checkout providing node_modules only
  --tsx-loader <path>       Explicit tsx ESM loader path
  --gi-root <path>          Gi repository root
  --baseline <path>         Baseline JSON relative to Gi root
  --cases <path>            Fixed differential cases in JSONL format
  --fixture <path>          Committed Pi result fixture in JSONL format
  --random-cases <count>    Deterministic generated cost cases (default: 512)
  --seed <integer>          Cost-matrix seed (default: 0x82012026)
  --allow-dirty             Permit an intentionally dirty Pi source checkout
  --format text|json        Output format
  --out <path>              Write output instead of stdout`);
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
	if (args.piRuntimeRoot) {
		args.piRuntimeRoot = expandHome(args.piRuntimeRoot);
	}
	if (args.tsxLoader) {
		args.tsxLoader = expandHome(args.tsxLoader);
	}
	for (const key of ["baseline", "cases", "fixture"]) {
		args[key] = path.isAbsolute(args[key]) ? args[key] : path.join(args.giRoot, args[key]);
	}
	if (args.out) {
		args.out = path.isAbsolute(args.out) ? args.out : path.join(args.giRoot, args.out);
	}
	return args;
}

function resolveRuntime(args) {
	const roots = [
		args.piRuntimeRoot,
		args.piRoot,
		path.join(os.homedir(), "Projects/agents/pi"),
	].filter(Boolean);
	const loaders = [
		args.tsxLoader,
		...roots.map((root) => path.join(root, "node_modules/tsx/dist/loader.mjs")),
	].filter(Boolean);
	const loader = loaders.find((candidate) => fs.existsSync(candidate));
	if (!loader) {
		throw new Error(
			[
				"tsx loader not found.",
				"Install Pi dependencies in the exact checkout, or point runtime-only dependencies at another checkout:",
				"  --pi-runtime-root ~/Projects/agents/pi",
			].join("\n"),
		);
	}

	const runtimeRoot =
		args.piRuntimeRoot ??
		roots.find((root) => loader.startsWith(`${path.resolve(root)}${path.sep}`)) ??
		args.piRoot;
	return {
		root: path.resolve(runtimeRoot),
		loader: path.resolve(loader),
	};
}

function runCommand(command, commandArgs, options) {
	const result = spawnSync(command, commandArgs, {
		cwd: options.cwd,
		env: options.env,
		input: options.input,
		encoding: "utf8",
		maxBuffer: 128 * 1024 * 1024,
		timeout: 120_000,
	});
	if (result.error) {
		throw new Error(`${options.label}: ${result.error.message}`);
	}
	if (result.status !== 0) {
		throw new Error(
			`${options.label} exited ${result.status}: ${result.stderr.trim() || result.stdout.trim() || "no output"}`,
		);
	}
	if (result.stderr.trim()) {
		throw new Error(`${options.label} wrote unexpected stderr: ${result.stderr.trim()}`);
	}
	return parseJSONLines(result.stdout, `${options.label} output`);
}

function runPiOracle(args, runtime, input) {
	const resolver = path.join(args.giRoot, "docs/pi-parity/differential/runtime-resolver.mjs");
	const oracle = path.join(args.giRoot, "docs/pi-parity/differential/pi-oracle.mjs");
	return runCommand(
		process.execPath,
		[
			"--import",
			resolver,
			"--import",
			runtime.loader,
			oracle,
			"--pi-root",
			args.piRoot,
		],
		{
			cwd: args.giRoot,
			env: {
				...process.env,
				PI_RUNTIME_REPO: runtime.root,
			},
			input,
			label: "Pi oracle",
		},
	);
}

function runGiOracle(args, input) {
	const goCache =
		process.env.GOCACHE && path.isAbsolute(process.env.GOCACHE)
			? process.env.GOCACHE
			: path.join(os.tmpdir(), "gi-parity-gocache");
	fs.mkdirSync(goCache, { recursive: true });
	return runCommand(
		"go",
		["run", "./gi-llm-provider/internal/cmd/conformance"],
		{
			cwd: args.giRoot,
			env: {
				...process.env,
				GOCACHE: goCache,
			},
			input,
			label: "Gi oracle",
		},
	);
}

function outputText(report) {
	const lines = [
		`Pi differential parity: ${report.ok ? "PASS" : "FAIL"}`,
		`baseline: ${report.baseline.tag} @ ${report.baseline.commit}`,
		`fixed cases: ${report.counts.fixed} (${report.counts.payload} payload, ${report.counts.fixedCost} cost)`,
		`generated cost cases: ${report.counts.randomCost} (seed ${report.seed})`,
		`fixture matches: ${report.fixtureComparison.matched}/${report.fixtureComparison.expected}`,
		`Gi matches Pi: ${report.giComparison.matched}/${report.giComparison.expected}`,
	];
	const failures = [
		...report.fixtureComparison.failures.map((failure) => ({
			scope: "fixture vs Pi",
			...failure,
		})),
		...report.giComparison.failures.map((failure) => ({
			scope: "Gi vs Pi",
			...failure,
		})),
	];
	for (const failure of failures.slice(0, 20)) {
		lines.push(
			`${failure.scope}: ${failure.id} at ${failure.path}`,
			`  expected: ${failure.expected}`,
			`  actual:   ${failure.actual}`,
		);
	}
	if (failures.length > 20) {
		lines.push(`... ${failures.length - 20} additional difference(s) omitted`);
	}
	return `${lines.join("\n")}\n`;
}

function writeOutput(args, report) {
	const output = args.format === "json" ? `${JSON.stringify(report, null, 2)}\n` : outputText(report);
	if (args.out) {
		fs.mkdirSync(path.dirname(args.out), { recursive: true });
		fs.writeFileSync(args.out, output);
	} else {
		process.stdout.write(output);
	}
}

function main() {
	const args = parseArgs(process.argv.slice(2));
	const baseline = readJson(args.baseline);
	const checkout = verifyPiCheckout(baseline, args.piRoot, {
		allowDirty: args.allowDirty,
	});
	if (!checkout.ok) {
		throw new Error(`Pi source oracle is not the exact baseline:\n- ${checkout.errors.join("\n- ")}`);
	}
	const runtime = resolveRuntime(args);
	const fixedCases = parseJSONLines(fs.readFileSync(args.cases, "utf8"), args.cases);
	const fixture = parseJSONLines(fs.readFileSync(args.fixture, "utf8"), args.fixture);
	const randomCases = generateCostCases(args.randomCases, args.seed);
	const allCases = [...fixedCases, ...randomCases];
	const input = formatJSONLines(allCases);

	const piResults = runPiOracle(args, runtime, input);
	const giResults = runGiOracle(args, input);
	const liveFixedPiResults = piResults.slice(0, fixedCases.length);
	const fixtureComparison = compareResultSets(fixture, liveFixedPiResults, {
		expected: "committed Pi fixture",
		actual: "live Pi oracle",
	});
	const giComparison = compareResultSets(piResults, giResults, {
		expected: "live Pi oracle",
		actual: "Gi oracle",
	});
	const report = {
		schemaVersion: 1,
		ok:
			checkout.ok &&
			fixtureComparison.failures.length === 0 &&
			giComparison.failures.length === 0,
		baseline: {
			tag: baseline.upstream.tag,
			commit: baseline.upstream.commit,
			releaseVersion: baseline.upstream.releaseVersion,
		},
		piRoot: args.piRoot,
		runtimeRoot: runtime.root,
		seed: args.seed,
		counts: {
			fixed: fixedCases.length,
			payload: fixedCases.filter((entry) => entry.kind === "payload").length,
			fixedCost: fixedCases.filter((entry) => entry.kind === "cost").length,
			randomCost: randomCases.length,
			total: allCases.length,
		},
		fixtureComparison,
		giComparison,
	};
	writeOutput(args, report);
	if (!report.ok) {
		process.exitCode = 1;
	}
}

try {
	main();
} catch (error) {
	console.error(error instanceof Error ? error.message : String(error));
	process.exitCode = 2;
}
