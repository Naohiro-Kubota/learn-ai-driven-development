package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/domain"
)

const (
	productionSessionCookieName  = "__Host-approval_flow_session"
	developmentSessionCookieName = "approval_flow_session"
)

var (
	ErrNotFound              = fmt.Errorf("auth record not found")
	ErrExpired               = fmt.Errorf("auth record expired")
	ErrConsumed              = fmt.Errorf("auth record consumed")
	ErrForbidden             = fmt.Errorf("auth selection forbidden")
	ErrCSRFValidation        = fmt.Errorf("auth CSRF validation failed")
	ErrInvalidAuthentication = fmt.Errorf("authentication response invalid")
)

type OrganizationSelectionInput struct {
	ID, Cookie, CSRFToken, IdentityID string
	MemberIDs                         []string
	ExpiresAt                         time.Time
}
type SessionInput struct {
	ID, Cookie, CSRFToken, MemberID             string
	CreatedAt, IdleExpiresAt, AbsoluteExpiresAt time.Time
}
type ConsumeOrganizationSelectionInput struct {
	Cookie, CSRFToken, MemberID string
	Now                         time.Time
	Session                     SessionInput
}
type Session struct {
	ID, MemberID                                string
	CreatedAt, IdleExpiresAt, AbsoluteExpiresAt time.Time
}

type Principal struct {
	MemberID       string
	OrganizationID string
	Roles          []domain.Role
}

type AuthenticatedSession struct {
	ID            string
	Principal     Principal
	CSRFTokenHash []byte
}

type OrganizationSelectionCandidate struct {
	MemberID, OrganizationID, OrganizationName string
}

type OrganizationSelection struct {
	Candidates []OrganizationSelectionCandidate
	CSRFToken  string
}

type SessionStore interface {
	Authenticate(context.Context, string, time.Time) (AuthenticatedSession, error)
	IssueCSRFToken(context.Context, string, time.Time) (string, error)
	// ValidateCSRFToken checks the active session's current token at the store
	// boundary, ordered against token rotation and revocation.
	ValidateCSRFToken(context.Context, string, string, time.Time) error
	Revoke(context.Context, string, time.Time) error
}

type SelectionStore interface {
	ReadAndIssueCSRFToken(context.Context, string, time.Time) (OrganizationSelection, error)
	Complete(context.Context, CompleteOrganizationSelectionInput) (SessionInput, error)
}

// SelectionRepository is the persistence boundary used by SelectionService.
// It intentionally accepts a generated SessionInput so that browser secrets
// originate in the auth package rather than in the PostgreSQL adapter.
type SelectionRepository interface {
	ReadAndIssueCSRFToken(context.Context, string, time.Time) (OrganizationSelection, error)
	CompleteOrganizationSelection(context.Context, CompleteOrganizationSelectionInput, SessionInput) error
}

type CompleteOrganizationSelectionInput struct {
	Cookie, CSRFToken, MemberID string
	Now                         time.Time
}

type SelectionService struct {
	repository           SelectionRepository
	idleTTL, absoluteTTL time.Duration
}

func NewSelectionService(repository SelectionRepository, idleTTL, absoluteTTL time.Duration) *SelectionService {
	return &SelectionService{repository: repository, idleTTL: idleTTL, absoluteTTL: absoluteTTL}
}

func (s *SelectionService) ReadAndIssueCSRFToken(ctx context.Context, cookie string, now time.Time) (OrganizationSelection, error) {
	return s.repository.ReadAndIssueCSRFToken(ctx, cookie, now)
}

func (s *SelectionService) Complete(ctx context.Context, input CompleteOrganizationSelectionInput) (SessionInput, error) {
	cookie, err := oidcOpaque()
	if err != nil {
		return SessionInput{}, err
	}
	csrfToken, err := NewCSRFToken()
	if err != nil {
		return SessionInput{}, err
	}
	id, err := oidcOpaque()
	if err != nil {
		return SessionInput{}, err
	}
	session := SessionInput{
		ID:                id,
		Cookie:            cookie,
		CSRFToken:         csrfToken,
		MemberID:          input.MemberID,
		CreatedAt:         input.Now,
		IdleExpiresAt:     input.Now.Add(s.idleTTL),
		AbsoluteExpiresAt: input.Now.Add(s.absoluteTTL),
	}
	if err := s.repository.CompleteOrganizationSelection(ctx, input, session); err != nil {
		return SessionInput{}, err
	}
	return session, nil
}

type AuthTransaction struct {
	ID, Cookie, State, Nonce      string
	EncryptedVerifier             []byte
	Issuer, ClientID, RedirectURI string
	ExpiresAt                     time.Time
}

func EncryptVerifier(key [32]byte, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func DecryptVerifier(key [32]byte, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return nil, fmt.Errorf("encrypted verifier is malformed")
	}
	return gcm.Open(nil, ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():], nil)
}

func SessionCookie(value string, secure bool) *http.Cookie {
	name := developmentSessionCookieName
	if secure {
		name = productionSessionCookieName
	}
	return &http.Cookie{Name: name, Value: value, Path: "/", Secure: secure, HttpOnly: true, SameSite: http.SameSiteLaxMode}
}

// NewCSRFToken returns a new opaque synchronizer token generated by the
// auth package's CSPRNG-backed opaque-value helper.
func NewCSRFToken() (string, error) {
	return oidcOpaque()
}
