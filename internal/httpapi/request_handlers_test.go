package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/application/requests"
	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/auth"
	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/config"
	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/domain"
)

type requestServiceFake struct {
	create   func(context.Context, requests.Actor, string, string, string) (domain.Request, error)
	update   func(context.Context, requests.Actor, string, int64, string, string) (domain.Request, error)
	submit   func(context.Context, requests.Actor, string, int64) (domain.Request, error)
	approve  func(context.Context, requests.Actor, string, int64) (domain.Request, error)
	get      func(context.Context, requests.Actor, string) (domain.Request, error)
	approval func(context.Context, requests.Actor, string) (*domain.Approval, error)
	pending  func(context.Context, requests.Actor) ([]domain.Request, error)
	audit    func(context.Context, requests.Actor, string) ([]domain.AuditEvent, error)
}

func (f *requestServiceFake) CreateDraft(c context.Context, a requests.Actor, o, t, d string) (domain.Request, error) {
	return f.create(c, a, o, t, d)
}
func (f *requestServiceFake) UpdateDraft(c context.Context, a requests.Actor, id string, v int64, t, d string) (domain.Request, error) {
	return f.update(c, a, id, v, t, d)
}
func (f *requestServiceFake) Submit(c context.Context, a requests.Actor, id string, v int64) (domain.Request, error) {
	return f.submit(c, a, id, v)
}
func (f *requestServiceFake) Approve(c context.Context, a requests.Actor, id string, v int64) (domain.Request, error) {
	return f.approve(c, a, id, v)
}
func (f *requestServiceFake) Get(c context.Context, a requests.Actor, id string) (domain.Request, error) {
	return f.get(c, a, id)
}
func (f *requestServiceFake) GetApproval(c context.Context, a requests.Actor, id string) (*domain.Approval, error) {
	return f.approval(c, a, id)
}
func (f *requestServiceFake) ListPending(c context.Context, a requests.Actor) ([]domain.Request, error) {
	return f.pending(c, a)
}
func (f *requestServiceFake) ListAuditEvents(c context.Context, a requests.Actor, id string) ([]domain.AuditEvent, error) {
	return f.audit(c, a, id)
}

func TestCreateRequestUsesAuthenticatedActorAndReturnsLocation(t *testing.T) {
	var gotActor requests.Actor
	d := requestTestDependencies()
	d.RequestService = &requestServiceFake{create: func(_ context.Context, actor requests.Actor, org, title, description string) (domain.Request, error) {
		gotActor = actor
		if org != "server-org" || title != "VPN" || description != "need access" {
			t.Fatalf("input = %q/%q/%q", org, title, description)
		}
		return domain.Request{ID: "req_opaque", OrganizationID: org, RequesterMemberID: actor.MemberID, Title: title, Description: description, Status: domain.RequestStatusDraft, Version: 1, CreatedAt: testNow, UpdatedAt: testNow}, nil
	}, approval: func(context.Context, requests.Actor, string) (*domain.Approval, error) { return nil, nil }}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/requests", strings.NewReader(`{"title":"VPN","description":"need access"}`))
	r.AddCookie(auth.SessionCookie("cookie", true))
	r.Header.Set("Origin", allowedOrigin)
	r.Header.Set("X-CSRF-Token", "token")
	rr := httptest.NewRecorder()
	NewRouter(d).ServeHTTP(rr, r)
	if rr.Code != http.StatusCreated || rr.Header().Get("Location") != "/api/v1/requests/req_opaque" {
		t.Fatalf("status=%d headers=%v body=%s", rr.Code, rr.Header(), rr.Body.String())
	}
	if gotActor.MemberID != "selected-member" || gotActor.OrganizationID != "server-org" || len(gotActor.Roles) != 1 || gotActor.Roles[0] != domain.RoleRequester {
		t.Fatalf("actor=%+v", gotActor)
	}
}

