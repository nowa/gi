#!/usr/bin/env node

import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import process from "node:process";

const modules = [
	{
		name: "llm",
		piSource: "packages/ai/src",
		giProduction: ["gi-llm-provider"],
		map: "docs/pi-parity/llm-provider-file-map.md",
	},
	{
		name: "agent",
		piSource: "packages/agent/src",
		giProduction: ["gi-agent-core"],
		map: "docs/pi-parity/agent-core-file-map.md",
	},
	{
		name: "tui",
		piSource: "packages/tui/src",
		giProduction: ["gi-tui"],
		map: "docs/pi-parity/tui-file-map.md",
	},
	{
		name: "coding",
		piSource: "packages/coding-agent/src",
		giProduction: ["gi-coding-agent", "cmd/gi"],
		map: "docs/pi-parity/coding-agent-file-map.md",
	},
];

function usage() {
	console.log(`Usage: node docs/pi-parity/verify-source-map.mjs [--pi-root <path>] [--gi-root <path>] [--scope exported|top-level|members] [--format text|markdown|json] [--include-covered] [--out <path>]

Verifies that every Pi source file and top-level exported symbol is mentioned in
the corresponding Gi parity map. With --scope top-level, checks non-exported
top-level implementation functions/classes too. With --scope members, also
checks class methods, getters/setters, and arrow-property methods as
Class.member symbols. With --include-covered and --format markdown, also emits
a per-file inventory of all extracted Pi symbols and their missing status. This
checks audit coverage, not behavior.`);
}

function parseArgs(argv) {
	const args = {
		giRoot: process.cwd(),
		piRoot: process.env.PI_REPO || path.join(os.homedir(), "Projects/agents/pi"),
		scope: "exported",
		format: "text",
		includeCovered: false,
		out: "",
	};
	for (let i = 0; i < argv.length; i += 1) {
		const arg = argv[i];
		if (arg === "--help" || arg === "-h") {
			usage();
			process.exit(0);
		}
		if (arg === "--pi-root") {
			args.piRoot = argv[++i];
			continue;
		}
		if (arg === "--gi-root") {
			args.giRoot = argv[++i];
			continue;
		}
		if (arg === "--scope") {
			args.scope = argv[++i];
			continue;
		}
		if (arg === "--format") {
			args.format = argv[++i];
			continue;
		}
		if (arg === "--include-covered") {
			args.includeCovered = true;
			continue;
		}
		if (arg === "--out") {
			args.out = argv[++i];
			continue;
		}
		throw new Error(`unknown argument: ${arg}`);
	}
	if (!["exported", "top-level", "members"].includes(args.scope)) {
		throw new Error(`unsupported --scope value: ${args.scope}`);
	}
	if (!["text", "markdown", "json"].includes(args.format)) {
		throw new Error(`unsupported --format value: ${args.format}`);
	}
	return {
		giRoot: expandHome(args.giRoot),
		piRoot: expandHome(args.piRoot),
		scope: args.scope,
		format: args.format,
		includeCovered: args.includeCovered,
		out: args.out ? expandHome(args.out) : "",
	};
}

function expandHome(input) {
	if (input === "~") {
		return os.homedir();
	}
	if (input.startsWith("~/")) {
		return path.join(os.homedir(), input.slice(2));
	}
	return path.resolve(input);
}

function walk(dir, predicate) {
	const out = [];
	for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
		if (entry.name.startsWith(".")) {
			continue;
		}
		const fullPath = path.join(dir, entry.name);
		if (entry.isDirectory()) {
			out.push(...walk(fullPath, predicate));
			continue;
		}
		if (!predicate || predicate(fullPath)) {
			out.push(fullPath);
		}
	}
	return out.sort();
}

function isTypeScriptSource(file) {
	return /\.tsx?$/.test(file);
}

function isProductionGo(file) {
	return file.endsWith(".go") && !file.endsWith("_test.go");
}

