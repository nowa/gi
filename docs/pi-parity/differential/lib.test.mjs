import assert from "node:assert/strict";
import test from "node:test";

import {
	compareResultSets,
	firstJSONDifference,
	generateCostCases,
	parseJSONLines,
	stableJSONStringify,
} from "./lib.mjs";

test("JSONL parsing skips comments and identifies malformed lines", () => {
	assert.deepEqual(parseJSONLines('# fixture\n{"id":"one"}\n\n{"id":"two"}\n'), [
		{ id: "one" },
		{ id: "two" },
	]);
	assert.throws(() => parseJSONLines('{"id":"one"}\n{', "fixture"), /fixture:2:/);
});

test("stable JSON comparison ignores object key order but not values", () => {
	assert.equal(stableJSONStringify({ b: 2, a: [{ z: 1, y: 0 }] }), '{"a":[{"y":0,"z":1}],"b":2}');
	assert.equal(firstJSONDifference({ a: 1, b: 2 }, { b: 2, a: 1 }), undefined);
	assert.deepEqual(firstJSONDifference({ a: [1, 2] }, { a: [1, 3] }), {
		path: "$.a[1]",
		expected: "2",
		actual: "3",
	});
});

test("result comparison detects missing, unexpected, and changed cases", () => {
	const comparison = compareResultSets(
		[
			{ id: "same", output: { a: 1 } },
			{ id: "changed", output: { a: 1 } },
			{ id: "missing", output: {} },
		],
		[
			{ id: "changed", output: { a: 2 } },
			{ id: "same", output: { a: 1 } },
			{ id: "extra", output: {} },
		],
	);
	assert.equal(comparison.matched, 1);
	assert.deepEqual(
		comparison.failures.map((failure) => failure.id),
		["changed", "missing", "extra"],
	);
});

test("cost matrix generation is deterministic and covers tier boundaries", () => {
	const first = generateCostCases(12, 1234);
	const second = generateCostCases(12, 1234);
	assert.deepEqual(first, second);
	assert.notDeepEqual(first, generateCostCases(12, 1235));
	assert.equal(first[0].input.model.cost.tiers, undefined);
	assert.ok(first.some((entry) => entry.input.model.cost.tiers?.length > 0));
	assert.ok(
		first.some((entry) =>
			entry.input.model.cost.tiers?.some(
				(tier) =>
					tier.inputTokensAbove ===
					entry.input.usage.input + entry.input.usage.cacheRead + entry.input.usage.cacheWrite,
			),
		),
	);
});
