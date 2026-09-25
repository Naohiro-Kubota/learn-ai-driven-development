package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/config"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type oidcStoreFake struct {
	created      AuthTransaction
	consumeCalls int
	consumed     bool
	members      []string
	membersErr   error
	session      SessionInput
	selection    OrganizationSelectionInput
}

func (f *oidcStoreFake) ConsumeAuthTransaction(_ context.Context, cookie, state string, _ time.Time) (AuthTransaction, error) {
	f.consumeCalls++
	if f.consumed {
		return AuthTransaction{}, ErrConsumed
	}
	f.consumed = true
	if cookie != f.created.Cookie {
		return AuthTransaction{}, ErrNotFound
	}
	if state != f.created.State {
		return AuthTransaction{}, ErrForbidden
	}
	return f.created, nil
}
func (f *oidcStoreFake) CreateAuthTransaction(_ context.Context, input AuthTransaction) error {
	f.created = input
	return nil
}
func (*oidcStoreFake) IdentityID(context.Context, string, string) (string, error) {
	return "identity-1", nil
}
func (f *oidcStoreFake) MembersForIdentity(context.Context, string, string) ([]string, error) {
	return f.members, f.membersErr
}
func (f *oidcStoreFake) CreateSession(_ context.Context, session SessionInput) error {
	f.session = session
	return nil
}
func (f *oidcStoreFake) CreateOrganizationSelection(_ context.Context, selection OrganizationSelectionInput) error {
	f.selection = selection
	return nil
}

func TestCompleteLoginRequiresTransaction(t *testing.T) {
	a := &Authenticator{}
	_, err := a.CompleteLogin(context.Background(), CallbackInput{TransactionCookie: "missing", State: "state", Code: "code"})
	if err == nil {
		t.Fatal("missing transaction was accepted")
	}
	if errors.Is(err, ErrInvalidAuthentication) {
		t.Fatalf("configuration failure was classified as an expected authentication rejection: %v", err)
	}
}

func TestBeginLoginDoesNotReuseBrowserSecretsAsRecordID(t *testing.T) {
	store := &oidcStoreFake{}
	a := &Authenticator{
		transactions: store,
		now:          func() time.Time { return time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC) },
		config:       config.Config{AuthTransactionTTL: time.Minute},
		verifier:     &oidc.IDTokenVerifier{},
	}
	start, err := a.BeginLogin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if store.created.ID == start.TransactionCookie || store.created.ID == store.created.State || store.created.ID == store.created.Nonce {
		t.Fatalf("record ID must be distinct from browser secrets: %#v", store.created)
	}
}

func TestCompleteLoginConsumesTransactionBeforeRejectingMissingCode(t *testing.T) {
	store := &oidcStoreFake{created: AuthTransaction{Cookie: "cookie", State: "state"}}
	a := &Authenticator{transactions: store, verifier: &oidc.IDTokenVerifier{}}
	_, err := a.CompleteLogin(context.Background(), CallbackInput{TransactionCookie: "cookie", State: "state"})
	if !errors.Is(err, ErrInvalidAuthentication) {
		t.Fatalf("err = %v, want invalid authentication", err)
	}
	if store.consumeCalls != 1 {
		t.Fatalf("consume calls = %d, want 1", store.consumeCalls)
	}
}

func TestCompleteLoginClassifiesMissingIDTokenAsInvalidAuthentication(t *testing.T) {
	store := &oidcStoreFake{}
	a, ctx := localCallbackAuthenticator(t, store, `{"access_token":"access","token_type":"Bearer"}`)

	_, err := a.CompleteLogin(ctx, CallbackInput{TransactionCookie: "cookie", State: "state", Code: "valid"})
	if !errors.Is(err, ErrInvalidAuthentication) {
		t.Fatalf("err = %v, want invalid authentication", err)
	}
}

func TestCompleteLoginClassifiesInvalidIDTokenAsInvalidAuthentication(t *testing.T) {
	store := &oidcStoreFake{}
	a, ctx := localCallbackAuthenticator(t, store, `{"access_token":"access","token_type":"Bearer","id_token":"not-a-jwt"}`)

	_, err := a.CompleteLogin(ctx, CallbackInput{TransactionCookie: "cookie", State: "state", Code: "valid"})
	if !errors.Is(err, ErrInvalidAuthentication) {
		t.Fatalf("err = %v, want invalid authentication", err)
	}
}

