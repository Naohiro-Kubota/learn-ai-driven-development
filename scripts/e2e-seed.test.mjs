import assert from "node:assert/strict";
import test from "node:test";
import { provisionE2E } from "./e2e-seed.mjs";

const subjects = {
	requester: "11111111-1111-4111-8111-111111111111",
	approver: "22222222-2222-4222-8222-222222222222",
	multi: "33333333-3333-4333-8333-333333333333",
};

function harness(users = subjects) {
	const calls = [];
	const exec = async (command, args, options = {}) => {
		calls.push({ command, args, options });
		if (args.includes("get") && args.includes("users")) {
			const username = args
				.find((arg) => arg.startsWith("username="))
				.slice("username=".length);
			return JSON.stringify(
				users[username] === undefined
					? []
					: [{ username, id: users[username] }],
			);
		}
		return "";
	};
	return { calls, exec };
}

const options = {
	issuer: "http://127.0.0.1:8081/realms/approval-flow-dev",
	password: "secret-sentinel",
	project: "approval-flow-e2e-test",
	env: { E2E_KEYCLOAK_ADMIN_PASSWORD: "admin-secret-sentinel" },
};

for (const [name, users] of [
	["missing", { ...subjects, multi: undefined }],
	["malformed", { ...subjects, requester: "'bad'" }],
	["duplicate", { ...subjects, multi: subjects.requester }],
]) {
	test(`rejects ${name} Keycloak subjects before SQL`, async () => {
		const { calls, exec } = harness(users);
		await assert.rejects(provisionE2E({ ...options, exec }), /subject/i);
		assert.equal(
			calls.some(({ args }) => args.includes("psql")),
			false,
		);
	});
}

test("provisions real identity mappings, memberships, roles, and default approver", async () => {
	const { calls, exec } = harness();
	await provisionE2E({ ...options, exec });
	const sqlCall = calls.find(({ args }) => args.includes("psql"));
	assert.ok(sqlCall);
	const sql = sqlCall.options.input;
	assert.match(sql, /BEGIN;/);
	assert.match(sql, /COMMIT;/);
	assert.match(sql, /org-a/);
	assert.match(sql, /org-b/);
	assert.match(sql, /member-requester-a/);
	assert.match(sql, /member-approver-a/);
	assert.match(sql, /member-multi-a/);
	assert.match(sql, /member-multi-b/);
	for (const subject of Object.values(subjects))
		assert.ok(sql.includes(subject));
	assert.match(sql, /default_approver_member_id/);
	assert.match(sql, /member_roles/);
	assert.match(sql, /member_oidc_identities/);
	assert.match(sql, /RAISE EXCEPTION/);
	assert.equal(
		JSON.stringify(calls.map(({ args }) => args)).includes("secret-sentinel"),
		false,
	);
	const admin = calls.find(({ args }) => args.includes("credentials"));
	assert.equal(
		admin.options.input,
		`${options.env.E2E_KEYCLOAK_ADMIN_PASSWORD}\n`,
	);
	assert.ok(admin.args.includes("sh"));
	for (const call of calls.filter(({ args }) =>
		args.includes("set-password"),
	)) {
		assert.equal(call.options.input, `${options.password}\n`);
		assert.ok(call.args.includes("sh"));
	}
});

test("rejects ambiguous and inexact Keycloak searches before SQL", async () => {
	for (const response of [
		[{ username: "requester-extra", id: subjects.requester }],
		[
			{ username: "requester", id: subjects.requester },
			{ username: "requester", id: subjects.approver },
		],
	]) {
		const { calls, exec } = harness();
		const searchingExec = async (command, args, callOptions) =>
			args.includes("get") && args.includes("username=requester")
				? JSON.stringify(response)
				: exec(command, args, callOptions);
		await assert.rejects(
			provisionE2E({ ...options, exec: searchingExec }),
			/subject/i,
		);
		assert.equal(
			calls.some(({ args }) => args.includes("psql")),
			false,
		);
	}
});

test("passes the Keycloak config option after each command", async () => {
	const { calls, exec } = harness();
	await provisionE2E({ ...options, exec });
	for (const { args } of calls.filter(({ args }) =>
		args.includes("--config"),
	)) {
		const command = args.findIndex((arg) =>
			["config", "create", "set-password", "get"].includes(arg),
		);
		assert.ok(command >= 0);
		assert.ok(args.indexOf("--config") > command);
	}
});
