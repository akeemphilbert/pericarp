package providers

import "time"

// Shared flow-binding primitives used by every federated provider in this
// package whose `Exchange`/`AuthCodeURLForX` pair binds caller state to a
// single-use, TTL'd codeChallenge slot. Mastodon and Bluesky both consume
// these; keeping them in a dedicated file (rather than in either provider's
// implementation file) makes the cross-provider coupling explicit and stops
// a refactor of one provider from silently breaking the other.

// flowStatus categorises the result of takeFlow so Exchange can return a
// distinguishable sentinel per case.
type flowStatus int

const (
	flowStatusMissing  flowStatus = iota // never bound
	flowStatusOK                         // bound, in-TTL, single-use claim succeeded
	flowStatusExpired                    // bound, but TTL'd
	flowStatusConsumed                   // bound previously, already taken
)

// flowTombstone records what happened to a previously-bound flow so a
// duplicate Exchange returns the same sentinel until TTL elapses, instead of
// degrading to flowStatusMissing once the bind entry has been removed.
type flowTombstone struct {
	status    flowStatus // flowStatusConsumed or flowStatusExpired
	expiresAt time.Time
}

// gcSweepEvery sets how often (per bindFlow call) a provider opportunistically
// scans for and removes expired flow bindings, bounding sync.Map retention for
// abandoned flows. Probabilistic so amortised cost is O(1) per bind.
const gcSweepEvery = 64
