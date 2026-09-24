import { spawn } from "node:child_process";
import { randomBytes } from "node:crypto";
import { mkdtemp, rm } from "node:fs/promises";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

const frontendOrigin = "http://127.0.0.1:5173";
const apiOrigin = "http://127.0.0.1:8080";
const issuer = "http://127.0.0.1:8081/realms/approval-flow-dev";
const ports = [8080, 8081, 5173, 55432];

async function checkPort(port) {
	return new Promise((resolve, reject) => {
		const server = createServer();
		server.once("error", (error) => {
			if (error.code === "EADDRINUSE" || error.code === "EACCES")
				resolve(false);
			else reject(error);
		});
		server.listen(port, "127.0.0.1", () => server.close(() => resolve(true)));
	});
}

function launch(spawnProcess, command, args, options) {
	const child = spawnProcess(command, args, options);
	const state = { child, exited: false, error: undefined, result: undefined };
	state.done = new Promise((resolve, reject) => {
		child.once("error", (error) => {
			state.exited = true;
			state.error = error;
			reject(error);
		});
		child.once("exit", (code, signal) => {
			state.exited = true;
			state.result = { code, signal };
			resolve(state.result);
		});
	});
	state.done.catch(() => {});
	return state;
}

async function command(spawnProcess, name, args, options = {}) {
	const { input, capture = false, onStart, ...spawnOptions } = options;
	const child = launch(spawnProcess, name, args, {
		stdio: [
			input === undefined ? "ignore" : "pipe",
			capture ? "pipe" : "ignore",
			"ignore",
		],
		...spawnOptions,
	});
	onStart?.(child);
	let output = "";
	if (capture && child.child.stdout) {
		child.child.stdout.setEncoding("utf8");
		child.child.stdout.on("data", (chunk) => {
			output += chunk;
			if (output.length > 1024 * 1024) child.child.kill("SIGTERM");
		});
	}
	if (input !== undefined && child.child.stdin) child.child.stdin.end(input);
	const result = await child.done;
	if (result.code !== 0) {
		const error = new Error(`${name} command failed`);
		error.exitCode = result.code ?? 1;
		throw error;
	}
	return output;
}

async function waitFor(
	fetchResource,
	url,
	expectedStatus,
	children,
	signal,
	timeoutMs = 90_000,
) {
	const deadline = Date.now() + timeoutMs;
	while (Date.now() < deadline) {
		if (signal?.aborted) throw new Error("E2E stack interrupted");
		for (const child of children) {
			if (child.exited) {
				const reason = child.result?.signal
					? `signal ${child.result.signal}`
					: child.result?.code === undefined
						? "spawn error"
						: `code ${child.result.code}`;
				throw new Error(`${child.label} exited before readiness (${reason})`);
			}
		}
		try {
			const response = await fetchResource(url, {
				signal: AbortSignal.timeout(2000),
			});
			if (response.status === expectedStatus) return;
		} catch {
			// Services may refuse connections while booting.
		}
		await new Promise((resolve) => setTimeout(resolve, 250));
	}
	throw new Error(`E2E readiness deadline exceeded for ${url}`);
}

async function stopChild(
	state,
	{ signalProcess, processGroupExists, stopGraceMs },
) {
	if (!state) return;
	const pid = state.child.pid;
	if (pid && process.platform !== "win32") {
		if (!processGroupExists(pid)) return;
		signalProcess(-pid, "SIGTERM");
		const deadline = Date.now() + stopGraceMs;
		while (processGroupExists(pid) && Date.now() < deadline) {
			await new Promise((resolve) => setTimeout(resolve, 100));
		}
		if (processGroupExists(pid)) signalProcess(-pid, "SIGKILL");
		return;
	}
	if (state.exited) return;
	state.child.kill("SIGTERM");
	let timeout;
	await Promise.race([
		state.done,
		new Promise((resolve) => {
			timeout = setTimeout(resolve, stopGraceMs);
		}),
	]);
	clearTimeout(timeout);
	if (!state.exited) {
		state.child.kill("SIGKILL");
		await state.done;
	}
}

function processGroupExists(pid) {
	try {
		process.kill(-pid, 0);
		return true;
	} catch (error) {
		if (error.code === "ESRCH") return false;
		throw error;
	}
}

