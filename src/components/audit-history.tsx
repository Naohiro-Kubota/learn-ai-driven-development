import type { ReactElement } from "react";
import type { AuditEvent } from "../api/types";

export function AuditHistory({
	events,
}: {
	events: AuditEvent[];
}): ReactElement {
	return (
		<section aria-label="Audit history">
			<h2>Audit history</h2>
			<ol>
				{events.map((event) => (
					<li key={event.id}>
						<p>
							{event.type} by {event.actorMemberId} at {event.occurredAt}
						</p>
						{event.requestContent && (
							<>
								<p>{event.requestContent.title}</p>
								<p>{event.requestContent.description || "Empty description"}</p>
							</>
						)}
					</li>
				))}
			</ol>
		</section>
	);
}
