package postgres

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	apprequests "github.com/Naohiro-Kubota/learn-ai-driven-development/internal/application/requests"
	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/domain"
)

type Repository struct {
	db *sql.DB
}

var _ apprequests.Repository = (*Repository)(nil)

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateDraft(ctx context.Context, command apprequests.CreateDraftCommand) (domain.Request, error) {
	request := command.Request
	if request.ID == "" {
		id, err := newOpaqueID()
		if err != nil {
			return domain.Request{}, err
		}
		request.ID = id
	}
	transaction, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Request{}, err
	}
	defer transaction.Rollback()
	_, err = transaction.ExecContext(ctx, `
		INSERT INTO requests (id, organization_id, requester_id, title, description, status, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		request.ID, request.OrganizationID, request.RequesterMemberID, request.Title, request.Description, request.Status, request.Version, request.CreatedAt, request.UpdatedAt)
	if err != nil {
		return domain.Request{}, err
	}
	if err := insertAuditEvent(ctx, transaction, command.AuditEvent, request.ID); err != nil {
		return domain.Request{}, err
	}
	if err := transaction.Commit(); err != nil {
		return domain.Request{}, err
	}
	return request, nil
}

func (r *Repository) Get(ctx context.Context, requestID string) (domain.Request, error) {
	return getRequest(ctx, r.db, requestID)
}

func (r *Repository) GetApproval(ctx context.Context, requestID string) (*domain.Approval, error) {
	return getApproval(ctx, r.db, requestID)
}

func (r *Repository) UpdateDraft(ctx context.Context, command apprequests.UpdateDraftCommand) (domain.Request, error) {
	transaction, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Request{}, err
	}
	defer transaction.Rollback()
	result, err := transaction.ExecContext(ctx, `
		UPDATE requests
		SET title = $1, description = $2, updated_at = $3, version = version + 1
		WHERE id = $4 AND version = $5 AND status = 'draft'`,
		command.Request.Title, command.Request.Description, command.Request.UpdatedAt, command.Request.ID, command.ExpectedVersion)
	if err != nil {
		return domain.Request{}, err
	}
	if err := requireConditionalUpdate(ctx, transaction, result, command.Request.ID, command.ExpectedVersion); err != nil {
		return domain.Request{}, err
	}
	request, err := getRequest(ctx, transaction, command.Request.ID)
	if err != nil {
		return domain.Request{}, err
	}
	if err := insertAuditEvent(ctx, transaction, command.AuditEvent, request.ID); err != nil {
		return domain.Request{}, err
	}
	if err := transaction.Commit(); err != nil {
		return domain.Request{}, err
	}
	return request, nil
}

func (r *Repository) DefaultApprover(ctx context.Context, organizationID string) (domain.DefaultApprover, error) {
	var memberID sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT default_approver_member_id FROM organizations WHERE id = $1`, organizationID).Scan(&memberID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.DefaultApprover{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.DefaultApprover{}, err
	}
	if !memberID.Valid {
		return domain.DefaultApprover{}, domain.ErrNotFound
	}
	var hasApproverRole bool
	if err := r.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM member_roles WHERE member_id = $1 AND role = 'approver')`, memberID.String).Scan(&hasApproverRole); err != nil {
		return domain.DefaultApprover{}, err
	}
	return domain.DefaultApprover{MemberID: memberID.String, HasApproverRole: hasApproverRole}, nil
}

func (r *Repository) Submit(ctx context.Context, command apprequests.SubmitCommand) (domain.Request, error) {
	transaction, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Request{}, err
	}
	defer transaction.Rollback()
	request, err := getRequest(ctx, transaction, command.RequestID)
	if err != nil {
		return domain.Request{}, err
	}
	if request.RequesterMemberID != command.RequesterMemberID || command.RequesterMemberID == command.AssigneeMemberID {
		return domain.Request{}, domain.ErrForbidden
	}
	defaultApprover, err := defaultApprover(ctx, transaction, request.OrganizationID)
	if err != nil {
		return domain.Request{}, err
	}
	if !defaultApprover.HasApproverRole || defaultApprover.MemberID != command.AssigneeMemberID {
		return domain.Request{}, domain.ErrForbidden
	}
	result, err := transaction.ExecContext(ctx, `
		UPDATE requests
		SET status = 'pending', updated_at = $1, version = version + 1
		WHERE id = $2 AND version = $3 AND status = 'draft'`,
		command.AuditEvent.OccurredAt, command.RequestID, command.ExpectedVersion)
	if err != nil {
		return domain.Request{}, err
	}
	if err := requireConditionalUpdate(ctx, transaction, result, command.RequestID, command.ExpectedVersion); err != nil {
		return domain.Request{}, err
	}
	approvalID, err := newOpaqueID()
	if err != nil {
		return domain.Request{}, err
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO approvals (id, request_id, assignee_id, status)
		VALUES ($1, $2, $3, 'pending')`, approvalID, command.RequestID, command.AssigneeMemberID); err != nil {
		return domain.Request{}, err
	}
	command.AuditEvent.ApprovalMetadata = &domain.ApprovalMetadata{AssigneeMemberID: command.AssigneeMemberID, ApprovalID: approvalID}
	if err := insertAuditEvent(ctx, transaction, command.AuditEvent, command.RequestID); err != nil {
		return domain.Request{}, err
	}
	request, err = getRequest(ctx, transaction, command.RequestID)
	if err != nil {
		return domain.Request{}, err
	}
	if err := transaction.Commit(); err != nil {
		return domain.Request{}, err
	}
	return request, nil
}

