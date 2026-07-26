#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import process from "node:process";
import { pathToFileURL } from "node:url";

import { readJson, verifyPiCheckout } from "./parity-baseline-lib.mjs";

const UNIT_SCALE = {
	scalar: 1n,
	milliseconds: 1_000_000n,
	nanoseconds: 1n,
};

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
		piRuntimeRoot: process.env.PI_RUNTIME_REPO,
		typescript: process.env.PI_TYPESCRIPT_PATH,
		baseline: "docs/pi-parity/baseline.json",
		contracts: "docs/pi-parity/constant-contracts.json",
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
		} else if (arg === "--typescript") {
			args.typescript = argv[++index];
		} else if (arg === "--baseline") {
			args.baseline = argv[++index];
		} else if (arg === "--contracts") {
			args.contracts = argv[++index];
		} else if (arg === "--format") {
			args.format = argv[++index];
		} else if (arg === "--out") {
			args.out = argv[++index];
		} else if (arg === "--allow-dirty") {
			args.allowDirty = true;
		} else if (arg === "--help" || arg === "-h") {
			console.log(`Usage: node docs/pi-parity/verify-constant-parity.mjs [options]

Options:
  --pi-root <path>          Exact Pi baseline checkout (source oracle)
  --pi-runtime-root <path>  Separate Pi checkout providing node_modules only
  --typescript <path>       Explicit typescript.js compiler path
  --gi-root <path>          Gi repository root
  --baseline <path>         Baseline JSON relative to Gi root
  --contracts <path>        Constant contracts JSON relative to Gi root
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
	if (args.typescript) {
		args.typescript = expandHome(args.typescript);
	}
	for (const key of ["baseline", "contracts"]) {
		args[key] = path.isAbsolute(args[key]) ? args[key] : path.join(args.giRoot, args[key]);
	}
	if (args.out) {
		args.out = path.isAbsolute(args.out) ? args.out : path.join(args.giRoot, args.out);
	}
	return args;
}

function resolveTypeScript(args) {
	const candidates = [
		args.typescript,
		path.join(args.piRoot, "node_modules/typescript/lib/typescript.js"),
		args.piRuntimeRoot
			? path.join(args.piRuntimeRoot, "node_modules/typescript/lib/typescript.js")
			: undefined,
	].filter(Boolean);
	for (const candidate of candidates) {
		if (fs.existsSync(candidate)) {
			return candidate;
		}
	}
	throw new Error(
		[
			"TypeScript compiler not found.",
			"The Pi source checkout remains the oracle; point only parser/runtime dependencies at another checkout:",
			"  --pi-runtime-root ~/Projects/agents/pi",
			"or pass --typescript /path/to/node_modules/typescript/lib/typescript.js.",
		].join("\n"),
	);
}

async function loadTypeScript(file) {
	const imported = await import(pathToFileURL(file).href);
	return imported.default ?? imported;
}

function walkFiles(root, accept) {
	const result = [];
	const visit = (directory) => {
		const entries = fs.readdirSync(directory, { withFileTypes: true });
		entries.sort((left, right) => left.name.localeCompare(right.name));
		for (const entry of entries) {
			const target = path.join(directory, entry.name);
			if (entry.isDirectory()) {
				visit(target);
			} else if (entry.isFile() && accept(target)) {
				result.push(target);
			}
		}
	};
	visit(root);
	return result;
}

function normalizeInteger(text) {
	const normalized = text.replaceAll("_", "");
	if (!/^[+-]?(?:0[xob][0-9a-f]+|\d+)$/iu.test(normalized)) {
		return undefined;
	}
	return BigInt(normalized).toString();
}

function regexpLiteral(text) {
	let escaped = false;
	let inCharacterClass = false;
	let closingSlash = -1;
	for (let index = 1; index < text.length; index += 1) {
		const character = text[index];
		if (escaped) {
			escaped = false;
			continue;
		}
		if (character === "\\") {
			escaped = true;
			continue;
		}
		if (character === "[") {
			inCharacterClass = true;
		} else if (character === "]") {
			inCharacterClass = false;
		} else if (character === "/" && !inCharacterClass) {
			closingSlash = index;
		}
	}
	if (closingSlash < 1) {
		return { pattern: text, flags: "" };
	}
	return {
		pattern: text.slice(1, closingSlash),
		flags: text.slice(closingSlash + 1),
	};
}

function propertyName(ts, node, sourceFile) {
	if (ts.isIdentifier(node) || ts.isPrivateIdentifier?.(node)) {
		return node.text;
	}
	if (ts.isStringLiteral(node) || ts.isNumericLiteral(node)) {
		return node.text;
	}
	return node.getText(sourceFile);
}

function unwrapExpression(ts, node) {
	let current = node;
	for (;;) {
		if (
			ts.isParenthesizedExpression(current) ||
			ts.isAsExpression(current) ||
			ts.isTypeAssertionExpression(current) ||
			ts.isNonNullExpression(current) ||
			ts.isSatisfiesExpression?.(current)
		) {
			current = current.expression;
			continue;
		}
		return current;
	}
}

function evaluateTypeScript(ts, sourceFile, expression) {
	const node = unwrapExpression(ts, expression);
	if (ts.isNumericLiteral(node)) {
		const number = normalizeInteger(node.getText(sourceFile));
		return number === undefined
			? { kind: "unknown", expression: node.getText(sourceFile) }
			: { kind: "number", number, expression: node.getText(sourceFile) };
	}
	if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) {
		return { kind: "string", text: node.text, expression: node.getText(sourceFile) };
	}
	if (node.kind === ts.SyntaxKind.RegularExpressionLiteral) {
		const parsed = regexpLiteral(node.getText(sourceFile));
		return {
			kind: "regexp",
			pattern: parsed.pattern,
			flags: parsed.flags,
			expression: node.getText(sourceFile),
		};
	}
	if (node.kind === ts.SyntaxKind.TrueKeyword || node.kind === ts.SyntaxKind.FalseKeyword) {
		return {
			kind: "boolean",
			text: node.kind === ts.SyntaxKind.TrueKeyword ? "true" : "false",
			expression: node.getText(sourceFile),
		};
	}
	if (ts.isPrefixUnaryExpression(node)) {
		const operand = evaluateTypeScript(ts, sourceFile, node.operand);
		if (operand.kind === "number") {
			const number = BigInt(operand.number);
			if (node.operator === ts.SyntaxKind.MinusToken) {
				return { kind: "number", number: (-number).toString(), expression: node.getText(sourceFile) };
			}
			if (node.operator === ts.SyntaxKind.PlusToken) {
				return { kind: "number", number: number.toString(), expression: node.getText(sourceFile) };
			}
		}
	}
	if (ts.isBinaryExpression(node)) {
		const left = evaluateTypeScript(ts, sourceFile, node.left);
		const right = evaluateTypeScript(ts, sourceFile, node.right);
		if (left.kind === "number" && right.kind === "number") {
			const leftNumber = BigInt(left.number);
			const rightNumber = BigInt(right.number);
			let number;
			switch (node.operatorToken.kind) {
				case ts.SyntaxKind.PlusToken:
					number = leftNumber + rightNumber;
					break;
				case ts.SyntaxKind.MinusToken:
					number = leftNumber - rightNumber;
					break;
				case ts.SyntaxKind.AsteriskToken:
					number = leftNumber * rightNumber;
					break;
				case ts.SyntaxKind.SlashToken:
					number = leftNumber / rightNumber;
					break;
				default:
					break;
			}
			if (number !== undefined) {
				return { kind: "number", number: number.toString(), expression: node.getText(sourceFile) };
			}
		}
	}
	if (ts.isArrayLiteralExpression(node)) {
		return {
			kind: "array",
			items: node.elements.map((element) => evaluateTypeScript(ts, sourceFile, element)),
			expression: node.getText(sourceFile),
		};
	}
	if (ts.isObjectLiteralExpression(node)) {
		const properties = {};
		for (const property of node.properties) {
			if (ts.isPropertyAssignment(property)) {
				properties[propertyName(ts, property.name, sourceFile)] = evaluateTypeScript(
					ts,
					sourceFile,
					property.initializer,
				);
			}
		}
		return { kind: "object", properties, expression: node.getText(sourceFile) };
	}
	if (ts.isCallExpression(node)) {
		const callee = node.expression.getText(sourceFile);
		if (callee === "buildProviderErrorPattern" && node.arguments.length === 1) {
			const patterns = evaluateTypeScript(ts, sourceFile, node.arguments[0]);
			return { ...patterns, expression: node.getText(sourceFile) };
		}
	}
	return { kind: "unknown", expression: node.getText(sourceFile) };
}

function addLiteral(accumulator, kind, value, file) {
	const key = `${kind}\x00${value}`;
	let entry = accumulator.get(key);
	if (!entry) {
		entry = { kind, value, count: 0, files: new Set() };
		accumulator.set(key, entry);
	}
	entry.count += 1;
	entry.files.add(file);
}

function collectTypeScriptInventory(ts, piRoot, scope) {
	const sourceRoot = path.join(piRoot, scope.piRoot);
	const files = walkFiles(sourceRoot, (file) => {
		if (!file.endsWith(".ts") || file.endsWith(".d.ts")) {
			return false;
		}
		return !(scope.piExclusions ?? []).some((suffix) => file.endsWith(suffix));
	});
	const declarations = [];
	const literalAccumulator = new Map();
	for (const file of files) {
		const relative = path.relative(piRoot, file).split(path.sep).join("/");
		const sourceFile = ts.createSourceFile(
			file,
			fs.readFileSync(file, "utf8"),
			ts.ScriptTarget.Latest,
			true,
			ts.ScriptKind.TS,
		);
		const visit = (node) => {
			if (
				ts.isVariableDeclaration(node) &&
				ts.isIdentifier(node.name) &&
				node.initializer !== undefined
			) {
				declarations.push({
					file: relative,
					name: node.name.text,
					value: evaluateTypeScript(ts, sourceFile, node.initializer),
				});
			}
			if (ts.isNumericLiteral(node)) {
				const number = normalizeInteger(node.getText(sourceFile));
				if (number !== undefined) {
					addLiteral(literalAccumulator, "number", number, relative);
				}
			} else if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) {
				addLiteral(literalAccumulator, "string", node.text, relative);
			} else if (node.kind === ts.SyntaxKind.RegularExpressionLiteral) {
				addLiteral(literalAccumulator, "regexp", regexpLiteral(node.getText(sourceFile)).pattern, relative);
			}
			ts.forEachChild(node, visit);
		};
		visit(sourceFile);
	}
	declarations.sort(
		(left, right) => left.file.localeCompare(right.file) || left.name.localeCompare(right.name),
	);
	const literals = [...literalAccumulator.values()]
		.map((entry) => ({
			kind: entry.kind,
			value: entry.value,
			count: entry.count,
			files: [...entry.files].sort(),
		}))
		.sort((left, right) => left.kind.localeCompare(right.kind) || left.value.localeCompare(right.value));
	return {
		parser: `typescript@${ts.version}`,
		files: files.map((file) => path.relative(piRoot, file).split(path.sep).join("/")),
		declarations,
		literals,
	};
}

function collectGoInventory(giRoot, paths) {
	const result = spawnSync(
		"go",
		[
			"run",
			"./docs/pi-parity/tools/go-constant-inventory",
			"--root",
			giRoot,
			"--paths",
			paths.join(","),
		],
		{
			cwd: giRoot,
			encoding: "utf8",
			env: {
				...process.env,
				GOCACHE: process.env.GOCACHE || path.join(os.tmpdir(), "gi-parity-gocache"),
			},
			maxBuffer: 128 * 1024 * 1024,
		},
	);
	if (result.error) {
		throw result.error;
	}
	if (result.status !== 0) {
		throw new Error(result.stderr.trim() || `Go constant inventory exited ${result.status}`);
	}
	try {
		return JSON.parse(result.stdout);
	} catch (error) {
		throw new Error(`Go constant inventory returned invalid JSON: ${error.message}`);
	}
}

function findDeclaration(inventory, reference, language) {
	const matches = inventory.declarations.filter(
		(declaration) => declaration.file === reference.file && declaration.name === reference.symbol,
	);
	if (matches.length !== 1) {
		throw new Error(
			`${language} declaration ${reference.file}#${reference.symbol} matched ${matches.length} entries`,
		);
	}
	let value = matches[0].value;
	for (const segment of reference.path ?? []) {
		if (value.kind !== "object" || value.properties?.[segment] === undefined) {
			throw new Error(
				`${language} declaration ${reference.file}#${reference.symbol} has no property ${segment}`,
			);
		}
		value = value.properties[segment];
	}
	return value;
}

