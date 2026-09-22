package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/config"
	"github.com/coreos/go-oidc/v3/oidc"
)

type oidcStoreFake struct {
	created      AuthTransaction
	consumeCalls int
	consumed     bool
	members      []string
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
	return f.members, nil
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
	store := &oidcStoreFake{}
	a := &Authenticator{transactions: store, verifier: &oidc.IDTokenVerifier{}}
	_, err := a.CompleteLogin(context.Background(), CallbackInput{TransactionCookie: "cookie", State: "state"})
	if err == nil {
		t.Fatal("missing authorization code was accepted")
	}
	if store.consumeCalls != 1 {
		t.Fatalf("consume calls = %d, want 1", store.consumeCalls)
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
			if _, err := a.CompleteLogin(context.Background(), CallbackInput{TransactionCookie: start.TransactionCookie, State: store.created.State, Code: "valid"}); err == nil {
				t.Fatal("invalid token was accepted")
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
	server          *httptest.Server
	private         *rsa.PrivateKey
	token, verifier string
	challenge       string
}

func newOIDCTestProvider(t *testing.T) *oidcTestProvider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := &oidcTestProvider{private: key}
	p.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]string{"issuer": p.server.URL, "authorization_endpoint": p.server.URL + "/authorize", "token_endpoint": p.server.URL + "/token", "jwks_uri": p.server.URL + "/jwks"})
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
	claims := map[string]any{"iss": p.server.URL, "aud": "client", "exp": time.Now().Add(time.Minute).Unix(), "iat": time.Now().Unix(), "nonce": nonce, "sub": "subject-1"}
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
