import {
	useCallback,
	useEffect,
	useRef,
	useState,
	type ReactElement,
} from "react";
import { ApiError, type ApiClient } from "./api/client";
import type { Session } from "./api/types";
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
	const sessionGeneration = useRef(0);
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
	void selectedRequestId;
	void onRequestIdChange;
	const loadSession = useCallback(() => {
		const id = ++sessionGeneration.current;
		setSessionState({ kind: "loading" });
		client.getSession().then(
			(session) => {
				if (sessionGeneration.current === id)
					setSessionState({ kind: "authenticated", session });
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
	return (
		<main>
			<h1>Signed in</h1>
			<section aria-label="Workspace" />
		</main>
	);
}
