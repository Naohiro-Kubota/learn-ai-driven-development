import {
	act,
	cleanup,
	fireEvent,
	render,
	screen,
	waitFor,
	within,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, type ApiClient } from "../api/client";
import type { AuditEvent, Request, Session } from "../api/types";
import { RequestWorkspace } from "./request-workspace";

afterEach(cleanup);

const session: Session = {
	actor: { memberId: "owner", roles: ["requester"] },
	csrfToken: "csrf",
};
const draft: Request = {
	id: "request-1",
	title: "VPN access",
	description: "",
	status: "draft",
	version: 3,
	requesterMemberId: "owner",
	approval: null,
	createdAt: "2026-09-23T00:00:00Z",
	updatedAt: "2026-09-23T00:00:00Z",
};
const audit: AuditEvent = {
	id: "event-1",
	type: "request_created",
	occurredAt: "2026-09-23T00:00:00Z",
	actorMemberId: "owner",
	requestContent: { title: "VPN access", description: "" },
	approvalAssigneeMemberId: null,
	approvalId: null,
};
function client(overrides: Partial<ApiClient> = {}): ApiClient {
	return {
		listPending: vi.fn().mockResolvedValue([]),
		getSession: vi.fn().mockResolvedValue(session),
		submitRequest: vi
			.fn()
			.mockResolvedValue({ ...draft, status: "pending", version: 4 }),
		approveRequest: vi
			.fn()
			.mockResolvedValue({ ...draft, status: "approved", version: 4 }),
		getRequest: vi.fn().mockResolvedValue(draft),
		listAuditEvents: vi.fn().mockResolvedValue([audit]),
		createRequest: vi.fn().mockResolvedValue(draft),
		updateRequest: vi.fn().mockResolvedValue({ ...draft, version: 4 }),
		...overrides,
	} as unknown as ApiClient;
}
function mount(
	api: ApiClient,
	requestId: string | null = null,
	actor = session,
) {
	const onRequestIdChange = vi.fn();
	const onNotice = vi.fn();
	const view = render(
		<RequestWorkspace
			client={api}
			session={actor}
			requestId={requestId}
			onRequestIdChange={onRequestIdChange}
			onSessionChange={vi.fn()}
			onAuthenticationRequired={vi.fn()}
			onNotice={onNotice}
		/>,
	);
	return { ...view, onRequestIdChange, onNotice };
}

