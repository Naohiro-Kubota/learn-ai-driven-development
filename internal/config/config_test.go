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
		"APP_FRONTEND_ORIGIN":  "http://127.0.0.1:5173",
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
	for _, test := range []struct {
		name, key, value string
	}{
		{"production", "APP_ENV", "production"},
		{"non-loopback listener", "APP_LISTEN_ADDR", "0.0.0.0:8080"},
		{"non-loopback frontend", "APP_FRONTEND_ORIGIN", "http://app.example.test"},
	} {
		t.Run(test.name, func(t *testing.T) {
			original := values[test.key]
			values[test.key] = test.value
			t.Cleanup(func() { values[test.key] = original })
			if _, err := Load(func(key string) string { return values[key] }); err == nil {
				t.Fatal("Load() accepted insecure cookies outside loopback development")
			}
		})
	}
}

func TestLoadReadsFrontendOriginAndDoesNotUseAlias(t *testing.T) {
	values := testEnvironment()
	values["APP_FRONTEND_ORIGIN"] = "https://app.example.test"
	values["APP_ALLOWED_ORIGIN"] = "https://legacy.example.test"
	cfg, err := Load(func(key string) string { return values[key] })
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.FrontendOrigin != "https://app.example.test" {
		t.Fatalf("FrontendOrigin = %q", cfg.FrontendOrigin)
	}
	delete(values, "APP_FRONTEND_ORIGIN")
	if _, err := Load(func(key string) string { return values[key] }); err == nil {
		t.Fatal("Load() accepted APP_ALLOWED_ORIGIN without APP_FRONTEND_ORIGIN")
	}
}

func TestLoadRejectsInvalidFrontendOrigin(t *testing.T) {
	for _, origin := range []string{
		"http://",
		"ftp://app.example.test",
		"https://user:pass@app.example.test",
		"https://app.example.test/path",
		"https://app.example.test?query=1",
		"https://app.example.test#fragment",
		" https://app.example.test",
		"https://app.example.test ",
		"https://app.example.test https://other.example.test",
	} {
		t.Run(origin, func(t *testing.T) {
			values := testEnvironment()
			values["APP_FRONTEND_ORIGIN"] = origin
			if _, err := Load(func(key string) string { return values[key] }); err == nil {
				t.Fatalf("Load() accepted invalid frontend origin %q", origin)
			}
		})
	}
}

func TestLoadCanonicalizesFrontendOriginForBrowserSerialization(t *testing.T) {
	for _, test := range []struct {
		name string
		in   string
		want string
	}{
		{name: "http default port and host case", in: "HTTP://APP.EXAMPLE.TEST:80", want: "http://app.example.test"},
		{name: "https default port and host case", in: "HTTPS://APP.EXAMPLE.TEST:443", want: "https://app.example.test"},
	} {
		t.Run(test.name, func(t *testing.T) {
			values := testEnvironment()
			values["APP_FRONTEND_ORIGIN"] = test.in
			cfg, err := Load(func(key string) string { return values[key] })
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.FrontendOrigin != test.want {
				t.Fatalf("FrontendOrigin = %q, want %q", cfg.FrontendOrigin, test.want)
			}
		})
	}
}

func testEnvironment() map[string]string {
	return map[string]string{
		"APP_ENV":              "production",
		"APP_COOKIE_SECURE":    "true",
		"APP_LISTEN_ADDR":      "127.0.0.1:8080",
		"APP_FRONTEND_ORIGIN":  "https://app.example.test",
		"DATABASE_URL":         "postgres://test",
		"OIDC_ISSUER":          "https://identity.example.test",
		"OIDC_CLIENT_ID":       "approval-flow",
		"OIDC_REDIRECT_URI":    "https://app.example.test/auth/oidc/callback",
		"AUTH_TRANSACTION_KEY": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		"SESSION_IDLE_TTL":     "15m",
		"SESSION_ABSOLUTE_TTL": "8h",
		"AUTH_TRANSACTION_TTL": "5m",
	}
}
