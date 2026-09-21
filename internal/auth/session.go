package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	productionSessionCookieName  = "__Host-approval_flow_session"
	developmentSessionCookieName = "approval_flow_session"
)

var (
	ErrNotFound  = fmt.Errorf("auth record not found")
	ErrExpired   = fmt.Errorf("auth record expired")
	ErrConsumed  = fmt.Errorf("auth record consumed")
	ErrForbidden = fmt.Errorf("auth selection forbidden")
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
