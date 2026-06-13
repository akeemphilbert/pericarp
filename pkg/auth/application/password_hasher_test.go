package application

import (
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// verifyPassword is the only place in the auth package where the
// salt-suffix concatenation actually happens. The application
// integration tests cover the call sites transitively, but a direct
// table here pins the contract — particularly that an empty saltSuffix
// is a no-op and that a non-empty suffix is rejected on a hash that
// was generated without one.
func TestVerifyPassword(t *testing.T) {
	t.Parallel()

	const plaintext = "hunter2"
	const salt = "abcde"

	plainHash, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt plain: %v", err)
	}
	saltedHash, err := bcrypt.GenerateFromPassword([]byte(plaintext+salt), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt salted: %v", err)
	}

	cases := []struct {
		name      string
		hash      string
		plaintext string
		salt      string
		wantErr   error
	}{
		{
			name:      "no-salt hash, empty suffix, correct plaintext",
			hash:      string(plainHash),
			plaintext: plaintext,
			salt:      "",
		},
		{
			name:      "no-salt hash, empty suffix, wrong plaintext",
			hash:      string(plainHash),
			plaintext: "wrong",
			salt:      "",
			wantErr:   ErrInvalidPassword,
		},
		{
			name:      "salted hash, correct suffix, correct plaintext",
			hash:      string(saltedHash),
			plaintext: plaintext,
			salt:      salt,
		},
		{
			name:      "salted hash, missing suffix on the input — proves salt is load-bearing",
			hash:      string(saltedHash),
			plaintext: plaintext,
			salt:      "",
			wantErr:   ErrInvalidPassword,
		},
		{
			name:      "salted hash, wrong suffix",
			hash:      string(saltedHash),
			plaintext: plaintext,
			salt:      "wrong",
			wantErr:   ErrInvalidPassword,
		},
		{
			name:      "no-salt hash, plaintext smuggled with salt — proves the salt is appended, not prepended or substituted",
			hash:      string(plainHash),
			plaintext: plaintext + salt,
			salt:      "",
			wantErr:   ErrInvalidPassword,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := verifyPassword("bcrypt", tc.hash, tc.plaintext, tc.salt)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Errorf("err = %v, want nil", err)
			}
		})
	}
}

// Unsupported algorithms must error loudly — never silently accept a
// salt+algorithm combo that verifyPassword doesn't actually consume.
// This pairs with the entity-level rejection in WithSalt: even if a row
// somehow lands with algorithm="argon2id" and a non-empty salt, the
// verify path errors rather than returns a misleading ErrInvalidPassword.
func TestVerifyPassword_UnsupportedAlgorithm(t *testing.T) {
	t.Parallel()

	err := verifyPassword("argon2id", "$argon2id$v=19$m=65536$x", "anything", "")
	if err == nil {
		t.Fatal("expected error for unsupported algorithm")
	}
	if errors.Is(err, ErrInvalidPassword) {
		t.Errorf("unsupported algorithm should NOT map to ErrInvalidPassword (mismatch sentinel), got %v", err)
	}
}

// A malformed bcrypt blob must surface as a non-mismatch error so the
// caller can log it as a corrupt-row event rather than logging it as a
// run-of-the-mill wrong password.
func TestVerifyPassword_CorruptHash(t *testing.T) {
	t.Parallel()

	err := verifyPassword("bcrypt", "not-a-bcrypt-hash", "anything", "")
	if err == nil {
		t.Fatal("expected error for corrupt hash")
	}
	if errors.Is(err, ErrInvalidPassword) {
		t.Errorf("corrupt hash should NOT map to ErrInvalidPassword, got %v", err)
	}
}