func TestRequestMutationRejectsStrictJSONAndExpectedVersionBeforeService(t *testing.T) {
	for _, body := range []string{"", `{"title":"VPN"} {}`, `{"title":"VPN","description":"d","expectedVersion":1,"unknown":true}`, `{"title":"VPN","description":"d","expectedVersion":1,"expectedVersion":2}`, `{"title":"VPN","expectedVersion":0}`, `{"title":"VPN","description":"d","expectedVersion":1} trailing"`} {
		t.Run(body, func(t *testing.T) {
			called := false
			d := requestTestDependencies()
			d.RequestService = &requestServiceFake{update: func(context.Context, requests.Actor, string, int64, string, string) (domain.Request, error) {
				called = true
				return domain.Request{}, nil
			}, approval: func(context.Context, requests.Actor, string) (*domain.Approval, error) { return nil, nil }}
			r := httptest.NewRequest(http.MethodPatch, "/api/v1/requests/req", strings.NewReader(body))
			r.AddCookie(auth.SessionCookie("cookie", true))
			r.Header.Set("Origin", allowedOrigin)
			r.Header.Set("X-CSRF-Token", "token")
			rr := httptest.NewRecorder()
			NewRouter(d).ServeHTTP(rr, r)
			if rr.Code != http.StatusBadRequest || called {
				t.Fatalf("status=%d called=%v body=%s", rr.Code, called, rr.Body.String())
			}
		})
	}
}

func TestRequestRejectsNullStringFieldsBeforeService(t *testing.T) {
	for _, endpoint := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/requests", `{"title":"T","description":null}`},
		{http.MethodPatch, "/api/v1/requests/r", `{"title":"T","description":null,"expectedVersion":1}`},
	} {
		t.Run(endpoint.method, func(t *testing.T) {
			called := false
			d := requestTestDependencies()
			d.RequestService = &requestServiceFake{
				create: func(context.Context, requests.Actor, string, string, string) (domain.Request, error) {
					called = true
					return domain.Request{}, nil
				},
				update: func(context.Context, requests.Actor, string, int64, string, string) (domain.Request, error) {
					called = true
					return domain.Request{}, nil
				},
			}
			r := httptest.NewRequest(endpoint.method, endpoint.path, strings.NewReader(endpoint.body))
			r.AddCookie(auth.SessionCookie("cookie", true))
			r.Header.Set("Origin", allowedOrigin)
			r.Header.Set("X-CSRF-Token", "token")
			rr := httptest.NewRecorder()
			NewRouter(d).ServeHTTP(rr, r)
			if rr.Code != http.StatusBadRequest || called {
				t.Fatalf("status=%d called=%v", rr.Code, called)
			}
		})
	}
}

func TestUpdateRequiresDescriptionAndDocumentedApprovalAuditPaths(t *testing.T) {
	called := false
	d := requestTestDependencies()
	d.RequestService = &requestServiceFake{
		update: func(context.Context, requests.Actor, string, int64, string, string) (domain.Request, error) {
			called = true
			return domain.Request{}, nil
		},
		approve: func(context.Context, requests.Actor, string, int64) (domain.Request, error) {
			called = true
			return domain.Request{}, nil
		},
		audit: func(context.Context, requests.Actor, string) ([]domain.AuditEvent, error) {
			called = true
			return nil, nil
		},
		approval: func(context.Context, requests.Actor, string) (*domain.Approval, error) { return nil, nil },
	}
	patch := httptest.NewRequest(http.MethodPatch, "/api/v1/requests/req", strings.NewReader(`{"title":"new","expectedVersion":1}`))
	patch.AddCookie(auth.SessionCookie("cookie", true))
	patch.Header.Set("Origin", allowedOrigin)
	patch.Header.Set("X-CSRF-Token", "token")
	rr := httptest.NewRecorder()
	NewRouter(d).ServeHTTP(rr, patch)
	if rr.Code != http.StatusBadRequest || called {
		t.Fatalf("patch status=%d called=%v", rr.Code, called)
	}
	for _, path := range []string{"/api/v1/requests/req/approvals", "/api/v1/requests/req/audit-events"} {
		r := httptest.NewRequest(map[string]string{"/api/v1/requests/req/approvals": http.MethodPost, "/api/v1/requests/req/audit-events": http.MethodGet}[path], path, strings.NewReader(`{"expectedVersion":1}`))
		r.AddCookie(auth.SessionCookie("cookie", true))
		r.Header.Set("Origin", allowedOrigin)
		r.Header.Set("X-CSRF-Token", "token")
		rr = httptest.NewRecorder()
		NewRouter(d).ServeHTTP(rr, r)
		if rr.Code == http.StatusNotFound {
			t.Fatalf("documented route not registered: %s", path)
		}
	}
}

