package providers

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

// unsignedIDToken builds a header.payload.signature string carrying claims.
// The providers here decode the payload without verifying the signature, so a
// placeholder signature is enough to exercise the claim mapping.
func unsignedIDToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal ID token claims: %v", err)
	}
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

func TestEmailVerifiedClaim_Decode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		doc  string
		want bool
	}{
		{name: "boolean true", doc: `{"email_verified":true}`, want: true},
		{name: "boolean false", doc: `{"email_verified":false}`, want: false},
		{name: "string true", doc: `{"email_verified":"true"}`, want: true},
		{name: "string false", doc: `{"email_verified":"false"}`, want: false},
		{name: "absent", doc: `{}`, want: false},
		{name: "null", doc: `{"email_verified":null}`, want: false},
		{name: "unrecognised string", doc: `{"email_verified":"yes"}`, want: false},
		{name: "number", doc: `{"email_verified":1}`, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var claims struct {
				EmailVerified emailVerifiedClaim `json:"email_verified"`
			}
			// A claim the provider sent in an unexpected shape must read as
			// unverified, not fail the whole token: failing would turn a
			// refusable sign-in into an opaque exchange error.
			if err := json.Unmarshal([]byte(tc.doc), &claims); err != nil {
				t.Fatalf("decode %s: %v", tc.doc, err)
			}
			if got := bool(claims.EmailVerified); got != tc.want {
				t.Errorf("decode %s = %v, want %v", tc.doc, got, tc.want)
			}
		})
	}
}
