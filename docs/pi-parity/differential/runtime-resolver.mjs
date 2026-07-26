import { createRequire, isBuiltin, registerHooks } from "node:module";
import path from "node:path";
import process from "node:process";
import { pathToFileURL } from "node:url";

const runtimeRoot = process.env.PI_RUNTIME_REPO;
if (!runtimeRoot) {
	throw new Error("PI_RUNTIME_REPO is required by the Pi parity runtime resolver");
}

const runtimeRequire = createRequire(path.join(path.resolve(runtimeRoot), "package.json"));
const isBareSpecifier = (specifier) =>
	!specifier.startsWith(".") &&
	!specifier.startsWith("/") &&
	!specifier.startsWith("file:") &&
	!specifier.startsWith("data:") &&
	!specifier.startsWith("node:") &&
	!isBuiltin(specifier);

registerHooks({
	resolve(specifier, context, nextResolve) {
		try {
			return nextResolve(specifier, context);
		} catch (error) {
			if (!isBareSpecifier(specifier)) {
				throw error;
			}
			const resolved = runtimeRequire.resolve(specifier);
			return {
				url: pathToFileURL(resolved).href,
				shortCircuit: true,
			};
		}
	},
});