function extractExportedSymbols(source) {
	const symbols = [];
	const lines = source.split(/\n/);
	for (let i = 0; i < lines.length; i += 1) {
		const line = lines[i];
		let match = line.match(
			/^\s*export\s+(?:async\s+)?(?:function|class|interface|type|const|let|var|enum)\s+([A-Za-z_$][\w$]*)/,
		);
		if (match) {
			symbols.push(match[1]);
		}
		match = line.match(/^\s*export\s+default\s+(?:async\s+)?(?:function|class)\s+([A-Za-z_$][\w$]*)/);
		if (match) {
			symbols.push(match[1]);
		}
		if (/^\s*export\s+(?:type\s+)?\{/.test(line)) {
			let block = line;
			while (!/}\s*(?:from\s+['"][^'"]+['"])?\s*;?\s*$/.test(block) && i + 1 < lines.length) {
				i += 1;
				block += `\n${lines[i]}`;
			}
			symbols.push(...extractExportBlockSymbols(block));
		}
	}
	return [...new Set(symbols)];
}

function extractTopLevelImplementationSymbols(source) {
	const symbols = [];
	const lines = source.split(/\n/);
	let depth = 0;
	for (const line of lines) {
		const currentDepth = depth;
		let match;
		if (currentDepth === 0) {
			match = line.match(/^\s*(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_$][\w$]*)/);
			if (match) {
				symbols.push(match[1]);
			}
			match = line.match(/^\s*(?:export\s+)?class\s+([A-Za-z_$][\w$]*)/);
			if (match) {
				symbols.push(match[1]);
			}
			match = line.match(
				/^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=\s*(?:async\s*)?(?:\([^)]*\)|[A-Za-z_$][\w$]*)\s*=>/,
			);
			if (match) {
				symbols.push(match[1]);
			}
		}
		for (const char of stripQuotedText(line)) {
			if (char === "{") {
				depth += 1;
			} else if (char === "}") {
				depth = Math.max(0, depth - 1);
			}
		}
	}
	return [...new Set(symbols)];
}

function extractMemberImplementationSymbols(source) {
	const symbols = extractTopLevelImplementationSymbols(source);
	const lines = source.split(/\n/);
	let depth = 0;
	const classStack = [];
	for (const line of lines) {
		const currentDepth = depth;
		const classMatch = line.match(/^\s*(?:export\s+)?(?:abstract\s+)?class\s+([A-Za-z_$][\w$]*)\b/);
		if (classMatch) {
			classStack.push({ name: classMatch[1], depth: currentDepth });
		}
		const currentClass = classStack[classStack.length - 1];
		if (currentClass && currentDepth === currentClass.depth + 1) {
			const member = matchClassMember(line);
			if (member) {
				symbols.push(`${currentClass.name}.${member}`);
			}
		}
		for (const char of stripQuotedText(line)) {
			if (char === "{") {
				depth += 1;
			} else if (char === "}") {
				depth = Math.max(0, depth - 1);
			}
		}
		while (classStack.length > 0 && depth <= classStack[classStack.length - 1].depth) {
			classStack.pop();
		}
	}
	return [...new Set(symbols)];
}

