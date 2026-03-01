package application_test

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"github.com/akeemphilbert/pericarp/pkg/auth/application"
)

func TestGenerateCodeVerifier(t *testing.T) {
	t.Parallel()

	verifier, err := application.GenerateCodeVerifier()
	if err != nil {
		t.Fatalf("GenerateCodeVerifier() error: %v", err)
	}

	if verifier == "" {
		t.Fatal("expected non-empty verifier")
	}

	// 32 bytes base64url-encoded = 43 characters
	if len(verifier) != 43 {
		t.Errorf("verifier length = %d, want 43", len(verifier))
	}

	// Verify it's valid base64url
	_, err = base64.RawURLEncoding.DecodeString(verifier)
	if err != nil {
		t.Errorf("verifier is not valid base64url: %v", err)
	}
}

func TestGenerateCodeVerifier_Uniqueness(t *testing.T) {
	t.Parallel()

	verifier1, err := application.GenerateCodeVerifier()
	if err != nil {
		t.Fatalf("GenerateCodeVerifier() error: %v", err)
	}

	verifier2, err := application.GenerateCodeVerifier()
	if err != nil {
		t.Fatalf("GenerateCodeVerifier() error: %v", err)
	}

	if verifier1 == verifier2 {
		t.Error("expected unique verifiers, got identical values")
	}
}

func TestGenerateCodeChallenge(t *testing.T) {
	t.Parallel()

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"

	challenge := application.GenerateCodeChallenge(verifier)
	if challenge == "" {
		t.Fatal("expected non-empty challenge")
	}

	// Verify S256 method: base64url(sha256(verifier))
	h := sha256.Sum256([]byte(verifier))
	expected := base64.RawURLEncoding.EncodeToString(h[:])

	if challenge != expected {
		t.Errorf("challenge = %q, want %q", challenge, expected)
	}
}

func TestGenerateCodeChallenge_Deterministic(t *testing.T) {
	t.Parallel()

	verifier := "test-verifier-value"
	challenge1 := application.GenerateCodeChallenge(verifier)
	challenge2 := application.GenerateCodeChallenge(verifier)

	if challenge1 != challenge2 {
		t.Error("expected deterministic challenge generation")
	}
}

func TestGenerateCodeChallenge_DifferentVerifiers(t *testing.T) {
	t.Parallel()

	challenge1 := application.GenerateCodeChallenge("verifier-one")
	challenge2 := application.GenerateCodeChallenge("verifier-two")

	if challenge1 == challenge2 {
		t.Error("expected different challenges for different verifiers")
	}
}

func TestGenerateState(t *testing.T) {
	t.Parallel()

	state, err := application.GenerateState()
	if err != nil {
		t.Fatalf("GenerateState() error: %v", err)
	}

	if state == "" {
		t.Fatal("expected non-empty state")
	}

	// 32 bytes base64url-encoded = 43 characters
	if len(state) != 43 {
		t.Errorf("state length = %d, want 43", len(state))
	}

	// Verify it's valid base64url
	_, err = base64.RawURLEncoding.DecodeString(state)
	if err != nil {
		t.Errorf("state is not valid base64url: %v", err)
	}
}

func TestGenerateState_Uniqueness(t *testing.T) {
	t.Parallel()

	state1, err := application.GenerateState()
	if err != nil {
		t.Fatalf("GenerateState() error: %v", err)
	}

	state2, err := application.GenerateState()
	if err != nil {
		t.Fatalf("GenerateState() error: %v", err)
	}

	if state1 == state2 {
		t.Error("expected unique states, got identical values")
	}
}

func TestGenerateNonce(t *testing.T) {
	t.Parallel()

	nonce, err := application.GenerateNonce()
	if err != nil {
		t.Fatalf("GenerateNonce() error: %v", err)
	}

	if nonce == "" {
		t.Fatal("expected non-empty nonce")
	}

	// 32 bytes base64url-encoded = 43 characters
	if len(nonce) != 43 {
		t.Errorf("nonce length = %d, want 43", len(nonce))
	}

	// Verify it's valid base64url
	_, err = base64.RawURLEncoding.DecodeString(nonce)
	if err != nil {
		t.Errorf("nonce is not valid base64url: %v", err)
	}
}

func TestGenerateNonce_Uniqueness(t *testing.T) {
	t.Parallel()

	nonce1, err := application.GenerateNonce()
	if err != nil {
		t.Fatalf("GenerateNonce() error: %v", err)
	}

	nonce2, err := application.GenerateNonce()
	if err != nil {
		t.Fatalf("GenerateNonce() error: %v", err)
	}

	if nonce1 == nonce2 {
		t.Error("expected unique nonces, got identical values")
	}
}