function canonicalNumber(value, reference, language) {
	if (value.kind !== "number") {
		throw new Error(`${language} ${reference.file}#${reference.symbol} is ${value.kind}, expected number`);
	}
	const unit = reference.unit ?? "scalar";
	const scale = UNIT_SCALE[unit];
	if (scale === undefined) {
		throw new Error(`unsupported numeric unit ${unit}`);
	}
	return (BigInt(value.number) * scale).toString();
}

function stripCaseInsensitivePrefix(pattern) {
	return pattern.startsWith("(?i)") ? pattern.slice(4) : pattern;
}

function hasSingleOuterGroup(pattern) {
	if (!pattern.startsWith("(") || !pattern.endsWith(")")) {
		return false;
	}
	let depth = 0;
	let escaped = false;
	let inCharacterClass = false;
	for (let index = 0; index < pattern.length; index += 1) {
		const character = pattern[index];
		if (escaped) {
			escaped = false;
			continue;
		}
		if (character === "\\") {
			escaped = true;
			continue;
		}
		if (character === "[") {
			inCharacterClass = true;
			continue;
		}
		if (character === "]") {
			inCharacterClass = false;
			continue;
		}
		if (inCharacterClass) {
			continue;
		}
		if (character === "(") {
			depth += 1;
		} else if (character === ")") {
			depth -= 1;
			if (depth === 0 && index !== pattern.length - 1) {
				return false;
			}
		}
	}
	return depth === 0;
}

