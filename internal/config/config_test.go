package config

import "testing"

func TestLoadRejectsProductionWithoutSecureCookie(t *testing.T) {
	_, err := Load(func(key string) string {
		return map[string]string{
			"APP_ENV":           "production",
			"APP_COOKIE_SECURE": "false",
		}[key]
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestLoadAllowsInsecureCookieOnlyForLoopbackDevelopment(t *testing.T) {
	values := map[string]string{
		"APP_ENV":              "development",
		"APP_COOKIE_SECURE":    "false",
		"APP_LISTEN_ADDR":      "127.0.0.1:8080",
		"APP_ALLOWED_ORIGIN":   "http://127.0.0.1:5173",
		"DATABASE_URL":         "postgres://test",
		"OIDC_ISSUER":          "http://127.0.0.1:8081/realms/dev",
		"OIDC_CLIENT_ID":       "approval-flow",
		"OIDC_REDIRECT_URI":    "http://127.0.0.1:8080/auth/oidc/callback",
		"AUTH_TRANSACTION_KEY": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		"SESSION_IDLE_TTL":     "15m",
		"SESSION_ABSOLUTE_TTL": "8h",
		"AUTH_TRANSACTION_TTL": "5m",
	}
	cfg, err := Load(func(key string) string { return values[key] })
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.CookieSecure {
		t.Fatal("CookieSecure = true")
	}
}