func (r *Repository) Approve(ctx context.Context, command apprequests.ApproveCommand) (domain.Request, error) {
	transaction, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Request{}, err
	}
	defer transaction.Rollback()
	approval, err := getApproval(ctx, transaction, command.RequestID)
	if err != nil {
		return domain.Request{}, err
	}
	if approval.AssigneeMemberID != command.AssigneeMemberID {
		return domain.Request{}, domain.ErrForbidden
	}
	result, err := transaction.ExecContext(ctx, `
		UPDATE requests
		SET status = 'approved', updated_at = $1, version = version + 1
		WHERE id = $2 AND version = $3 AND status = 'pending'`,
		command.AuditEvent.OccurredAt, command.RequestID, command.ExpectedVersion)
	if err != nil {
		return domain.Request{}, err
	}
	if err := requireConditionalUpdate(ctx, transaction, result, command.RequestID, command.ExpectedVersion); err != nil {
		return domain.Request{}, err
	}
	result, err = transaction.ExecContext(ctx, `
		UPDATE approvals
		SET status = 'approved', approved_at = $1
		WHERE request_id = $2 AND assignee_id = $3 AND status = 'pending'`,
		command.AuditEvent.OccurredAt, command.RequestID, command.AssigneeMemberID)
	if err != nil {
		return domain.Request{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return domain.Request{}, err
	}
	if rows != 1 {
		return domain.Request{}, domain.ErrInvalidState
	}
	command.AuditEvent.ApprovalMetadata = &domain.ApprovalMetadata{AssigneeMemberID: command.AssigneeMemberID, ApprovalID: approval.ID}
	if err := insertAuditEvent(ctx, transaction, command.AuditEvent, command.RequestID); err != nil {
		return domain.Request{}, err
	}
	request, err := getRequest(ctx, transaction, command.RequestID)
	if err != nil {
		return domain.Request{}, err
	}
	if err := transaction.Commit(); err != nil {
		return domain.Request{}, err
	}
	return request, nil
}