describe("RequestWorkspace", () => {
	it("reports authentication_required only once when both reads return 401", async () => {
		const unauthorized = new ApiError(401, {
			code: "authentication_required",
			message: "expired",
		});
		const onAuthenticationRequired = vi.fn();
		const api = client({
			getRequest: vi.fn().mockRejectedValue(unauthorized),
			listAuditEvents: vi.fn().mockRejectedValue(unauthorized),
		});
		render(
			<RequestWorkspace
				client={api}
				session={session}
				requestId={draft.id}
				onRequestIdChange={vi.fn()}
				onSessionChange={vi.fn()}
				onAuthenticationRequired={onAuthenticationRequired}
				onNotice={vi.fn()}
			/>,
		);
		await waitFor(() =>
			expect(onAuthenticationRequired).toHaveBeenCalledOnce(),
		);
		expect(screen.queryByText("Could not load request.")).toBeNull();
	});

	it("ignores a late 401 from a previously selected Request", async () => {
		let rejectOld!: (reason: unknown) => void;
		const oldRead = new Promise<Request>((_resolve, reject) => {
			rejectOld = reject;
		});
		const next = { ...draft, id: "next", title: "Next request" };
		const api = client({
			getRequest: vi
				.fn()
				.mockReturnValueOnce(oldRead)
				.mockResolvedValueOnce(next),
		});
		const onAuthenticationRequired = vi.fn();
		const props = {
			client: api,
			session,
			onRequestIdChange: vi.fn(),
			onSessionChange: vi.fn(),
			onAuthenticationRequired,
			onNotice: vi.fn(),
		};
		const { rerender } = render(
			<RequestWorkspace {...props} requestId="old" />,
		);
		rerender(<RequestWorkspace {...props} requestId="next" />);
		await screen.findByRole("heading", { name: "Next request" });
		await act(async () =>
			rejectOld(
				new ApiError(401, {
					code: "authentication_required",
					message: "expired",
				}),
			),
		);
		expect(onAuthenticationRequired).not.toHaveBeenCalled();
		expect(
			screen.getByRole("heading", { name: "Next request" }),
		).toBeInTheDocument();
	});
	it.each([
		["title", "😀".repeat(61), ""],
		["description", "Valid", "😀".repeat(1001)],
	] as const)(
		"accepts %s by Unicode codepoint count",
		async (_field, title, description) => {
			const api = client();
			mount(api);
			fireEvent.change(screen.getByLabelText("Title"), {
				target: { value: ` ${title} ` },
			});
			fireEvent.change(screen.getByLabelText("Description"), {
				target: { value: description },
			});
			fireEvent.click(screen.getByRole("button", { name: "Create Draft" }));
			await waitFor(() =>
				expect(api.createRequest).toHaveBeenCalledWith(
					{ title, description },
					"csrf",
				),
			);
		},
	);

	it.each(["request", "audit"] as const)(
		"prioritizes Audit/Request 401 after the other %s read fails",
		async (firstFailure) => {
			const unauthenticated = new ApiError(401, {
				code: "authentication_required",
				message: "expired",
			});
			let failRequest!: (reason: unknown) => void;
			let failAudit!: (reason: unknown) => void;
			const requestRead = new Promise<Request>((_resolve, reject) => {
				failRequest = reject;
			});
			const auditRead = new Promise<AuditEvent[]>((_resolve, reject) => {
				failAudit = reject;
			});
			const onAuthenticationRequired = vi.fn();
			const api = client({
				getRequest: vi.fn().mockReturnValue(requestRead),
				listAuditEvents: vi.fn().mockReturnValue(auditRead),
			});
			render(
				<RequestWorkspace
					client={api}
					session={session}
					requestId={draft.id}
					onRequestIdChange={vi.fn()}
					onSessionChange={vi.fn()}
					onAuthenticationRequired={onAuthenticationRequired}
					onNotice={vi.fn()}
				/>,
			);
			await act(async () => {
				if (firstFailure === "request") failRequest(new Error("offline"));
				else failAudit(new Error("offline"));
			});
			await act(async () => {
				if (firstFailure === "request") failAudit(unauthenticated);
				else failRequest(unauthenticated);
			});
			await waitFor(() =>
				expect(onAuthenticationRequired).toHaveBeenCalledOnce(),
			);
			expect(screen.queryByText("Could not load request.")).toBeNull();
		},
	);

	it("keeps access to a created Draft when selection changes during Create", async () => {
		let finish!: (request: Request) => void;
		const createRequest = vi.fn().mockReturnValue(
			new Promise<Request>((resolve) => {
				finish = resolve;
			}),
		);
		const api = client({
			createRequest,
			getRequest: vi
				.fn()
				.mockImplementation((id: string) =>
					Promise.resolve({ ...draft, id, title: id }),
				),
		});
		const onRequestIdChange = vi.fn();
		const props = {
			client: api,
			session,
			onRequestIdChange,
			onSessionChange: vi.fn(),
			onAuthenticationRequired: vi.fn(),
			onNotice: vi.fn(),
		};
		const { rerender } = render(
			<RequestWorkspace {...props} requestId="first" />,
		);
		await screen.findByRole("heading", { name: "first" });
		fireEvent.change(
			within(
				screen.getByRole("region", { name: "Create request" }),
			).getByLabelText("Title"),
			{ target: { value: "New draft" } },
		);
		fireEvent.click(screen.getByRole("button", { name: "Create Draft" }));
		rerender(<RequestWorkspace {...props} requestId="second" />);
		await screen.findByRole("heading", { name: "second" });
		await act(async () =>
			finish({ ...draft, id: "created", title: "New draft" }),
		);
		expect(onRequestIdChange).not.toHaveBeenCalled();
		fireEvent.click(screen.getByRole("button", { name: /Open created Draft/ }));
		expect(onRequestIdChange).toHaveBeenCalledWith("created");
	});
	it.each(["create", "update"] as const)(
		"propagates stale %s authentication_required after selection changes",
		async (kind) => {
			let rejectMutation!: (reason: unknown) => void;
			const delayed = new Promise<Request>((_resolve, reject) => {
				rejectMutation = reject;
			});
			const another = { ...draft, id: "request-2", title: "Other request" };
			const api = client({
				getRequest: vi
					.fn()
					.mockResolvedValueOnce(draft)
					.mockResolvedValueOnce(another),
				[kind === "create" ? "createRequest" : "updateRequest"]: vi
					.fn()
					.mockReturnValue(delayed),
			});
			const onAuthenticationRequired = vi.fn();
			const props = {
				client: api,
				session,
				onRequestIdChange: vi.fn(),
				onSessionChange: vi.fn(),
				onAuthenticationRequired,
				onNotice: vi.fn(),
			};
			const { rerender } = render(
				<RequestWorkspace {...props} requestId={draft.id} />,
			);
			if (kind === "create") {
				fireEvent.change(
					within(
						screen.getByRole("region", { name: "Create request" }),
					).getByLabelText("Title"),
					{ target: { value: "New" } },
				);
				fireEvent.click(screen.getByRole("button", { name: "Create Draft" }));
			} else
				fireEvent.click(
					await screen.findByRole("button", { name: "Update Draft" }),
				);
			rerender(<RequestWorkspace {...props} requestId={another.id} />);
			await screen.findByRole("heading", { name: another.title });
			await act(async () =>
				rejectMutation(
					new ApiError(401, {
						code: "authentication_required",
						message: "expired",
					}),
				),
			);
			expect(onAuthenticationRequired).toHaveBeenCalledOnce();
			expect(
				screen.queryByRole("region", { name: "Request detail" }),
			).toBeNull();
		},
	);
	it.each(["submit", "update"] as const)(
		"ignores a stale %s failure after selecting another Request",
		async (kind) => {
			let rejectMutation!: (reason: unknown) => void;
			const delayed = new Promise<Request>((_resolve, reject) => {
				rejectMutation = reject;
			});
			const another = { ...draft, id: "request-2", title: "Other request" };
			const getRequest = vi
				.fn()
				.mockResolvedValueOnce(draft)
				.mockResolvedValueOnce(another);
			const api = client({
				getRequest,
				[kind === "submit" ? "submitRequest" : "updateRequest"]: vi
					.fn()
					.mockReturnValue(delayed),
			});
			const onNotice = vi.fn();
			const onAuthenticationRequired = vi.fn();
			const props = {
				client: api,
				session,
				onRequestIdChange: vi.fn(),
				onSessionChange: vi.fn(),
				onAuthenticationRequired,
				onNotice,
			};
			const { rerender } = render(
				<RequestWorkspace {...props} requestId={draft.id} />,
			);
			fireEvent.click(
				await screen.findByRole("button", {
					name: kind === "submit" ? "Submit" : "Update Draft",
				}),
			);
			rerender(<RequestWorkspace {...props} requestId={another.id} />);
			await screen.findByRole("heading", { name: another.title });
			await act(async () =>
				rejectMutation(
					new ApiError(409, { code: "version_conflict", message: "stale" }),
				),
			);
			expect(
				screen.getByRole("heading", { name: another.title }),
			).toBeInTheDocument();
			expect(getRequest).toHaveBeenCalledTimes(2);
			expect(onNotice).not.toHaveBeenCalled();
		},
	);

	it.each(["submit", "update"] as const)(
		"clears authentication after a post-%s Audit 401",
		async (kind) => {
			const api = client({
				listAuditEvents: vi
					.fn()
					.mockResolvedValueOnce([audit])
					.mockRejectedValueOnce(
						new ApiError(401, {
							code: "authentication_required",
							message: "expired",
						}),
					),
			});
			const onAuthenticationRequired = vi.fn();
			render(
				<RequestWorkspace
					client={api}
					session={session}
					requestId={draft.id}
					onRequestIdChange={vi.fn()}
					onSessionChange={vi.fn()}
					onAuthenticationRequired={onAuthenticationRequired}
					onNotice={vi.fn()}
				/>,
			);
			fireEvent.click(
				await screen.findByRole("button", {
					name: kind === "submit" ? "Submit" : "Update Draft",
				}),
			);
			await waitFor(() =>
				expect(onAuthenticationRequired).toHaveBeenCalledOnce(),
			);
			expect(
				screen.queryByRole("region", { name: "Request detail" }),
			).toBeNull();
		},
	);
	it("loads only an Approver's pending queue and selects its opaque ID", async () => {
		const api = client({
			listPending: vi
				.fn()
				.mockResolvedValue([{ ...draft, id: "a/b", status: "pending" }]),
		});
		const { onRequestIdChange } = mount(api, null, {
			actor: { memberId: "assignee", roles: ["approver"] },
			csrfToken: "csrf",
		});
		fireEvent.click(await screen.findByRole("button", { name: /VPN access/ }));
		expect(api.listPending).toHaveBeenCalledOnce();
		expect(onRequestIdChange).toHaveBeenCalledWith("a/b");
	});

	it("shows an empty Approver queue without loading it for a Requester", async () => {
		const api = client();
		const { rerender } = mount(api);
		expect(api.listPending).not.toHaveBeenCalled();
		rerender(
			<RequestWorkspace
				client={api}
				session={{
					actor: { memberId: "assignee", roles: ["approver"] },
					csrfToken: "csrf",
				}}
				requestId={null}
				onRequestIdChange={vi.fn()}
				onSessionChange={vi.fn()}
				onAuthenticationRequired={vi.fn()}
				onNotice={vi.fn()}
			/>,
		);
		expect(await screen.findByText("No pending requests.")).toBeInTheDocument();
		expect(api.listPending).toHaveBeenCalledOnce();
	});

	it("submits the displayed version and refreshes audit", async () => {
		const api = client();
		mount(api, draft.id);
		fireEvent.click(await screen.findByRole("button", { name: "Submit" }));
		await waitFor(() =>
			expect(api.submitRequest).toHaveBeenCalledWith(draft.id, 3, "csrf"),
		);
		await waitFor(() => expect(api.listAuditEvents).toHaveBeenCalledTimes(2));
		expect(screen.getByText("Status: pending")).toBeInTheDocument();
	});

	it("approves an assigned Pending request and refreshes audit and queue", async () => {
		const pending = {
			...draft,
			status: "pending" as const,
			approval: {
				id: "approval-1",
				assigneeMemberId: "assignee",
				status: "pending" as const,
				approvedAt: null,
			},
		};
		const api = client({
			getRequest: vi.fn().mockResolvedValue(pending),
			approveRequest: vi
				.fn()
				.mockResolvedValue({ ...pending, status: "approved", version: 4 }),
		});
		mount(api, draft.id, {
			actor: { memberId: "assignee", roles: ["approver"] },
			csrfToken: "csrf",
		});
		fireEvent.click(await screen.findByRole("button", { name: "Approve" }));
		await waitFor(() =>
			expect(api.approveRequest).toHaveBeenCalledWith(draft.id, 3, "csrf"),
		);
		await waitFor(() => expect(api.listAuditEvents).toHaveBeenCalledTimes(2));
		expect(api.listPending).toHaveBeenCalledTimes(2);
	});

	it("prevents a repeated click while a mutation is in flight", async () => {
		let complete!: (request: Request) => void;
		const api = client({
			submitRequest: vi.fn().mockReturnValue(
				new Promise<Request>((resolve) => {
					complete = resolve;
				}),
			),
		});
		mount(api, draft.id);
		const submit = await screen.findByRole("button", { name: "Submit" });
		fireEvent.click(submit);
		fireEvent.click(submit);
		expect(api.submitRequest).toHaveBeenCalledTimes(1);
		await act(async () =>
			complete({ ...draft, status: "pending", version: 4 }),
		);
	});

	it("clears workspace data after a mutation requires authentication", async () => {
		const api = client({
			submitRequest: vi.fn().mockRejectedValue(
				new ApiError(401, {
					code: "authentication_required",
					message: "expired",
				}),
			),
		});
		const onAuthenticationRequired = vi.fn();
		render(
			<RequestWorkspace
				client={api}
				session={session}
				requestId={draft.id}
				onRequestIdChange={vi.fn()}
				onSessionChange={vi.fn()}
				onAuthenticationRequired={onAuthenticationRequired}
				onNotice={vi.fn()}
			/>,
		);
		fireEvent.click(await screen.findByRole("button", { name: "Submit" }));
		await waitFor(() =>
			expect(onAuthenticationRequired).toHaveBeenCalledOnce(),
		);
		expect(screen.queryByRole("region", { name: "Request detail" })).toBeNull();
	});

	it("offers an explicit read after an uncertain transition result", async () => {
		const api = client({
			submitRequest: vi.fn().mockRejectedValue(new Error("offline")),
		});
		const { onNotice } = mount(api, draft.id);
		fireEvent.click(await screen.findByRole("button", { name: "Submit" }));
		await waitFor(() => expect(onNotice).toHaveBeenCalled());
		expect(api.submitRequest).toHaveBeenCalledTimes(1);
		expect(onNotice.mock.lastCall?.[0].text).toMatch(
			/refresh request details/i,
		);
		fireEvent.click(screen.getByRole("button", { name: "Refresh request" }));
		await waitFor(() => expect(api.getRequest).toHaveBeenCalledTimes(2));
	});

	it.each(["version_conflict", "invalid_state"] as const)(
		"refreshes reads once after %s without replay",
		async (code) => {
			const api = client({
				submitRequest: vi
					.fn()
					.mockRejectedValue(new ApiError(409, { code, message: "stale" })),
			});
			const { onNotice } = mount(api, draft.id);
			fireEvent.click(await screen.findByRole("button", { name: "Submit" }));
			await waitFor(() => expect(api.getRequest).toHaveBeenCalledTimes(2));
			expect(api.listAuditEvents).toHaveBeenCalledTimes(2);
			expect(api.submitRequest).toHaveBeenCalledTimes(1);
			expect(onNotice.mock.lastCall?.[0].text).toMatch(/changed|refresh/i);
		},
	);

	it("refreshes CSRF once and waits for another click", async () => {
		const rotated = { ...session, csrfToken: "rotated" };
		const api = client({
			submitRequest: vi
				.fn()
				.mockRejectedValueOnce(
					new ApiError(403, {
						code: "csrf_validation_failed",
						message: "stale",
					}),
				)
				.mockResolvedValueOnce({ ...draft, status: "pending" }),
			getSession: vi.fn().mockResolvedValue(rotated),
		});
		const onSessionChange = vi.fn();
		const { rerender } = render(
			<RequestWorkspace
				client={api}
				session={session}
				requestId={draft.id}
				onRequestIdChange={vi.fn()}
				onSessionChange={onSessionChange}
				onAuthenticationRequired={vi.fn()}
				onNotice={vi.fn()}
			/>,
		);
		fireEvent.click(await screen.findByRole("button", { name: "Submit" }));
		await waitFor(() => expect(onSessionChange).toHaveBeenCalledWith(rotated));
		expect(api.submitRequest).toHaveBeenCalledTimes(1);
		rerender(
			<RequestWorkspace
				client={api}
				session={rotated}
				requestId={draft.id}
				onRequestIdChange={vi.fn()}
				onSessionChange={onSessionChange}
				onAuthenticationRequired={vi.fn()}
				onNotice={vi.fn()}
			/>,
		);
		fireEvent.click(await screen.findByRole("button", { name: "Submit" }));
		await waitFor(() =>
			expect(api.submitRequest).toHaveBeenLastCalledWith(
				draft.id,
				3,
				"rotated",
			),
		);
	});

	it("offers a session read retry when CSRF refresh fails", async () => {
		const api = client({
			submitRequest: vi.fn().mockRejectedValue(
				new ApiError(403, {
					code: "csrf_validation_failed",
					message: "stale",
				}),
			),
			getSession: vi
				.fn()
				.mockRejectedValueOnce(new Error("offline"))
				.mockResolvedValueOnce({ ...session, csrfToken: "rotated" }),
		});
		const onSessionChange = vi.fn();
		render(
			<RequestWorkspace
				client={api}
				session={session}
				requestId={draft.id}
				onRequestIdChange={vi.fn()}
				onSessionChange={onSessionChange}
				onAuthenticationRequired={vi.fn()}
				onNotice={vi.fn()}
			/>,
		);
		fireEvent.click(await screen.findByRole("button", { name: "Submit" }));
		fireEvent.click(
			await screen.findByRole("button", { name: "Retry session read" }),
		);
		await waitFor(() =>
			expect(onSessionChange).toHaveBeenCalledWith({
				...session,
				csrfToken: "rotated",
			}),
		);
		expect(api.submitRequest).toHaveBeenCalledTimes(1);
	});
	it("ignores an old read after the selected ID changes", async () => {
		let resolveOld!: (request: Request) => void;
		const old = new Promise<Request>((resolve) => {
			resolveOld = resolve;
		});
		const api = client({
			getRequest: vi
				.fn()
				.mockReturnValueOnce(old)
				.mockResolvedValueOnce({ ...draft, id: "new", title: "New request" }),
		});
		const { rerender } = mount(api, "old");
		rerender(
			<RequestWorkspace
				client={api}
				session={session}
				requestId="new"
				onRequestIdChange={vi.fn()}
				onSessionChange={vi.fn()}
				onAuthenticationRequired={vi.fn()}
				onNotice={vi.fn()}
			/>,
		);
		expect(
			await within(
				await screen.findByRole("region", { name: "Request detail" }),
			).findByRole("heading", { name: "New request" }),
		).toBeInTheDocument();
		resolveOld({ ...draft, id: "old", title: "Old request" });
		await waitFor(() =>
			expect(screen.queryByRole("heading", { name: "Old request" })).toBeNull(),
		);
	});
	it("creates a Draft with a trimmed title and intentional empty description", async () => {
		const api = client();
		const { onRequestIdChange, rerender } = mount(api);
		fireEvent.change(screen.getByLabelText("Title"), {
			target: { value: "  VPN access  " },
		});
		fireEvent.click(screen.getByRole("button", { name: "Create Draft" }));
		await waitFor(() =>
			expect(api.createRequest).toHaveBeenCalledWith(
				{ title: "VPN access", description: "" },
				"csrf",
			),
		);
		expect(onRequestIdChange).toHaveBeenCalledWith("request-1");
		rerender(
			<RequestWorkspace
				client={api}
				session={session}
				requestId="request-1"
				onRequestIdChange={onRequestIdChange}
				onSessionChange={vi.fn()}
				onAuthenticationRequired={vi.fn()}
				onNotice={vi.fn()}
			/>,
		);
		await waitFor(() =>
			expect(api.getRequest).toHaveBeenCalledWith("request-1"),
		);
		expect(api.listAuditEvents).toHaveBeenCalledWith("request-1");
		expect(
			within(screen.getByRole("region", { name: "Audit history" })).getByText(
				"Empty description",
			),
		).toBeInTheDocument();
	});

	it("rejects invalid content locally and maps server field errors without clearing edits", async () => {
		const api = client({
			createRequest: vi.fn().mockRejectedValue(
				new ApiError(400, {
					code: "invalid_request",
					message: "opaque",
					fieldErrors: [
						{
							field: "title",
							code: "invalid",
							message: "Server title error",
						},
					],
				}),
			),
		});
		mount(api);
		fireEvent.click(screen.getByRole("button", { name: "Create Draft" }));
		expect(api.createRequest).not.toHaveBeenCalled();
		expect(screen.getByText(/Title is required/)).toBeInTheDocument();
		fireEvent.change(screen.getByLabelText("Title"), {
			target: { value: "x".repeat(121) },
		});
		fireEvent.click(screen.getByRole("button", { name: "Create Draft" }));
		expect(api.createRequest).not.toHaveBeenCalled();
		fireEvent.change(screen.getByLabelText("Title"), {
			target: { value: "Valid" },
		});
		fireEvent.change(screen.getByLabelText("Description"), {
			target: { value: "x".repeat(2001) },
		});
		fireEvent.click(screen.getByRole("button", { name: "Create Draft" }));
		expect(api.createRequest).not.toHaveBeenCalled();
		fireEvent.change(screen.getByLabelText("Description"), {
			target: { value: "kept" },
		});
		fireEvent.click(screen.getByRole("button", { name: "Create Draft" }));
		expect(await screen.findByText("Server title error")).toBeInTheDocument();
		expect(screen.getByLabelText("Description")).toHaveValue("kept");
	});

	it("offers owned Draft update and submit, using its current version", async () => {
		const api = client();
		mount(api, draft.id);
		fireEvent.click(
			await screen.findByRole("button", { name: "Update Draft" }),
		);
		await waitFor(() =>
			expect(api.updateRequest).toHaveBeenCalledWith(
				"request-1",
				{ title: "VPN access", description: "", expectedVersion: 3 },
				"csrf",
			),
		);
		expect(screen.getByRole("button", { name: "Submit" })).toBeEnabled();
	});

	it("shows Approve only for the assigned Approver on Pending", async () => {
		const pending: Request = {
			...draft,
			status: "pending",
			approval: {
				id: "approval",
				assigneeMemberId: "assignee",
				status: "pending",
				approvedAt: null,
			},
		};
		const api = client({ getRequest: vi.fn().mockResolvedValue(pending) });
		const { rerender } = mount(api, draft.id, {
			actor: { memberId: "other", roles: ["approver"] },
			csrfToken: "csrf",
		});
		expect(
			await screen.findByRole("heading", { name: "VPN access" }),
		).toBeInTheDocument();
		expect(screen.queryByRole("button", { name: "Approve" })).toBeNull();
		rerender(
			<RequestWorkspace
				client={api}
				session={{
					actor: { memberId: "assignee", roles: ["approver"] },
					csrfToken: "csrf",
				}}
				requestId={draft.id}
				onRequestIdChange={vi.fn()}
				onSessionChange={vi.fn()}
				onAuthenticationRequired={vi.fn()}
				onNotice={vi.fn()}
			/>,
		);
		expect(
			await screen.findByRole("button", { name: "Approve" }),
		).toBeEnabled();
		expect(screen.queryByRole("button", { name: "Update Draft" })).toBeNull();
	});

	it("treats an uncertain create failure as unknown outcome", async () => {
		const api = client({
			createRequest: vi.fn().mockRejectedValue(new Error("connection lost")),
		});
		const { onNotice } = mount(api);
		fireEvent.change(screen.getByLabelText("Title"), {
			target: { value: "VPN" },
		});
		fireEvent.click(screen.getByRole("button", { name: "Create Draft" }));
		await waitFor(() => expect(onNotice).toHaveBeenCalled());
		expect(onNotice.mock.calls[0][0].text).toMatch(
			/outcome.*unknown|could not confirm/i,
		);
		expect(onNotice.mock.calls[0][0].text).not.toMatch(/try again/i);
		expect(screen.getByLabelText("Title")).toHaveValue("VPN");
	});

	it("offers a read retry when audit refresh fails after a successful update", async () => {
		const api = client({
			listAuditEvents: vi
				.fn()
				.mockResolvedValueOnce([audit])
				.mockRejectedValueOnce(new Error("offline"))
				.mockResolvedValueOnce([audit]),
		});
		const { onNotice } = mount(api, draft.id);
		fireEvent.click(
			await screen.findByRole("button", { name: "Update Draft" }),
		);
		fireEvent.click(await screen.findByRole("button", { name: "Retry read" }));
		await waitFor(() => expect(api.listAuditEvents).toHaveBeenCalledTimes(3));
		expect(onNotice).toHaveBeenCalled();
	});

	it("does not let a late post-update audit response overwrite a newer read of the same ID", async () => {
		let resolveAudit!: (events: AuditEvent[]) => void;
		const late = new Promise<AuditEvent[]>((resolve) => {
			resolveAudit = resolve;
		});
		const api = client({
			getRequest: vi.fn().mockResolvedValue(draft),
			listAuditEvents: vi
				.fn()
				.mockResolvedValueOnce([audit])
				.mockReturnValueOnce(late),
		});
		const { rerender } = mount(api, draft.id);
		fireEvent.click(
			await screen.findByRole("button", { name: "Update Draft" }),
		);
		await waitFor(() => expect(api.listAuditEvents).toHaveBeenCalledTimes(2));
		const fresh = { ...audit, id: "fresh", type: "request_updated" as const };
		const nextClient = client({
			listAuditEvents: vi.fn().mockResolvedValue([fresh]),
		});
		rerender(
			<RequestWorkspace
				client={nextClient}
				session={session}
				requestId={draft.id}
				onRequestIdChange={vi.fn()}
				onSessionChange={vi.fn()}
				onAuthenticationRequired={vi.fn()}
				onNotice={vi.fn()}
			/>,
		);
		expect(
			await screen.findByText(/request_updated by owner/),
		).toBeInTheDocument();
		await act(async () => resolveAudit([audit]));
		const history = screen.getByRole("region", { name: "Audit history" });
		expect(
			within(history).getByText(/request_updated by owner/),
		).toBeInTheDocument();
		expect(within(history).queryByText(/request_created by owner/)).toBeNull();
	});

	it.each(["resolve", "reject"] as const)(
		"ignores the older same-ID audit refresh when it later %s",
		async (settlement) => {
			let resolveOlder!: (events: AuditEvent[]) => void;
			let rejectOlder!: (reason: Error) => void;
			const older = new Promise<AuditEvent[]>((resolve, reject) => {
				resolveOlder = resolve;
				rejectOlder = reject;
			});
			const newer = {
				...audit,
				id: "newer",
				type: "request_updated" as const,
				requestContent: { title: "Newer audit", description: "" },
			};
			const olderEvent = {
				...audit,
				id: "older",
				requestContent: { title: "Older audit", description: "" },
			};
			const api = client({
				updateRequest: vi
					.fn()
					.mockResolvedValueOnce({ ...draft, version: 4 })
					.mockResolvedValueOnce({ ...draft, version: 5 }),
				listAuditEvents: vi
					.fn()
					.mockResolvedValueOnce([audit])
					.mockReturnValueOnce(older)
					.mockResolvedValueOnce([newer]),
			});
			const { onNotice } = mount(api, draft.id);
			fireEvent.click(
				await screen.findByRole("button", { name: "Update Draft" }),
			);
			await waitFor(() => expect(api.listAuditEvents).toHaveBeenCalledTimes(2));
			await waitFor(() =>
				expect(
					screen.getByRole("button", { name: "Save Draft" }),
				).not.toBeDisabled(),
			);
			fireEvent.click(screen.getByRole("button", { name: "Update Draft" }));
			await waitFor(() =>
				expect(api.updateRequest).toHaveBeenLastCalledWith(
					draft.id,
					{ title: draft.title, description: "", expectedVersion: 4 },
					"csrf",
				),
			);
			const history = await screen.findByRole("region", {
				name: "Audit history",
			});
			expect(
				await within(history).findByText("Newer audit"),
			).toBeInTheDocument();
			await act(async () => {
				if (settlement === "resolve") resolveOlder([olderEvent]);
				else rejectOlder(new Error("late failure"));
			});
			expect(within(history).getByText("Newer audit")).toBeInTheDocument();
			expect(within(history).queryByText("Older audit")).toBeNull();
			expect(screen.queryByRole("button", { name: "Retry read" })).toBeNull();
			expect(onNotice).not.toHaveBeenCalled();
		},
	);

	it("displays approval assignment identifiers from an audit event", async () => {
		const api = client({
			listAuditEvents: vi.fn().mockResolvedValue([
				{
					...audit,
					id: "event-2",
					type: "request_submitted",
					approvalAssigneeMemberId: "assignee-1",
					approvalId: "approval-1",
				},
			]),
		});
		mount(api, draft.id);
		const history = await screen.findByRole("region", {
			name: "Audit history",
		});
		expect(within(history).getByText(/assignee-1/)).toBeInTheDocument();
		expect(within(history).getByText(/approval-1/)).toBeInTheDocument();
	});

	it("keeps the ID and offers Retry after a failed read", async () => {
		const api = client({
			getRequest: vi
				.fn()
				.mockRejectedValueOnce(new Error("offline"))
				.mockResolvedValueOnce(draft),
		});
		mount(api, draft.id);
		fireEvent.click(await screen.findByRole("button", { name: "Retry" }));
		expect(
			await screen.findByRole("heading", { name: "VPN access" }),
		).toBeInTheDocument();
		expect(api.getRequest).toHaveBeenCalledTimes(2);
	});

	it("never requests dot segment IDs", () => {
		const api = client();
		const { rerender } = mount(api, ".");
		rerender(
			<RequestWorkspace
				client={api}
				session={session}
				requestId=".."
				onRequestIdChange={vi.fn()}
				onSessionChange={vi.fn()}
				onAuthenticationRequired={vi.fn()}
				onNotice={vi.fn()}
			/>,
		);
		expect(api.getRequest).not.toHaveBeenCalled();
	});
});
