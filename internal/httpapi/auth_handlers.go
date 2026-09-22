package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/auth"
)

const (
	transactionCookieName         = "approval_flow_auth_transaction"
	selectionCookieName           = "approval_flow_organization_selection"
	applicationLocation           = "/"
	organizationSelectionLocation = "/organization-selection"
)

func (r *router) login(w http.ResponseWriter, request *http.Request) {
	result, err := r.dependencies.Authenticator.BeginLogin(request.Context())
	if err != nil {
		WriteError(w, APIError{Status: http.StatusInternalServerError, Code: "internal_error"})
		return
	}
	now := r.dependencies.Now()
	cookie := r.flowCookie(transactionCookieName, result.TransactionCookie)
	setCookieExpiry(cookie, now, now.Add(r.dependencies.Config.AuthTransactionTTL))
	http.SetCookie(w, cookie)
	redirect(w, result.AuthorizationURL)
}

func (r *router) callback(w http.ResponseWriter, request *http.Request) {
	cookie := r.readFlowCookie(request, transactionCookieName)
	if cookie == "" {
		WriteError(w, APIError{Status: http.StatusBadRequest, Code: "invalid_auth_transaction"})
		return
	}
	r.clearFlowCookie(w, transactionCookieName)
	query, err := parseCallbackQuery(request)
	result, completeErr := r.dependencies.Authenticator.CompleteLogin(request.Context(), auth.CallbackInput{TransactionCookie: cookie, State: query.state, Code: query.code})
	// Even a provider denial or malformed callback reaches the authenticator with
	// no exchangeable code so that its recognized transaction is consumed.
	if err != nil || errors.Is(completeErr, auth.ErrForbidden) {
		WriteError(w, APIError{Status: http.StatusBadRequest, Code: "invalid_auth_transaction"})
		return
	}
	if completeErr != nil {
		WriteError(w, authAPIError(completeErr))
		return
	}
	switch {
	case result.Session != nil && result.Selection == nil:
		http.SetCookie(w, auth.SessionCookie(result.Session.Cookie, r.dependencies.Config.CookieSecure))
		redirect(w, applicationLocation)
	case result.Selection != nil && result.Session == nil:
		selection := r.flowCookie(selectionCookieName, result.Selection.Cookie)
		setCookieExpiry(selection, r.dependencies.Now(), result.Selection.ExpiresAt)
		http.SetCookie(w, selection)
		redirect(w, organizationSelectionLocation)
	default:
		WriteError(w, APIError{Status: http.StatusInternalServerError, Code: "internal_error"})
	}
}

type callbackQuery struct{ state, code string }

func parseCallbackQuery(request *http.Request) (callbackQuery, error) {
	values := request.URL.Query()
	query := callbackQuery{state: values.Get("state")}
	if len(values["state"]) != 1 {
		query.state = ""
	}
	if query.state == "" || len(values["code"]) != 1 || values.Get("code") == "" || values.Has("error") {
		return query, errors.New("invalid callback parameters")
	}
	query.code = values.Get("code")
	return query, nil
}

type organizationSelectionCandidate struct {
	MemberID         string `json:"memberId"`
	OrganizationID   string `json:"organizationId"`
	OrganizationName string `json:"organizationName"`
}

func (r *router) getOrganizationSelection(w http.ResponseWriter, request *http.Request) {
	cookie := r.readFlowCookie(request, selectionCookieName)
	if cookie == "" {
		WriteError(w, APIError{Status: http.StatusBadRequest, Code: "invalid_auth_transaction"})
		return
	}
	selection, err := r.dependencies.SelectionStore.ReadAndIssueCSRFToken(request.Context(), cookie, r.dependencies.Now())
	if err != nil {
		WriteError(w, authAPIError(err))
		return
	}
	candidates := make([]organizationSelectionCandidate, 0, len(selection.Candidates))
	for _, candidate := range selection.Candidates {
		candidates = append(candidates, organizationSelectionCandidate{candidate.MemberID, candidate.OrganizationID, candidate.OrganizationName})
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(struct {
		Candidates []organizationSelectionCandidate `json:"candidates"`
		CSRFToken  string                           `json:"csrfToken"`
	}{candidates, selection.CSRFToken})
}

func (r *router) selectOrganization(w http.ResponseWriter, request *http.Request) {
	cookie := r.readFlowCookie(request, selectionCookieName)
	if cookie == "" {
		WriteError(w, APIError{Status: http.StatusBadRequest, Code: "invalid_auth_transaction"})
		return
	}
	if !r.validOrigin(request) {
		WriteError(w, APIError{Status: http.StatusForbidden, Code: "csrf_validation_failed"})
		return
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, request.Body, 64<<10))
	memberID, err := decodeSelectedMemberID(decoder)
	if err != nil {
		WriteError(w, APIError{Status: http.StatusBadRequest, Code: "invalid_request"})
		return
	}
	session, err := r.dependencies.SelectionStore.Complete(request.Context(), auth.CompleteOrganizationSelectionInput{Cookie: cookie, CSRFToken: singleHeader(request.Header, "X-CSRF-Token"), MemberID: memberID, Now: r.dependencies.Now()})
	if err != nil {
		WriteError(w, authAPIError(err))
		return
	}
	r.clearFlowCookie(w, selectionCookieName)
	http.SetCookie(w, auth.SessionCookie(session.Cookie, r.dependencies.Config.CookieSecure))
	redirect(w, applicationLocation)
}

func decodeSelectedMemberID(decoder *json.Decoder) (string, error) {
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return "", errors.New("selection input must be an object")
	}
	var memberID string
	seenMemberID := false
	for decoder.More() {
		name, err := decoder.Token()
		if err != nil || name != "memberId" || seenMemberID {
			return "", errors.New("selection input has an unknown or duplicate property")
		}
		if err := decoder.Decode(&memberID); err != nil || memberID == "" {
			return "", errors.New("selection memberId must be a non-empty string")
		}
		seenMemberID = true
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') || !seenMemberID {
		return "", errors.New("selection input is incomplete")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return "", errors.New("selection input must contain one object")
	}
	return memberID, nil
}

func (r *router) flowCookie(name, value string) *http.Cookie {
	if r.dependencies.Config.CookieSecure {
		name = "__Host-" + name
	}
	return &http.Cookie{Name: name, Value: value, Secure: r.dependencies.Config.CookieSecure, HttpOnly: true, Path: "/", SameSite: http.SameSiteLaxMode}
}

func (r *router) readFlowCookie(request *http.Request, name string) string {
	cookies := request.CookiesNamed(r.flowCookie(name, "").Name)
	if len(cookies) != 1 {
		return ""
	}
	return cookies[0].Value
}

func (r *router) clearFlowCookie(w http.ResponseWriter, name string) {
	cookie := r.flowCookie(name, "")
	cookie.MaxAge = -1
	cookie.Expires = time.Unix(1, 0)
	http.SetCookie(w, cookie)
}

func setCookieExpiry(cookie *http.Cookie, now, expires time.Time) {
	cookie.Expires = expires
	cookie.MaxAge = max(1, int(expires.Sub(now).Seconds()))
}

func redirect(w http.ResponseWriter, location string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Location", location)
	w.WriteHeader(http.StatusFound)
}
