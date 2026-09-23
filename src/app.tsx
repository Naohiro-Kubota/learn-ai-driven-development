import {
	useCallback,
	useEffect,
	useRef,
	useState,
	type ReactElement,
} from "react";
import { ApiError, type ApiClient } from "./api/client";
import type { Session } from "./api/types";
import { ErrorNotice, type Notice } from "./components/error-notice";
import { RequestWorkspace } from "./components/request-workspace";
import { OrganizationSelection } from "./components/organization-selection";
import { SignIn } from "./components/sign-in";

function currentRequestId(): string | null {
	const id = new URLSearchParams(window.location.search).get("requestId");
	return id && id !== "." && id !== ".." ? id : null;
}

type SessionState =
	| { kind: "loading" }
	| { kind: "authenticated"; session: Session }
	| { kind: "unauthenticated" }
	| { kind: "error" };

export function App({
	client,
	login,
}: {
	client: ApiClient;
	login: () => void;
}): ReactElement {
	const [sessionState, setSessionState] = useState<SessionState>({
		kind: "loading",
	});
	const [selectedRequestId, setSelectedRequestId] = useState(currentRequestId);
	const [notice, setNotice] = useState<Notice | null>(null);
	const [logoutPending, setLogoutPending] = useState(false);
	const sessionGeneration = useRef(0);
	const authenticated = useRef(false);
	const logoutInFlight = useRef(false);
	const onAuthenticationRequired = useCallback(() => {
		sessionGeneration.current++;
		authenticated.current = false;
		setSelectedRequestId(null);
		setNotice(null);
		setSessionState({ kind: "unauthenticated" });
	}, []);
	const onSessionChange = useCallback((session: Session) => {
		if (authenticated.current)
			setSessionState({ kind: "authenticated", session });
	}, []);
	const isSelectionPath =
		window.location.pathname === "/organization-selection";
	const onRequestIdChange = useCallback((id: string | null) => {
		const validId = id && id !== "." && id !== ".." ? id : null;
		const url = new URL(window.location.href);
		if (validId) url.searchParams.set("requestId", validId);
		else url.searchParams.delete("requestId");
		window.history.pushState(null, "", url);
		setSelectedRequestId(validId);
	}, []);
	const loadSession = useCallback(() => {
		const id = ++sessionGeneration.current;
		authenticated.current = false;
		setSessionState({ kind: "loading" });
		client.getSession().then(
			(session) => {
				if (sessionGeneration.current === id) {
					authenticated.current = true;
					setSessionState({ kind: "authenticated", session });
				}
			},
			(error: unknown) => {
				if (sessionGeneration.current !== id) return;
				setSessionState(
					error instanceof ApiError &&
						error.body.code === "authentication_required"
						? { kind: "unauthenticated" }
						: { kind: "error" },
				);
			},
		);
	}, [client]);

	useEffect(() => {
		if (isSelectionPath) return;
		loadSession();
		return () => {
			sessionGeneration.current += 1;
		};
	}, [isSelectionPath, loadSession]);

	useEffect(() => {
		const onPopState = () => setSelectedRequestId(currentRequestId());
		window.addEventListener("popstate", onPopState);
		return () => window.removeEventListener("popstate", onPopState);
	}, []);

	if (isSelectionPath)
		return (
			<OrganizationSelection
				client={client}
				login={login}
				onSelected={() => window.location.assign("/")}
			/>
		);

	if (sessionState.kind === "loading") return <main>Loading…</main>;
	if (sessionState.kind === "unauthenticated") return <SignIn login={login} />;
	if (sessionState.kind === "error") {
		return (
			<main>
				<p>Could not load your session.</p>
				<button type="button" onClick={loadSession}>
					Retry
				</button>
			</main>
		);
	}
	const logout = () => {
		if (logoutInFlight.current) return;
		logoutInFlight.current = true;
		setLogoutPending(true);
		client
			.logout(sessionState.session.csrfToken)
			.then(
				() => onAuthenticationRequired(),
				async (error: unknown) => {
					if (
						error instanceof ApiError &&
						error.body.code === "authentication_required"
					) {
						onAuthenticationRequired();
						return;
					}
					if (
						error instanceof ApiError &&
						error.body.code === "csrf_validation_failed"
					) {
						try {
							const refreshed = await client.getSession();
							onSessionChange(refreshed);
							setNotice({
								code: "csrf_validation_failed",
								text: "Session token refreshed. Click Log out again if you still want to sign out.",
							});
						} catch (refreshError) {
							if (
								refreshError instanceof ApiError &&
								refreshError.body.code === "authentication_required"
							)
								onAuthenticationRequired();
							else
								setNotice({
									code:
										refreshError instanceof ApiError
											? refreshError.body.code
											: "transport_failure",
									text: "Could not refresh your session. Retry Sign in or Log out after checking your connection.",
								});
						}
						return;
					}
					setNotice({
						code:
							error instanceof ApiError ? error.body.code : "transport_failure",
						text: "Could not confirm logout. Check your session before trying again.",
					});
				},
			)
			.finally(() => {
				logoutInFlight.current = false;
				setLogoutPending(false);
			});
	};
	return (
		<main>
			<h1>Signed in</h1>
			<button type="button" onClick={logout} disabled={logoutPending}>
				Log out
			</button>
			{notice && <ErrorNotice notice={notice} />}
			<RequestWorkspace
				client={client}
				session={sessionState.session}
				requestId={selectedRequestId}
				onRequestIdChange={onRequestIdChange}
				onSessionChange={onSessionChange}
				onAuthenticationRequired={onAuthenticationRequired}
				onNotice={setNotice}
			/>
		</main>
	);
}
