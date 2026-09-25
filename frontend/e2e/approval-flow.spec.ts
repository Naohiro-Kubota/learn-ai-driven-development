import { expect, test, type Page, type Response } from "@playwright/test";

declare const process: { env: { E2E_TEST_PASSWORD?: string } };

const frontendOrigin = "http://127.0.0.1:5173";
const apiOrigin = "http://127.0.0.1:8080";

function watchErrors(page: Page): () => void {
	let errors = 0;
	page.on("pageerror", () => errors++);
	page.on("console", (message) => {
		if (message.type() !== "error") return;
		const location = message.location().url;
		const expectedSessionRead =
			location.startsWith(`${apiOrigin}/api/v1/session`) &&
			message.text().includes("401");
		if (!expectedSessionRead) errors++;
	});
	return () => expect(errors, "unexpected browser errors").toBe(0);
}

async function expectFrontendState(
	page: Page,
	pathname: string,
	expectedRequestId: string | null = null,
): Promise<void> {
	await expect.poll(() => new URL(page.url()).origin).toBe(frontendOrigin);
	await expect.poll(() => new URL(page.url()).pathname).toBe(pathname);
	const exposed = await page.evaluate((requestId) => {
		const url = new URL(window.location.href);
		const query = [...url.searchParams.entries()];
		return {
			queryMatches:
				requestId === null
					? query.length === 0
					: query.length === 1 &&
						query[0][0] === "requestId" &&
						query[0][1] === requestId,
			fragment: url.hash,
			localStorageLength: window.localStorage.length,
			sessionStorageLength: window.sessionStorage.length,
		};
	}, expectedRequestId);
	expect(exposed).toEqual({
		queryMatches: true,
		fragment: "",
		localStorageLength: 0,
		sessionStorageLength: 0,
	});
}

async function signIn(page: Page, username: string): Promise<void> {
	const password = process.env.E2E_TEST_PASSWORD;
	if (!password) throw new Error("E2E_TEST_PASSWORD is required");
	let callbackResponse: Response | undefined;
	page.on("response", (response) => {
		const url = new URL(response.url());
		if (url.origin === apiOrigin && url.pathname === "/auth/oidc/callback")
			callbackResponse = response;
	});
	await page.goto("/");
	await page.getByRole("button", { name: "Sign in" }).click();
	await expect
		.poll(() => new URL(page.url()).pathname)
		.toBe("/realms/approval-flow-dev/protocol/openid-connect/auth");
	await page.getByLabel(/username or email/i).fill(username);
	await page.getByLabel("Password", { exact: true }).fill(password);
	await page.getByRole("button", { name: /sign in/i }).click();
	try {
		await expect.poll(() => new URL(page.url()).origin).toBe(frontendOrigin);
	} catch {
		const location = new URL(page.url());
		const status = callbackResponse?.status() ?? null;
		let code = "unknown";
		if (callbackResponse) {
			try {
				const body: { code?: string } = await callbackResponse.json();
				if (
					new Set([
						"invalid_auth_transaction",
						"internal_error",
						"authentication_required",
						"forbidden",
						"csrf_validation_failed",
						"invalid_request",
					]).has(body.code ?? "")
				)
					code = body.code ?? "unknown";
			} catch {
				// Only the public error code is used in diagnostics.
			}
		}
		const cookie = callbackResponse
			? ((await callbackResponse.request().allHeaders()).cookie?.includes(
					"approval_flow_auth_transaction=",
				) ?? false)
			: false;
		console.log(
			`E2E_DIAGNOSTIC:${JSON.stringify({
				phase: "callback",
				status,
				code,
				cookiePresent: cookie,
				pathname:
					location.pathname === "/auth/oidc/callback"
						? location.pathname
						: null,
				testId:
					username === "multi" ? "organization_selection" : "approval_flow",
			})}`,
		);
		throw new Error("OIDC callback did not return to the frontend");
	}
}

async function memberId(page: Page): Promise<string> {
	const result = await page.evaluate(async (origin) => {
		const response = await fetch(`${origin}/api/v1/session`, {
			credentials: "include",
		});
		if (!response.ok) return null;
		const session: { actor?: { memberId?: string } } = await response.json();
		return session.actor?.memberId ?? null;
	}, apiOrigin);
	expect(result).toBeTruthy();
	return result as string;
}

