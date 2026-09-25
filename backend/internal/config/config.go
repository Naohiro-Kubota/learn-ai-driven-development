package config

import (
	"encoding/base64"
	"fmt"
	"math"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL, ListenAddress, FrontendOrigin string
	OIDCIssuer, OIDCClientID, OIDCRedirectURI  string
	OIDCInternalAddress                        string
	LocalCompose                               bool
	CookieSecure                               bool
	AuthTransactionKey                         [32]byte
	SessionIdleTTL, SessionAbsoluteTTL         time.Duration
	AuthTransactionTTL                         time.Duration
	OTLPEndpoint                               string
	OTLPTimeout                                time.Duration
	TraceSampleRatio                           float64
}

func Load(lookup func(string) string) (Config, error) {
	cfg := Config{
		DatabaseURL:     lookup("DATABASE_URL"),
		ListenAddress:   lookup("APP_LISTEN_ADDR"),
		FrontendOrigin:  lookup("APP_FRONTEND_ORIGIN"),
		OIDCIssuer:      lookup("OIDC_ISSUER"),
		OIDCClientID:    lookup("OIDC_CLIENT_ID"),
		OIDCRedirectURI: lookup("OIDC_REDIRECT_URI"),
	}
	for name, value := range map[string]string{
		"DATABASE_URL": cfg.DatabaseURL, "APP_LISTEN_ADDR": cfg.ListenAddress,
		"APP_FRONTEND_ORIGIN": cfg.FrontendOrigin, "OIDC_ISSUER": cfg.OIDCIssuer,
		"OIDC_CLIENT_ID": cfg.OIDCClientID, "OIDC_REDIRECT_URI": cfg.OIDCRedirectURI,
	} {
		if strings.TrimSpace(value) == "" {
			return Config{}, fmt.Errorf("%s is required", name)
		}
	}
	parsedOrigin, err := parseOrigin(cfg.FrontendOrigin)
	if err != nil {
		return Config{}, fmt.Errorf("APP_FRONTEND_ORIGIN %w", err)
	}
	cfg.FrontendOrigin = parsedOrigin.String()
	composeMode := lookup("APP_LOCAL_COMPOSE")
	if composeMode != "" && composeMode != "true" {
		return Config{}, fmt.Errorf("APP_LOCAL_COMPOSE must be true or unset")
	}
	cfg.LocalCompose = composeMode == "true"
	cfg.OIDCInternalAddress = lookup("OIDC_INTERNAL_ADDR")
	if cfg.LocalCompose {
		if lookup("APP_ENV") != "development" || !isLoopbackOrigin(cfg.FrontendOrigin) || cfg.OIDCIssuer != "http://127.0.0.1:8081/realms/approval-flow-dev" || cfg.OIDCInternalAddress != "keycloak:8080" {
			return Config{}, fmt.Errorf("local Compose network settings require approved development endpoints")
		}
	} else if cfg.OIDCInternalAddress != "" {
		return Config{}, fmt.Errorf("OIDC_INTERNAL_ADDR requires local Compose mode")
	}
	secure, err := parseBool(lookup("APP_COOKIE_SECURE"))
	if err != nil {
		return Config{}, err
	}
	cfg.CookieSecure = secure
	if !secure && (lookup("APP_ENV") != "development" || !(isLoopback(cfg.ListenAddress) || cfg.LocalCompose && cfg.ListenAddress == "0.0.0.0:8080") || !isLoopbackOrigin(cfg.FrontendOrigin)) {
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
	cfg.OTLPEndpoint = lookup("APP_OTLP_ENDPOINT")
	cfg.TraceSampleRatio = 0.1
	if value := lookup("APP_TRACE_SAMPLE_RATIO"); value != "" {
		cfg.TraceSampleRatio, err = strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(cfg.TraceSampleRatio) || cfg.TraceSampleRatio < 0 || cfg.TraceSampleRatio > 1 {
			return Config{}, fmt.Errorf("APP_TRACE_SAMPLE_RATIO must be between 0 and 1")
		}
	}
	if cfg.OTLPEndpoint == "" {
		if lookup("APP_OTLP_TIMEOUT") != "" {
			return Config{}, fmt.Errorf("APP_OTLP_TIMEOUT requires APP_OTLP_ENDPOINT")
		}
		return cfg, nil
	}
	endpoint, err := url.Parse(cfg.OTLPEndpoint)
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" || endpoint.User != nil || endpoint.Path != "" || endpoint.RawQuery != "" || endpoint.Fragment != "" || strings.TrimSpace(cfg.OTLPEndpoint) != cfg.OTLPEndpoint {
		return Config{}, fmt.Errorf("APP_OTLP_ENDPOINT must be an HTTP(S) origin without credentials")
	}
	cfg.OTLPTimeout = 3 * time.Second
	if value := lookup("APP_OTLP_TIMEOUT"); value != "" {
		cfg.OTLPTimeout, err = time.ParseDuration(value)
		if err != nil || cfg.OTLPTimeout <= 0 || cfg.OTLPTimeout > 10*time.Second {
			return Config{}, fmt.Errorf("APP_OTLP_TIMEOUT must be greater than zero and at most 10s")
		}
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
	parsed, err := parseOrigin(raw)
	return err == nil && (parsed.Hostname() == "localhost" || net.ParseIP(parsed.Hostname()).IsLoopback())
}

func isLoopbackOrigin(raw string) bool {
	return isLoopbackURL(raw)
}

func parseOrigin(raw string) (*url.URL, error) {
	if strings.TrimSpace(raw) != raw {
		return nil, fmt.Errorf("origin must not contain surrounding whitespace")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("origin must be an absolute HTTP(S) origin")
	}
	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "" {
		return nil, fmt.Errorf("origin must be an absolute HTTP(S) origin")
	}
	host := hostname
	if strings.Contains(hostname, ":") {
		host = "[" + hostname + "]"
	}
	port := parsed.Port()
	if (parsed.Scheme == "http" && port == "80") || (parsed.Scheme == "https" && port == "443") {
		port = ""
	}
	if port != "" {
		host += ":" + port
	}
	return &url.URL{Scheme: strings.ToLower(parsed.Scheme), Host: host}, nil
}
