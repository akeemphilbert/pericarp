package providers

import "encoding/json"

// emailVerifiedClaim decodes an OIDC email_verified claim. Google sends it as a
// JSON boolean; Apple sends either a boolean or the string "true" / "false".
//
// Any other shape — null, a number, an unrecognised string — decodes to false
// instead of failing the token. The claim can only withhold trust, so an
// unexpected value reads as unverified and the sign-in is refused with a clear
// reason, rather than surfacing as an opaque token-decode error. An absent
// claim is never decoded and keeps the zero value, which is also false.
type emailVerifiedClaim bool

func (c *emailVerifiedClaim) UnmarshalJSON(data []byte) error {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	switch v := raw.(type) {
	case bool:
		*c = emailVerifiedClaim(v)
	case string:
		*c = emailVerifiedClaim(v == "true")
	default:
		*c = false
	}
	return nil
}
