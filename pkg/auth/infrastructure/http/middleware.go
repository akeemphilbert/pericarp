package authhttp

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/akeemphilbert/pericarp/pkg/auth/application"
	"github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/session"
)

// SessionContextKey is the typed context key used to store SessionInfo in request context.
type SessionContextKey struct{}

// RequireAuth returns HTTP middleware that validates the session cookie and
// injects the SessionInfo into the request context under SessionContextKey{}.
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

			ctx := context.WithValue(r.Context(), SessionContextKey{}, sessionInfo)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetSessionInfo retrieves the SessionInfo from the request context.
// Returns nil if the request was not authenticated via RequireAuth middleware.
func GetSessionInfo(ctx context.Context) *application.SessionInfo {
	info, _ := ctx.Value(SessionContextKey{}).(*application.SessionInfo)
	return info
}

// writeJSONError is a lightweight helper for middleware that has no logger access.
func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
