package auth

import (
	"bytes"
	"testing"
)

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