func TestRequestGetDoesNotRequireCSRFAndPendingEnrichesEveryEntry(t *testing.T) {
	approvalCalls := 0
	d := requestTestDependencies()
	d.RequestService = &requestServiceFake{
		get: func(context.Context, requests.Actor, string) (domain.Request, error) {
			return domain.Request{ID: "req", OrganizationID: "server-org", Status: domain.RequestStatusDraft, Version: 1}, nil
		},
		pending: func(context.Context, requests.Actor) ([]domain.Request, error) {
			return []domain.Request{{ID: "one"}, {ID: "two"}}, nil
		},
		approval: func(_ context.Context, _ requests.Actor, id string) (*domain.Approval, error) {
			approvalCalls++
			return &domain.Approval{ID: "approval-" + id, AssigneeMemberID: "approver", Status: domain.ApprovalStatusPending}, nil
		},
	}
	get := httptest.NewRequest(http.MethodGet, "/api/v1/requests/req", nil)
	get.AddCookie(auth.SessionCookie("cookie", true))
	rr := httptest.NewRecorder()
	NewRouter(d).ServeHTTP(rr, get)
	if rr.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", rr.Code, rr.Body.String())
	}
	pending := httptest.NewRequest(http.MethodGet, "/api/v1/requests/pending", nil)
	pending.AddCookie(auth.SessionCookie("cookie", true))
	rr = httptest.NewRecorder()
	NewRouter(d).ServeHTTP(rr, pending)
	if rr.Code != http.StatusOK || approvalCalls != 3 || !strings.Contains(rr.Body.String(), `"id":"approval-one"`) || !strings.Contains(rr.Body.String(), `"id":"approval-two"`) {
		t.Fatalf("pending status=%d calls=%d body=%s", rr.Code, approvalCalls, rr.Body.String())
	}
}

func TestRequestMutationsRequireSessionAndCSRFBeforeService(t *testing.T) {
	paths := []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/requests", `{"title":"T"}`},
		{http.MethodPatch, "/api/v1/requests/r", `{"title":"T","description":"D","expectedVersion":1}`},
		{http.MethodPost, "/api/v1/requests/r/submit", `{"expectedVersion":1}`},
		{http.MethodPost, "/api/v1/requests/r/approvals", `{"expectedVersion":1}`},
	}
	for _, endpoint := range paths {
		t.Run(endpoint.method+endpoint.path, func(t *testing.T) {
			calls := 0
			d := requestTestDependencies()
			d.RequestService = &requestServiceFake{create: func(context.Context, requests.Actor, string, string, string) (domain.Request, error) {
				calls++
				return domain.Request{}, nil
			}, update: func(context.Context, requests.Actor, string, int64, string, string) (domain.Request, error) {
				calls++
				return domain.Request{}, nil
			}, submit: func(context.Context, requests.Actor, string, int64) (domain.Request, error) {
				calls++
				return domain.Request{}, nil
			}, approve: func(context.Context, requests.Actor, string, int64) (domain.Request, error) {
				calls++
				return domain.Request{}, nil
			}}
			r := httptest.NewRequest(endpoint.method, endpoint.path, strings.NewReader(endpoint.body))
			r.AddCookie(auth.SessionCookie("cookie", true))
			r.Header.Set("Origin", "https://other.example")
			r.Header.Set("X-CSRF-Token", "token")
			rr := httptest.NewRecorder()
			NewRouter(d).ServeHTTP(rr, r)
			if rr.Code != http.StatusForbidden || calls != 0 {
				t.Fatalf("status=%d calls=%d", rr.Code, calls)
			}
		})
	}
}

