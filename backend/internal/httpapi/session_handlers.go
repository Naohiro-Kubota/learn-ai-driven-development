package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/application/requests"
	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/auth"
	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/domain"
)

type sessionContextKey struct{}

type authenticatedRequest struct {
	session auth.AuthenticatedSession
	actor   requests.Actor
	cookie  string
}

// ActorFromContext exposes only the server-derived application actor.
func ActorFromContext(ctx context.Context) (requests.Actor, bool) {
	value, ok := ctx.Value(sessionContextKey{}).(authenticatedRequest)
	return value.actor, ok
}

// RequireSession authenticates the configured cookie once per request.
func (r *router) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		cookie, err := request.Cookie(auth.SessionCookie("", r.dependencies.Config.CookieSecure).Name)
		if err != nil || cookie.Value == "" {
			WriteError(w, APIError{Status: http.StatusUnauthorized, Code: "authentication_required"})
			return
		}
		session, err := r.dependencies.SessionStore.Authenticate(request.Context(), cookie.Value, r.dependencies.Now())
		if err != nil {
			WriteError(w, sessionAPIError(err))
			return
		}
		value := authenticatedRequest{
			session: session,
			actor:   requests.Actor{MemberID: session.Principal.MemberID, OrganizationID: session.Principal.OrganizationID, Roles: session.Principal.Roles},
			cookie:  cookie.Value,
		}
		next.ServeHTTP(w, request.WithContext(context.WithValue(request.Context(), sessionContextKey{}, value)))
	})
}

// RequireCSRF protects unsafe methods after RequireSession has authenticated them.
func (r *router) RequireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if !r.validOrigin(request) {
			WriteError(w, APIError{Status: http.StatusForbidden, Code: "csrf_validation_failed"})
			return
		}
		value, ok := request.Context().Value(sessionContextKey{}).(authenticatedRequest)
		token := singleHeader(request.Header, "X-CSRF-Token")
		if !ok || token == "" {
			WriteError(w, APIError{Status: http.StatusForbidden, Code: "csrf_validation_failed"})
			return
		}
		if err := r.dependencies.SessionStore.ValidateCSRFToken(request.Context(), value.cookie, token, r.dependencies.Now()); err != nil {
			WriteError(w, sessionAPIError(err))
			return
		}
		next.ServeHTTP(w, request)
	})
}

func (r *router) getSession(w http.ResponseWriter, request *http.Request) {
	value := request.Context().Value(sessionContextKey{}).(authenticatedRequest)
	token, err := r.dependencies.SessionStore.IssueCSRFToken(request.Context(), value.session.ID, r.dependencies.Now())
	if err != nil {
		WriteError(w, sessionAPIError(err))
		return
	}
	type actorDTO struct {
		MemberID string        `json:"memberId"`
		Roles    []domain.Role `json:"roles"`
	}
	roles := append([]domain.Role{}, value.actor.Roles...)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(struct {
		Actor     actorDTO `json:"actor"`
		CSRFToken string   `json:"csrfToken"`
	}{actorDTO{value.actor.MemberID, roles}, token})
}

func (r *router) logout(w http.ResponseWriter, request *http.Request) {
	value := request.Context().Value(sessionContextKey{}).(authenticatedRequest)
	if err := r.dependencies.SessionStore.Revoke(request.Context(), value.cookie, r.dependencies.Now()); err != nil {
		WriteError(w, sessionAPIError(err))
		return
	}
	cookie := auth.SessionCookie("", r.dependencies.Config.CookieSecure)
	cookie.MaxAge = -1
	cookie.Expires = time.Unix(1, 0)
	http.SetCookie(w, cookie)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func sessionAPIError(err error) APIError {
	switch {
	case errors.Is(err, auth.ErrCSRFValidation):
		return APIError{Status: http.StatusForbidden, Code: "csrf_validation_failed"}
	case errors.Is(err, auth.ErrNotFound), errors.Is(err, auth.ErrExpired), errors.Is(err, auth.ErrInvalidAuthentication):
		return APIError{Status: http.StatusUnauthorized, Code: "authentication_required"}
	default:
		return APIError{Status: http.StatusInternalServerError, Code: "internal_error"}
	}
}