func TestCompleteLoginClassifiesInvalidNonceAndSubjectAsInvalidAuthentication(t *testing.T) {
	for _, tc := range []struct {
		name   string
		claims map[string]any
	}{
		{name: "nonce", claims: map[string]any{"nonce": "wrong", "sub": "subject-1"}},
		{name: "subject", claims: map[string]any{"nonce": "nonce", "sub": ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &oidcStoreFake{}
			a, ctx := localCallbackAuthenticator(t, store, callbackTokenResponse(t, tc.claims))

			_, err := a.CompleteLogin(ctx, CallbackInput{TransactionCookie: "cookie", State: "state", Code: "valid"})
			if !errors.Is(err, ErrInvalidAuthentication) {
				t.Fatalf("err = %v, want invalid authentication", err)
			}
		})
	}
}

func TestCompleteLoginPreservesMembershipLookupFailure(t *testing.T) {
	databaseErr := errors.New("database unavailable")
	store := &oidcStoreFake{membersErr: databaseErr}
	a, ctx := localCallbackAuthenticator(t, store, callbackTokenResponse(t, nil))

	_, err := a.CompleteLogin(ctx, CallbackInput{TransactionCookie: "cookie", State: "state", Code: "valid"})
	if !errors.Is(err, databaseErr) {
		t.Fatalf("err = %v, want database failure", err)
	}
	if errors.Is(err, ErrInvalidAuthentication) || errors.Is(err, ErrNotFound) {
		t.Fatalf("database failure was classified as an expected authentication rejection: %v", err)
	}
}

func TestCompleteLoginDoesNotClassifyTokenExchangeTransportFailureAsInvalidAuthentication(t *testing.T) {
	store := &oidcStoreFake{}
	a, _ := localCallbackAuthenticator(t, store, "")
	transportErr := errors.New("identity provider unavailable")
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, transportErr
	})}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, client)

	_, err := a.CompleteLogin(ctx, CallbackInput{TransactionCookie: "cookie", State: "state", Code: "valid"})
	if err == nil {
		t.Fatal("transport failure was accepted")
	}
	if errors.Is(err, ErrInvalidAuthentication) {
		t.Fatalf("transport failure was classified as an expected authentication rejection: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func localCallbackAuthenticator(t *testing.T, store *oidcStoreFake, tokenResponse string) (*Authenticator, context.Context) {
	t.Helper()
	const issuer = "https://issuer.example"
	var key [32]byte
	encryptedVerifier, err := EncryptVerifier(key, []byte("pkce-verifier"))
	if err != nil {
		t.Fatal(err)
	}
	store.created = AuthTransaction{
		Cookie:            "cookie",
		State:             "state",
		Nonce:             "nonce",
		EncryptedVerifier: encryptedVerifier,
		Issuer:            issuer,
		ClientID:          "client",
		RedirectURI:       "https://app.example/callback",
		ExpiresAt:         time.Now().Add(time.Minute),
	}
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(tokenResponse)),
			Request:    request,
		}, nil
	})}
	a := &Authenticator{
		transactions: store,
		config: config.Config{
			OIDCIssuer:         issuer,
			OIDCClientID:       "client",
			OIDCRedirectURI:    "https://app.example/callback",
			AuthTransactionKey: key,
		},
		oauth:    oauth2.Config{ClientID: "client", Endpoint: oauth2.Endpoint{TokenURL: issuer + "/token"}, RedirectURL: "https://app.example/callback"},
		verifier: oidc.NewVerifier(issuer, nil, &oidc.Config{ClientID: "client", InsecureSkipSignatureCheck: true}),
	}
	return a, context.WithValue(context.Background(), oauth2.HTTPClient, client)
}

func callbackTokenResponse(t *testing.T, claimOverrides map[string]any) string {
	t.Helper()
	claims := map[string]any{
		"iss":   "https://issuer.example",
		"aud":   "client",
		"exp":   time.Now().Add(time.Minute).Unix(),
		"iat":   time.Now().Unix(),
		"nonce": "nonce",
		"sub":   "subject-1",
	}
	for name, value := range claimOverrides {
		claims[name] = value
	}
	header, err := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	rawToken := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString([]byte("signature"))
	response, err := json.Marshal(map[string]any{"access_token": "access", "token_type": "Bearer", "id_token": rawToken})
	if err != nil {
		t.Fatal(err)
	}
	return string(response)
}

