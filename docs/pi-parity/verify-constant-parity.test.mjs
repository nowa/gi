import assert from "node:assert/strict";
import test from "node:test";

import { splitTopLevelAlternatives } from "./verify-constant-parity.mjs";

test("splitTopLevelAlternatives preserves nested groups and escaped pipes", () => {
	assert.deepEqual(
		splitTopLevelAlternatives(
			String.raw`(?i)(first|second(?: nested| alternate)|literal\|pipe|[a|b]|last)`,
		),
		[
			"first",
			"second(?: nested| alternate)",
			String.raw`literal\|pipe`,
			"[a|b]",
			"last",
		],
	);
});

test("splitTopLevelAlternatives accepts an unwrapped expression", () => {
	assert.deepEqual(splitTopLevelAlternatives("first|second"), ["first", "second"]);
});
