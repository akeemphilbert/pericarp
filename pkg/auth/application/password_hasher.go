package application

import (
	"errors"
	"fmt"

	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
	"golang.org/x/crypto/bcrypt"
)

// ErrInvalidPassword is returned when a plaintext password fails to match
// the stored hash. To avoid revealing which emails are registered,
// VerifyPassword also returns this sentinel when no matching credential is
// found at all.
var ErrInvalidPassword = errors.New("authentication: invalid password")

// hashPassword produces an algorithm identifier and hash for the given
// plaintext using bcrypt at the requested cost. cost <= 0 selects
// bcrypt.DefaultCost.
func hashPassword(plaintext string, cost int) (algorithm, hash string, err error) {
	if cost <= 0 {
		cost = bcrypt.DefaultCost
	}
	out, err := bcrypt.GenerateFromPassword([]byte(plaintext), cost)
	if err != nil {
		return "", "", fmt.Errorf("bcrypt hash: %w", err)
	}
	return entities.PasswordAlgorithmBcrypt, string(out), nil
}

// verifyPassword compares plaintext against the stored hash for the named
// algorithm. The saltSuffix, when non-empty, is appended to plaintext
// before comparison — supporting legacy imports where the original
// system applied an extra application-layer salt suffix on top of
// bcrypt. Pass an empty saltSuffix for credentials registered through
// pericarp.
//
// Returns ErrInvalidPassword on mismatch, and a wrapped error for any
// other failure (corrupt hash, unsupported algorithm) so the caller can
// log the detail internally while still reporting only ErrInvalidPassword
// to the end user.
func verifyPassword(algorithm, hash, plaintext, saltSuffix string) error {
	switch algorithm {
	case entities.PasswordAlgorithmBcrypt:
		err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext+saltSuffix))
		if err == nil {
			return nil
		}
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return ErrInvalidPassword
		}
		return fmt.Errorf("bcrypt compare: %w", err)
	default:
		return fmt.Errorf("unsupported password algorithm: %q", algorithm)
	}
}
