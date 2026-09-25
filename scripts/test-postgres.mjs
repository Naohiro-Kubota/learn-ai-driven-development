import { spawnSync } from "node:child_process";
import { randomBytes } from "node:crypto";
import { pathToFileURL } from "node:url";

const compose = ["compose", "-f", "compose.test.yaml"];

export function runDatabaseTests({
	spawnSync: spawnProcess = spawnSync,
	randomBytes: random = randomBytes,
	env: outerEnv = process.env,
} = {}) {
	const password = random(24).toString("hex");
	const env = { ...outerEnv, TEST_DB_PASSWORD: password };
	const databaseURL = `postgres://test_user:${encodeURIComponent(password)}@127.0.0.1:55432/approval_flow_test?sslmode=disable`;
	const run = (command, args, runEnv = env) =>
		spawnProcess(command, args, {
			stdio: "inherit",
			env: runEnv,
			...(command === "go" ? { cwd: "backend" } : {}),
		}).status ?? 1;
	let status = 0;
	try {
		status = run("docker", [...compose, "up", "-d", "--wait"]);
		if (status === 0) {
			status = run(
				"go",
				["test", "-mod=readonly", "./internal/store/postgres", "-count=1"],
				{
					...env,
					GOTOOLCHAIN: "go1.27.1",
					TEST_DATABASE_URL: databaseURL,
				},
			);
		}
	} finally {
		const cleanupStatus = run("docker", [
			...compose,
			"down",
			"-v",
			"--remove-orphans",
		]);
		if (status === 0) status = cleanupStatus;
	}
	return status;
}

if (
	process.argv[1] &&
	import.meta.url === pathToFileURL(process.argv[1]).href
) {
	process.exitCode = runDatabaseTests();
}