export function splitTopLevelAlternatives(input) {
	let pattern = stripCaseInsensitivePrefix(input);
	if (hasSingleOuterGroup(pattern)) {
		pattern = pattern.slice(1, -1);
	}
	const alternatives = [];
	let start = 0;
	let depth = 0;
	let escaped = false;
	let inCharacterClass = false;
	for (let index = 0; index < pattern.length; index += 1) {
		const character = pattern[index];
		if (escaped) {
			escaped = false;
			continue;
		}
		if (character === "\\") {
			escaped = true;
			continue;
		}
		if (character === "[") {
			inCharacterClass = true;
			continue;
		}
		if (character === "]") {
			inCharacterClass = false;
			continue;
		}
		if (inCharacterClass) {
			continue;
		}
		if (character === "(") {
			depth += 1;
		} else if (character === ")") {
			depth -= 1;
		} else if (character === "|" && depth === 0) {
			alternatives.push(pattern.slice(start, index));
			start = index + 1;
		}
	}
	alternatives.push(pattern.slice(start));
	return alternatives;
}

function patternList(value, reference, language) {
	if (language === "Gi" && reference.transform === "case-insensitive-alternatives") {
		if (value.kind !== "regexp") {
			throw new Error(`${language} ${reference.file}#${reference.symbol} is ${value.kind}, expected regexp`);
		}
		return splitTopLevelAlternatives(value.pattern);
	}
	if (value.kind !== "array") {
		throw new Error(`${language} ${reference.file}#${reference.symbol} is ${value.kind}, expected array`);
	}
	return value.items.map((item, index) => {
		if (item.kind === "string") {
			return item.text;
		}
		if (item.kind === "regexp") {
			return language === "Gi" ? stripCaseInsensitivePrefix(item.pattern) : item.pattern;
		}
		throw new Error(
			`${language} ${reference.file}#${reference.symbol}[${index}] is ${item.kind}, expected string or regexp`,
		);
	});
}