test("requester and approver complete one request with separate sessions", async ({
	browser,
}) => {
	const requester = await browser.newContext();
	const approver = await browser.newContext();
	expect(requester).not.toBe(approver);
	try {
		const requesterPage = await requester.newPage();
		const checkRequesterErrors = watchErrors(requesterPage);
		await signIn(requesterPage, "requester");
		await expect(
			requesterPage.getByRole("heading", { name: "Signed in" }),
		).toBeVisible();
		await expectFrontendState(requesterPage, "/");
		await requesterPage
			.getByRole("region", { name: "Create request" })
			.getByLabel("Title")
			.fill("E2E approval request");
		await requesterPage.getByRole("button", { name: "Create Draft" }).click();
		await expect(requesterPage.getByText("Status: draft")).toBeVisible();
		const requestId = new URL(requesterPage.url()).searchParams.get(
			"requestId",
		);
		if (!requestId) throw new Error("New draft did not expose a Request ID");
		await expectFrontendState(requesterPage, "/", requestId);
		await requesterPage.getByRole("button", { name: "Submit" }).click();
		await expect(requesterPage.getByText("Status: pending")).toBeVisible();
		await expectFrontendState(requesterPage, "/", requestId);
		const requesterMemberId = await memberId(requesterPage);
		expect(requesterMemberId).toBe("member-requester-a");

		const approverPage = await approver.newPage();
		const checkApproverErrors = watchErrors(approverPage);
		await signIn(approverPage, "approver");
		await expect(
			approverPage.getByRole("heading", { name: "Signed in" }),
		).toBeVisible();
		await expectFrontendState(approverPage, "/");
		await approverPage
			.getByRole("region", { name: "Pending requests" })
			.getByRole("button", { name: "E2E approval request" })
			.click();
		await expect(approverPage.getByText("Status: pending")).toBeVisible();
		await expectFrontendState(approverPage, "/", requestId);
		await approverPage.getByRole("button", { name: "Approve" }).click();
		await expect(
			approverPage.getByRole("alert").getByText("Request approved."),
		).toBeVisible();
		await expect(
			approverPage.getByRole("region", { name: "Request detail" }),
		).toHaveCount(0);
		await expectFrontendState(approverPage, "/");
		const approvedRequestRead = await approver.request.get(
			`${apiOrigin}/api/v1/requests/${requestId}`,
		);
		const approvedAuditRead = await approver.request.get(
			`${apiOrigin}/api/v1/requests/${requestId}/audit-events`,
		);
		expect([approvedRequestRead.status(), approvedAuditRead.status()]).toEqual([
			404, 404,
		]);
		const approverMemberId = await memberId(approverPage);
		expect(approverMemberId).toBe("member-approver-a");
		expect(approverMemberId).not.toBe(requesterMemberId);

		await requesterPage
			.getByRole("button", { name: "Refresh request" })
			.click();
		await expect(requesterPage.getByText("Status: approved")).toBeVisible();
		await expect(
			requesterPage
				.getByRole("region", { name: "Audit history" })
				.getByText(/request_approved by member-approver-a/),
		).toBeVisible();
		await expectFrontendState(requesterPage, "/", requestId);
		await expectFrontendState(approverPage, "/");
		checkRequesterErrors();
		checkApproverErrors();
	} finally {
		await requester.close();
		await approver.close();
	}
});

test("multi membership selects only a presented organization", async ({
	browser,
}) => {
	const context = await browser.newContext();
	try {
		const page = await context.newPage();
		const checkErrors = watchErrors(page);
		await signIn(page, "multi");
		await expect(
			page.getByRole("heading", { name: "Choose an organization" }),
		).toBeVisible();
		await expectFrontendState(page, "/organization-selection");
		const choices = page.getByRole("button");
		await expect(choices).toHaveCount(2);
		await expect(choices).toHaveText(["Organization A", "Organization B"]);
		const candidates = await choices.evaluateAll((buttons) =>
			buttons.map((button) => (button as HTMLButtonElement).value),
		);
		expect(candidates).toEqual(["member-multi-a", "member-multi-b"]);
		const beforeSelection = await page.evaluate(
			async (origin) =>
				(await fetch(`${origin}/api/v1/session`, { credentials: "include" }))
					.status,
			apiOrigin,
		);
		expect(beforeSelection).toBe(401);
		await page.getByRole("button", { name: "Organization A" }).click();
		await expect(
			page.getByRole("heading", { name: "Signed in" }),
		).toBeVisible();
		await expectFrontendState(page, "/");
		expect(await memberId(page)).toBe("member-multi-a");
		checkErrors();
	} finally {
		await context.close();
	}
});

test("frontend URL check rejects an unrelated query value", async ({
	page,
}) => {
	await page.goto("/");
	await page.evaluate(() =>
		window.history.replaceState(null, "", "/?unexpected=opaque"),
	);
	await expect(expectFrontendState(page, "/")).rejects.toThrow();
});
