import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import YAML from "yaml";

const workflow = YAML.parse(
	readFileSync(
		new URL("../.github/workflows/ci.yaml", import.meta.url),
		"utf8",
	),
);
const jobs = Object.values(workflow.jobs);
const steps = jobs.flatMap((job) => job.steps);
const commands = steps.map((step) => step.run ?? "");

test("CI starts when a pull request is opened, updated, or reopened", () => {
	assert.deepEqual(workflow.on, {
		pull_request: { types: ["opened", "synchronize", "reopened"] },
	});
	assert.deepEqual(workflow.permissions, { contents: "read" });
});

test("CI pins actions and toolchains and does not bypass failures", () => {
	for (const step of steps) {
		if (step.uses) {
			assert.match(step.uses, /^[\w-]+\/[\w-]+@[0-9a-f]{40}$/);
		}
		assert.notEqual(step["continue-on-error"], true);
	}
	assert.equal(JSON.stringify(workflow).includes("continue-on-error"), false);
	assert.equal(JSON.stringify(workflow).includes("skip-ci"), false);
	assert.ok(steps.some((step) => step.with?.["node-version"] === "26.9.0"));
	assert.ok(steps.some((step) => step.with?.version === "12.5.1"));
	assert.ok(steps.some((step) => step.with?.["go-version"] === "1.27.1"));
});

test("CI runs every required check and isolates database and browser jobs", () => {
	for (const command of [
		"pnpm install --frozen-lockfile",
		"pnpm run verify:openapi",
		"pnpm run format:check",
		"pnpm run lint",
		"pnpm run typecheck",
		"pnpm test",
		"node --test scripts/verify-openapi.test.mjs scripts/test-postgres.test.mjs scripts/verify-ci-workflow.test.mjs scripts/e2e-seed.test.mjs",
		"pnpm run test:e2e:runner",
		"bash -n scripts/provision-keycloak.sh scripts/provision-keycloak.test.sh",
		"bash scripts/provision-keycloak.test.sh",
		"pnpm run build",
		"pnpm run check:gofmt",
		"go vet ./...",
		"go mod verify",
		"pnpm run test:db",
		"pnpm exec playwright install --with-deps chromium",
		"pnpm run test:e2e",
	]) {
		assert.ok(
			commands.some((run) => run.includes(command)),
			`missing ${command}`,
		);
	}
	const dbJob = jobs.find((job) =>
		job.steps.some((step) => step.run?.includes("pnpm run test:db")),
	);
	const e2eJob = jobs.find((job) =>
		job.steps.some((step) => step.run?.trim() === "pnpm run test:e2e"),
	);
	assert.ok(dbJob);
	assert.ok(e2eJob);
	assert.notEqual(dbJob, e2eJob);
});
