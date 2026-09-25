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
	ApproveWithApproval(context.Context, requests.Actor, string, int64) (domain.Request, *domain.Approval, error)
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
	patterns := make(map[string]struct{})
	register := func(pattern string, handler http.Handler) {
		mux.Handle(pattern, handler)
		patterns[pattern] = struct{}{}
	}
	register("GET /auth/oidc/login", http.HandlerFunc(r.login))
	register("GET /auth/oidc/callback", http.HandlerFunc(r.callback))
	register("GET /auth/oidc/organization-selection", http.HandlerFunc(r.getOrganizationSelection))
	register("POST /auth/oidc/organization-selection", http.HandlerFunc(r.selectOrganization))
	register("GET /api/v1/session", r.RequireSession(http.HandlerFunc(r.getSession)))
	register("POST /api/v1/session/logout", r.RequireSession(r.RequireCSRF(http.HandlerFunc(r.logout))))
	register("POST /api/v1/requests", r.RequireSession(r.RequireCSRF(http.HandlerFunc(r.createRequest))))
	register("GET /api/v1/requests/pending", r.RequireSession(http.HandlerFunc(r.listPending)))
	register("GET /api/v1/requests/{requestId}", r.RequireSession(http.HandlerFunc(r.getRequest)))
	register("PATCH /api/v1/requests/{requestId}", r.RequireSession(r.RequireCSRF(http.HandlerFunc(r.updateRequest))))
	register("POST /api/v1/requests/{requestId}/submit", r.RequireSession(r.RequireCSRF(http.HandlerFunc(r.submitRequest))))
	register("POST /api/v1/requests/{requestId}/approvals", r.RequireSession(r.RequireCSRF(http.HandlerFunc(r.approveRequest))))
	register("GET /api/v1/requests/{requestId}/audit-events", r.RequireSession(http.HandlerFunc(r.listAuditEvents)))
	return routedHandler{Handler: NewCORS(dependencies.Config.FrontendOrigin, mux), mux: mux, patterns: patterns}
}

type routedHandler struct {
	http.Handler
	mux      *http.ServeMux
	patterns map[string]struct{}
}

func (h routedHandler) RoutePattern(r *http.Request) string {
	_, pattern := h.mux.Handler(r)
	if _, registered := h.patterns[pattern]; !registered {
		return ""
	}
	return pattern
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
