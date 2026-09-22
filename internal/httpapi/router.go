package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/auth"
	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/config"
)

// Authenticator is the HTTP boundary of the OIDC login use case.
type Authenticator interface {
	BeginLogin(context.Context) (auth.LoginStart, error)
	CompleteLogin(context.Context, auth.CallbackInput) (auth.LoginResult, error)
}

type Dependencies struct {
	Config         config.Config
	Authenticator  Authenticator
	SelectionStore auth.SelectionStore
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
	return mux
}

func singleHeader(header http.Header, name string) string {
	values := header.Values(name)
	if len(values) != 1 {
		return ""
	}
	return values[0]
}

func (r *router) validOrigin(request *http.Request) bool {
	return r.dependencies.Config.AllowedOrigin != "" && singleHeader(request.Header, "Origin") == r.dependencies.Config.AllowedOrigin
}
