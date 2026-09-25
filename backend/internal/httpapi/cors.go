package httpapi

import (
	"net/http"
	"strings"
)

const (
	allowedCORSMethods = "GET, POST, PATCH, OPTIONS"
	allowedCORSHeaders = "Content-Type, X-CSRF-Token"
)

// NewCORS applies the single-origin credentialed CORS policy before route dispatch.
func NewCORS(allowedOrigin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		originValues := request.Header.Values("Origin")
		if len(originValues) == 0 {
			next.ServeHTTP(w, request)
			return
		}
		if len(originValues) != 1 || originValues[0] != allowedOrigin {
			WriteError(w, APIError{Status: http.StatusForbidden, Code: "csrf_validation_failed"})
			return
		}
		if request.Method == http.MethodOptions {
			if !allowedCORSPreflight(request) {
				WriteError(w, APIError{Status: http.StatusForbidden, Code: "csrf_validation_failed"})
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			appendVaryToken(w.Header(), "Origin")
			w.Header().Set("Access-Control-Allow-Methods", allowedCORSMethods)
			w.Header().Set("Access-Control-Allow-Headers", allowedCORSHeaders)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
		appendVaryToken(w.Header(), "Origin")
		next.ServeHTTP(w, request)
	})
}

func appendVaryToken(header http.Header, token string) {
	for _, value := range header.Values("Vary") {
		for _, existing := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(existing), token) {
				return
			}
		}
	}

	values := header.Values("Vary")
	values = append(values, token)
	header.Set("Vary", strings.Join(values, ", "))
}

func allowedCORSPreflight(request *http.Request) bool {
	methodValues := request.Header.Values("Access-Control-Request-Method")
	if len(methodValues) != 1 {
		return false
	}
	method := methodValues[0]
	if method != http.MethodGet && method != http.MethodPost && method != http.MethodPatch {
		return false
	}
	headerValues := request.Header.Values("Access-Control-Request-Headers")
	if len(headerValues) > 1 {
		return false
	}
	if len(headerValues) == 0 || headerValues[0] == "" {
		return true
	}
	for _, header := range strings.Split(headerValues[0], ",") {
		switch strings.ToLower(strings.TrimSpace(header)) {
		case "content-type", "x-csrf-token":
		default:
			return false
		}
	}
	return true
}
