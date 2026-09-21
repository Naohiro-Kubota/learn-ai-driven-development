package auth

import (
	"context"
	"testing"
)

func TestCompleteLoginRequiresTransaction(t *testing.T) {
	a := &Authenticator{}
	_, err := a.CompleteLogin(context.Background(), CallbackInput{TransactionCookie: "missing", State: "state", Code: "code"})
	if err == nil {
		t.Fatal("missing transaction was accepted")
	}
}
