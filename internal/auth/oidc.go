package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/config"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type transactionStore interface {
	ConsumeAuthTransaction(context.Context, string, string, time.Time) (AuthTransaction, error)
	CreateAuthTransaction(context.Context, AuthTransaction) error
	IdentityID(context.Context, string, string) (string, error)
	MembersForIdentity(context.Context, string, string) ([]string, error)
	CreateSession(context.Context, SessionInput) error
	CreateOrganizationSelection(context.Context, OrganizationSelectionInput) error
}

type Authenticator struct {
	transactions transactionStore
	now          func() time.Time
	config       config.Config
	oauth        oauth2.Config
	verifier     *oidc.IDTokenVerifier
}

type CallbackInput struct {
	TransactionCookie string
	State             string
	Code              string
}

type LoginStart struct {
	AuthorizationURL, TransactionCookie string
}
type LoginResult struct {
	Session   *SessionInput
	Selection *OrganizationSelectionInput
}

func (a *Authenticator) BeginLogin(ctx context.Context) (LoginStart, error) {
	if a.verifier == nil || a.transactions == nil {
		return LoginStart{}, fmt.Errorf("authenticator is not configured")
	}
	state, err := oidcOpaque()
	if err != nil {
		return LoginStart{}, err
	}
	nonce, err := oidcOpaque()
	if err != nil {
		return LoginStart{}, err
	}
	cookie, err := oidcOpaque()
	if err != nil {
		return LoginStart{}, err
	}
	recordID, err := oidcOpaque()
	if err != nil {
		return LoginStart{}, err
	}
	verifier := oauth2.GenerateVerifier()
	encrypted, err := EncryptVerifier(a.config.AuthTransactionKey, []byte(verifier))
	if err != nil {
		return LoginStart{}, err
	}
	now := a.currentTime()
	transaction := AuthTransaction{ID: recordID, Cookie: cookie, State: state, Nonce: nonce, EncryptedVerifier: encrypted, Issuer: a.config.OIDCIssuer, ClientID: a.config.OIDCClientID, RedirectURI: a.config.OIDCRedirectURI, ExpiresAt: now.Add(a.config.AuthTransactionTTL)}
	if err := a.transactions.CreateAuthTransaction(ctx, transaction); err != nil {
		return LoginStart{}, err
	}
	return LoginStart{AuthorizationURL: a.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), TransactionCookie: cookie}, nil
}

func NewAuthenticator(transactions transactionStore, now func() time.Time) *Authenticator {
	return &Authenticator{transactions: transactions, now: now}
}

func NewOIDCAuthenticator(ctx context.Context, cfg config.Config, transactions transactionStore, now func() time.Time) (*Authenticator, error) {
	provider, err := oidc.NewProvider(ctx, cfg.OIDCIssuer)
	if err != nil {
		return nil, err
	}
	return &Authenticator{transactions: transactions, now: now, config: cfg, oauth: oauth2.Config{ClientID: cfg.OIDCClientID, Endpoint: provider.Endpoint(), RedirectURL: cfg.OIDCRedirectURI, Scopes: []string{oidc.ScopeOpenID}}, verifier: provider.Verifier(&oidc.Config{ClientID: cfg.OIDCClientID})}, nil
}

func (a *Authenticator) CompleteLogin(ctx context.Context, input CallbackInput) (LoginResult, error) {
	if a.transactions == nil || a.verifier == nil {
		return LoginResult{}, fmt.Errorf("authenticator is not configured")
	}
	transaction, err := a.transactions.ConsumeAuthTransaction(ctx, input.TransactionCookie, input.State, a.currentTime())
	if err != nil {
		return LoginResult{}, err
	}
	if input.Code == "" {
		return LoginResult{}, fmt.Errorf("%w: authorization code is required", ErrInvalidAuthentication)
	}
	if transaction.Issuer != a.config.OIDCIssuer || transaction.ClientID != a.config.OIDCClientID || transaction.RedirectURI != a.config.OIDCRedirectURI {
		return LoginResult{}, fmt.Errorf("auth transaction configuration mismatch")
	}
	verifier, err := DecryptVerifier(a.config.AuthTransactionKey, transaction.EncryptedVerifier)
	if err != nil {
		return LoginResult{}, err
	}
	token, err := a.oauth.Exchange(ctx, input.Code, oauth2.VerifierOption(string(verifier)))
	if err != nil {
		return LoginResult{}, fmt.Errorf("authorization code exchange failed")
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		return LoginResult{}, fmt.Errorf("%w: ID token missing", ErrInvalidAuthentication)
	}
	idToken, err := a.verifier.Verify(ctx, raw)
	if err != nil {
		return LoginResult{}, fmt.Errorf("%w: ID token invalid", ErrInvalidAuthentication)
	}
	var claims struct {
		Nonce   string `json:"nonce"`
		Subject string `json:"sub"`
	}
	if err := idToken.Claims(&claims); err != nil || claims.Nonce != transaction.Nonce {
		return LoginResult{}, fmt.Errorf("%w: ID token nonce invalid", ErrInvalidAuthentication)
	}
	if claims.Subject == "" {
		return LoginResult{}, fmt.Errorf("%w: ID token subject invalid", ErrInvalidAuthentication)
	}
	members, err := a.transactions.MembersForIdentity(ctx, idToken.Issuer, claims.Subject)
	if err != nil {
		return LoginResult{}, err
	}
	if len(members) == 0 {
		return LoginResult{}, ErrNotFound
	}
	if len(members) == 1 {
		cookie, err := oidcOpaque()
		if err != nil {
			return LoginResult{}, err
		}
		csrf, err := oidcOpaque()
		if err != nil {
			return LoginResult{}, err
		}
		sessionID, err := oidcOpaque()
		if err != nil {
			return LoginResult{}, err
		}
		now := a.currentTime()
		session := SessionInput{ID: sessionID, Cookie: cookie, CSRFToken: csrf, MemberID: members[0], CreatedAt: now, IdleExpiresAt: now.Add(a.config.SessionIdleTTL), AbsoluteExpiresAt: now.Add(a.config.SessionAbsoluteTTL)}
		if err := a.transactions.CreateSession(ctx, session); err != nil {
			return LoginResult{}, err
		}
		return LoginResult{Session: &session}, nil
	}
	identityID, err := a.transactions.IdentityID(ctx, idToken.Issuer, claims.Subject)
	if err != nil {
		return LoginResult{}, err
	}
	cookie, err := oidcOpaque()
	if err != nil {
		return LoginResult{}, err
	}
	csrf, err := oidcOpaque()
	if err != nil {
		return LoginResult{}, err
	}
	selectionID, err := oidcOpaque()
	if err != nil {
		return LoginResult{}, err
	}
	selection := OrganizationSelectionInput{ID: selectionID, Cookie: cookie, CSRFToken: csrf, IdentityID: identityID, MemberIDs: members, ExpiresAt: a.currentTime().Add(a.config.AuthTransactionTTL)}
	if err := a.transactions.CreateOrganizationSelection(ctx, selection); err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Selection: &selection}, nil
}

func (a *Authenticator) currentTime() time.Time {
	if a.now != nil {
		return a.now()
	}
	return time.Now()
}
func oidcOpaque() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
