import {
	act,
	cleanup,
	fireEvent,
	render,
	screen,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StrictMode } from "react";
import { ApiError, type ApiClient } from "../api/client";
import { OrganizationSelection } from "./organization-selection";

afterEach(cleanup);

const choice = {
	candidates: [
		{
			memberId: "member-north",
			organizationId: "north",
			organizationName: "North Office",
		},
		{
			memberId: "member-south",
			organizationId: "south",
			organizationName: "South Office",
		},
	],
	csrfToken: "current-selection-csrf",
};

function clientWith(
	getOrganizationSelection: ApiClient["getOrganizationSelection"],
	selectOrganization: ApiClient["selectOrganization"],
): ApiClient {
	return { getOrganizationSelection, selectOrganization } as ApiClient;
}

function deferred<T>() {
	let resolve!: (value: T) => void;
	let reject!: (reason: unknown) => void;
	const promise = new Promise<T>((yes, no) => {
		resolve = yes;
		reject = no;
	});
	return { promise, resolve, reject };
}

describe("OrganizationSelection", () => {
	it("reads choices once in StrictMode and posts with that read's current token", async () => {
		const getOrganizationSelection = vi
			.fn()
			.mockResolvedValueOnce(choice)
			.mockResolvedValueOnce({ ...choice, csrfToken: "unexpected-rotation" });
		const selectOrganization = vi.fn().mockResolvedValue(undefined);
		render(
			<StrictMode>
				<OrganizationSelection
					client={clientWith(getOrganizationSelection, selectOrganization)}
					login={vi.fn()}
					onSelected={vi.fn()}
				/>
			</StrictMode>,
		);
		fireEvent.click(
			await screen.findByRole("button", { name: "North Office" }),
		);
		expect(getOrganizationSelection).toHaveBeenCalledOnce();
		expect(selectOrganization).toHaveBeenCalledWith(
			"member-north",
			"current-selection-csrf",
		);
	});

	it("shows candidate names without exposing identifiers or the selection token", async () => {
		const { container } = render(
			<OrganizationSelection
				client={clientWith(vi.fn().mockResolvedValue(choice), vi.fn())}
				login={vi.fn()}
				onSelected={vi.fn()}
			/>,
		);
		expect(
			await screen.findByRole("button", { name: "North Office" }),
		).toBeInTheDocument();
		expect(
			screen.getByRole("button", { name: "South Office" }),
		).toBeInTheDocument();
		expect(container.textContent).not.toContain("member-north");
		expect(container.textContent).not.toContain("current-selection-csrf");
	});

	it("disables selection while a POST is pending and invokes success callback once", async () => {
		const pending = deferred<void>();
		const selectOrganization = vi.fn().mockReturnValue(pending.promise);
		const onSelected = vi.fn();
		render(
			<OrganizationSelection
				client={clientWith(
					vi.fn().mockResolvedValue(choice),
					selectOrganization,
				)}
				login={vi.fn()}
				onSelected={onSelected}
			/>,
		);
		fireEvent.click(
			await screen.findByRole("button", { name: "North Office" }),
		);
		expect(screen.getByRole("button", { name: "North Office" })).toBeDisabled();
		expect(screen.getByRole("button", { name: "South Office" })).toBeDisabled();
		expect(selectOrganization).toHaveBeenCalledWith(
			"member-north",
			"current-selection-csrf",
		);
		await act(async () => pending.resolve());
		expect(onSelected).toHaveBeenCalledOnce();
	});

	it.each(["invalid_auth_transaction", "csrf_validation_failed"] as const)(
		"requires a fresh sign in after %s and never retries the consumed token",
		async (code) => {
			const selectOrganization = vi.fn().mockRejectedValue(
				new ApiError(code === "invalid_auth_transaction" ? 400 : 403, {
					code,
					message: "Failure",
				}),
			);
			const login = vi.fn();
			render(
				<OrganizationSelection
					client={clientWith(
						vi.fn().mockResolvedValue(choice),
						selectOrganization,
					)}
					login={login}
					onSelected={vi.fn()}
				/>,
			);
			fireEvent.click(
				await screen.findByRole("button", { name: "North Office" }),
			);
			fireEvent.click(await screen.findByRole("button", { name: "Sign in" }));
			expect(login).toHaveBeenCalledOnce();
			expect(screen.queryByRole("button", { name: "North Office" })).toBeNull();
			expect(selectOrganization).toHaveBeenCalledOnce();
		},
	);
});
