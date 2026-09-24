const expectedIssuer = "http://127.0.0.1:8081/realms/approval-flow-dev";
const realm = "approval-flow-dev";
const configPath = "/tmp/e2e-kcadm-config";
const uuid =
	/^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

function seedSql(subjects) {
	return `BEGIN;
INSERT INTO organizations (id, name) VALUES
  ('org-a', 'Organization A'), ('org-b', 'Organization B');
INSERT INTO members (id, organization_id, oidc_subject) VALUES
  ('member-requester-a', 'org-a', '${subjects.requester}'),
  ('member-approver-a', 'org-a', '${subjects.approver}'),
  ('member-multi-a', 'org-a', '${subjects.multi}'),
  ('member-multi-b', 'org-b', '${subjects.multi}');
INSERT INTO member_roles (member_id, role) VALUES
  ('member-requester-a', 'requester'),
  ('member-approver-a', 'approver'),
  ('member-multi-a', 'requester'),
  ('member-multi-b', 'requester');
INSERT INTO oidc_identities (id, issuer, subject) VALUES
  ('identity-requester', '${expectedIssuer}', '${subjects.requester}'),
  ('identity-approver', '${expectedIssuer}', '${subjects.approver}'),
  ('identity-multi', '${expectedIssuer}', '${subjects.multi}');
INSERT INTO member_oidc_identities (identity_id, member_id) VALUES
  ('identity-requester', 'member-requester-a'),
  ('identity-approver', 'member-approver-a'),
  ('identity-multi', 'member-multi-a'),
  ('identity-multi', 'member-multi-b');
UPDATE organizations SET default_approver_member_id = 'member-approver-a' WHERE id = 'org-a';
DO $$ BEGIN
  IF (SELECT count(*) FROM organizations) <> 2
    OR (SELECT count(*) FROM oidc_identities) <> 3
    OR (SELECT count(*) FROM members) <> 4
    OR (SELECT count(*) FROM member_roles) <> 4
    OR (SELECT count(*) FROM member_oidc_identities) <> 4
    OR (SELECT count(*) FROM member_oidc_identities WHERE identity_id = 'identity-multi') <> 2
    OR (SELECT default_approver_member_id FROM organizations WHERE id = 'org-a') <> 'member-approver-a'
  THEN RAISE EXCEPTION 'E2E seed verification failed'; END IF;
END $$;
COMMIT;`;
}

export async function provisionE2E({ exec, issuer, password, project, env }) {
	if (issuer !== expectedIssuer) throw new Error("Unexpected E2E issuer");
	if (!/^approval-flow-e2e-[a-z0-9]+$/.test(project))
		throw new Error("Invalid E2E project");
	if (!password || !env?.E2E_KEYCLOAK_ADMIN_PASSWORD)
		throw new Error("E2E credentials are required");
	const compose = [
		"compose",
		"-p",
		project,
		"-f",
		"compose.e2e.yaml",
		"exec",
		"-T",
	];
	const kcadm = async (args, options = {}) =>
		exec(
			"docker",
			[
				...compose,
				"keycloak",
				...(options.secret
					? [
							"sh",
							"-c",
							'IFS= read -r KC_CLI_PASSWORD; export KC_CLI_PASSWORD; exec /opt/keycloak/bin/kcadm.sh "$@"',
							"sh",
						]
					: ["/opt/keycloak/bin/kcadm.sh"]),
				...args,
				"--config",
				configPath,
			],
			options.secret ? { input: `${options.secret}\n` } : options,
		);
	const subjects = {};
	try {
		await kcadm(
			[
				"config",
				"credentials",
				"--server",
				"http://127.0.0.1:8080",
				"--realm",
				"master",
				"--user",
				"e2e_admin",
			],
			{ secret: env.E2E_KEYCLOAK_ADMIN_PASSWORD },
		);
		for (const username of ["requester", "approver", "multi"]) {
			await kcadm([
				"create",
				"users",
				"-r",
				realm,
				"-s",
				`username=${username}`,
				"-s",
				"enabled=true",
				"-s",
				`email=${username}@approval-flow.invalid`,
			]);
			await kcadm(["set-password", "-r", realm, "--username", username], {
				secret: password,
			});
			const response = await kcadm(
				["get", "users", "-r", realm, "-q", `username=${username}`],
				{ capture: true },
			);
			let users;
			try {
				users = JSON.parse(response);
			} catch {
				throw new Error(`Invalid Keycloak subject response for ${username}`);
			}
			if (
				!Array.isArray(users) ||
				users.length !== 1 ||
				users[0]?.username !== username ||
				!uuid.test(users[0]?.id ?? "")
			)
				throw new Error(`Invalid Keycloak subject for ${username}`);
			subjects[username] = users[0].id.toLowerCase();
		}
		if (new Set(Object.values(subjects)).size !== 3)
			throw new Error("Duplicate Keycloak subject");
		await exec(
			"docker",
			[
				...compose,
				"postgres",
				"psql",
				"-U",
				"e2e_user",
				"-d",
				"approval_flow_e2e",
				"-v",
				"ON_ERROR_STOP=1",
			],
			{ input: seedSql(subjects) },
		);
	} finally {
		await exec("docker", [...compose, "keycloak", "rm", "-f", configPath]);
	}
}
