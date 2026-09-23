import {
	act,
	cleanup,
	fireEvent,
	render,
	screen,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StrictMode } from "react";
import { ApiError, type ApiClient } from "./api/client";
import type { Session } from "./api/types";
import { App } from "./app";

afterEach(() => {
	cleanup();
	window.history.replaceState(null, "", "/");
});

const session: Session = {
	actor: { memberId: "member-1", roles: ["requester"] },
	csrfToken: "secret-csrf-token",
};

function deferred<T>() {
	let resolve!: (value: T) => void;
	let reject!: (reason: unknown) => void;
	const promise = new Promise<T>((yes, no) => {
		resolve = yes;
		reject = no;
	});
	return { promise, resolve, reject };
}

function clientWith(getSession: ApiClient["getSession"]): ApiClient {
	return { getSession } as ApiClient;
}

describe("App session bootstrap", () => {
	it("reads an opaque URL ID and follows popstate without requesting dot segments", async () => {
		window.history.replaceState(null, "", "/?requestId=a%2Fb");
		const getRequest = vi.fn().mockResolvedValue({
			id: "a/b",
			title: "First",
			description: "",
			status: "approved",
			version: 1,
			requesterMemberId: "member-1",
			approval: null,
			createdAt: "2026-09-23T00:00:00Z",
			updatedAt: "2026-09-23T00:00:00Z",
		});
		const listAuditEvents = vi.fn().mockResolvedValue([]);
		render(
			<App
				client={
					{
						getSession: vi.fn().mockResolvedValue(session),
						getRequest,
						listAuditEvents,
					} as unknown as ApiClient
				}
				login={vi.fn()}
			/>,
		);
		await screen.findByRole("heading", { name: "First" });
		expect(getRequest).toHaveBeenCalledWith("a/b");
		window.history.replaceState(null, "", "/?requestId=prior");
		fireEvent.popState(window);
		await act(async () => {
			expect(getRequest).toHaveBeenCalledWith("prior");
		});
		window.history.replaceState(null, "", "/?requestId=..");
		fireEvent.popState(window);
		expect(getRequest).toHaveBeenCalledTimes(2);
	});
	it("loads organization choices on the selection path without bootstrapping a session", async () => {
		window.history.replaceState(null, "", "/organization-selection");
		const getSession = vi.fn();
		const getOrganizationSelection = vi.fn().mockResolvedValue({
			candidates: [
				{
					memberId: "member-north",
					organizationId: "north",
					organizationName: "North Office",
				},
			],
			csrfToken: "selection-secret",
		});
		render(
			<App
				client={
					{ getSession, getOrganizationSelection } as unknown as ApiClient
				}
				login={vi.fn()}
			/>,
		);
		expect(
			await screen.findByRole("button", { name: "North Office" }),
		).toBeInTheDocument();
		expect(getOrganizationSelection).toHaveBeenCalledOnce();
		expect(getSession).not.toHaveBeenCalled();
	});

	it("shows loading until the session resolves, then an authenticated shell", async () => {
		const pending = deferred<Session>();
		const getSession = vi.fn().mockReturnValue(pending.promise);
		const { container } = render(
			<App client={clientWith(getSession)} login={vi.fn()} />,
		);
		expect(screen.getByText("Loading…")).toBeInTheDocument();
		expect(getSession).toHaveBeenCalledOnce();
		await act(async () => pending.resolve(session));
		expect(screen.getByRole("main")).toBeInTheDocument();
		expect(screen.getByText("Signed in")).toBeInTheDocument();
		expect(container.textContent).not.toContain("secret-csrf-token");
		expect(container.textContent).not.toContain("member-1");
	});

	it("offers Sign in only for authentication_required and invokes login", async () => {
		const getSession = vi.fn().mockRejectedValue(
			new ApiError(401, {
				code: "authentication_required",
				message: "Authentication is required.",
			}),
		);
		const login = vi.fn();
		render(<App client={clientWith(getSession)} login={login} />);
		fireEvent.click(await screen.findByRole("button", { name: "Sign in" }));
		expect(login).toHaveBeenCalledOnce();
		expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
	});

	it.each([
		new ApiError(403, { code: "forbidden", message: "Forbidden" }),
		new ApiError(401, { code: "invalid_request", message: "Bad request" }),
		new Error("network down"),
	])("offers Retry for other failures (%s)", async (failure) => {
		const getSession = vi
			.fn()
			.mockRejectedValueOnce(failure)
			.mockResolvedValueOnce(session);
		render(<App client={clientWith(getSession)} login={vi.fn()} />);
		fireEvent.click(await screen.findByRole("button", { name: "Retry" }));
		await screen.findByText("Signed in");
		expect(getSession).toHaveBeenCalledTimes(2);
	});

	it("does not let an old client response replace a newer one", async () => {
		const old = deferred<Session>();
		const current = deferred<Session>();
		const first = clientWith(vi.fn().mockReturnValue(old.promise));
		const second = clientWith(vi.fn().mockReturnValue(current.promise));
		const { rerender } = render(<App client={first} login={vi.fn()} />);
		rerender(<App client={second} login={vi.fn()} />);
		await act(async () => current.reject(new Error("current failure")));
		expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
		await act(async () => old.resolve(session));
		expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
		expect(screen.queryByText("Signed in")).toBeNull();
	});

	it("ignores the first StrictMode effect response after the second effect fails", async () => {
		const stale = deferred<Session>();
		const current = deferred<Session>();
		const getSession = vi
			.fn()
			.mockReturnValueOnce(stale.promise)
			.mockReturnValueOnce(current.promise);
		render(
			<StrictMode>
				<App client={clientWith(getSession)} login={vi.fn()} />
			</StrictMode>,
		);
		expect(getSession).toHaveBeenCalledTimes(2);
		await act(async () => current.reject(new Error("current failure")));
		expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
		await act(async () => stale.resolve(session));
		expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
		expect(screen.queryByText("Signed in")).toBeNull();
	});
});
