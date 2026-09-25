import type {
	AuditEvent,
	CreateRequestInput,
	ErrorResponse,
	OrganizationSelection,
	Request,
	Session,
	UpdateDraftRequestInput,
} from "./types";

export class ApiError extends Error {
	constructor(
		readonly status: number,
		readonly body: ErrorResponse,
	) {
		super(body.message);
		this.name = "ApiError";
	}
}

export class TransportError extends Error {
	constructor() {
		super("The server response could not be read. Please try again.");
		this.name = "TransportError";
	}
}

const errorCodes = new Set([
	"invalid_request",
	"invalid_auth_transaction",
	"authentication_required",
	"forbidden",
	"csrf_validation_failed",
	"request_not_found",
	"version_conflict",
	"invalid_state",
	"approval_routing_unavailable",
	"internal_error",
]);

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isErrorResponse(value: unknown): value is ErrorResponse {
	return (
		isRecord(value) &&
		typeof value.code === "string" &&
		errorCodes.has(value.code) &&
		typeof value.message === "string" &&
		(value.fieldErrors === undefined ||
			(Array.isArray(value.fieldErrors) &&
				value.fieldErrors.every(
					(item) =>
						isRecord(item) &&
						typeof item.field === "string" &&
						typeof item.code === "string" &&
						typeof item.message === "string",
				)))
	);
}

function isSession(value: unknown): value is Session {
	if (!isRecord(value) || !isRecord(value.actor)) return false;
	const { memberId, roles } = value.actor;
	return (
		typeof memberId === "string" &&
		memberId.length > 0 &&
		Array.isArray(roles) &&
		roles.every(
			(role) => role === "admin" || role === "requester" || role === "approver",
		) &&
		new Set(roles).size === roles.length &&
		typeof value.csrfToken === "string" &&
		value.csrfToken.length > 0
	);
}

async function readJson(response: Response): Promise<Record<string, unknown>> {
	if (
		!response.headers
			.get("Content-Type")
			?.toLowerCase()
			.startsWith("application/json")
	) {
		throw new TransportError();
	}
	try {
		const value: unknown = await response.json();
		if (isRecord(value)) return value;
	} catch {
		throw new TransportError();
	}
	throw new TransportError();
}

export function createApiClient(apiOrigin: URL, fetchFn: typeof fetch = fetch) {
	const url = (path: string) => `${apiOrigin.origin}${path}`;
	const requestPath = (id: string) => {
		if (id === "" || id === "." || id === "..") {
			throw new TypeError("Invalid request ID");
		}
		return `/api/v1/requests/${encodeURIComponent(id)}`;
	};

	async function request<T>(
		path: string,
		method: "GET" | "POST" | "PATCH",
		options: {
			csrfToken?: string;
			body?: unknown;
			redirect?: RequestRedirect;
			allowNoContent?: boolean;
		} = {},
	): Promise<T> {
		const headers: Record<string, string> = {};
		if (options.csrfToken !== undefined)
			headers["X-CSRF-Token"] = options.csrfToken;
		if (options.body !== undefined)
			headers["Content-Type"] = "application/json";
		const init: RequestInit = { method, credentials: "include" };
		if (Object.keys(headers).length) init.headers = headers;
		if (options.body !== undefined) init.body = JSON.stringify(options.body);
		if (options.redirect) init.redirect = options.redirect;
		const response = await fetchFn(url(path), init);
		if (options.redirect === "manual" && response.type === "opaqueredirect")
			return undefined as T;
		if (response.ok && options.redirect === "manual")
			throw new TransportError();
		if (response.ok && options.allowNoContent && response.status !== 204)
			throw new TransportError();
		if (response.ok) {
			if (response.status === 204) {
				if (options.allowNoContent) return undefined as T;
				throw new TransportError();
			}
			return (await readJson(response)) as T;
		}
		const body = await readJson(response);
		if (!isErrorResponse(body)) throw new TransportError();
		throw new ApiError(response.status, body);
	}

	return {
		getSession: async (): Promise<Session> => {
			const session = await request<Record<string, unknown>>(
				"/api/v1/session",
				"GET",
			);
			if (!isSession(session)) throw new TransportError();
			return session;
		},
		getOrganizationSelection: (): Promise<OrganizationSelection> =>
			request("/auth/oidc/organization-selection", "GET"),
		selectOrganization: (memberId: string, csrfToken: string): Promise<void> =>
			request("/auth/oidc/organization-selection", "POST", {
				body: { memberId },
				csrfToken,
				redirect: "manual",
			}),
		createRequest: (
			input: CreateRequestInput,
			csrfToken: string,
		): Promise<Request> =>
			request("/api/v1/requests", "POST", { body: input, csrfToken }),
		updateRequest: async (
			id: string,
			input: UpdateDraftRequestInput,
			csrfToken: string,
		): Promise<Request> =>
			request(requestPath(id), "PATCH", { body: input, csrfToken }),
		submitRequest: async (
			id: string,
			expectedVersion: number,
			csrfToken: string,
		): Promise<Request> =>
			request(`${requestPath(id)}/submit`, "POST", {
				body: { expectedVersion },
				csrfToken,
			}),
		approveRequest: async (
			id: string,
			expectedVersion: number,
			csrfToken: string,
		): Promise<Request> =>
			request(`${requestPath(id)}/approvals`, "POST", {
				body: { expectedVersion },
				csrfToken,
			}),
		getRequest: async (id: string): Promise<Request> =>
			request(requestPath(id), "GET"),
		listPending: async (): Promise<Request[]> => {
			const body = await request<Record<string, unknown>>(
				"/api/v1/requests/pending",
				"GET",
			);
			if (!Array.isArray(body.requests)) throw new TransportError();
			return body.requests;
		},
		listAuditEvents: async (id: string): Promise<AuditEvent[]> => {
			const body = await request<Record<string, unknown>>(
				`${requestPath(id)}/audit-events`,
				"GET",
			);
			if (!Array.isArray(body.events)) throw new TransportError();
			return body.events;
		},
		logout: (csrfToken: string): Promise<void> =>
			request("/api/v1/session/logout", "POST", {
				csrfToken,
				allowNoContent: true,
			}),
	};
}

export type ApiClient = ReturnType<typeof createApiClient>;
