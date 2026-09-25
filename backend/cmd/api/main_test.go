package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/auth"
	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/config"
	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/store/postgres"
)

func TestStartupFailureLogDoesNotExposeError(t *testing.T) {
	var output bytes.Buffer
	logStartupFailure(slog.New(slog.NewJSONHandler(&output, nil)), errors.New("password=secret-value"))
	if strings.Contains(output.String(), "secret-value") {
		t.Fatalf("startup log exposed secret error details: %q", output.String())
	}
	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil || entry["msg"] != "api stopped" {
		t.Fatalf("startup log is not expected JSON: %s, err=%v", output.String(), err)
	}
}

func TestRunReturnsConfigurationError(t *testing.T) {
	err := run(context.Background(), func(string) string { return "" })
	if err == nil {
		t.Fatal("expected configuration error")
	}
}

func TestRunReturnsDatabaseInitializationError(t *testing.T) {
	wantErr := errors.New("database initialization failed")
	original := openDatabase
	openDatabase = func(string, string) (*sql.DB, error) { return nil, wantErr }
	t.Cleanup(func() { openDatabase = original })

	err := run(context.Background(), validEnvironment())
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want wrapped %v", err, wantErr)
	}
}

func TestNewHandlerReturnsOIDCInitializationError(t *testing.T) {
	wantErr := errors.New("OIDC initialization failed")
	original := newOIDCAuthenticator
	newOIDCAuthenticator = func(context.Context, config.Config, *postgres.Repository, func() time.Time) (*auth.Authenticator, error) {
		return nil, wantErr
	}
	t.Cleanup(func() { newOIDCAuthenticator = original })

	handler, err := newHandler(context.Background(), validConfig(), &sql.DB{})
	if handler != nil {
		t.Fatal("handler was constructed after OIDC initialization failed")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want wrapped %v", err, wantErr)
	}
}

func validConfig() config.Config {
	return config.Config{
		FrontendOrigin:     "https://app.example.test",
		OIDCIssuer:         "https://identity.example.test",
		OIDCClientID:       "approval-flow",
		OIDCRedirectURI:    "https://app.example.test/auth/oidc/callback",
		CookieSecure:       true,
		SessionIdleTTL:     30 * time.Minute,
		SessionAbsoluteTTL: 8 * time.Hour,
		AuthTransactionTTL: 5 * time.Minute,
	}
}

func validEnvironment() func(string) string {
	values := map[string]string{
		"APP_FRONTEND_ORIGIN":  "https://app.example.test",
		"APP_COOKIE_SECURE":    "true",
		"APP_LISTEN_ADDR":      "127.0.0.1:8080",
		"AUTH_TRANSACTION_KEY": base64.StdEncoding.EncodeToString(make([]byte, 32)),
		"DATABASE_URL":         "postgres://localhost/approval_flow",
		"OIDC_CLIENT_ID":       "approval-flow",
		"OIDC_ISSUER":          "https://identity.example.test",
		"OIDC_REDIRECT_URI":    "https://app.example.test/auth/oidc/callback",
		"SESSION_ABSOLUTE_TTL": "8h",
		"SESSION_IDLE_TTL":     "30m",
		"AUTH_TRANSACTION_TTL": "5m",
	}
	return func(name string) string { return values[name] }
}
