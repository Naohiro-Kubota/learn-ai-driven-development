import { useEffect, useRef, useState, type ReactElement } from "react";
import { ApiError, type ApiClient } from "../api/client";
import type { OrganizationSelection as Selection } from "../api/types";
import { ErrorNotice } from "./error-notice";
import { SignIn } from "./sign-in";

type State =
	| { kind: "loading" }
	| { kind: "ready"; selection: Selection }
	| { kind: "pending"; selection: Selection }
	| { kind: "sign-in"; message: string };

export function OrganizationSelection({
	client,
	login,
	onSelected,
}: {
	client: ApiClient;
	login: () => void;
	onSelected: () => void;
}): ReactElement {
	const [state, setState] = useState<State>({ kind: "loading" });
	const attempted = useRef(false);
	useEffect(() => {
		let active = true;
		client.getOrganizationSelection().then(
			(selection) => {
				if (active) setState({ kind: "ready", selection });
			},
			() => {
				if (active)
					setState({
						kind: "sign-in",
						message:
							"Could not load organization choices. Please sign in again.",
					});
			},
		);
		return () => {
			active = false;
		};
	}, [client]);

	function select(memberId: string, selection: Selection): void {
		if (attempted.current) return;
		attempted.current = true;
		setState({ kind: "pending", selection });
		client.selectOrganization(memberId, selection.csrfToken).then(
			() => onSelected(),
			(error: unknown) => {
				const message =
					error instanceof ApiError &&
					(error.body.code === "invalid_auth_transaction" ||
						error.body.code === "csrf_validation_failed")
						? "Organization selection expired. Please sign in again."
						: "Could not select the organization. Please sign in again.";
				setState({ kind: "sign-in", message });
			},
		);
	}

	if (state.kind === "loading") return <main>Loading organizations…</main>;
	if (state.kind === "sign-in")
		return (
			<>
				<ErrorNotice
					notice={{ code: "invalid_auth_transaction", text: state.message }}
				/>
				<SignIn login={login} />
			</>
		);
	return (
		<main>
			<h1>Choose an organization</h1>
			{state.selection.candidates.map((candidate) => (
				<button
					key={candidate.memberId}
					type="button"
					value={candidate.memberId}
					disabled={state.kind === "pending"}
					onClick={() => select(candidate.memberId, state.selection)}
				>
					{candidate.organizationName}
				</button>
			))}
			{state.kind === "pending" && <p>Choosing organization…</p>}
		</main>
	);
}
