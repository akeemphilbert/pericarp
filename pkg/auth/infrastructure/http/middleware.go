package authhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/akeemphilbert/pericarp/pkg/auth"
	"github.com/akeemphilbert/pericarp/pkg/auth/application"
	"github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/session"
)

// contextKey is an unexported type for context keys defined in this package,
// preventing collisions with keys defined in other packages.
type contextKey struct{ name string }

var (
	jwtContextKey     = &contextKey{"pericarp-jwt-claims"}
	sessionContextKey = &contextKey{"pericarp-session-info"}
)

// RequireAuth returns HTTP middleware that validates the session cookie and
// injects the SessionInfo into the request context.
// It also injects an auth.Identity for use via auth.AgentFromCtx. The
// injected Identity.Subscription is always nil — sessions don't snapshot
// subscription state. Services that mix RequireAuth and RequireJWT will
// see different IsActive() results for the same agent depending on which
// middleware served the request; gate paid-tier features behind RequireJWT
// or fetch subscription state explicitly under RequireAuth.
// Unauthenticated requests receive a 401 JSON response.
//
// A session that is not scoped to an account is treated as unauthenticated.
// Sessions stored before sessions carried an account are all unscoped, and
// there is no backfill, so their holders are refused once and sign in again.
//
// # Telling the 401s apart
//
// Three refusals share the 401 status and need opposite handling, so two of
// them carry a "code" field in the JSON body. A client that treats every 401
// the same will either loop forever or give up when retrying would have
// worked.
//
//	code                    meaning                        what the client should do
//	----------------------  -----------------------------  --------------------------------
//	(none)                  no session, or it expired      sign in again
//	                        or was revoked
//	unscoped_session        the agent has no active        do NOT retry sign-in; it
//	                        account to act in              produces another unscoped
//	                        session. Surface it.
//	account_access_revoked  the agent was removed from     retry sign-in; it MAY recover.
//	                        the session's account          The next session is scoped to
//	                                                       an account they still belong
//	                                                       to — if they have one. If the
//	                                                       retry answers unscoped_session,
//	                                                       that is the terminal state.
//	account_deactivated     the agent is still a member,   retry sign-in only helps if
//	                        but the account is suspended   they belong to another ACTIVE
//	                                                       account. Otherwise this is an
//	                                                       operator problem — reactivate
//	                                                       the account. Say so; do not
//	                                                       tell the user to sign in again.
//
// An account_access_revoked refusal can be transient. It is recomputed per
// request against live memberships, so a non-transactional membership rewrite,
// replica lag, or an asynchronous projection can briefly hide a valid
// membership. The session is deliberately not revoked, so it starts working
// again by itself once the membership is visible. Treat a single refusal as a
// signal, not as proof the access is gone — and prefer
// AccountRepository.RemoveMember, whose single-row delete has no such window,
// over rewriting a membership set.
//
// The check is skipped entirely when memberships cannot be read — no account
// repository is configured, or the lookup failed. That is deliberate, so a
// database blip cannot log every user out at once, but it means a persistent
// lookup failure silently disables revocation. The skip is logged; alarm on a
// sustained rate if you rely on this enforcement.
func RequireAuth(
	sm session.SessionManager,
	as application.AuthenticationService,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sessionData, err := sm.GetHTTPSession(r)
			if err != nil {
				writeJSONError(w, http.StatusUnauthorized, "not authenticated")
				return
			}

			sessionInfo, err := as.ValidateSession(r.Context(), sessionData.SessionID)
			if err != nil {
				// The session names an account the agent has since been
				// removed from. Coded separately because signing in again
				// does resolve it — the next session is scoped to an account
				// they still belong to.
				if errors.Is(err, application.ErrSessionAccountRevoked) {
					writeJSONErrorCode(w, http.StatusUnauthorized, "not authenticated", "account_access_revoked")
					return
				}
				// Distinct from revocation: the agent is still a member, the
				// account itself is suspended. Signing in again cannot help
				// unless they belong to another active account, and an
				// operator debugging this needs to know which happened.
				if errors.Is(err, application.ErrSessionAccountDeactivated) {
					writeJSONErrorCode(w, http.StatusUnauthorized, "not authenticated", "account_deactivated")
					return
				}
				writeJSONError(w, http.StatusUnauthorized, "not authenticated")
				return
			}

			// An unscoped session cannot own resources, so admitting the
			// request would only defer the failure to a handler that cannot
			// explain it. Refuse at the door instead.
			//
			// This carries its own code because the remedy differs: an expired
			// session is fixed by signing in again, an unscoped one is not —
			// the agent has no active account, so a client that retries login
			// on a bare 401 loops forever.
			if sessionInfo.AccountID == "" {
				writeJSONErrorCode(w, http.StatusUnauthorized, "not authenticated", "unscoped_session")
				return
			}

			accountIDs := sessionInfo.AccountIDs
			if len(accountIDs) == 0 {
				accountIDs = []string{sessionInfo.AccountID}
			}

			ctx := context.WithValue(r.Context(), sessionContextKey, sessionInfo)
			id := &auth.Identity{
				AgentID:         sessionInfo.AgentID,
				AccountIDs:      accountIDs,
				ActiveAccountID: sessionInfo.AccountID,
			}
			ctx = auth.ContextWithAgent(ctx, id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetSessionInfo retrieves the SessionInfo from the request context.
// Returns nil if the request was not authenticated via RequireAuth middleware.
func GetSessionInfo(ctx context.Context) *application.SessionInfo {
	info, _ := ctx.Value(sessionContextKey).(*application.SessionInfo)
	return info
}

// RequireJWT returns HTTP middleware that validates a JWT from the Authorization
// header (Bearer token, case-insensitive scheme) falling back to a named cookie,
// and injects the PericarpClaims into the request context.
// It also injects an auth.Identity for use via auth.AgentFromCtx.
// When cookieName is empty it defaults to "pericarp_token".
// Unauthenticated or invalid-token requests receive a 401 JSON response.
func RequireJWT(jwtService application.JWTService, cookieName string) func(http.Handler) http.Handler {
	if cookieName == "" {
		cookieName = "pericarp_token"
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString := extractBearerToken(r)
			if tokenString == "" {
				if cookie, err := r.Cookie(cookieName); err == nil {
					tokenString = cookie.Value
				}
			}
			if tokenString == "" {
				writeJSONError(w, http.StatusUnauthorized, "missing token")
				return
			}

			claims, err := jwtService.ValidateToken(r.Context(), tokenString)
			if err != nil {
				writeJSONError(w, http.StatusUnauthorized, "invalid token")
				return
			}

			ctx := context.WithValue(r.Context(), jwtContextKey, claims)
			id := &auth.Identity{
				AgentID:         claims.AgentID,
				AccountIDs:      claims.AccountIDs,
				ActiveAccountID: claims.ActiveAccountID,
				Subscription:    claims.Subscription,
			}
			ctx = auth.ContextWithAgent(ctx, id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetJWTClaims retrieves the PericarpClaims from the request context.
// Returns nil if the request was not authenticated via RequireJWT middleware.
func GetJWTClaims(ctx context.Context) *application.PericarpClaims {
	claims, _ := ctx.Value(jwtContextKey).(*application.PericarpClaims)
	return claims
}

// extractBearerToken extracts the token from an "Authorization: Bearer <token>" header.
// The scheme comparison is case-insensitive per RFC 7235.
func extractBearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return h[len(prefix):]
	}
	return ""
}

// writeJSONError is a lightweight helper for middleware that has no logger access.
func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// writeJSONErrorCode writes an error carrying a stable machine-readable code,
// for cases a client must tell apart from the generic one.
func writeJSONErrorCode(w http.ResponseWriter, status int, msg, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg, "code": code})
}
