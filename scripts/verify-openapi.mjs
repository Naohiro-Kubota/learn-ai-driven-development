import { readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

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
const requiredSecuritySchemes = [
	"sessionCookie",
	"organizationSelectionCookie",
	"csrfToken",
];
const httpMethods = new Set([
	"get",
	"put",
	"post",
	"delete",
	"options",
	"head",
	"patch",
	"trace",
]);

async function parseYaml(source) {
	try {
		const yaml = await import("yaml");
		return yaml.parse(source);
	} catch (error) {
		if (error.code === "ERR_MODULE_NOT_FOUND") {
			throw new Error(
				"Unable to load OpenAPI YAML: the 'yaml' package is required",
				{ cause: error },
			);
		}
		throw error;
	}
}

export async function loadOpenAPI(filePath) {
	const source = await readFile(filePath, "utf8");
	return parseYaml(source);
}

export function references(value) {
	const found = [];
	const visit = (current) => {
		if (Array.isArray(current)) {
			current.forEach(visit);
			return;
		}
		if (!current || typeof current !== "object") return;
		if (Object.hasOwn(current, "$ref")) found.push(current.$ref);
		Object.values(current).forEach(visit);
	};
	visit(value);
	return found;
}

export function resolveLocalReference(document, reference) {
	if (typeof reference !== "string" || !reference.startsWith("#/")) {
		throw new Error(`Reference is not a local JSON Pointer: ${reference}`);
	}
	if (reference === "#/")
		throw new Error(`Reference has an empty JSON Pointer: ${reference}`);

	let current = document;
	for (const rawToken of reference.slice(2).split("/")) {
		if (/~(?![01])/.test(rawToken))
			throw new Error(
				`Reference has a malformed JSON Pointer escape: ${reference}`,
			);
		const token = rawToken.replaceAll("~1", "/").replaceAll("~0", "~");
		if (Array.isArray(current)) {
			if (!/^(0|[1-9]\d*)$/.test(token)) {
				throw new Error(`Reference does not resolve: ${reference}`);
			}
			const index = Number(token);
			current =
				Number.isSafeInteger(index) && Object.hasOwn(current, token)
					? current[index]
					: undefined;
		} else if (current && typeof current === "object") {
			current = Object.hasOwn(current, token) ? current[token] : undefined;
		} else {
			current = undefined;
		}
		if (current === undefined) {
			throw new Error(`Reference does not resolve: ${reference}`);
		}
	}
	return current;
}

export function validateOpenAPI(document) {
	if (!document || typeof document !== "object" || Array.isArray(document)) {
		throw new Error("OpenAPI document must be an object");
	}
	if (
		typeof document.openapi !== "string" ||
		!/^3\.1\./.test(document.openapi)
	) {
		throw new Error("OpenAPI version must be 3.1.x");
	}
	if (
		!document.paths ||
		typeof document.paths !== "object" ||
		Array.isArray(document.paths)
	) {
		throw new Error("OpenAPI paths must be an object");
	}
	if (
		!document.components ||
		typeof document.components !== "object" ||
		Array.isArray(document.components)
	) {
		throw new Error("OpenAPI components must be an object");
	}

	for (const reference of references(document)) {
		resolveLocalReference(document, reference);
	}

	const operationIds = new Set();
	for (const pathItem of Object.values(document.paths)) {
		if (!pathItem || typeof pathItem !== "object" || Array.isArray(pathItem))
			continue;
		for (const [method, operation] of Object.entries(pathItem)) {
			if (
				!httpMethods.has(method) ||
				!operation ||
				typeof operation !== "object"
			)
				continue;
			if (
				typeof operation.operationId !== "string" ||
				operation.operationId.length === 0
			)
				continue;
			if (operationIds.has(operation.operationId)) {
				throw new Error(`Duplicate operationId: ${operation.operationId}`);
			}
			operationIds.add(operation.operationId);
		}
	}
	for (const operationId of requiredOperationIds) {
		if (!operationIds.has(operationId))
			throw new Error(`Required operationId is missing: ${operationId}`);
	}

	const schemes = document.components.securitySchemes;
	if (!schemes || typeof schemes !== "object" || Array.isArray(schemes)) {
		throw new Error("OpenAPI components.securitySchemes must be an object");
	}
	for (const scheme of requiredSecuritySchemes) {
		if (!Object.hasOwn(schemes, scheme))
			throw new Error(`Required security scheme is missing: ${scheme}`);
	}
	return true;
}

const isMain =
	process.argv[1] &&
	path.resolve(process.argv[1]) === fileURLToPath(import.meta.url);
if (isMain) {
	const filePath = path.resolve(process.argv[2] ?? "api/openapi.yaml");
	try {
		const document = await loadOpenAPI(filePath);
		validateOpenAPI(document);
		console.log(`OpenAPI contract valid: ${filePath}`);
	} catch (error) {
		console.error(`OpenAPI contract invalid: ${error.message}`);
		process.exitCode = 1;
	}
}