export async function runStack(deps = {}) {
	const spawnProcess = deps.spawn ?? spawn;
	const fetchResource = deps.fetch ?? fetch;
	const portAvailable = deps.checkPort ?? checkPort;
	const random = deps.randomBytes ?? randomBytes;
	const logger =
		deps.logger ?? ((message) => process.stderr.write(`${message}\n`));
	const outerEnv = deps.env ?? process.env;
	const stopOptions = {
		signalProcess: deps.signalProcess ?? process.kill.bind(process),
		processGroupExists: deps.processGroupExists ?? processGroupExists,
		stopGraceMs: deps.stopGraceMs ?? 5000,
	};
	for (const port of ports) {
		if (!(await portAvailable(port)))
			throw new Error(`E2E port ${port} is occupied`);
	}
	const project = `approval-flow-e2e-${random(8).toString("hex")}`;
	const goCache = await mkdtemp(join(tmpdir(), "approval-flow-e2e-go-cache-"));
	let playwrightOutputDir;
	try {
		playwrightOutputDir = await mkdtemp(
			join(tmpdir(), "approval-flow-e2e-playwright-"),
		);
	} catch (error) {
		await rm(goCache, { recursive: true, force: true });
		throw error;
	}
	const password = random(24).toString("base64url");
	const dbPassword = random(24).toString("base64url");
	const adminPassword = random(24).toString("base64url");
	const env = {
		...outerEnv,
		E2E_DB_PASSWORD: dbPassword,
		E2E_KEYCLOAK_ADMIN_PASSWORD: adminPassword,
		DATABASE_URL: `postgres://e2e_user:${encodeURIComponent(dbPassword)}@127.0.0.1:55432/approval_flow_e2e?sslmode=disable`,
		OIDC_ISSUER: issuer,
		OIDC_CLIENT_ID: "approval-flow",
		OIDC_REDIRECT_URI: `${apiOrigin}/auth/oidc/callback`,
		APP_ENV: "development",
		APP_LISTEN_ADDR: "127.0.0.1:8080",
		APP_FRONTEND_ORIGIN: frontendOrigin,
		APP_COOKIE_SECURE: "false",
		AUTH_TRANSACTION_KEY: random(32).toString("base64"),
		SESSION_IDLE_TTL: "15m",
		SESSION_ABSOLUTE_TTL: "8h",
		AUTH_TRANSACTION_TTL: "5m",
		VITE_API_ORIGIN: apiOrigin,
		E2E_TEST_PASSWORD: password,
		GOTOOLCHAIN: "go1.27.1",
		GOCACHE: goCache,
		E2E_PLAYWRIGHT_OUTPUT_DIR: playwrightOutputDir,
	};
	const compose = ["compose", "-p", project, "-f", "compose.e2e.yaml"];
	const controller = new AbortController();
	const execute = (name, args, options = {}) =>
		command(spawnProcess, name, args, {
			...options,
			env: { ...env, ...options.env },
			signal: options.signal === null ? undefined : controller.signal,
		});
	let composeStarted = false;
	let api;
	let vite;
	let playwright;
	const onSignal = () => controller.abort();
	process.on("SIGINT", onSignal);
	process.on("SIGTERM", onSignal);
	let failure;
	try {
		composeStarted = true;
		logger(`Starting E2E project ${project}`);
		await execute("docker", [...compose, "up", "-d", "--wait"]);
		await waitFor(
			fetchResource,
			`${issuer}/.well-known/openid-configuration`,
			200,
			[],
			controller.signal,
		);
		await execute("go", ["run", "./cmd/migrate-local", "up"]);
		const seed =
			deps.seed ??
			(async (seedOptions) => {
				const { provisionE2E } = await import("./e2e-seed.mjs");
				await provisionE2E(seedOptions);
			});
		await seed({ exec: execute, issuer, password, project, env });
		api = launch(spawnProcess, "go", ["run", "./cmd/api"], {
			env,
			stdio: "ignore",
			detached: true,
		});
		api.label = "API";
		await waitFor(
			fetchResource,
			`${apiOrigin}/api/v1/session`,
			401,
			[api],
			controller.signal,
		);
		vite = launch(
			spawnProcess,
			"pnpm",
			["exec", "vite", "--host", "127.0.0.1", "--port", "5173", "--strictPort"],
			{ env, stdio: "ignore", detached: true },
		);
		vite.label = "Vite";
		await waitFor(
			fetchResource,
			frontendOrigin,
			200,
			[api, vite],
			controller.signal,
		);
		try {
			await execute("pnpm", ["exec", "playwright", "test"], {
				detached: true,
				onStart: (state) => {
					playwright = state;
				},
			});
		} catch (error) {
			const browserError = new Error("Playwright command failed");
			browserError.exitCode = error.exitCode ?? 1;
			throw browserError;
		}
		logger("Playwright completed");
	} catch (error) {
		failure = error;
	} finally {
		const cleanupErrors = [];
		for (const child of [playwright, vite, api]) {
			try {
				await stopChild(child, stopOptions);
			} catch (error) {
				cleanupErrors.push(error);
			}
		}
		if (composeStarted) {
			try {
				await execute(
					"docker",
					[...compose, "down", "--volumes", "--remove-orphans"],
					{ signal: null },
				);
			} catch (error) {
				cleanupErrors.push(error);
			}
		}
		if (cleanupErrors.length)
			failure = new AggregateError(
				[failure, ...cleanupErrors].filter(Boolean),
				"E2E cleanup failed",
			);
		for (const directory of [playwrightOutputDir, goCache]) {
			try {
				await rm(directory, { recursive: true, force: true });
			} catch (error) {
				failure = new AggregateError(
					[failure, error].filter(Boolean),
					"E2E cleanup failed",
				);
			}
		}
		process.off("SIGINT", onSignal);
		process.off("SIGTERM", onSignal);
	}
	if (failure) throw failure;
}

if (
	process.argv[1] &&
	import.meta.url === pathToFileURL(process.argv[1]).href
) {
	runStack().catch((error) => {
		process.stderr.write(`${error.message}\n`);
		process.exitCode = error.exitCode ?? 1;
	});
}
