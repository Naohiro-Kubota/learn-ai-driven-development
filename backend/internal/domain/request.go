package domain

import "time"

type Role string

const (
	RoleRequester Role = "requester"
	RoleApprover  Role = "approver"
	RoleAdmin     Role = "admin"
)

type RequestStatus string

const (
	RequestStatusDraft    RequestStatus = "draft"
	RequestStatusPending  RequestStatus = "pending"
	RequestStatusApproved RequestStatus = "approved"
)

type ApprovalStatus string

const (
	ApprovalStatusPending  ApprovalStatus = "pending"
	ApprovalStatusApproved ApprovalStatus = "approved"
)

type Request struct {
	ID                string
	OrganizationID    string
	RequesterMemberID string
	Title             string
	Description       string
	Status            RequestStatus
	Version           int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type Approval struct {
	ID               string
	RequestID        string
	AssigneeMemberID string
	Status           ApprovalStatus
	ApprovedAt       *time.Time
}

type DefaultApprover struct {
	MemberID        string
	HasApproverRole bool
}

type ContentSnapshot struct{ Title, Description string }

type AuditEvent struct {
	ID               string
	RequestID        string
	ActorMemberID    string
	Type             string
	OccurredAt       time.Time
	ContentSnapshot  *ContentSnapshot
	ApprovalMetadata *ApprovalMetadata
}

type ApprovalMetadata struct {
	AssigneeMemberID string
	ApprovalID       string
}
