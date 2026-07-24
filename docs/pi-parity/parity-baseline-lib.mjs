import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

export function readJson(file) {
	return JSON.parse(fs.readFileSync(file, "utf8"));
}

export function writeJson(file, value) {
	fs.mkdirSync(path.dirname(file), { recursive: true });
	fs.writeFileSync(file, `${JSON.stringify(value, null, 2)}\n`);
}

export function runJsonAudit(giRoot, script, args = []) {
	const command = path.join(giRoot, script);
	const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), "gi-parity-audit-"));
	const outputFile = path.join(tempDir, "result.json");
	try {
		const result = spawnSync(
			process.execPath,
			[command, ...args, "--format", "json", "--out", outputFile],
			{
				cwd: giRoot,
				encoding: "utf8",
				maxBuffer: 128 * 1024 * 1024,
			},
		);
		if (result.error) {
			throw result.error;
		}
		if (![0, 1].includes(result.status)) {
			throw new Error(
				`${script} exited ${result.status}: ${result.stderr.trim() || result.stdout.trim() || "no output"}`,
			);
		}
		if (!fs.existsSync(outputFile)) {
			throw new Error(`${script} produced no JSON output file`);
		}
		try {
			return readJson(outputFile);
		} catch (error) {
			throw new Error(`${script} produced invalid JSON: ${error.message}`);
		}
	} finally {
		fs.rmSync(tempDir, { recursive: true, force: true });
	}
}

function sortedStrings(values) {
	return [...new Set(values ?? [])].sort();
}

function sortedFileRefs(values) {
	return sortedStrings(
		(values ?? []).map((value) => (typeof value === "string" ? value : value.relative)),
	);
}

function sortedCases(values) {
	const cases = (values ?? []).map((value) => ({
		file: value.file,
		line: value.line,
		name: value.name,
	}));
	return cases.sort(
		(a, b) => a.file.localeCompare(b.file) || a.line - b.line || a.name.localeCompare(b.name),
	);
}

export function buildGapSnapshot(baseline, source, boundaries, tests) {
	return {
		schemaVersion: 1,
		baseline: {
			tag: baseline.upstream.tag,
			commit: baseline.upstream.commit,
		},
		generatedBy: {
			sourceScope: source.scope,
			sourceVerifier: "docs/pi-parity/verify-source-map.mjs",
			boundaryVerifier: "docs/pi-parity/verify-module-boundaries.mjs",
			testVerifier: "docs/pi-parity/verify-test-case-map.mjs",
		},
		source: source.results.map((result) => ({
			module: result.name,
			missingFiles: sortedStrings(result.missingFiles),
			missingSymbols: sortedStrings(result.missingSymbols),
		})),
		boundaries: boundaries.results.map((result) => ({
			module: result.name,
			missingDirectories: sortedStrings(result.missingDirectories),
		})),
		tests: tests.results.map((result) => ({
			module: result.name,
			undocumentedFiles: sortedFileRefs(result.undocumentedFiles),
			noCandidateFiles: sortedFileRefs(result.noCandidateFiles),
			noCandidateCases: sortedCases(result.noCandidateCases),
		})),
	};
}

function addStringEntries(entries, kind, module, values) {
	for (const value of values ?? []) {
		const key = JSON.stringify([kind, module, value]);
		entries.set(key, { kind, module, value });
	}
}

export function gapEntries(snapshot) {
	const entries = new Map();
	for (const module of snapshot.source ?? []) {
		addStringEntries(entries, "source-file", module.module, module.missingFiles);
		addStringEntries(entries, "source-symbol", module.module, module.missingSymbols);
	}
	for (const module of snapshot.boundaries ?? []) {
		addStringEntries(entries, "module-boundary", module.module, module.missingDirectories);
	}
	for (const module of snapshot.tests ?? []) {
		addStringEntries(entries, "test-undocumented-file", module.module, module.undocumentedFiles);
		addStringEntries(entries, "test-no-candidate-file", module.module, module.noCandidateFiles);
		for (const testCase of module.noCandidateCases ?? []) {
			const value = `${testCase.file}:${testCase.line}:${testCase.name}`;
			const key = JSON.stringify(["test-no-candidate-case", module.module, testCase.file, testCase.line, testCase.name]);
			entries.set(key, {
				kind: "test-no-candidate-case",
				module: module.module,
				value,
			});
		}
	}
	return entries;
}

export function compareGapSnapshots(known, current) {
	const knownEntries = gapEntries(known);
	const currentEntries = gapEntries(current);
	const unexpected = [];
	const resolved = [];
	for (const [key, entry] of currentEntries) {
		if (!knownEntries.has(key)) {
			unexpected.push(entry);
		}
	}
	for (const [key, entry] of knownEntries) {
		if (!currentEntries.has(key)) {
			resolved.push(entry);
		}
	}
	const byEntry = (a, b) =>
		a.kind.localeCompare(b.kind) || a.module.localeCompare(b.module) || a.value.localeCompare(b.value);
	return {
		known: knownEntries.size,
		current: currentEntries.size,
		unexpected: unexpected.sort(byEntry),
		resolved: resolved.sort(byEntry),
	};
}

