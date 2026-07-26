export const DIFFERENTIAL_SCHEMA_VERSION = 1;

export function parseJSONLines(text, label = "JSONL") {
	const values = [];
	for (const [index, source] of text.split(/\r?\n/u).entries()) {
		const line = source.trim();
		if (!line || line.startsWith("#")) {
			continue;
		}
		try {
			values.push(JSON.parse(line));
		} catch (error) {
			throw new Error(`${label}:${index + 1}: ${error.message}`);
		}
	}
	return values;
}

export function formatJSONLines(values) {
	return values.map((value) => JSON.stringify(value)).join("\n") + (values.length > 0 ? "\n" : "");
}

export function canonicalizeJSON(value) {
	if (Array.isArray(value)) {
		return value.map(canonicalizeJSON);
	}
	if (value !== null && typeof value === "object") {
		return Object.fromEntries(
			Object.keys(value)
				.sort()
				.map((key) => [key, canonicalizeJSON(value[key])]),
		);
	}
	return value;
}

export function stableJSONStringify(value) {
	return JSON.stringify(canonicalizeJSON(value));
}

function describeValue(value) {
	const encoded = stableJSONStringify(value);
	return encoded === undefined ? String(value) : encoded;
}

export function firstJSONDifference(expected, actual, path = "$") {
	if (Object.is(expected, actual)) {
		return undefined;
	}
	if (typeof expected !== typeof actual || expected === null || actual === null) {
		return {
			path,
			expected: describeValue(expected),
			actual: describeValue(actual),
		};
	}
	if (Array.isArray(expected) || Array.isArray(actual)) {
		if (!Array.isArray(expected) || !Array.isArray(actual)) {
			return {
				path,
				expected: describeValue(expected),
				actual: describeValue(actual),
			};
		}
		if (expected.length !== actual.length) {
			return {
				path: `${path}.length`,
				expected: String(expected.length),
				actual: String(actual.length),
			};
		}
		for (let index = 0; index < expected.length; index += 1) {
			const difference = firstJSONDifference(expected[index], actual[index], `${path}[${index}]`);
			if (difference) {
				return difference;
			}
		}
		return undefined;
	}
	if (typeof expected === "object") {
		const expectedKeys = Object.keys(expected).sort();
		const actualKeys = Object.keys(actual).sort();
		const keyDifference = firstJSONDifference(expectedKeys, actualKeys, `${path}.[keys]`);
		if (keyDifference) {
			return keyDifference;
		}
		for (const key of expectedKeys) {
			const difference = firstJSONDifference(expected[key], actual[key], `${path}.${key}`);
			if (difference) {
				return difference;
			}
		}
		return undefined;
	}
	return {
		path,
		expected: describeValue(expected),
		actual: describeValue(actual),
	};
}

function indexResults(results, label) {
	const indexed = new Map();
	for (const result of results) {
		if (!result || typeof result.id !== "string" || result.id.length === 0) {
			throw new Error(`${label} contains a result without an id`);
		}
		if (indexed.has(result.id)) {
			throw new Error(`${label} contains duplicate id ${JSON.stringify(result.id)}`);
		}
		indexed.set(result.id, result);
	}
	return indexed;
}

export function compareResultSets(expectedResults, actualResults, labels = {}) {
	const expectedLabel = labels.expected ?? "expected";
	const actualLabel = labels.actual ?? "actual";
	const expected = indexResults(expectedResults, expectedLabel);
	const actual = indexResults(actualResults, actualLabel);
	const failures = [];

	for (const [id, expectedResult] of expected) {
		const actualResult = actual.get(id);
		if (!actualResult) {
			failures.push({
				id,
				path: "$",
				expected: "result",
				actual: "missing",
			});
			continue;
		}
		const difference = firstJSONDifference(expectedResult, actualResult);
		if (difference) {
			failures.push({ id, ...difference });
		}
	}
	for (const id of actual.keys()) {
		if (!expected.has(id)) {
			failures.push({
				id,
				path: "$",
				expected: "missing",
				actual: "unexpected result",
			});
		}
	}

	return {
		expected: expected.size,
		actual: actual.size,
		matched: expected.size - failures.filter((failure) => expected.has(failure.id)).length,
		failures,
	};
}

