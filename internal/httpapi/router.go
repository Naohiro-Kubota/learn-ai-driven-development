package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/application/requests"
	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/auth"
	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/config"
	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/domain"
)

// Authenticator is the HTTP boundary of the OIDC login use case.
type Authenticator interface {
	BeginLogin(context.Context) (auth.LoginStart, error)
	CompleteLogin(context.Context, auth.CallbackInput) (auth.LoginResult, error)
}

// RequestService is the application boundary for request, approval and audit operations.
type RequestService interface {
	CreateDraft(context.Context, requests.Actor, string, string, string) (domain.Request, error)
	UpdateDraft(context.Context, requests.Actor, string, int64, string, string) (domain.Request, error)
	Submit(context.Context, requests.Actor, string, int64) (domain.Request, error)
	Approve(context.Context, requests.Actor, string, int64) (domain.Request, error)
	Get(context.Context, requests.Actor, string) (domain.Request, error)
	GetApproval(context.Context, requests.Actor, string) (*domain.Approval, error)
	ListPending(context.Context, requests.Actor) ([]domain.Request, error)
	ListAuditEvents(context.Context, requests.Actor, string) ([]domain.AuditEvent, error)
}

type Dependencies struct {
	Config         config.Config
	Authenticator  Authenticator
	SelectionStore auth.SelectionStore
	SessionStore   auth.SessionStore
	RequestService RequestService
	Now            func() time.Time
}

type router struct{ dependencies Dependencies }

func NewRouter(dependencies Dependencies) http.Handler {
	if dependencies.Now == nil {
		dependencies.Now = time.Now
	}
	r := &router{dependencies: dependencies}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /auth/oidc/login", r.login)
	mux.HandleFunc("GET /auth/oidc/callback", r.callback)
	mux.HandleFunc("GET /auth/oidc/organization-selection", r.getOrganizationSelection)
	mux.HandleFunc("POST /auth/oidc/organization-selection", r.selectOrganization)
	mux.Handle("GET /api/v1/session", r.RequireSession(http.HandlerFunc(r.getSession)))
	mux.Handle("POST /api/v1/session/logout", r.RequireSession(r.RequireCSRF(http.HandlerFunc(r.logout))))
	mux.Handle("POST /api/v1/requests", r.RequireSession(r.RequireCSRF(http.HandlerFunc(r.createRequest))))
	mux.Handle("GET /api/v1/requests/pending", r.RequireSession(http.HandlerFunc(r.listPending)))
	mux.Handle("GET /api/v1/requests/{requestId}", r.RequireSession(http.HandlerFunc(r.getRequest)))
	mux.Handle("PATCH /api/v1/requests/{requestId}", r.RequireSession(r.RequireCSRF(http.HandlerFunc(r.updateRequest))))
	mux.Handle("POST /api/v1/requests/{requestId}/submit", r.RequireSession(r.RequireCSRF(http.HandlerFunc(r.submitRequest))))
	mux.Handle("POST /api/v1/requests/{requestId}/approvals", r.RequireSession(r.RequireCSRF(http.HandlerFunc(r.approveRequest))))
	mux.Handle("GET /api/v1/requests/{requestId}/audit-events", r.RequireSession(http.HandlerFunc(r.listAuditEvents)))
	return NewCORS(dependencies.Config.FrontendOrigin, mux)
}

func singleHeader(header http.Header, name string) string {
	values := header.Values(name)
	if len(values) != 1 {
		return ""
	}
	return values[0]
}

func (r *router) validOrigin(request *http.Request) bool {
	return r.dependencies.Config.FrontendOrigin != "" && singleHeader(request.Header, "Origin") == r.dependencies.Config.FrontendOrigin
}
