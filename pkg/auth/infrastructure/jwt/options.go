package jwt

import (
	"crypto/rsa"
	"time"
)

const (
	defaultTokenTTL = 15 * time.Minute
	defaultIssuer   = "pericarp"
)

// RSAJWTServiceOption configures the RSAJWTService.
type RSAJWTServiceOption func(*RSAJWTService)

// WithSigningKey sets the RSA private key for signing tokens.
// The corresponding public key is derived automatically for validation.
func WithSigningKey(key *rsa.PrivateKey) RSAJWTServiceOption {
	return func(s *RSAJWTService) {
		if key != nil {
			s.privateKey = key
			s.publicKey = &key.PublicKey
		}
	}
}

// WithTokenTTL sets the token time-to-live. Default is 15 minutes.
func WithTokenTTL(ttl time.Duration) RSAJWTServiceOption {
	return func(s *RSAJWTService) {
		if ttl > 0 {
			s.tokenTTL = ttl
		}
	}
}

// WithIssuer sets the JWT issuer claim. Default is "pericarp".
func WithIssuer(issuer string) RSAJWTServiceOption {
	return func(s *RSAJWTService) {
		if issuer != "" {
			s.issuer = issuer
		}
	}
}