function createPRNG(seed) {
	let state = seed >>> 0;
	return () => {
		state = (state + 0x6d2b79f5) >>> 0;
		let value = state;
		value = Math.imul(value ^ (value >>> 15), value | 1);
		value ^= value + Math.imul(value ^ (value >>> 7), value | 61);
		return ((value ^ (value >>> 14)) >>> 0) / 0x1_0000_0000;
	};
}

function randomInteger(random, maximum) {
	return Math.floor(random() * (maximum + 1));
}

function pick(random, values) {
	return values[randomInteger(random, values.length - 1)];
}

function shuffle(random, values) {
	const result = [...values];
	for (let index = result.length - 1; index > 0; index -= 1) {
		const target = randomInteger(random, index);
		[result[index], result[target]] = [result[target], result[index]];
	}
	return result;
}

const COST_RATES = [
	0, 0.01, 0.03, 0.075, 0.1, 0.15, 0.1875, 0.2, 0.3, 0.4, 0.5, 0.6, 0.8, 1, 1.25,
	2, 2.5, 3, 3.75, 4, 5, 6, 7.5, 8, 10, 12, 15, 18, 24, 25,
];

function randomRates(random) {
	return {
		input: pick(random, COST_RATES),
		output: pick(random, COST_RATES),
		cacheRead: pick(random, COST_RATES),
		cacheWrite: pick(random, COST_RATES),
	};
}

export function generateCostCases(count, seed = 0x82_01_2026) {
	if (!Number.isSafeInteger(count) || count < 0) {
		throw new Error(`random cost case count must be a non-negative safe integer, got ${count}`);
	}
	if (!Number.isSafeInteger(seed)) {
		throw new Error(`random cost seed must be a safe integer, got ${seed}`);
	}

	const random = createPRNG(seed);
	const cases = [];
	for (let index = 0; index < count; index += 1) {
		const input = randomInteger(random, 500_000);
		const output = randomInteger(random, 100_000);
		const cacheRead = randomInteger(random, 500_000);
		const cacheWrite = randomInteger(random, 500_000);
		const cacheWrite1h = randomInteger(random, cacheWrite);
		const requestInput = input + cacheRead + cacheWrite;
		const cost = randomRates(random);

		if (index % 3 !== 0) {
			const anchorThreshold =
				index % 3 === 1 ? requestInput : Math.max(0, requestInput - 1);
			const thresholdCandidates = [
				0,
				Math.max(0, requestInput - 1),
				requestInput,
				requestInput + 1,
				100_000,
				150_000,
				200_000,
			];
			const thresholdCount = 1 + randomInteger(random, 2);
			const thresholds = [
				...new Set([
					anchorThreshold,
					...shuffle(
						random,
						thresholdCandidates.filter((threshold) => threshold !== anchorThreshold),
					).slice(0, thresholdCount - 1),
				]),
			];
			cost.tiers = shuffle(
				random,
				thresholds.map((inputTokensAbove) => ({
					inputTokensAbove,
					...randomRates(random),
				})),
			);
		}

		cases.push({
			schemaVersion: DIFFERENTIAL_SCHEMA_VERSION,
			id: `cost/random-${String(index).padStart(4, "0")}`,
			kind: "cost",
			input: {
				model: {
					id: `cost-random-${index}`,
					name: `Cost random ${index}`,
					api: "openai-responses",
					provider: "parity",
					reasoning: false,
					cost,
					contextWindow: 2_000_000,
					maxTokens: 128_000,
				},
				usage: {
					input,
					output,
					cacheRead,
					cacheWrite,
					cacheWrite1h,
					totalTokens: requestInput + output,
					cost: {
						input: 0,
						output: 0,
						cacheRead: 0,
						cacheWrite: 0,
						total: 0,
					},
				},
			},
		});
	}
	return cases;
}