func TestBeginLoginRequestsOpenIDScope(t *testing.T) {
	provider := newOIDCTestProvider(t)
	defer provider.server.Close()
	store := &oidcStoreFake{}
	a, err := NewOIDCAuthenticator(context.Background(), provider.config(), store, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	start, err := a.BeginLogin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	authorizationURL, err := url.Parse(start.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(strings.Fields(authorizationURL.Query().Get("scope")), "openid") {
		t.Fatal("authorization scope does not include openid")
	}
}

func TestComposeOIDCTransportUsesInternalDialAndRetainsPublicIssuer(t *testing.T) {
	provider := newOIDCTestProvider(t)
	defer provider.server.Close()
	provider.issuerOverride = "http://127.0.0.1:8081/realms/approval-flow-dev"
	store := &oidcStoreFake{members: []string{"member-1"}}
	cfg := provider.config()
	cfg.OIDCIssuer = provider.issuerOverride
	cfg.OIDCInternalAddress = strings.TrimPrefix(provider.server.URL, "http://")
	a, err := NewOIDCAuthenticator(context.Background(), cfg, store, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	start, err := a.BeginLogin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(start.AuthorizationURL, provider.issuerOverride+"/authorize") {
		t.Fatalf("authorization URL = %q", start.AuthorizationURL)
	}
	provider.setChallenge(t, start.AuthorizationURL)
	provider.token = provider.signedToken(t, store.created.Nonce, nil)
	result, err := a.CompleteLogin(context.Background(), CallbackInput{TransactionCookie: start.TransactionCookie, State: store.created.State, Code: "valid"})
	if err != nil || result.Session == nil {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if provider.lastHost != "127.0.0.1:8081" {
		t.Fatalf("Host header = %q", provider.lastHost)
	}
}

func TestOIDCCompleteLoginVerifiesTokenAndBranchesByMembershipCount(t *testing.T) {
	provider := newOIDCTestProvider(t)
	defer provider.server.Close()
	for _, tc := range []struct {
		name    string
		members []string
	}{
		{name: "single", members: []string{"member-1"}},
		{name: "multiple", members: []string{"member-1", "member-2"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &oidcStoreFake{members: tc.members}
			a, err := NewOIDCAuthenticator(context.Background(), provider.config(), store, time.Now)
			if err != nil {
				t.Fatal(err)
			}
			start, err := a.BeginLogin(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			provider.setChallenge(t, start.AuthorizationURL)
			provider.token = provider.signedToken(t, store.created.Nonce, nil)
			result, err := a.CompleteLogin(context.Background(), CallbackInput{TransactionCookie: start.TransactionCookie, State: store.created.State, Code: "valid"})
			if err != nil {
				t.Fatal(err)
			}
			if provider.verifier == "" {
				t.Fatal("PKCE verifier was not sent")
			}
			if len(tc.members) == 1 && (result.Session == nil || result.Selection != nil || store.session.ID == store.session.Cookie) {
				t.Fatalf("single membership result = %#v", result)
			}
			if len(tc.members) > 1 && (result.Session != nil || result.Selection == nil || store.selection.ID == store.selection.Cookie) {
				t.Fatalf("multiple membership result = %#v", result)
			}
		})
	}
}

func TestOIDCCompleteLoginRejectsInvalidTokenClaimsAndConsumesTransaction(t *testing.T) {
	provider := newOIDCTestProvider(t)
	defer provider.server.Close()
	for _, tc := range []struct {
		name   string
		claims map[string]any
	}{
		{name: "nonce", claims: map[string]any{"nonce": "wrong"}},
		{name: "issuer", claims: map[string]any{"iss": "https://wrong.example"}},
		{name: "audience", claims: map[string]any{"aud": "wrong-client"}},
		{name: "expiry", claims: map[string]any{"exp": time.Now().Add(-time.Minute).Unix()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &oidcStoreFake{members: []string{"member-1"}}
			a, err := NewOIDCAuthenticator(context.Background(), provider.config(), store, time.Now)
			if err != nil {
				t.Fatal(err)
			}
			start, err := a.BeginLogin(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			provider.setChallenge(t, start.AuthorizationURL)
			provider.token = provider.signedToken(t, store.created.Nonce, tc.claims)
			if _, err := a.CompleteLogin(context.Background(), CallbackInput{TransactionCookie: start.TransactionCookie, State: store.created.State, Code: "valid"}); !errors.Is(err, ErrInvalidAuthentication) {
				t.Fatalf("err = %v, want invalid authentication", err)
			}
			if !store.consumed || store.session.ID != "" {
				t.Fatalf("consumed=%v session=%#v", store.consumed, store.session)
			}
		})
	}
}

func TestOIDCTestProviderRejectsPKCEVerifierMismatch(t *testing.T) {
	provider := newOIDCTestProvider(t)
	defer provider.server.Close()
	store := &oidcStoreFake{}
	a, err := NewOIDCAuthenticator(context.Background(), provider.config(), store, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	start, err := a.BeginLogin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	provider.setChallenge(t, start.AuthorizationURL)
	response, err := http.PostForm(provider.server.URL+"/token", url.Values{"code": {"valid"}, "code_verifier": {"wrong-verifier"}})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusBadRequest)
	}
}

func TestOIDCCompleteLoginConsumesTransactionWhenPKCERejected(t *testing.T) {
	provider := newOIDCTestProvider(t)
	defer provider.server.Close()
	store := &oidcStoreFake{members: []string{"member-1"}}
	a, err := NewOIDCAuthenticator(context.Background(), provider.config(), store, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	start, err := a.BeginLogin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	provider.setChallenge(t, start.AuthorizationURL)
	provider.challenge = "wrong-challenge"
	provider.token = provider.signedToken(t, store.created.Nonce, nil)
	if _, err := a.CompleteLogin(context.Background(), CallbackInput{TransactionCookie: start.TransactionCookie, State: store.created.State, Code: "valid"}); err == nil {
		t.Fatal("PKCE mismatch was accepted")
	}
	if !store.consumed || store.session.ID != "" {
		t.Fatalf("consumed=%v session=%#v", store.consumed, store.session)
	}
}

type oidcTestProvider struct {
	server                   *httptest.Server
	private                  *rsa.PrivateKey
	token, verifier          string
	challenge                string
	issuerOverride, lastHost string
}

func newOIDCTestProvider(t *testing.T) *oidcTestProvider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := &oidcTestProvider{private: key}
	p.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.lastHost = r.Host
		w.Header().Set("Content-Type", "application/json")
		issuer := p.server.URL
		path := r.URL.Path
		if p.issuerOverride != "" {
			issuer = p.issuerOverride
			path = strings.TrimPrefix(path, "/realms/approval-flow-dev")
		}
		switch path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]string{"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/jwks"})
		case "/jwks":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{"kty": "RSA", "kid": "test", "use": "sig", "alg": "RS256", "n": base64.RawURLEncoding.EncodeToString(p.private.PublicKey.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString([]byte{1, 0, 1})}}})
		case "/token":
			_ = r.ParseForm()
			p.verifier = r.Form.Get("code_verifier")
			verifierHash := sha256.Sum256([]byte(p.verifier))
			if p.verifier == "" || r.Form.Get("code") != "valid" || p.challenge == "" || base64.RawURLEncoding.EncodeToString(verifierHash[:]) != p.challenge {
				http.Error(w, "invalid grant", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "access", "token_type": "Bearer", "id_token": p.token})
		default:
			http.NotFound(w, r)
		}
	}))
	return p
}
func (p *oidcTestProvider) config() config.Config {
	return config.Config{OIDCIssuer: p.server.URL, OIDCClientID: "client", OIDCRedirectURI: "https://app.example/callback", AuthTransactionTTL: time.Minute, SessionIdleTTL: time.Hour, SessionAbsoluteTTL: 2 * time.Hour}
}
func (p *oidcTestProvider) setChallenge(t *testing.T, rawAuthorizationURL string) {
	t.Helper()
	authorizationURL, err := url.Parse(rawAuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	p.challenge = authorizationURL.Query().Get("code_challenge")
	if p.challenge == "" || authorizationURL.Query().Get("code_challenge_method") != "S256" {
		t.Fatalf("authorization URL has invalid PKCE parameters: %s", rawAuthorizationURL)
	}
}
func (p *oidcTestProvider) signedToken(t *testing.T, nonce string, overrides map[string]any) string {
	t.Helper()
	issuer := p.server.URL
	if p.issuerOverride != "" {
		issuer = p.issuerOverride
	}
	claims := map[string]any{"iss": issuer, "aud": "client", "exp": time.Now().Add(time.Minute).Unix(), "iat": time.Now().Unix(), "nonce": nonce, "sub": "subject-1"}
	for k, v := range overrides {
		claims[k] = v
	}
	header, err := json.Marshal(map[string]string{"alg": "RS256", "kid": "test", "typ": "JWT"})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, p.private, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
}
