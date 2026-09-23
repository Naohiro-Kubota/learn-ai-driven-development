import type { ReactElement } from "react";
import type { ErrorCode } from "../api/types";

export type Notice = { code: ErrorCode | "transport_failure"; text: string };

export function ErrorNotice({
	notice,
	action,
}: {
	notice: Notice;
	action?: { label: string; onClick: () => void };
}): ReactElement {
	return (
		<div role="alert">
			<p>{notice.text}</p>
			{action && (
				<button type="button" onClick={action.onClick}>
					{action.label}
				</button>
			)}
		</div>
	);
}