function compareSets(left, right) {
	const leftSet = new Set(left);
	const rightSet = new Set(right);
	return {
		missingInGi: [...leftSet].filter((entry) => !rightSet.has(entry)).sort(),
		extraInGi: [...rightSet].filter((entry) => !leftSet.has(entry)).sort(),
		duplicatesInPi: left.filter((entry, index) => left.indexOf(entry) !== index),
		duplicatesInGi: right.filter((entry, index) => right.indexOf(entry) !== index),
	};
}

function literalCandidateSummary(piInventory, giInventory) {
	const piKeys = new Set(piInventory.literals.map((entry) => `${entry.kind}\x00${entry.value}`));
	const giKeys = new Set(giInventory.literals.map((entry) => `${entry.kind}\x00${entry.value}`));
	const byKind = {};
	for (const kind of ["number", "string", "regexp"]) {
		const pi = [...piKeys].filter((key) => key.startsWith(`${kind}\x00`));
		const gi = [...giKeys].filter((key) => key.startsWith(`${kind}\x00`));
		byKind[kind] = {
			pi: pi.length,
			gi: gi.length,
			shared: pi.filter((key) => giKeys.has(key)).length,
			piOnly: pi.filter((key) => !giKeys.has(key)).length,
			giOnly: gi.filter((key) => !piKeys.has(key)).length,
		};
	}
	return byKind;
}

