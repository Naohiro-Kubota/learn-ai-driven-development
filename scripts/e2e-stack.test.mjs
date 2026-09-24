import assert from "node:assert/strict";
import { EventEmitter } from "node:events";
import test from "node:test";
import { runStack } from "./e2e-stack.mjs";

function fakeDeps({
	occupiedPort,
	playwrightExit = 0,
	secret = "private-sentinel",
} = {}) {
	const calls = [];
	const killed = [];
	const log = [];
	return {
		calls,
		killed,
		log,
		deps: {
			checkPort: async (port) => port !== occupiedPort,
			fetch: async (url) => ({
				status: url.includes("/api/v1/session") ? 401 : 200,
			}),
			seed: async () => {},
			spawn(command, args, options) {
				const child = new EventEmitter();
				child.kill = (signal) => {
					killed.push([command, signal]);
					queueMicrotask(() => child.emit("exit", 0, signal));
				};
				calls.push({ command, args, options });
				if (command !== "go" || args[0] !== "run" || args[1] !== "./cmd/api") {
					if (command !== "pnpm" || args[1] !== "vite") {
						queueMicrotask(() =>
							child.emit(
								"exit",
								args.includes("playwright") ? playwrightExit : 0,
							),
						);
					}
				}
				return child;
			},
			logger: (message) => log.push(message),
			randomBytes: (size) => Buffer.alloc(size, 0xab),
			env: { E2E_TEST_SECRET: secret },
		},
	};
}

test("occupied port leaves existing services untouched", async () => {
	const { deps, calls } = fakeDeps({ occupiedPort: 8081 });
	await assert.rejects(runStack(deps), /8081/);
	assert.deepEqual(calls, []);
});

test("failed browser run stops children and tears down only its own project", async () => {
	const { deps, calls, killed } = fakeDeps({ playwrightExit: 1 });
	await assert.rejects(runStack(deps), /Playwright/);
	assert.deepEqual(killed, [
		["pnpm", "SIGTERM"],
		["go", "SIGTERM"],
	]);
	const down = calls.at(-1);
	assert.equal(down.command, "docker");
	assert.match(down.args[2], /^approval-flow-e2e-[a-f0-9]+$/);
	assert.deepEqual(down.args.slice(3), [
		"-f",
		"compose.e2e.yaml",
		"down",
		"--volumes",
		"--remove-orphans",
	]);
});

test("runner does not print secrets or pass them as command arguments", async () => {
	const { deps, calls, log } = fakeDeps();
	await runStack(deps);
	assert.equal(log.join(" ").includes("private-sentinel"), false);
	assert.equal(
		JSON.stringify(calls.map((call) => call.args)).includes("private-sentinel"),
		false,
	);
	assert.equal(
		calls.some((call) => call.options.stdio === "inherit"),
		false,
	);
});
