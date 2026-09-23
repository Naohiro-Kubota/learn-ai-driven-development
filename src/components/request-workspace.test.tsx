import {
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
	const view = render(
		<RequestWorkspace
			client={api}
			session={actor}
			requestId={requestId}
			onRequestIdChange={onRequestIdChange}
			onSessionChange={vi.fn()}
			onAuthenticationRequired={vi.fn()}
			onNotice={vi.fn()}
		/>,
	);
	return { ...view, onRequestIdChange };
}

describe("RequestWorkspace", () => {
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
		expect(screen.getByRole("button", { name: "Submit" })).toBeInTheDocument();
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
		).toBeInTheDocument();
		expect(screen.queryByRole("button", { name: "Update Draft" })).toBeNull();
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
