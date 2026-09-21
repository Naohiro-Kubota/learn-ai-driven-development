package config

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL, ListenAddress, AllowedOrigin string
	OIDCIssuer, OIDCClientID, OIDCRedirectURI string
	CookieSecure                              bool
	AuthTransactionKey                        [32]byte
	SessionIdleTTL, SessionAbsoluteTTL        time.Duration
	AuthTransactionTTL                        time.Duration
}

func Load(lookup func(string) string) (Config, error) {
	cfg := Config{
		DatabaseURL:     lookup("DATABASE_URL"),
		ListenAddress:   lookup("APP_LISTEN_ADDR"),
		AllowedOrigin:   lookup("APP_ALLOWED_ORIGIN"),
		OIDCIssuer:      lookup("OIDC_ISSUER"),
		OIDCClientID:    lookup("OIDC_CLIENT_ID"),
		OIDCRedirectURI: lookup("OIDC_REDIRECT_URI"),
	}
	for name, value := range map[string]string{
		"DATABASE_URL": cfg.DatabaseURL, "APP_LISTEN_ADDR": cfg.ListenAddress,
		"APP_ALLOWED_ORIGIN": cfg.AllowedOrigin, "OIDC_ISSUER": cfg.OIDCIssuer,
		"OIDC_CLIENT_ID": cfg.OIDCClientID, "OIDC_REDIRECT_URI": cfg.OIDCRedirectURI,
	} {
		if strings.TrimSpace(value) == "" {
			return Config{}, fmt.Errorf("%s is required", name)
		}
	}
	secure, err := parseBool(lookup("APP_COOKIE_SECURE"))
	if err != nil {
		return Config{}, err
	}
	cfg.CookieSecure = secure
	if !secure && (lookup("APP_ENV") != "development" || !isLoopback(cfg.ListenAddress) || !isLoopbackURL(cfg.AllowedOrigin)) {
		return Config{}, fmt.Errorf("insecure cookies require loopback development")
	}
	key, err := base64.StdEncoding.DecodeString(lookup("AUTH_TRANSACTION_KEY"))
	if err != nil || len(key) != len(cfg.AuthTransactionKey) {
		return Config{}, fmt.Errorf("AUTH_TRANSACTION_KEY must be base64-encoded 32 bytes")
	}
	copy(cfg.AuthTransactionKey[:], key)
	if cfg.SessionIdleTTL, err = positiveDuration("SESSION_IDLE_TTL", lookup); err != nil {
		return Config{}, err
	}
	if cfg.SessionAbsoluteTTL, err = positiveDuration("SESSION_ABSOLUTE_TTL", lookup); err != nil {
		return Config{}, err
	}
	if cfg.AuthTransactionTTL, err = positiveDuration("AUTH_TRANSACTION_TTL", lookup); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func parseBool(value string) (bool, error) {
	if value == "true" {
		return true, nil
	}
	if value == "false" {
		return false, nil
	}
	return false, fmt.Errorf("APP_COOKIE_SECURE must be true or false")
}
func positiveDuration(name string, lookup func(string) string) (time.Duration, error) {
	d, err := time.ParseDuration(lookup(name))
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return d, nil
}
func isLoopback(address string) bool {
	host, _, err := net.SplitHostPort(address)
	return err == nil && (host == "localhost" || net.ParseIP(host).IsLoopback())
}
func isLoopbackURL(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && (parsed.Hostname() == "localhost" || net.ParseIP(parsed.Hostname()).IsLoopback())
}
