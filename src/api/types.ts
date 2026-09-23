export type Role = "admin" | "requester" | "approver";

export interface Actor {
	memberId: string;
	roles: Role[];
}

export interface Session {
	actor: Actor;
	csrfToken: string;
}

export interface OrganizationSelectionCandidate {
	memberId: string;
	organizationId: string;
	organizationName: string;
}

export interface OrganizationSelection {
	candidates: OrganizationSelectionCandidate[];
	csrfToken: string;
}

export interface SelectOrganizationInput {
	memberId: string;
}

export interface CreateRequestInput {
	title: string;
	description?: string;
}

export interface UpdateDraftRequestInput {
	title: string;
	description: string;
	expectedVersion: number;
}

export interface ExpectedVersionInput {
	expectedVersion: number;
}

export interface Approval {
	id: string;
	assigneeMemberId: string;
	status: "pending" | "approved";
	approvedAt: string | null;
}

export interface Request {
	id: string;
	title: string;
	description: string;
	status: "draft" | "pending" | "approved";
	version: number;
	requesterMemberId: string;
	approval: Approval | null;
	createdAt: string;
	updatedAt: string;
}

export interface PendingRequestList {
	requests: Request[];
}

export interface RequestContentSnapshot {
	title: string;
	description: string;
}

export interface AuditEvent {
	id: string;
	type:
		| "request_created"
		| "request_updated"
		| "request_submitted"
		| "request_approved";
	occurredAt: string;
	actorMemberId: string;
	requestContent: RequestContentSnapshot | null;
	approvalAssigneeMemberId: string | null;
	approvalId: string | null;
}

export interface AuditEventList {
	events: AuditEvent[];
}

export type ErrorCode =
	| "invalid_request"
	| "invalid_auth_transaction"
	| "authentication_required"
	| "forbidden"
	| "csrf_validation_failed"
	| "request_not_found"
	| "version_conflict"
	| "invalid_state"
	| "approval_routing_unavailable"
	| "internal_error";

export interface FieldError {
	field: string;
	code: string;
	message: string;
}

export interface ErrorResponse {
	code: ErrorCode;
	message: string;
	fieldErrors?: FieldError[];
}
