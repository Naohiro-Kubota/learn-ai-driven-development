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
	const generation = useRef(0);
	const validId =
		requestId && requestId !== "." && requestId !== ".." ? requestId : null;

	const load = useCallback(
		(id: string) => {
			const current = ++generation.current;
			setRead({ kind: "loading" });
			Promise.all([client.getRequest(id), client.listAuditEvents(id)]).then(
				([request, events]) => {
					if (generation.current !== current) return;
					setRead({ kind: "ready", request, events });
					setEditTitle(request.title);
					setEditDescription(request.description);
				},
				() => {
					if (generation.current === current) setRead({ kind: "error" });
				},
			);
		},
		[client],
	);

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

	function handleError(
		error: unknown,
		setErrors: (errors: FieldError[]) => void,
	): void {
		if (error instanceof ApiError && error.body.fieldErrors?.length)
			setErrors(error.body.fieldErrors);
		else
			onNotice({
				code: error instanceof ApiError ? error.body.code : "transport_failure",
				text: "The request could not be saved. Please try again.",
			});
	}

	function create(): void {
		const errors = validate(createTitle, createDescription);
		setCreateErrors(errors);
		if (errors.length || pending) return;
		setPending(true);
		client
			.createRequest(
				{ title: createTitle.trim(), description: createDescription },
				session.csrfToken,
			)
			.then(
				(request) => {
					setCreateTitle("");
					setCreateDescription("");
					setCreateErrors([]);
					onRequestIdChange(request.id);
					setRead({ kind: "ready", request, events: [] });
				},
				(error: unknown) => handleError(error, setCreateErrors),
			)
			.finally(() => setPending(false));
	}

	function update(): void {
		if (read.kind !== "ready" || !validId) return;
		const errors = validate(editTitle, editDescription);
		setEditErrors(errors);
		if (errors.length || pending) return;
		setPending(true);
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
					setRead((current) =>
						current.kind === "ready" && current.request.id === request.id
							? { ...current, request }
							: current,
					);
					client
						.listAuditEvents(request.id)
						.then((events) =>
							setRead((current) =>
								current.kind === "ready" && current.request.id === request.id
									? { ...current, events }
									: current,
							),
						);
				},
				(error: unknown) => handleError(error, setEditErrors),
			)
			.finally(() => setPending(false));
	}

	return (
		<section aria-label="Workspace">
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
					<RequestDetail
						request={read.request}
						actor={session.actor}
						onUpdate={update}
						onSubmit={() => {}}
						onApprove={() => {}}
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
