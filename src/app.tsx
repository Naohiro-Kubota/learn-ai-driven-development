import {
	useCallback,
	useEffect,
	useRef,
	useState,
	type ReactElement,
} from "react";
import { ApiError, type ApiClient } from "./api/client";
import type { Session } from "./api/types";

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
	const requestId = useRef(0);
	const loadSession = useCallback(() => {
		const id = ++requestId.current;
		setSessionState({ kind: "loading" });
		client.getSession().then(
			(session) => {
				if (requestId.current === id)
					setSessionState({ kind: "authenticated", session });
			},
			(error: unknown) => {
				if (requestId.current !== id) return;
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
		loadSession();
		return () => {
			requestId.current += 1;
		};
	}, [loadSession]);

	if (sessionState.kind === "loading") return <main>Loading…</main>;
	if (sessionState.kind === "unauthenticated") {
		return (
			<main>
				<button type="button" onClick={login}>
					Sign in
				</button>
			</main>
		);
	}
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
