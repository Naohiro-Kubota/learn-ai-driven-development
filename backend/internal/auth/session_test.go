package auth

import (
	"bytes"
	"context"
	"testing"
	"time"
)

type selectionCompleterFake struct {
	input   CompleteOrganizationSelectionInput
	session SessionInput
}

func (f *selectionCompleterFake) ReadAndIssueCSRFToken(context.Context, string, time.Time) (OrganizationSelection, error) {
	return OrganizationSelection{}, nil
}

func (f *selectionCompleterFake) CompleteOrganizationSelection(_ context.Context, input CompleteOrganizationSelectionInput, session SessionInput) error {
	f.input = input
	f.session = session
	return nil
}

func TestEncryptVerifierRoundTrip(t *testing.T) {
	key := [32]byte{1}
	plaintext := []byte("pkce-verifier")
	ciphertext, err := EncryptVerifier(key, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, plaintext) {
		t.Fatal("ciphertext contains verifier")
	}
	got, err := DecryptVerifier(key, ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("plaintext = %q, want %q", got, plaintext)
	}
}

func TestSessionCookiePolicy(t *testing.T) {
	production := SessionCookie("value", true)
	if production.Name != "__Host-approval_flow_session" || !production.Secure || !production.HttpOnly || production.SameSite != 2 || production.Path != "/" || production.Domain != "" {
		t.Fatalf("production cookie = %#v", production)
	}
	development := SessionCookie("value", false)
	if development.Name == production.Name || development.Secure {
		t.Fatalf("development cookie = %#v", development)
	}
}

func TestNewCSRFTokenGeneratesFreshOpaqueValues(t *testing.T) {
	first, err := NewCSRFToken()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewCSRFToken()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("CSRF token was reused")
	}
	if first == "" || second == "" {
		t.Fatal("CSRF token is empty")
	}
}

func TestSelectionServiceCompleteGeneratesSessionSecrets(t *testing.T) {
	now := time.Date(2026, time.September, 22, 9, 0, 0, 0, time.UTC)
	completer := &selectionCompleterFake{}
	store := NewSelectionService(completer, time.Hour, 2*time.Hour)

	session, err := store.Complete(context.Background(), CompleteOrganizationSelectionInput{Cookie: "selection-cookie", CSRFToken: "selection-csrf", MemberID: "member-1", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if session.ID == "" || session.Cookie == "" || session.CSRFToken == "" {
		t.Fatalf("session has empty opaque value: %#v", session)
	}
	if session.MemberID != "member-1" || !session.CreatedAt.Equal(now) || !session.IdleExpiresAt.Equal(now.Add(time.Hour)) || !session.AbsoluteExpiresAt.Equal(now.Add(2*time.Hour)) {
		t.Fatalf("session = %#v", session)
	}
	if completer.input.MemberID != "member-1" || completer.session != session {
		t.Fatalf("completion input = %#v, session = %#v", completer.input, completer.session)
	}
}
