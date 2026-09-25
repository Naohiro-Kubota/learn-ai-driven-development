import type { ReactElement } from "react";

export function SignIn({ login }: { login: () => void }): ReactElement {
	return (
		<main>
			<p>Sign in to continue.</p>
			<button type="button" onClick={login}>
				Sign in
			</button>
		</main>
	);
}
