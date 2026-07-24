import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import {
	assertMatchingBaseline,
	buildGapSnapshot,
	compareGapSnapshots,
	gapEntries,
	gapSummary,
	verifyPiCheckout,
} from "./parity-baseline-lib.mjs";

const baseline = {
	upstream: {
		tag: "v0.82.0",
		commit: "083e61621276bff9f6faefab87ce07fcd98734e2",
	},
};

function snapshot() {
	return buildGapSnapshot(
		baseline,
		{
			scope: "members",
			results: [
				{
					name: "llm",
					missingFiles: ["z.ts", "a.ts", "a.ts"],
					missingSymbols: ["z.ts::stream"],
					files: [{ noisy: true }],
				},
			],
		},
		{
			results: [{ name: "llm", missingDirectories: ["packages/ai/src/auth"] }],
		},
		{
			results: [
				{
					name: "llm",
					undocumentedFiles: [{ relative: "packages/ai/test/retry.test.ts", noisy: true }],
					noCandidateFiles: [{ relative: "packages/ai/test/retry.test.ts", noisy: true }],
					noCandidateCases: [
						{
							file: "packages/ai/test/retry.test.ts",
							line: 20,
							name: "aborts retry waits",
						},
					],
					files: [{ noisy: true }],
				},
			],
		},
	);
}

test("buildGapSnapshot keeps only deterministic open-gap fields", () => {
	const value = snapshot();
	assert.deepEqual(value.source[0].missingFiles, ["a.ts", "z.ts"]);
	assert.equal(value.source[0].files, undefined);
	assert.equal(gapEntries(value).size, 7);
	assert.deepEqual(gapSummary(value), {
		total: 7,
		byKind: {
			"module-boundary": 1,
			"source-file": 2,
			"source-symbol": 1,
			"test-no-candidate-case": 1,
			"test-no-candidate-file": 1,
			"test-undocumented-file": 1,
		},
		byModule: { llm: 7 },
	});
});

test("compareGapSnapshots separates new gaps from resolved debt", () => {
	const known = snapshot();
	const current = structuredClone(known);
	current.source[0].missingFiles = ["a.ts", "new.ts"];
	const comparison = compareGapSnapshots(known, current);
	assert.deepEqual(
		comparison.unexpected.map((entry) => entry.value),
		["new.ts"],
	);
	assert.deepEqual(
		comparison.resolved.map((entry) => entry.value),
		["z.ts"],
	);
});

test("verifyPiCheckout requires the exact clean tagged release", (t) => {
	const root = fs.mkdtempSync(path.join(os.tmpdir(), "gi-pi-baseline-test-"));
	t.after(() => fs.rmSync(root, { recursive: true, force: true }));
	const modules = [
		["llm", "packages/ai"],
		["agent", "packages/agent"],
		["tui", "packages/tui"],
		["coding", "packages/coding-agent"],
	];
	for (const [, packagePath] of modules) {
		const directory = path.join(root, packagePath);
		fs.mkdirSync(directory, { recursive: true });
		fs.writeFileSync(path.join(directory, "package.json"), '{"version":"0.82.0"}\n');
	}
	const git = (...args) => {
		const result = spawnSync("git", ["-C", root, ...args], { encoding: "utf8" });
		assert.equal(result.status, 0, result.stderr);
		return result.stdout.trim();
	};
	git("init", "-q");
	git("add", "packages");
	git("-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-qm", "fixture");
	const commit = git("rev-parse", "HEAD");
	git("tag", "v0.82.0");
	const fixtureBaseline = {
		upstream: {
			tag: "v0.82.0",
			commit,
			releaseVersion: "0.82.0",
		},
		scope: {
			modules: modules.map(([name, piPackage]) => ({ name, piPackage })),
		},
	};
	assert.equal(verifyPiCheckout(fixtureBaseline, root).ok, true);
	fs.writeFileSync(path.join(root, "dirty.txt"), "dirty\n");
	const dirty = verifyPiCheckout(fixtureBaseline, root);
	assert.equal(dirty.ok, false);
	assert.match(dirty.errors.join("\n"), /uncommitted path/);
	assert.equal(verifyPiCheckout(fixtureBaseline, root, { allowDirty: true }).ok, true);
});

test("assertMatchingBaseline rejects a snapshot for another Pi release", () => {
	const value = snapshot();
	assert.doesNotThrow(() => assertMatchingBaseline(baseline, value, "known gaps"));
	value.baseline.tag = "v0.83.0";
	assert.throws(
		() => assertMatchingBaseline(baseline, value, "known gaps"),
		/targets v0\.83\.0@/,
	);
});
