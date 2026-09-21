import { spawnSync } from "node:child_process";

const compose = ["compose", "-f", "compose.test.yaml"];
const databaseURL =
	"postgres://test_user:test_password@127.0.0.1:55432/approval_flow_test?sslmode=disable";

function run(command, args, options = {}) {
	const result = spawnSync(command, args, { stdio: "inherit", ...options });
	if (result.status !== 0) process.exitCode = result.status ?? 1;
	return result.status === 0;
}

try {
	if (run("docker", [...compose, "up", "-d", "--wait"])) {
		run("go", ["test", "-mod=mod", "./internal/store/postgres", "-count=1"], {
			env: {
				...process.env,
				GOTOOLCHAIN: "go1.27.1",
				TEST_DATABASE_URL: databaseURL,
			},
		});
	}
} finally {
	run("docker", [...compose, "down", "-v"]);
}
