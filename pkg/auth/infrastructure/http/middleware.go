package authhttp

import (
	"context"
	"encoding/json"
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
// It also injects an auth.Identity for use via auth.AgentFromCtx.
// Unauthenticated requests receive a 401 JSON response.
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
				writeJSONError(w, http.StatusUnauthorized, "not authenticated")
				return
			}

			ctx := context.WithValue(r.Context(), sessionContextKey, sessionInfo)
			id := &auth.Identity{
				AgentID:         sessionInfo.AgentID,
				AccountIDs:      []string{sessionInfo.AccountID},
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