func (r *Repository) ListPending(ctx context.Context, assigneeMemberID string) ([]domain.Request, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT r.id, r.organization_id, r.requester_id, r.title, r.description, r.status, r.version, r.created_at, r.updated_at
		FROM requests r
		JOIN approvals a ON a.request_id = r.id
		WHERE a.assignee_id = $1 AND a.status = 'pending' AND r.status = 'pending'
		ORDER BY r.updated_at, r.id`, assigneeMemberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	requests := make([]domain.Request, 0)
	for rows.Next() {
		request, err := scanRequest(rows)
		if err != nil {
			return nil, err
		}
		requests = append(requests, request)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return requests, nil
}

func (r *Repository) ListAuditEvents(ctx context.Context, requestID string) ([]domain.AuditEvent, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, request_id, actor_member_id, event_type, occurred_at, content_snapshot, approval_metadata
		FROM audit_events WHERE request_id = $1 ORDER BY occurred_at, id`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]domain.AuditEvent, 0)
	for rows.Next() {
		var event domain.AuditEvent
		var snapshot, metadata []byte
		if err := rows.Scan(&event.ID, &event.RequestID, &event.ActorMemberID, &event.Type, &event.OccurredAt, &snapshot, &metadata); err != nil {
			return nil, err
		}
		if snapshot != nil {
			event.ContentSnapshot = &domain.ContentSnapshot{}
			if err := json.Unmarshal(snapshot, event.ContentSnapshot); err != nil {
				return nil, err
			}
		}
		if metadata != nil {
			event.ApprovalMetadata = &domain.ApprovalMetadata{}
			if err := json.Unmarshal(metadata, event.ApprovalMetadata); err != nil {
				return nil, err
			}
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

type sqlQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func getRequest(ctx context.Context, queryer sqlQueryer, requestID string) (domain.Request, error) {
	request, err := scanRequest(queryer.QueryRowContext(ctx, `
		SELECT id, organization_id, requester_id, title, description, status, version, created_at, updated_at
		FROM requests WHERE id = $1`, requestID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Request{}, domain.ErrNotFound
	}
	return request, err
}

func getApproval(ctx context.Context, queryer sqlQueryer, requestID string) (*domain.Approval, error) {
	var approval domain.Approval
	err := queryer.QueryRowContext(ctx, `
		SELECT id, request_id, assignee_id, status, approved_at FROM approvals WHERE request_id = $1`, requestID).
		Scan(&approval.ID, &approval.RequestID, &approval.AssigneeMemberID, &approval.Status, &approval.ApprovedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &approval, nil
}

func defaultApprover(ctx context.Context, queryer sqlQueryer, organizationID string) (domain.DefaultApprover, error) {
	var memberID sql.NullString
	err := queryer.QueryRowContext(ctx, `SELECT default_approver_member_id FROM organizations WHERE id = $1`, organizationID).Scan(&memberID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.DefaultApprover{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.DefaultApprover{}, err
	}
	if !memberID.Valid {
		return domain.DefaultApprover{}, domain.ErrNotFound
	}
	var hasApproverRole bool
	if err := queryer.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM member_roles WHERE member_id = $1 AND role = 'approver')`, memberID.String).Scan(&hasApproverRole); err != nil {
		return domain.DefaultApprover{}, err
	}
	return domain.DefaultApprover{MemberID: memberID.String, HasApproverRole: hasApproverRole}, nil
}

type requestScanner interface {
	Scan(...any) error
}

func scanRequest(scanner requestScanner) (domain.Request, error) {
	var request domain.Request
	err := scanner.Scan(&request.ID, &request.OrganizationID, &request.RequesterMemberID, &request.Title, &request.Description, &request.Status, &request.Version, &request.CreatedAt, &request.UpdatedAt)
	return request, err
}

func requireConditionalUpdate(ctx context.Context, transaction *sql.Tx, result sql.Result, requestID string, expectedVersion int64) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 1 {
		return nil
	}
	request, err := getRequest(ctx, transaction, requestID)
	if err != nil {
		return err
	}
	if request.Version != expectedVersion {
		return domain.ErrVersionConflict
	}
	return domain.ErrInvalidState
}

func insertAuditEvent(ctx context.Context, transaction *sql.Tx, event domain.AuditEvent, requestID string) error {
	var snapshot, metadata any
	if event.ContentSnapshot != nil {
		encoded, err := json.Marshal(event.ContentSnapshot)
		if err != nil {
			return err
		}
		snapshot = encoded
	}
	if event.ApprovalMetadata != nil {
		encoded, err := json.Marshal(event.ApprovalMetadata)
		if err != nil {
			return err
		}
		metadata = encoded
	}
	id, err := newOpaqueID()
	if err != nil {
		return err
	}
	_, err = transaction.ExecContext(ctx, `
		INSERT INTO audit_events (id, request_id, actor_member_id, event_type, occurred_at, content_snapshot, approval_metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, requestID, event.ActorMemberID, event.Type, event.OccurredAt, snapshot, metadata)
	return err
}

func newOpaqueID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate opaque identifier: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}
