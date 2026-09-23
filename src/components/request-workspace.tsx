import {
	useCallback,
	useEffect,
	useRef,
	useState,
	type ReactElement,
} from "react";
import { ApiError, type ApiClient } from "../api/client";
import type { AuditEvent, FieldError, Request, Session } from "../api/types";
import { AuditHistory } from "./audit-history";
import type { Notice } from "./error-notice";
import { PendingList } from "./pending-list";
import { RequestDetail } from "./request-detail";
import { RequestForm } from "./request-form";

export type WorkspaceProps = {
	client: ApiClient;
	session: Session;
	requestId: string | null;
	onRequestIdChange: (id: string | null) => void;
	onSessionChange: (session: Session) => void;
	onAuthenticationRequired: () => void;
	onNotice: (notice: Notice | null) => void;
};

type ReadState =
	| { kind: "empty" | "loading" | "error" }
	| { kind: "ready"; request: Request; events: AuditEvent[] };

function validate(title: string, description: string): FieldError[] {
	const errors: FieldError[] = [];
	if (!title.trim())
		errors.push({
			field: "title",
			code: "required",
			message: "Title is required.",
		});
	else if (title.trim().length > 120)
		errors.push({
			field: "title",
			code: "too_long",
			message: "Title must be at most 120 characters.",
		});
	if (description.length > 2000)
		errors.push({
			field: "description",
			code: "too_long",
			message: "Description must be at most 2000 characters.",
		});
	return errors;
}