export function gapSummary(snapshot) {
	const byKind = {};
	const byModule = {};
	for (const entry of gapEntries(snapshot).values()) {
		byKind[entry.kind] = (byKind[entry.kind] ?? 0) + 1;
		byModule[entry.module] = (byModule[entry.module] ?? 0) + 1;
	}
	return {
		total: Object.values(byKind).reduce((sum, count) => sum + count, 0),
		byKind: Object.fromEntries(Object.entries(byKind).sort()),
		byModule: Object.fromEntries(Object.entries(byModule).sort()),
	};
}

export function assertMatchingBaseline(baseline, snapshot, label) {
	if (snapshot.schemaVersion !== 1) {
		throw new Error(`${label} has unsupported schemaVersion ${snapshot.schemaVersion}`);
	}
	if (
		snapshot.baseline?.tag !== baseline.upstream.tag ||
		snapshot.baseline?.commit !== baseline.upstream.commit
	) {
		throw new Error(
			`${label} targets ${snapshot.baseline?.tag ?? "unknown"}@${snapshot.baseline?.commit ?? "unknown"}, expected ${baseline.upstream.tag}@${baseline.upstream.commit}`,
		);
	}
}

function gitOutput(piRoot, args) {
	const result = spawnSync("git", ["-C", piRoot, ...args], {
		encoding: "utf8",
		maxBuffer: 16 * 1024 * 1024,
	});
	if (result.error) {
		throw result.error;
	}
	if (result.status !== 0) {
		throw new Error(result.stderr.trim() || `git ${args.join(" ")} exited ${result.status}`);
	}
	return result.stdout.trim();
}

export function verifyPiCheckout(baseline, piRoot, options = {}) {
	const errors = [];
	let head = "";
	let tags = [];
	let dirty = [];
	try {
		head = gitOutput(piRoot, ["rev-parse", "HEAD"]);
		if (head !== baseline.upstream.commit) {
			errors.push(`HEAD is ${head}, expected ${baseline.upstream.commit}`);
		}
		tags = gitOutput(piRoot, ["tag", "--points-at", "HEAD"])
			.split(/\r?\n/)
			.filter(Boolean);
		if (!tags.includes(baseline.upstream.tag)) {
			errors.push(`HEAD is not tagged ${baseline.upstream.tag}`);
		}
		dirty = gitOutput(piRoot, ["status", "--porcelain"])
			.split(/\r?\n/)
			.filter(Boolean);
		if (dirty.length > 0 && !options.allowDirty) {
			errors.push(`checkout has ${dirty.length} uncommitted path(s)`);
		}
	} catch (error) {
		errors.push(`cannot inspect Pi git checkout: ${error.message}`);
	}

	const packages = [];
	for (const module of baseline.scope.modules) {
		const packageFile = path.join(piRoot, module.piPackage, "package.json");
		if (!fs.existsSync(packageFile)) {
			errors.push(`${module.piPackage}/package.json is missing`);
			continue;
		}
		try {
			const packageJson = readJson(packageFile);
			packages.push({
				module: module.name,
				path: module.piPackage,
				version: packageJson.version,
			});
			if (packageJson.version !== baseline.upstream.releaseVersion) {
				errors.push(
					`${module.piPackage} version is ${packageJson.version}, expected ${baseline.upstream.releaseVersion}`,
				);
			}
		} catch (error) {
			errors.push(`${module.piPackage}/package.json cannot be read: ${error.message}`);
		}
	}

	return {
		ok: errors.length === 0,
		piRoot,
		expected: {
			tag: baseline.upstream.tag,
			commit: baseline.upstream.commit,
			releaseVersion: baseline.upstream.releaseVersion,
		},
		actual: { head, tags, dirtyPaths: dirty },
		packages,
		errors,
	};
}

export function collectCurrentGaps(giRoot, piRoot, baseline) {
	const source = runJsonAudit(giRoot, "docs/pi-parity/verify-source-map.mjs", [
		"--pi-root",
		piRoot,
		"--gi-root",
		giRoot,
		"--scope",
		"members",
	]);
	const boundaries = runJsonAudit(giRoot, "docs/pi-parity/verify-module-boundaries.mjs", [
		"--pi-root",
		piRoot,
		"--gi-root",
		giRoot,
	]);
	const tests = runJsonAudit(giRoot, "docs/pi-parity/verify-test-case-map.mjs", [
		"--pi-root",
		piRoot,
		"--gi-root",
		giRoot,
	]);
	return buildGapSnapshot(baseline, source, boundaries, tests);
}
