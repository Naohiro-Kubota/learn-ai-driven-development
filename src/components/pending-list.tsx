import type { ReactElement } from "react";
import type { Request } from "../api/types";

export function PendingList({
	requests,
	onSelect,
	loading,
	retry,
}: {
	requests: Request[];
	onSelect: (id: string) => void;
	loading: boolean;
	retry: () => void;
}): ReactElement {
	return (
		<section aria-label="Pending requests">
			<h2>Pending requests</h2>
			{loading ? (
				<p>Loading pending requests…</p>
			) : requests.length ? (
				<ul>
					{requests.map((request) => (
						<li key={request.id}>
							<button type="button" onClick={() => onSelect(request.id)}>
								{request.title}
							</button>
						</li>
					))}
				</ul>
			) : (
				<p>No pending requests.</p>
			)}
			<button type="button" onClick={retry}>
				Refresh pending
			</button>
		</section>
	);
}
