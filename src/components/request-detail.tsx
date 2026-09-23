import type { ReactElement } from "react";
import type { Actor, Request } from "../api/types";

export function RequestDetail({
	request,
	actor,
	onUpdate,
	onSubmit,
	onApprove,
}: {
	request: Request;
	actor: Actor;
	onUpdate: () => void;
	onSubmit: () => void;
	onApprove: () => void;
}): ReactElement {
	const editable =
		request.status === "draft" &&
		request.requesterMemberId === actor.memberId &&
		actor.roles.includes("requester");
	const approvable =
		request.status === "pending" &&
		request.approval?.assigneeMemberId === actor.memberId &&
		actor.roles.includes("approver");
	return (
		<section aria-label="Request detail">
			<h2>{request.title}</h2>
			<p>Status: {request.status}</p>
			<p>{request.description || "Empty description"}</p>
			{editable && (
				<>
					<button type="button" onClick={onUpdate}>
						Update Draft
					</button>
					<button type="button" onClick={onSubmit}>
						Submit
					</button>
				</>
			)}
			{approvable && (
				<button type="button" onClick={onApprove}>
					Approve
				</button>
			)}
		</section>
	);
}