function matchClassMember(line) {
	const withoutComment = line.replace(/\/\/.*$/, "").trim();
	if (!withoutComment || withoutComment.startsWith("*") || withoutComment.startsWith("@")) {
		return "";
	}
	if (/^constructor\s*\(/.test(withoutComment)) {
		return "constructor";
	}
	let match = withoutComment.match(/^(?:(?:public|private|protected|static|readonly|abstract|override|async)\s+)*(get|set)\s+([A-Za-z_$][\w$]*)\s*(?:\([^)]*\))?\s*[:{]/);
	if (match) {
		return match[2];
	}
	match = withoutComment.match(/^(?:(?:public|private|protected|static|readonly|abstract|override|async)\s+)*([A-Za-z_$][\w$]*)\s*(?:<[^>]+>)?\s*\(/);
	if (match && !classMemberKeywordDenylist.has(match[1])) {
		return match[1];
	}
	match = withoutComment.match(
		/^(?:(?:public|private|protected|static|readonly|abstract|override|async)\s+)*([A-Za-z_$][\w$]*)\s*(?:\?)?\s*(?::[^=]+)?=\s*(?:async\s*)?(?:\([^)]*\)|[A-Za-z_$][\w$]*)\s*=>/,
	);
	if (match && !classMemberKeywordDenylist.has(match[1])) {
		return match[1];
	}
	return "";
}

const classMemberKeywordDenylist = new Set(["if", "for", "while", "switch", "catch", "function", "return"]);

function stripQuotedText(line) {
	let out = "";
	let quote = "";
	let escaped = false;
	for (const char of line) {
		if (escaped) {
			escaped = false;
			continue;
		}
		if (quote) {
			if (char === "\\") {
				escaped = true;
				continue;
			}
			if (char === quote) {
				quote = "";
			}
			continue;
		}
		if (char === "'" || char === "\"" || char === "`") {
			quote = char;
			continue;
		}
		out += char;
	}
	return out;
}

function extractExportBlockSymbols(block) {
	const open = block.indexOf("{");
	const close = block.lastIndexOf("}");
	if (open === -1 || close === -1 || close <= open) {
		return [];
	}
	const body = block.slice(open + 1, close);
	const symbols = [];
	for (const rawPart of body.split(",")) {
		const withoutComment = rawPart.replace(/\/\/.*$/gm, "").trim();
		if (!withoutComment) {
			continue;
		}
		const part = withoutComment.replace(/^type\s+/, "").trim();
		const aliasParts = part.split(/\s+as\s+/);
		const name = aliasParts[aliasParts.length - 1]?.trim();
		if (/^[A-Za-z_$][\w$]*$/.test(name)) {
			symbols.push(name);
		}
	}
	return symbols;
}

function rel(from, file) {
	return path.relative(from, file).split(path.sep).join("/");
}

function verifyModule({ giRoot, piRoot, scope }, module) {
	const piSourceRoot = path.join(piRoot, module.piSource);
	const giProductionFiles = module.giProduction.flatMap((dir) => walk(path.join(giRoot, dir), isProductionGo));
	const piFiles = walk(piSourceRoot, isTypeScriptSource);
	const mapPath = path.join(giRoot, module.map);
	const mapText = fs.readFileSync(mapPath, "utf8");
	const missingFiles = [];
	const missingSymbols = [];
	const files = [];
	let symbolCount = 0;

	for (const piFile of piFiles) {
		const relativeFile = rel(piSourceRoot, piFile);
		if (!mapText.includes(relativeFile)) {
			missingFiles.push(relativeFile);
		}
		const source = fs.readFileSync(piFile, "utf8");
		const symbols =
			scope === "members"
				? extractMemberImplementationSymbols(source)
				: scope === "top-level"
					? extractTopLevelImplementationSymbols(source)
					: extractExportedSymbols(source);
		symbolCount += symbols.length;
		const fileMissingSymbols = [];
		for (const symbol of symbols) {
			if (!mapText.includes(symbol)) {
				missingSymbols.push(`${relativeFile}::${symbol}`);
				fileMissingSymbols.push(symbol);
			}
		}
		files.push({ file: relativeFile, symbols, missingSymbols: fileMissingSymbols });
	}

	return {
		name: module.name,
		piFiles: piFiles.length,
		giProductionFiles: giProductionFiles.length,
		symbols: symbolCount,
		missingFiles,
		missingSymbols,
		files,
	};
}

function main() {
	const args = parseArgs(process.argv.slice(2));
	if (!fs.existsSync(args.piRoot)) {
		throw new Error(`Pi root not found: ${args.piRoot}`);
	}
	if (!fs.existsSync(args.giRoot)) {
		throw new Error(`Gi root not found: ${args.giRoot}`);
	}
	const results = modules.map((module) => verifyModule(args, module));
	const failed = results.some((result) => result.missingFiles.length > 0 || result.missingSymbols.length > 0);
	const output = formatResults(args, results);
	if (args.out) {
		fs.writeFileSync(args.out, output);
	} else {
		process.stdout.write(output);
	}
	process.exit(failed ? 1 : 0);
}

function formatResults(args, results) {
	switch (args.format) {
		case "json":
			return `${JSON.stringify({ piRoot: args.piRoot, giRoot: args.giRoot, scope: args.scope, includeCovered: args.includeCovered, results }, null, 2)}\n`;
		case "markdown":
			return formatMarkdown(args, results);
		default:
			return formatText(results);
	}
}

function formatText(results) {
	const lines = [];
	for (const result of results) {
		lines.push(
			`${result.name}: piFiles=${result.piFiles} giProductionFiles=${result.giProductionFiles} ` +
				`symbols=${result.symbols} missingFiles=${result.missingFiles.length} ` +
				`missingSymbols=${result.missingSymbols.length}`,
		);
		if (result.missingFiles.length > 0) {
			lines.push(`  missing files:\n    ${result.missingFiles.join("\n    ")}`);
		}
		if (result.missingSymbols.length > 0) {
			lines.push(`  missing symbols:\n    ${result.missingSymbols.join("\n    ")}`);
		}
	}
	return `${lines.join("\n")}\n`;
}

function formatMarkdown(args, results) {
	const lines = [
		"# Pi Source Map Inventory",
		"",
		"Generated by `docs/pi-parity/verify-source-map.mjs`.",
		"",
		`- Pi root: \`${args.piRoot}\``,
		`- Gi root: \`${args.giRoot}\``,
		`- Scope: \`${args.scope}\``,
		"",
		"| Area | Pi source files | Gi production files | Pi symbols | Missing Pi files | Missing Pi symbols |",
		"| --- | ---: | ---: | ---: | ---: | ---: |",
	];
	for (const result of results) {
		lines.push(
			`| ${result.name} | ${result.piFiles} | ${result.giProductionFiles} | ${result.symbols} | ${result.missingFiles.length} | ${result.missingSymbols.length} |`,
		);
	}
	lines.push("");
	for (const result of results) {
		lines.push(`## ${result.name}`, "");
		if (result.missingFiles.length === 0 && result.missingSymbols.length === 0) {
			lines.push("No missing files or symbols.", "");
		}
		if (result.missingFiles.length > 0) {
			lines.push("### Missing Files", "", "| Pi source file |", "| --- |");
			for (const file of result.missingFiles) {
				lines.push(`| \`${file}\` |`);
			}
			lines.push("");
		}
		if (result.missingSymbols.length > 0) {
			lines.push("### Missing Symbols", "", "| Pi symbol |", "| --- |");
			for (const symbol of result.missingSymbols) {
				lines.push(`| \`${symbol}\` |`);
			}
			lines.push("");
		}
		if (args.includeCovered) {
			lines.push(
				"### Per-File Symbol Inventory",
				"",
				"| Pi source file | Extracted symbols | Missing from map |",
				"| --- | --- | --- |",
			);
			for (const file of result.files) {
				const symbols = file.symbols.length > 0 ? file.symbols.map((symbol) => `\`${symbol}\``).join("<br>") : "_none_";
				const missing =
					file.missingSymbols.length > 0
						? file.missingSymbols.map((symbol) => `\`${symbol}\``).join("<br>")
						: "";
				lines.push(`| \`${file.file}\` | ${symbols} | ${missing} |`);
			}
			lines.push("");
		}
	}
	return `${lines.join("\n")}\n`;
}

main();