export function RequestWorkspace({
	client,
	session,
	requestId,
	onRequestIdChange,
	onSessionChange,
	onAuthenticationRequired,
	onNotice,
}: WorkspaceProps): ReactElement {
	const [createTitle, setCreateTitle] = useState("");
	const [createDescription, setCreateDescription] = useState("");
	const [createErrors, setCreateErrors] = useState<FieldError[]>([]);
	const [editTitle, setEditTitle] = useState("");
	const [editDescription, setEditDescription] = useState("");
	const [editErrors, setEditErrors] = useState<FieldError[]>([]);
	const [pending, setPending] = useState(false);
	const [read, setRead] = useState<ReadState>({ kind: "empty" });
	const [auditReadFailed, setAuditReadFailed] = useState(false);
	const [pendingRequests, setPendingRequests] = useState<Request[]>([]);
	const [pendingLoading, setPendingLoading] = useState(false);
	const [pendingFailed, setPendingFailed] = useState(false);
	const [sessionReadFailed, setSessionReadFailed] = useState(false);
	const mutationInFlight = useRef(false);
	const generation = useRef(0);
	const auditRefreshSequence = useRef(0);
	const pendingGeneration = useRef(0);
	const approverMemberId = session.actor.roles.includes("approver")
		? session.actor.memberId
		: null;
	const validId =
		requestId && requestId !== "." && requestId !== ".." ? requestId : null;

	const load = useCallback(
		(id: string) => {
			const current = ++generation.current;
			auditRefreshSequence.current++;
			setAuditReadFailed(false);
			setRead({ kind: "loading" });
			Promise.all([client.getRequest(id), client.listAuditEvents(id)]).then(
				([request, events]) => {
					if (generation.current !== current) return;
					setRead({ kind: "ready", request, events });
					setEditTitle(request.title);
					setEditDescription(request.description);
				},
				(error: unknown) => {
					if (generation.current !== current) return;
					if (
						error instanceof ApiError &&
						error.body.code === "authentication_required"
					) {
						setRead({ kind: "empty" });
						onAuthenticationRequired();
					} else setRead({ kind: "error" });
				},
			);
		},
		[client, onAuthenticationRequired],
	);

	const loadPending = useCallback(() => {
		const current = ++pendingGeneration.current;
		setPendingLoading(true);
		setPendingFailed(false);
		client.listPending().then(
			(requests) => {
				if (pendingGeneration.current !== current) return;
				setPendingRequests(requests);
				setPendingLoading(false);
			},
			(error: unknown) => {
				if (pendingGeneration.current !== current) return;
				setPendingLoading(false);
				setPendingFailed(true);
				if (
					error instanceof ApiError &&
					error.body.code === "authentication_required"
				)
					onAuthenticationRequired();
				else
					onNotice({
						code:
							error instanceof ApiError ? error.body.code : "transport_failure",
						text: "Could not load pending requests. Refresh pending to retry.",
					});
			},
		);
	}, [client, onAuthenticationRequired, onNotice]);

	useEffect(() => {
		if (!approverMemberId) return;
		loadPending();
		return () => {
			pendingGeneration.current++;
		};
	}, [approverMemberId, loadPending]);

	useEffect(() => {
		if (validId) load(validId);
		else {
			generation.current++;
			setRead({ kind: "empty" });
		}
		return () => {
			generation.current++;
		};
	}, [validId, load]);

	async function refreshSession(): Promise<void> {
		setSessionReadFailed(false);
		try {
			const refreshed = await client.getSession();
			onSessionChange(refreshed);
			onNotice({
				code: "csrf_validation_failed",
				text: "Session token refreshed. Review and click again if you still want to continue.",
			});
		} catch (refreshError) {
			if (
				refreshError instanceof ApiError &&
				refreshError.body.code === "authentication_required"
			)
				onAuthenticationRequired();
			else {
				setSessionReadFailed(true);
				onNotice({
					code:
						refreshError instanceof ApiError
							? refreshError.body.code
							: "transport_failure",
					text: "Could not refresh your session. Retry the session read before another action.",
				});
			}
		}
	}

	async function recover(error: unknown, id?: string): Promise<void> {
		const code =
			error instanceof ApiError ? error.body.code : "transport_failure";
		if (code === "authentication_required") {
			generation.current++;
			setRead({ kind: "empty" });
			onAuthenticationRequired();
			return;
		}
		if (code === "version_conflict" || code === "invalid_state") {
			if (id) load(id);
			onNotice({
				code,
				text: "Request changed. Review the refreshed details before deciding whether to act again.",
			});
			return;
		}
		if (code === "csrf_validation_failed") {
			await refreshSession();
			return;
		}
		onNotice({
			code,
			text:
				code === "transport_failure"
					? "Could not confirm the request outcome. Refresh request details before taking another action."
					: "The action could not be completed. Review or refresh request details before another action.",
		});
	}

	function handleError(
		error: unknown,
		setErrors: (errors: FieldError[]) => void,
	): void {
		if (error instanceof ApiError && error.body.fieldErrors?.length)
			setErrors(error.body.fieldErrors);
		else void recover(error, validId ?? undefined);
	}

	function create(): void {
		const errors = validate(createTitle, createDescription);
		setCreateErrors(errors);
		if (errors.length || mutationInFlight.current) return;
		mutationInFlight.current = true;
		setPending(true);
		const currentGeneration = generation.current;
		client
			.createRequest(
				{ title: createTitle.trim(), description: createDescription },
				session.csrfToken,
			)
			.then(
				(request) => {
					if (generation.current !== currentGeneration) return;
					setCreateTitle("");
					setCreateDescription("");
					setCreateErrors([]);
					onRequestIdChange(request.id);
					setRead({ kind: "ready", request, events: [] });
				},
				(error: unknown) => {
					if (generation.current === currentGeneration)
						handleError(error, setCreateErrors);
				},
			)
			.finally(() => {
				mutationInFlight.current = false;
				setPending(false);
			});
	}

	function update(): void {
		if (read.kind !== "ready" || !validId) return;
		const errors = validate(editTitle, editDescription);
		setEditErrors(errors);
		if (errors.length || mutationInFlight.current) return;
		mutationInFlight.current = true;
		setPending(true);
		const currentGeneration = generation.current;
		client
			.updateRequest(
				validId,
				{
					title: editTitle.trim(),
					description: editDescription,
					expectedVersion: read.request.version,
				},
				session.csrfToken,
			)
			.then(
				(request) => {
					if (generation.current !== currentGeneration) return;
					setRead((current) =>
						current.kind === "ready" && current.request.id === request.id
							? { ...current, request }
							: current,
					);
					const refresh = ++auditRefreshSequence.current;
					client.listAuditEvents(request.id).then(
						(events) => {
							if (
								generation.current !== currentGeneration ||
								auditRefreshSequence.current !== refresh
							)
								return;
							setAuditReadFailed(false);
							setRead((current) =>
								current.kind === "ready" && current.request.id === request.id
									? { ...current, events }
									: current,
							);
						},
						(error: unknown) => {
							if (
								generation.current !== currentGeneration ||
								auditRefreshSequence.current !== refresh
							)
								return;
							if (
								error instanceof ApiError &&
								error.body.code === "authentication_required"
							) {
								void recover(error);
								return;
							}
							setAuditReadFailed(true);
							onNotice({
								code: "transport_failure",
								text: "Could not refresh audit history. Retry the read to see current events.",
							});
						},
					);
				},
				(error: unknown) => {
					if (generation.current === currentGeneration)
						handleError(error, setEditErrors);
				},
			)
			.finally(() => {
				mutationInFlight.current = false;
				setPending(false);
			});
	}

	function transition(kind: "submit" | "approve"): void {
		if (read.kind !== "ready" || !validId || mutationInFlight.current) return;
		mutationInFlight.current = true;
		setPending(true);
		const currentGeneration = generation.current;
		const action =
			kind === "submit" ? client.submitRequest : client.approveRequest;
		action(validId, read.request.version, session.csrfToken)
			.then(
				(request) => {
					if (generation.current !== currentGeneration) return;
					setRead((current) =>
						current.kind === "ready" && current.request.id === request.id
							? { ...current, request }
							: current,
					);
					const refresh = ++auditRefreshSequence.current;
					client.listAuditEvents(request.id).then(
						(events) => {
							if (
								generation.current !== currentGeneration ||
								auditRefreshSequence.current !== refresh
							)
								return;
							setAuditReadFailed(false);
							setRead((current) =>
								current.kind === "ready" && current.request.id === request.id
									? { ...current, events }
									: current,
							);
						},
						(error: unknown) => {
							if (
								generation.current !== currentGeneration ||
								auditRefreshSequence.current !== refresh
							)
								return;
							if (
								error instanceof ApiError &&
								error.body.code === "authentication_required"
							) {
								void recover(error);
								return;
							}
							setAuditReadFailed(true);
							onNotice({
								code: "transport_failure",
								text: "Could not refresh audit history. Retry the read to see current events.",
							});
						},
					);
					if (session.actor.roles.includes("approver")) loadPending();
				},
				(error: unknown) => {
					if (generation.current === currentGeneration)
						void recover(error, validId);
				},
			)
			.finally(() => {
				mutationInFlight.current = false;
				setPending(false);
			});
	}

	return (
		<section aria-label="Workspace">
			{sessionReadFailed && (
				<button type="button" onClick={() => void refreshSession()}>
					Retry session read
				</button>
			)}
			{session.actor.roles.includes("approver") && (
				<>
					<PendingList
						requests={pendingRequests}
						onSelect={onRequestIdChange}
						loading={pendingLoading}
						retry={loadPending}
					/>
					{pendingFailed && (
						<p role="alert">Pending requests could not be loaded.</p>
					)}
				</>
			)}
			{session.actor.roles.includes("requester") && (
				<section aria-label="Create request">
					<h2>New request</h2>
					<RequestForm
						title={createTitle}
						description={createDescription}
						fieldErrors={createErrors}
						onTitleChange={setCreateTitle}
						onDescriptionChange={setCreateDescription}
						onSubmit={create}
						submitLabel="Create Draft"
						pending={pending}
					/>
				</section>
			)}
			{read.kind === "loading" && <p>Loading request…</p>}
			{read.kind === "error" && (
				<div role="alert">
					<p>Could not load request.</p>
					<button
						type="button"
						onClick={() => {
							if (validId) load(validId);
						}}
					>
						Retry
					</button>
				</div>
			)}
			{read.kind === "ready" && (
				<>
					<button
						type="button"
						onClick={() => {
							if (validId) load(validId);
						}}
					>
						Refresh request
					</button>
					{auditReadFailed && (
						<div role="alert">
							<p>Audit history may be out of date.</p>
							<button
								type="button"
								onClick={() => {
									if (validId) load(validId);
								}}
							>
								Retry read
							</button>
						</div>
					)}
					<RequestDetail
						request={read.request}
						actor={session.actor}
						onUpdate={update}
						onSubmit={() => transition("submit")}
						onApprove={() => transition("approve")}
						submitEnabled={!pending}
						approveEnabled={!pending}
					/>
					{read.request.status === "draft" &&
						read.request.requesterMemberId === session.actor.memberId &&
						session.actor.roles.includes("requester") && (
							<section aria-label="Edit request">
								<h3>Edit Draft</h3>
								<RequestForm
									title={editTitle}
									description={editDescription}
									fieldErrors={editErrors}
									onTitleChange={setEditTitle}
									onDescriptionChange={setEditDescription}
									onSubmit={update}
									submitLabel="Save Draft"
									pending={pending}
								/>
							</section>
						)}
					<AuditHistory events={read.events} />
				</>
			)}
		</section>
	);
}