function verifyContracts(contracts, piInventory, giInventory) {
	const numeric = contracts.numeric.map((contract) => {
		const piValue = findDeclaration(piInventory, contract.pi, "Pi");
		const giValue = findDeclaration(giInventory, contract.gi, "Gi");
		const piCanonical = canonicalNumber(piValue, contract.pi, "Pi");
		const giCanonical = canonicalNumber(giValue, contract.gi, "Gi");
		return {
			id: contract.id,
			ok: piCanonical === giCanonical,
			pi: { value: piValue.number, unit: contract.pi.unit ?? "scalar", canonical: piCanonical },
			gi: { value: giValue.number, unit: contract.gi.unit ?? "scalar", canonical: giCanonical },
		};
	});
	const patterns = contracts.patterns.map((contract) => {
		const piValue = findDeclaration(piInventory, contract.pi, "Pi");
		const giValue = findDeclaration(giInventory, contract.gi, "Gi");
		const piPatterns = patternList(piValue, contract.pi, "Pi");
		const giPatterns = patternList(giValue, contract.gi, "Gi");
		const comparison = compareSets(piPatterns, giPatterns);
		return {
			id: contract.id,
			ok:
				comparison.missingInGi.length === 0 &&
				comparison.extraInGi.length === 0 &&
				comparison.duplicatesInPi.length === 0 &&
				comparison.duplicatesInGi.length === 0,
			piCount: piPatterns.length,
			giCount: giPatterns.length,
			...comparison,
		};
	});
	return { numeric, patterns };
}

function renderText(result) {
	const checks = [...result.checks.numeric, ...result.checks.patterns];
	const lines = [
		`Pi constant parity ${result.ok ? "passed" : "failed"} for ${result.baseline.tag}@${result.baseline.commit}`,
		`Explicit contracts: ${checks.filter((check) => check.ok).length}/${checks.length} passed`,
		`Parsers: ${result.inventory.pi.parser}; ${result.inventory.gi.parser}`,
		`Source files: Pi ${result.inventory.pi.fileCount}, Gi ${result.inventory.gi.fileCount}`,
		"Literal candidate inventory (non-gating):",
	];
	for (const [kind, summary] of Object.entries(result.literalCandidates)) {
		lines.push(
			`  ${kind}: Pi ${summary.pi}, Gi ${summary.gi}, shared ${summary.shared}, Pi-only ${summary.piOnly}, Gi-only ${summary.giOnly}`,
		);
	}
	for (const check of checks.filter((entry) => !entry.ok)) {
		lines.push(`FAIL ${check.id}`);
		if ("pi" in check) {
			lines.push(`  Pi ${check.pi.value} ${check.pi.unit}; Gi ${check.gi.value} ${check.gi.unit}`);
		} else {
			for (const missing of check.missingInGi) {
				lines.push(`  missing in Gi: ${missing}`);
			}
			for (const extra of check.extraInGi) {
				lines.push(`  extra in Gi: ${extra}`);
			}
		}
	}
	return `${lines.join("\n")}\n`;
}

function writeOutput(args, text) {
	if (args.out) {
		fs.mkdirSync(path.dirname(args.out), { recursive: true });
		fs.writeFileSync(args.out, text);
		return;
	}
	process.stdout.write(text);
}

async function main() {
	const args = parseArgs(process.argv.slice(2));
	const baseline = readJson(args.baseline);
	const contracts = readJson(args.contracts);
	if (contracts.schemaVersion !== 1) {
		throw new Error(`unsupported constant-contract schemaVersion ${contracts.schemaVersion}`);
	}
	const checkout = verifyPiCheckout(baseline, args.piRoot, { allowDirty: args.allowDirty });
	if (!checkout.ok) {
		throw new Error(`invalid Pi checkout:\n${checkout.errors.join("\n")}`);
	}
	const typeScriptFile = resolveTypeScript(args);
	const ts = await loadTypeScript(typeScriptFile);
	const piInventory = collectTypeScriptInventory(ts, args.piRoot, contracts.scope);
	const giInventory = collectGoInventory(args.giRoot, contracts.scope.giPaths);
	const checks = verifyContracts(contracts, piInventory, giInventory);
	const allChecks = [...checks.numeric, ...checks.patterns];
	const result = {
		schemaVersion: 1,
		ok: allChecks.every((check) => check.ok),
		baseline: {
			tag: baseline.upstream.tag,
			commit: baseline.upstream.commit,
		},
		checks,
		inventory: {
			pi: {
				parser: piInventory.parser,
				fileCount: piInventory.files.length,
				literalCount: piInventory.literals.length,
				literals: args.format === "json" ? piInventory.literals : undefined,
			},
			gi: {
				parser: giInventory.parser,
				fileCount: giInventory.files.length,
				literalCount: giInventory.literals.length,
				literals: args.format === "json" ? giInventory.literals : undefined,
			},
		},
		literalCandidates: literalCandidateSummary(piInventory, giInventory),
	};
	const text = args.format === "json" ? `${JSON.stringify(result, null, 2)}\n` : renderText(result);
	writeOutput(args, text);
	if (!result.ok) {
		process.exitCode = 1;
	}
}

const invokedAsScript =
	process.argv[1] !== undefined && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href;
if (invokedAsScript) {
	main().catch((error) => {
		console.error(error.message);
		process.exitCode = 1;
	});
}
