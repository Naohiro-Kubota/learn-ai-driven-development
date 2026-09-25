import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import YAML from "yaml";

test("local Compose starts the application after migrations on isolated services", () => {
	const stack = YAML.parse(
		readFileSync(new URL("../compose.local.yaml", import.meta.url), "utf8"),
	);
	assert.equal(stack.name, "approval-flow-local");
	for (const name of [
		"frontend",
		"backend",
		"postgres",
		"keycloak",
		"migrate",
	]) {
		assert.ok(stack.services[name], `missing ${name}`);
	}
	assert.equal(
		stack.services.backend.depends_on.migrate.condition,
		"service_completed_successfully",
	);
	assert.equal(
		stack.services.migrate.depends_on.postgres.condition,
		"service_healthy",
	);
	for (const name of ["frontend", "backend", "postgres", "keycloak"]) {
		for (const port of stack.services[name].ports ?? [])
			assert.match(port, /^127\.0\.0\.1:/);
	}
	assert.ok(Object.hasOwn(stack.volumes, "local_postgres"));
	assert.ok(!Object.hasOwn(stack.volumes, "e2e_postgres"));
});
