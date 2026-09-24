import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { EventEmitter } from "node:events";
import { existsSync, mkdirSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { PassThrough } from "node:stream";
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

test("uses a writable per-run Go cache and removes it after cleanup", async () => {
	const { deps, calls } = fakeDeps();
	await runStack(deps);
	const cache = calls.find(({ command }) => command === "go").options.env
		.GOCACHE;
	assert.match(cache, /approval-flow-e2e-go-cache-/);
	assert.equal(existsSync(cache), false);
});

test("removes only its per-run Playwright artifacts on success and failure", async () => {
	const outputDirs = [];
	for (const playwrightExit of [0, 1]) {
		const { deps } = fakeDeps({ playwrightExit });
		const originalSpawn = deps.spawn;
		deps.spawn = (command, args, options) => {
			if (args.includes("playwright")) {
				const outputDir = options.env.E2E_PLAYWRIGHT_OUTPUT_DIR;
				outputDirs.push(outputDir);
				if (outputDir) {
					assert.equal(existsSync(outputDir), true);
					mkdirSync(join(outputDir, "case"));
					writeFileSync(
						join(outputDir, "case", "error-context.md"),
						"private-sentinel",
					);
				}
			}
			return originalSpawn(command, args, options);
		};
		if (playwrightExit) await assert.rejects(runStack(deps), /Playwright/);
		else await runStack(deps);
	}
	assert.equal(new Set(outputDirs).size, 2);
	for (const outputDir of outputDirs) {
		assert.match(outputDir, /^\/.*approval-flow-e2e-playwright-/);
		assert.equal(outputDir.startsWith(tmpdir()), true);
		assert.equal(existsSync(outputDir), false);
	}
});

test("passes the API all required session and transaction durations", async () => {
	const { deps, calls } = fakeDeps();
	await runStack(deps);
	const api = calls.find(
		({ command, args }) => command === "go" && args[1] === "./cmd/api",
	);
	assert.equal(api.options.env.SESSION_IDLE_TTL, "15m");
	assert.equal(api.options.env.SESSION_ABSOLUTE_TTL, "8h");
	assert.equal(api.options.env.AUTH_TRANSACTION_TTL, "5m");
});

test("identifies the API and exit code without printing its stderr", async () => {
	const { deps } = fakeDeps({ secret: "private-sentinel" });
	const originalSpawn = deps.spawn;
	deps.spawn = (command, args, options) => {
		const child = originalSpawn(command, args, options);
		if (command === "go" && args[1] === "./cmd/api") {
			queueMicrotask(() => child.emit("exit", 1));
		}
		return child;
	};
	await assert.rejects(runStack(deps), (error) => {
		assert.match(error.message, /API exited before readiness \(code 1\)/);
		assert.doesNotMatch(error.message, /private-sentinel/);
		return true;
	});
});

test("stops a detached service group after its wrapper exits, escalating if descendants survive", async () => {
	const { deps, calls } = fakeDeps();
	const groups = new Set([4101]);
	const signals = [];
	const originalSpawn = deps.spawn;
	deps.spawn = (command, args, options) => {
		const child = originalSpawn(command, args, options);
		if (command === "go" && args[1] === "./cmd/api") {
			child.pid = 4101;
			queueMicrotask(() => child.emit("exit", 0));
		}
		return child;
	};
	deps.signalProcess = (pid, signal) => {
		signals.push([pid, signal]);
		if (signal === "SIGKILL") groups.delete(-pid);
	};
	deps.processGroupExists = (pid) => groups.has(pid);
	deps.stopGraceMs = 0;
	await assert.rejects(runStack(deps), /exited before readiness/);
	assert.deepEqual(signals, [
		[-4101, "SIGTERM"],
		[-4101, "SIGKILL"],
	]);
	assert.equal(calls.at(-1).args.includes("down"), true);
});

test("removes a real descendant after its detached wrapper exits", {
	skip: process.platform === "win32",
}, async () => {
	const { deps } = fakeDeps();
	const originalSpawn = deps.spawn;
	let groupPid;
	deps.spawn = (command, args, options) => {
		if (command === "go" && args[1] === "./cmd/api") {
			const script = [
				'const { spawn } = require("node:child_process");',
				'spawn(process.execPath, ["-e", "setInterval(() => {}, 1000)"], { stdio: "ignore" });',
				"setTimeout(() => process.exit(0), 20);",
			].join(" ");
			const wrapper = spawn(process.execPath, ["-e", script], {
				stdio: "ignore",
				detached: true,
			});
			groupPid = wrapper.pid;
			return wrapper;
		}
		return originalSpawn(command, args, options);
	};
	deps.fetch = async (url) => ({
		status: url.includes("/api/v1/session") ? 503 : 200,
	});
	await assert.rejects(runStack(deps), /exited before readiness/);
	assert.ok(groupPid > 0);
	await new Promise((resolve) => setTimeout(resolve, 100));
	assert.throws(() => process.kill(-groupPid, 0), { code: "ESRCH" });
});

test("stops detached Playwright descendants after its wrapper exits", async () => {
	const { deps } = fakeDeps();
	const groups = new Set([4202]);
	const signals = [];
	const originalSpawn = deps.spawn;
	deps.spawn = (command, args, options) => {
		const child = originalSpawn(command, args, options);
		if (args.includes("playwright")) child.pid = 4202;
		return child;
	};
	deps.signalProcess = (pid, signal) => {
		signals.push([pid, signal]);
		groups.delete(-pid);
	};
	deps.processGroupExists = (pid) => groups.has(pid);
	await runStack(deps);
	assert.deepEqual(signals, [[-4202, "SIGTERM"]]);
});

test("keeps signal handlers installed until compose down completes", async () => {
	const { deps, calls } = fakeDeps({ playwrightExit: 1 });
	const priorTerm = process.listenerCount("SIGTERM");
	const priorInt = process.listenerCount("SIGINT");
	const originalSpawn = deps.spawn;
	deps.spawn = (command, args, options) => {
		if (args.includes("down")) {
			assert.equal(process.listenerCount("SIGTERM"), priorTerm + 1);
			assert.equal(process.listenerCount("SIGINT"), priorInt + 1);
			process.emit("SIGTERM");
			process.emit("SIGINT");
		}
		return originalSpawn(command, args, options);
	};
	await assert.rejects(runStack(deps), /Playwright/);
	assert.equal(calls.at(-1).args.includes("down"), true);
	assert.equal(process.listenerCount("SIGTERM"), priorTerm);
	assert.equal(process.listenerCount("SIGINT"), priorInt);
});

test("preserves Playwright's nonzero exit code", async () => {
	const { deps } = fakeDeps({ playwrightExit: 2 });
	await assert.rejects(runStack(deps), (error) => {
		assert.match(error.message, /Playwright/);
		assert.equal(error.exitCode, 2);
		return true;
	});
});

test("prints only validated Playwright diagnostic fields from mixed raw output", async () => {
	const { deps, log } = fakeDeps();
	const originalSpawn = deps.spawn;
	deps.spawn = (command, args, options) => {
		if (!args.includes("playwright"))
			return originalSpawn(command, args, options);
		const child = new EventEmitter();
		child.stdout = new PassThrough();
		child.stderr = new PassThrough();
		child.kill = () => {};
		queueMicrotask(() => {
			child.stdout.write("raw token=private-sentinel\nE2E_DIAG");
			child.stdout.write(
				'NOSTIC:{"phase":"callback","status":400,"code":"invalid_auth_transaction","cookiePresent":true,"pathname":"/","testId":"approval_flow","file":"e2e/approval-flow.spec.ts","line":85}\n',
			);
			child.stderr.write(
				'E2E_DIAGNOSTIC:{"phase":"callback","status":400,"code":"private-sentinel","cookiePresent":true,"pathname":"/"}\n',
			);
			child.stderr.write(
				'E2E_DIAGNOSTIC:{"phase":"callback","status":400,"code":"unknown","cookiePresent":true,"pathname":"/","rawCookie":"private-sentinel"}\n',
			);
			child.stdout.end();
			child.stderr.end();
			child.emit("exit", 1);
		});
		return child;
	};
	await assert.rejects(runStack(deps), /Playwright/);
	assert.deepEqual(
		log.filter((line) => line.startsWith("E2E diagnostic:")),
		[
			"E2E diagnostic: test=approval_flow file=e2e/approval-flow.spec.ts:85 phase=callback status=400 code=invalid_auth_transaction cookiePresent=true pathname=/",
		],
	);
	assert.equal(log.join(" ").includes("private-sentinel"), false);
});
