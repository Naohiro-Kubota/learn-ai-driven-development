package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/domain"
)

func TestRequestDTOUsesOpenAPIFieldsAndNullApprovalForDraft(t *testing.T) {
	now := time.Date(2026, 9, 22, 1, 2, 3, 456000000, time.UTC)
	dto := toRequestDTO(domain.Request{ID: "req_opaque", Title: "VPN", Description: "", Status: domain.RequestStatusDraft, Version: 1, RequesterMemberID: "member_opaque", CreatedAt: now, UpdatedAt: now}, nil)
	b, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":"req_opaque","title":"VPN","description":"","status":"draft","version":1,"requesterMemberId":"member_opaque","approval":null,"createdAt":"2026-09-22T01:02:03.456Z","updatedAt":"2026-09-22T01:02:03.456Z"}`
	if string(b) != want {
		t.Fatalf("JSON = %s, want %s", b, want)
	}
}

func TestRequestDTOMapsPendingAndApprovedApproval(t *testing.T) {
	approvedAt := time.Date(2026, 9, 22, 2, 3, 4, 0, time.UTC)
	for _, tc := range []struct {
		status domain.ApprovalStatus
		at     *time.Time
		want   string
	}{
		{domain.ApprovalStatusPending, nil, `"status":"pending","approvedAt":null`},
		{domain.ApprovalStatusApproved, &approvedAt, `"status":"approved","approvedAt":"2026-09-22T02:03:04Z"`},
	} {
		dto := toRequestDTO(domain.Request{ID: "req", Title: "T", Status: domain.RequestStatusPending, Version: 2, RequesterMemberID: "requester"}, &domain.Approval{ID: "approval_opaque", AssigneeMemberID: "approver", Status: tc.status, ApprovedAt: tc.at})
		b, err := json.Marshal(dto)
		if err != nil {
			t.Fatal(err)
		}
		text := string(b)
		for _, part := range []string{`"id":"approval_opaque"`, `"assigneeMemberId":"approver"`, tc.want} {
			if !strings.Contains(text, part) {
				t.Fatalf("JSON = %s, missing %s", text, part)
			}
		}
	}
}

func TestAuditEventDTOMapsNullabilityAndOpaqueIDs(t *testing.T) {
	now := time.Date(2026, 9, 22, 3, 4, 5, 0, time.UTC)
	dto := toAuditEventDTO(domain.AuditEvent{ID: "event_opaque", Type: "request_created", OccurredAt: now, ActorMemberID: "actor_opaque"})
	b, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":"event_opaque","type":"request_created","occurredAt":"2026-09-22T03:04:05Z","actorMemberId":"actor_opaque","requestContent":null,"approvalAssigneeMemberId":null,"approvalId":null}`
	if string(b) != want {
		t.Fatalf("JSON = %s, want %s", b, want)
	}
	dto = toAuditEventDTO(domain.AuditEvent{ID: "event_2", Type: "request_submitted", OccurredAt: now, ActorMemberID: "actor", ContentSnapshot: &domain.ContentSnapshot{Title: "T", Description: ""}, ApprovalMetadata: &domain.ApprovalMetadata{AssigneeMemberID: "assignee_opaque", ApprovalID: "approval_opaque"}})
	b, err = json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"approvalAssigneeMemberId":"assignee_opaque"`) || !strings.Contains(string(b), `"approvalId":"approval_opaque"`) {
		t.Fatalf("JSON = %s, missing approval metadata", b)
	}
}

func TestListDTOsUseEmptyArraysForNilSlices(t *testing.T) {
	if b, err := json.Marshal(toPendingRequestListDTO(nil)); err != nil || string(b) != `{"requests":[]}` {
		t.Fatalf("pending JSON = %s, err = %v", b, err)
	}
	if b, err := json.Marshal(toAuditEventListDTO(nil)); err != nil || string(b) != `{"events":[]}` {
		t.Fatalf("audit JSON = %s, err = %v", b, err)
	}
}

func TestWriteErrorIncludesFieldErrorsWithoutWrappedSecret(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteError(rr, APIError{Status: http.StatusBadRequest, Code: "invalid_request", FieldErrors: []fieldErrorDTO{{Field: "title", Code: "required", Message: "Title is required."}}})
	if rr.Header().Get("Content-Type") != "application/json" || rr.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("headers = %#v", rr.Header())
	}
	if !strings.Contains(rr.Body.String(), `"fieldErrors":[{"field":"title","code":"required","message":"Title is required."}]`) {
		t.Fatalf("body = %s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "secret") {
		t.Fatalf("body leaked secret: %s", rr.Body.String())
	}
}

func TestRequestAPIErrorMapsDomainErrorsAndHidesWrappedDetails(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{domain.ErrInvalidRequest, 400, "invalid_request"}, {domain.ErrForbidden, 403, "forbidden"}, {domain.ErrNotFound, 404, "request_not_found"},
		{domain.ErrVersionConflict, 409, "version_conflict"}, {domain.ErrInvalidState, 409, "invalid_state"}, {domain.ErrApprovalRoutingUnavailable, 409, "approval_routing_unavailable"},
		{errors.New("db secret"), 500, "internal_error"},
	} {
		got := requestAPIError(tc.err)
		if got.Status != tc.status || got.Code != tc.code {
			t.Errorf("requestAPIError(%v) = %#v, want %d/%s", tc.err, got, tc.status, tc.code)
		}
	}
}

func TestRequestAPIErrorMapsFieldViolations(t *testing.T) {
	err := &domain.ValidationError{Fields: []domain.FieldViolation{{Field: "title", Code: "required"}, {Field: "description", Code: "too_long"}}}
	got := requestAPIError(err)
	if got.Status != 400 || got.Code != "invalid_request" || len(got.FieldErrors) != 2 {
		t.Fatalf("requestAPIError() = %#v", got)
	}
	if got.FieldErrors[0] != (fieldErrorDTO{Field: "title", Code: "required", Message: "Title must not be blank."}) || got.FieldErrors[1] != (fieldErrorDTO{Field: "description", Code: "too_long", Message: "Description must be at most 2000 characters."}) {
		t.Fatalf("field errors = %#v", got.FieldErrors)
	}
}

func TestWriteErrorUsesDocumentedRequestMessages(t *testing.T) {
	for _, tc := range []struct {
		code, message string
	}{
		{"request_not_found", "Request was not found."},
		{"version_conflict", "Request changed. Retrieve the latest state before retrying."},
		{"invalid_state", "This operation is not valid for the current Request state."},
		{"approval_routing_unavailable", "A default Approver cannot be assigned."},
	} {
		rr := httptest.NewRecorder()
		WriteError(rr, APIError{Status: http.StatusConflict, Code: tc.code})
		var body struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Code != tc.code || body.Message != tc.message {
			t.Errorf("WriteError(%s) = %#v, want code/message %q/%q", tc.code, body, tc.code, tc.message)
		}
	}
}
