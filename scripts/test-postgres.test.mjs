import assert from "node:assert/strict";
import { test } from "node:test";
import { runDatabaseTests } from "./test-postgres.mjs";

test("database runner creates a fresh password and passes it only in process environments", () => {
	const calls = [];
	const run = (command, args, options) => {
		calls.push({ command, args, options });
		return { status: 0 };
	};
	const result = runDatabaseTests({
		spawnSync: run,
		randomBytes: () => Buffer.alloc(24, 0xab),
		env: {},
	});
	assert.equal(result, 0);
	assert.deepEqual(
		calls.map(({ command }) => command),
		["docker", "go", "docker"],
	);
	const password = "ab".repeat(24);
	assert.equal(calls[0].options.env.TEST_DB_PASSWORD, password);
	assert.equal(
		calls[1].options.env.TEST_DATABASE_URL,
		`postgres://test_user:${password}@127.0.0.1:55432/approval_flow_test?sslmode=disable`,
	);
	assert.equal(calls[2].options.env.TEST_DB_PASSWORD, password);
	assert.ok(calls[1].args.includes("-mod=readonly"));
	assert.equal(calls[1].options.cwd, "backend");
	assert.equal(
		JSON.stringify(calls.map(({ args }) => args)).includes(password),
		false,
	);
});

test("database runner cleans up after test failure", () => {
	const calls = [];
	const result = runDatabaseTests({
		spawnSync: (command, args) => {
			calls.push({ command, args });
			return { status: command === "go" ? 7 : 0 };
		},
		randomBytes: () => Buffer.alloc(24),
		env: {},
	});
	assert.equal(result, 7);
	assert.deepEqual(calls.at(-1).args.slice(-3), [
		"down",
		"-v",
		"--remove-orphans",
	]);
});
