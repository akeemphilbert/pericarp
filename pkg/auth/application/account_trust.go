package application

// AccountTrustOption tells an operation how much to trust the account ID it
// was handed.
type AccountTrustOption func(*accountTrust)

type accountTrust struct {
	verified bool
}

// AccountAlreadyVerified declares that the account was resolved by this same
// request, from a path that already established the agent's membership and
// that the account is active — so the operation may skip re-reading it.
//
// Pass it only from a sign-in path that resolved the account itself:
// FindOrCreateAgent, VerifyPassword and AcceptInvite all qualify. Do not pass
// it for an account that arrived from a client, which is exactly what the
// check exists to catch.
//
// This is not only an optimisation. Sign-in writes the membership row and then
// verifies it moments later in the same request; on a read replica or an
// asynchronous read model that row may not be visible yet, so a brand-new user
// could fail their very first sign-in. Vouching removes the race along with
// the redundant read.
func AccountAlreadyVerified() AccountTrustOption {
	return func(t *accountTrust) { t.verified = true }
}

func newAccountTrust(opts []AccountTrustOption) accountTrust {
	var t accountTrust
	for _, opt := range opts {
		if opt != nil {
			opt(&t)
		}
	}
	return t
}
