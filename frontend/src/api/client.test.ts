import { describe, expect, it, vi } from "vitest";
import { ApiError, createApiClient, TransportError } from "./client";

const origin = new URL("https://api.example.test");
const draft = {
	id: "req-1",
	title: "Laptop",
	description: "",
	status: "draft",
	version: 1,
	requesterMemberId: "member-1",
	approval: null,
	createdAt: "2026-09-23T00:00:00Z",
	updatedAt: "2026-09-23T00:00:00Z",
};
const auditEvent = {
	id: "event-1",
	type: "request_created",
	occurredAt: "2026-09-23T00:00:00Z",
	actorMemberId: "member-1",
	requestContent: { title: "Laptop", description: "" },
	approvalAssigneeMemberId: null,
	approvalId: null,
};
function json(body: unknown, status = 200) {
	return new Response(JSON.stringify(body), {
		status,
		headers: { "Content-Type": "application/json" },
	});
}
function clientWith(response: Response) {
	const fetchFn = vi.fn<typeof fetch>().mockResolvedValue(response);
	return { client: createApiClient(origin, fetchFn), fetchFn };
}

describe("API client request contracts", () => {
	it.each([
		[
			"getSession",
			"/api/v1/session",
			{
				actor: { memberId: "member-1", roles: ["requester"] },
				csrfToken: "csrf",
			},
		],
		[
			"getOrganizationSelection",
			"/auth/oidc/organization-selection",
			{
				candidates: [
					{
						memberId: "member-1",
						organizationId: "org-1",
						organizationName: "Org",
					},
				],
				csrfToken: "csrf",
			},
		],
		["getRequest", "/api/v1/requests/a%2Fb%3Fc%23d", draft],
		["listPending", "/api/v1/requests/pending", [draft]],
		[
			"listAuditEvents",
			"/api/v1/requests/a%2Fb%3Fc%23d/audit-events",
			[auditEvent],
		],
	] as const)(
		"%s sends credentialed GET and returns DTO",
		async (method, path, result) => {
			const payload =
				method === "listPending"
					? { requests: result }
					: method === "listAuditEvents"
						? { events: result }
						: result;
			const { client, fetchFn } = clientWith(json(payload));
			const actual =
				method === "getRequest" || method === "listAuditEvents"
					? await client[method]("a/b?c#d")
					: await client[method]();
			expect(actual).toEqual(result);
			expect(fetchFn).toHaveBeenCalledWith(`https://api.example.test${path}`, {
				method: "GET",
				credentials: "include",
			});
		},
	);

	it.each([
		[
			"createRequest",
			[{ title: "Laptop", description: "" }, "csrf-current"],
			"POST",
			"/api/v1/requests",
			{ title: "Laptop", description: "" },
			201,
		],
		[
			"updateRequest",
			[
				"a/b?c#d",
				{ title: "Laptop", description: "new", expectedVersion: 1 },
				"csrf-current",
			],
			"PATCH",
			"/api/v1/requests/a%2Fb%3Fc%23d",
			{ title: "Laptop", description: "new", expectedVersion: 1 },
			200,
		],
		[
			"submitRequest",
			["a/b?c#d", 1, "csrf-current"],
			"POST",
			"/api/v1/requests/a%2Fb%3Fc%23d/submit",
			{ expectedVersion: 1 },
			200,
		],
		[
			"approveRequest",
			["a/b?c#d", 1, "csrf-current"],
			"POST",
			"/api/v1/requests/a%2Fb%3Fc%23d/approvals",
			{ expectedVersion: 1 },
			200,
		],
	] as const)(
		"%s sends JSON and current CSRF token",
		async (method, args, httpMethod, path, body, status) => {
			const { client, fetchFn } = clientWith(json(draft, status));
			const actual = await (
				client[method] as (...args: unknown[]) => Promise<unknown>
			)(...args);
			expect(actual).toEqual(draft);
			expect(fetchFn).toHaveBeenCalledWith(`https://api.example.test${path}`, {
				method: httpMethod,
				credentials: "include",
				headers: {
					"Content-Type": "application/json",
					"X-CSRF-Token": "csrf-current",
				},
				body: JSON.stringify(body),
			});
		},
	);

	it.each(["", ".", ".."])(
		"rejects unsafe request ID %j before fetch",
		async (id) => {
			const { client, fetchFn } = clientWith(json(draft));
			await expect(client.getRequest(id)).rejects.toBeInstanceOf(TypeError);
			await expect(
				client.updateRequest(
					id,
					{ title: "Laptop", description: "", expectedVersion: 1 },
					"csrf",
				),
			).rejects.toBeInstanceOf(TypeError);
			await expect(client.submitRequest(id, 1, "csrf")).rejects.toBeInstanceOf(
				TypeError,
			);
			await expect(client.approveRequest(id, 1, "csrf")).rejects.toBeInstanceOf(
				TypeError,
			);
			await expect(client.listAuditEvents(id)).rejects.toBeInstanceOf(
				TypeError,
			);
			expect(fetchFn).not.toHaveBeenCalled();
		},
	);

	it.each(["getSession", "listPending"] as const)(
		"%s rejects unexpected 204 as transport failure",
		async (method) => {
			const { client } = clientWith(new Response(null, { status: 204 }));
			await expect(client[method]()).rejects.toBeInstanceOf(TransportError);
		},
	);

	it("logout accepts 204 without parsing and sends CSRF without Content-Type", async () => {
		const { client, fetchFn } = clientWith(new Response(null, { status: 204 }));
		expect(await client.logout("csrf-current")).toBeUndefined();
		expect(fetchFn).toHaveBeenCalledWith(
			"https://api.example.test/api/v1/session/logout",
			{
				method: "POST",
				credentials: "include",
				headers: { "X-CSRF-Token": "csrf-current" },
			},
		);
	});

	it("logout rejects JSON 200 instead of treating it as a successful revocation", async () => {
		const { client } = clientWith(json({}));
		await expect(client.logout("csrf-current")).rejects.toBeInstanceOf(
			TransportError,
		);
	});

	it("organization selection rejects JSON 200 instead of treating it as a redirect", async () => {
		const { client } = clientWith(json({}));
		await expect(
			client.selectOrganization("member-1", "csrf-current"),
		).rejects.toBeInstanceOf(TransportError);
	});

	it("organization selection uses manual redirect and accepts opaqueredirect", async () => {
		const response = { type: "opaqueredirect" } as Response;
		const fetchFn = vi.fn<typeof fetch>().mockResolvedValue(response);
		const client = createApiClient(origin, fetchFn);
		expect(
			await client.selectOrganization("member-1", "csrf-current"),
		).toBeUndefined();
		expect(fetchFn).toHaveBeenCalledWith(
			"https://api.example.test/auth/oidc/organization-selection",
			{
				method: "POST",
				credentials: "include",
				redirect: "manual",
				headers: {
					"Content-Type": "application/json",
					"X-CSRF-Token": "csrf-current",
				},
				body: JSON.stringify({ memberId: "member-1" }),
			},
		);
	});

	it.each([400, 401, 403, 409])(
		"retains structured HTTP error %i",
		async (status) => {
			const body = {
				code: "invalid_request",
				message: "Invalid",
				fieldErrors: [
					{ field: "title", code: "required", message: "Required" },
				],
			};
			const { client } = clientWith(json(body, status));
			await expect(
				client.createRequest({ title: "" }, "csrf"),
			).rejects.toMatchObject({ status, body });
		},
	);

	it("throws ApiError for a valid HTTP error", async () => {
		const { client } = clientWith(
			json({ code: "authentication_required", message: "Sign in" }, 401),
		);
		await expect(client.getSession()).rejects.toBeInstanceOf(ApiError);
	});

	it("organization selection retains a normal JSON error", async () => {
		const { client } = clientWith(
			json({ code: "invalid_auth_transaction", message: "Expired" }, 400),
		);
		await expect(
			client.selectOrganization("member-1", "csrf"),
		).rejects.toMatchObject({
			status: 400,
			body: { code: "invalid_auth_transaction" },
		});
	});

	it.each([
		[
			"non-JSON success",
			new Response("oops", {
				status: 200,
				headers: { "Content-Type": "text/plain" },
			}),
		],
		[
			"empty success",
			new Response(null, {
				status: 200,
				headers: { "Content-Type": "application/json" },
			}),
		],
		[
			"malformed success",
			new Response("{", {
				status: 200,
				headers: { "Content-Type": "application/json" },
			}),
		],
		["non-object success", json([])],
		[
			"non-JSON error",
			new Response("oops", {
				status: 500,
				headers: { "Content-Type": "text/plain" },
			}),
		],
		["malformed error", json({ code: 7, message: "bad" }, 500)],
	])("%s becomes TransportError", async (_, response) => {
		const { client } = clientWith(response);
		await expect(client.getSession()).rejects.toBeInstanceOf(TransportError);
	});

	it.each([
		{},
		{ actor: {}, csrfToken: "csrf" },
		{ actor: { memberId: "", roles: ["requester"] }, csrfToken: "csrf" },
		{ actor: { memberId: "member-1", roles: "requester" }, csrfToken: "csrf" },
		{ actor: { memberId: "member-1", roles: ["owner"] }, csrfToken: "csrf" },
		{ actor: { memberId: "member-1", roles: ["requester"] }, csrfToken: "" },
	])("rejects malformed Session 200 as transport failure: %j", async (body) => {
		const { client } = clientWith(json(body));
		await expect(client.getSession()).rejects.toBeInstanceOf(TransportError);
	});

	it.each([
		["listPending", {}],
		["listPending", { requests: null }],
		["listAuditEvents", {}],
		["listAuditEvents", { events: null }],
	] as const)(
		"%s rejects malformed list envelope %j as transport failure",
		async (method, body) => {
			const { client } = clientWith(json(body));
			const result =
				method === "listPending"
					? client.listPending()
					: client.listAuditEvents("req-1");
			await expect(result).rejects.toBeInstanceOf(TransportError);
		},
	);

	it("propagates a fetch rejection without retrying", async () => {
		const failure = new TypeError("offline");
		const fetchFn = vi.fn<typeof fetch>().mockRejectedValue(failure);
		const client = createApiClient(origin, fetchFn);
		await expect(client.submitRequest("req-1", 1, "csrf")).rejects.toBe(
			failure,
		);
		expect(fetchFn).toHaveBeenCalledTimes(1);
	});
});