func TestRequestServiceErrorsMapToDocumentedResponses(t *testing.T) {
	for _, tc := range []struct {
		name, method, path string
		err                error
		status             int
		code               string
	}{
		{"patch forbidden", http.MethodPatch, "/api/v1/requests/r", domain.ErrForbidden, http.StatusForbidden, "forbidden"},
		{"patch not found", http.MethodPatch, "/api/v1/requests/r", domain.ErrNotFound, http.StatusNotFound, "request_not_found"},
		{"patch version", http.MethodPatch, "/api/v1/requests/r", domain.ErrVersionConflict, http.StatusConflict, "version_conflict"},
		{"patch state", http.MethodPatch, "/api/v1/requests/r", domain.ErrInvalidState, http.StatusConflict, "invalid_state"},
		{"submit forbidden", http.MethodPost, "/api/v1/requests/r/submit", domain.ErrForbidden, http.StatusForbidden, "forbidden"},
		{"submit not found", http.MethodPost, "/api/v1/requests/r/submit", domain.ErrNotFound, http.StatusNotFound, "request_not_found"},
		{"submit version", http.MethodPost, "/api/v1/requests/r/submit", domain.ErrVersionConflict, http.StatusConflict, "version_conflict"},
		{"submit state", http.MethodPost, "/api/v1/requests/r/submit", domain.ErrInvalidState, http.StatusConflict, "invalid_state"},
		{"submit routing", http.MethodPost, "/api/v1/requests/r/submit", domain.ErrApprovalRoutingUnavailable, http.StatusConflict, "approval_routing_unavailable"},
		{"approve forbidden", http.MethodPost, "/api/v1/requests/r/approvals", domain.ErrForbidden, http.StatusForbidden, "forbidden"},
		{"approve not found", http.MethodPost, "/api/v1/requests/r/approvals", domain.ErrNotFound, http.StatusNotFound, "request_not_found"},
		{"approve version", http.MethodPost, "/api/v1/requests/r/approvals", domain.ErrVersionConflict, http.StatusConflict, "version_conflict"},
		{"approve state", http.MethodPost, "/api/v1/requests/r/approvals", domain.ErrInvalidState, http.StatusConflict, "invalid_state"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := requestTestDependencies()
			d.RequestService = &requestServiceFake{
				update: func(context.Context, requests.Actor, string, int64, string, string) (domain.Request, error) {
					return domain.Request{}, tc.err
				},
				submit: func(context.Context, requests.Actor, string, int64) (domain.Request, error) {
					return domain.Request{}, tc.err
				},
				approve: func(context.Context, requests.Actor, string, int64) (domain.Request, error) {
					return domain.Request{}, tc.err
				},
			}
			body := `{"title":"T","description":"D","expectedVersion":1}`
			if tc.method != http.MethodPatch {
				body = `{"expectedVersion":1}`
			}
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(body))
			r.AddCookie(auth.SessionCookie("cookie", true))
			r.Header.Set("Origin", allowedOrigin)
			r.Header.Set("X-CSRF-Token", "token")
			rr := httptest.NewRecorder()
			NewRouter(d).ServeHTTP(rr, r)
			if rr.Code != tc.status || !strings.Contains(rr.Body.String(), `"code":"`+tc.code+`"`) {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestRequestOperationSuccessContracts(t *testing.T) {
	result := domain.Request{ID: "r", OrganizationID: "server-org", Title: "T", Description: "D", Status: domain.RequestStatusPending, Version: 2, RequesterMemberID: "selected-member", CreatedAt: testNow, UpdatedAt: testNow}
	approval := &domain.Approval{ID: "a", RequestID: "r", AssigneeMemberID: "selected-member", Status: domain.ApprovalStatusPending}
	d := requestTestDependencies()
	d.RequestService = &requestServiceFake{
		get: func(context.Context, requests.Actor, string) (domain.Request, error) { return result, nil },
		update: func(context.Context, requests.Actor, string, int64, string, string) (domain.Request, error) {
			result.Status = domain.RequestStatusDraft
			return result, nil
		},
		submit: func(context.Context, requests.Actor, string, int64) (domain.Request, error) { return result, nil },
		approve: func(context.Context, requests.Actor, string, int64) (domain.Request, error) {
			result.Status = domain.RequestStatusApproved
			approval.Status = domain.ApprovalStatusApproved
			return result, nil
		},
		approval: func(context.Context, requests.Actor, string) (*domain.Approval, error) { return approval, nil },
		audit: func(context.Context, requests.Actor, string) ([]domain.AuditEvent, error) {
			return []domain.AuditEvent{{ID: "e", Type: "request_submitted", OccurredAt: testNow, ActorMemberID: "selected-member"}}, nil
		},
	}
	for _, tc := range []struct{ method, path, body, want string }{
		{http.MethodPatch, "/api/v1/requests/r", `{"title":"T","description":"D","expectedVersion":1}`, `"id":"r"`},
		{http.MethodPost, "/api/v1/requests/r/submit", `{"expectedVersion":1}`, `"status":"pending"`},
		{http.MethodPost, "/api/v1/requests/r/approvals", `{"expectedVersion":2}`, `"status":"approved"`},
		{http.MethodGet, "/api/v1/requests/r/audit-events", "", `"events"`},
	} {
		t.Run(tc.path, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			r.AddCookie(auth.SessionCookie("cookie", true))
			r.Header.Set("Origin", allowedOrigin)
			r.Header.Set("X-CSRF-Token", "token")
			rr := httptest.NewRecorder()
			NewRouter(d).ServeHTTP(rr, r)
			if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), tc.want) {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestRequestMutationsRejectUnauthenticatedAndInvalidCSRF(t *testing.T) {
	for _, endpoint := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/requests", `{"title":"T"}`}, {http.MethodPatch, "/api/v1/requests/r", `{"title":"T","description":"D","expectedVersion":1}`}, {http.MethodPost, "/api/v1/requests/r/submit", `{"expectedVersion":1}`}, {http.MethodPost, "/api/v1/requests/r/approvals", `{"expectedVersion":1}`},
	} {
		t.Run(endpoint.path, func(t *testing.T) {
			calls := 0
			d := requestTestDependencies()
			d.RequestService = &requestServiceFake{create: func(context.Context, requests.Actor, string, string, string) (domain.Request, error) {
				calls++
				return domain.Request{}, nil
			}, update: func(context.Context, requests.Actor, string, int64, string, string) (domain.Request, error) {
				calls++
				return domain.Request{}, nil
			}, submit: func(context.Context, requests.Actor, string, int64) (domain.Request, error) {
				calls++
				return domain.Request{}, nil
			}, approve: func(context.Context, requests.Actor, string, int64) (domain.Request, error) {
				calls++
				return domain.Request{}, nil
			}}
			unauth := httptest.NewRequest(endpoint.method, endpoint.path, strings.NewReader(endpoint.body))
			rr := httptest.NewRecorder()
			NewRouter(d).ServeHTTP(rr, unauth)
			if rr.Code != http.StatusUnauthorized || calls != 0 {
				t.Fatalf("unauth status=%d calls=%d", rr.Code, calls)
			}
			invalid := httptest.NewRequest(endpoint.method, endpoint.path, strings.NewReader(endpoint.body))
			invalid.AddCookie(auth.SessionCookie("cookie", true))
			invalid.Header.Set("Origin", allowedOrigin)
			rr = httptest.NewRecorder()
			NewRouter(d).ServeHTTP(rr, invalid)
			if rr.Code != http.StatusForbidden || calls != 0 {
				t.Fatalf("csrf status=%d calls=%d", rr.Code, calls)
			}
		})
	}
}

func TestStaleSubmitReturnsConflictWithoutAuditOrApprovalRead(t *testing.T) {
	auditEvents, approvalReads := []string{}, 0
	currentVersion := int64(2)
	d := requestTestDependencies()
	d.RequestService = &requestServiceFake{
		submit: func(_ context.Context, _ requests.Actor, _ string, version int64) (domain.Request, error) {
			if version != currentVersion {
				return domain.Request{}, domain.ErrVersionConflict
			}
			auditEvents = append(auditEvents, "request_submitted")
			return domain.Request{ID: "r", Version: currentVersion + 1}, nil
		},
		approval: func(context.Context, requests.Actor, string) (*domain.Approval, error) {
			approvalReads++
			return nil, nil
		},
	}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/requests/r/submit", strings.NewReader(`{"expectedVersion":1}`))
	r.AddCookie(auth.SessionCookie("cookie", true))
	r.Header.Set("Origin", allowedOrigin)
	r.Header.Set("X-CSRF-Token", "token")
	rr := httptest.NewRecorder()
	NewRouter(d).ServeHTTP(rr, r)
	if rr.Code != http.StatusConflict || len(auditEvents) != 0 || approvalReads != 0 {
		t.Fatalf("status=%d audit=%d approval=%d", rr.Code, len(auditEvents), approvalReads)
	}
}

func requestTestDependencies() Dependencies {
	return Dependencies{Config: testConfig(), SessionStore: fakeSessionStore{authenticate: func(context.Context, string, time.Time) (auth.AuthenticatedSession, error) {
		return authenticatedSession("token"), nil
	}, validate: func(context.Context, string, string, time.Time) error { return nil }}, Now: func() time.Time { return testNow }}
}
func testConfig() config.Config {
	return config.Config{AllowedOrigin: allowedOrigin, CookieSecure: true}
}
