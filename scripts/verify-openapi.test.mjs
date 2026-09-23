import assert from "node:assert/strict";
import { mkdtemp, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { test } from "node:test";
import { spawnSync } from "node:child_process";

import {
	loadOpenAPI,
	references,
	resolveLocalReference,
	validateOpenAPI,
} from "./verify-openapi.mjs";

const repositoryRoot = path.resolve(import.meta.dirname, "..");
const openApiPath = path.join(repositoryRoot, "api/openapi.yaml");
const yamlAvailable = await import("yaml").then(() => true).catch(() => false);
const requiredOperationIds = [
	"beginOidcLogin",
	"completeOidcLogin",
	"getOrganizationSelection",
	"selectOrganization",
	"getCurrentSession",
	"logout",
	"createRequest",
	"listPendingRequests",
	"getRequest",
	"updateDraftRequest",
	"submitRequest",
	"approveRequest",
	"listRequestAuditEvents",
];

function validDocument(overrides = {}) {
	return {
		openapi: "3.1.0",
		paths: Object.fromEntries(
			requiredOperationIds.map((operationId, index) => [
				`/operation-${index}`,
				{ get: { operationId } },
			]),
		),
		components: {
			securitySchemes: {
				sessionCookie: {},
				organizationSelectionCookie: {},
				csrfToken: {},
			},
		},
		...overrides,
	};
}

test("loads the repository OpenAPI document and validates its required contract", {
	skip: !yamlAvailable,
}, async () => {
	const document = await loadOpenAPI(openApiPath);

	assert.match(document.openapi, /^3\.1\./);
	assert.doesNotThrow(() => validateOpenAPI(document));

	const localReferences = references(document);
	assert.ok(localReferences.length > 0);
	for (const reference of localReferences) {
		assert.doesNotThrow(() => resolveLocalReference(document, reference));
	}

	const operationIds = Object.values(document.paths)
		.flatMap((pathItem) => Object.values(pathItem))
		.filter((operation) => operation && typeof operation === "object")
		.map((operation) => operation.operationId)
		.filter(Boolean);
	assert.deepEqual(
		requiredOperationIds.filter(
			(operationId) => !operationIds.includes(operationId),
		),
		[],
	);
	assert.deepEqual(
		["sessionCookie", "organizationSelectionCookie", "csrfToken"].filter(
			(scheme) => !Object.hasOwn(document.components.securitySchemes, scheme),
		),
		[],
	);
});

test("documents CORS rejection and internal errors for every affected route", {
	skip: !yamlAvailable,
}, async () => {
	const document = await loadOpenAPI(openApiPath);
	const forbiddenOrCsrf =
		"#/components/responses/ForbiddenOrCsrfValidationFailed";
	const internalServerError = "#/components/responses/InternalServerError";
	const operations = [
		["/auth/oidc/login", "get"],
		["/auth/oidc/callback", "get"],
		["/auth/oidc/organization-selection", "get"],
		["/auth/oidc/organization-selection", "post"],
		["/api/v1/session", "get"],
		["/api/v1/session/logout", "post"],
		["/api/v1/requests", "post"],
		["/api/v1/requests/pending", "get"],
		["/api/v1/requests/{requestId}", "get"],
		["/api/v1/requests/{requestId}", "patch"],
		["/api/v1/requests/{requestId}/submit", "post"],
		["/api/v1/requests/{requestId}/approvals", "post"],
		["/api/v1/requests/{requestId}/audit-events", "get"],
	];

	for (const [route, method] of operations) {
		assert.deepEqual(document.paths[route][method].responses["403"], {
			$ref: forbiddenOrCsrf,
		});
	}

	for (const [route, method] of operations.filter(
		([route]) => route !== "/auth/oidc/callback",
	)) {
		assert.deepEqual(document.paths[route][method].responses["500"], {
			$ref: internalServerError,
		});
	}

	assert.equal(
		document.paths["/auth/oidc/callback"].get.responses["500"].description,
		"Unexpected callback failure after accepting exactly one named, nonempty authentication transaction cookie; that cookie is cleared.",
	);
	for (const [route, method] of operations) {
		const response = document.paths[route][method].responses["500"];
		assert.ok(
			response,
			`${method.toUpperCase()} ${route} documents a 500 response`,
		);
	}
});

test("resolves escaped JSON Pointer reference tokens for object keys", () => {
	const document = { components: { schemas: { "a/b~c": { type: "string" } } } };

	assert.deepEqual(
		resolveLocalReference(document, "#/components/schemas/a~1b~0c"),
		{ type: "string" },
	);
});

test("rejects missing and non-local references", () => {
	const document = { components: { schemas: {} } };

	assert.throws(
		() => resolveLocalReference(document, "#/components/schemas/Missing"),
		/does not resolve/i,
	);
	assert.throws(
		() => resolveLocalReference(document, "https://example.test/schema.json"),
		/local JSON Pointer/i,
	);
});

test("rejects external, fragmentless, empty, and malformed references during validation", () => {
	for (const reference of [
		"https://example.test/schema.json#/Thing",
		"components/schemas/Thing",
		"#",
		"#/",
		"#/components/schemas/a~2b",
		"#/components/schemas/a~",
	]) {
		assert.throws(
			() =>
				validateOpenAPI(
					validDocument({
						components: {
							schemas: { Thing: {}, a: {} },
							securitySchemes: validDocument().components.securitySchemes,
						},
						reference: { $ref: reference },
					}),
				),
			/local JSON Pointer|empty|malformed|does not resolve/i,
			reference,
		);
	}
});

test("resolves only own properties and valid array indexes", () => {
	const inherited = Object.create({ inherited: { type: "string" } });
	inherited.own = { type: "number" };
	const document = { inherited, values: ["first"] };

	assert.deepEqual(resolveLocalReference(document, "#/inherited/own"), {
		type: "number",
	});
	assert.throws(
		() => resolveLocalReference(document, "#/inherited/inherited"),
		/does not resolve/i,
	);
	assert.equal(resolveLocalReference(document, "#/values/0"), "first");
	assert.throws(
		() => resolveLocalReference(document, "#/values/01"),
		/does not resolve/i,
	);
	assert.throws(
		() => resolveLocalReference(document, "#/values/-"),
		/does not resolve/i,
	);
});

test("reports missing required operations and security schemes", () => {
	assert.throws(
		() =>
			validateOpenAPI({
				openapi: "3.1.0",
				paths: {},
				components: { securitySchemes: {} },
			}),
		/operationId.*missing.*beginOidcLogin/i,
	);
});

test("reports missing security schemes after operations are present", () => {
	const document = validDocument();
	delete document.components.securitySchemes.csrfToken;

	assert.throws(
		() => validateOpenAPI(document),
		/security scheme is missing.*csrfToken/i,
	);
});

test("rejects duplicate operation IDs", () => {
	const document = {
		openapi: "3.1.0",
		paths: {
			"/one": { get: { operationId: "beginOidcLogin" } },
			"/two": { get: { operationId: "beginOidcLogin" } },
		},
		components: {
			securitySchemes: {
				sessionCookie: {},
				organizationSelectionCookie: {},
				csrfToken: {},
			},
		},
	};

	assert.throws(() => validateOpenAPI(document), /duplicate.*beginOidcLogin/i);
});

test("CLI reports a valid contract and exits successfully", {
	skip: !yamlAvailable,
}, () => {
	const result = spawnSync(process.execPath, ["scripts/verify-openapi.mjs"], {
		cwd: repositoryRoot,
		encoding: "utf8",
	});

	assert.equal(result.status, 0, result.stderr);
	assert.match(
		result.stdout,
		/OpenAPI contract valid: .*api[\\/]openapi\.yaml/,
	);
});

test("reports a clear error when the yaml dependency is unavailable", {
	skip: yamlAvailable,
}, async () => {
	await assert.rejects(
		() => loadOpenAPI(openApiPath),
		/yaml.*package.*required/i,
	);
});

test("CLI reports invalid input on stderr and exits with status 1", async () => {
	const tempDirectory = await mkdtemp(
		path.join(os.tmpdir(), "verify-openapi-"),
	);
	const invalidPath = path.join(tempDirectory, "invalid.yaml");
	await writeFile(invalidPath, "openapi: 3.0.0\npaths: {}\ncomponents: {}\n");

	const result = spawnSync(
		process.execPath,
		["scripts/verify-openapi.mjs", invalidPath],
		{
			cwd: repositoryRoot,
			encoding: "utf8",
		},
	);

	assert.equal(result.status, 1);
	assert.match(result.stderr, /OpenAPI contract invalid/i);
});
