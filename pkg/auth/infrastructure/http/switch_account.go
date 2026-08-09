package authhttp

import (
	"encoding/json"
	"net/http"

	"github.com/akeemphilbert/pericarp/pkg/auth/application"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/repositories"
	"github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/session"
)

// SwitchAccountOption configures the SwitchActiveAccountHandler.
type SwitchAccountOption func(*switchAccountConfig)

type switchAccountConfig struct {
	logger       application.Logger
	cookieName   string
	cookieMaxAge int
}

// WithSwitchLogger sets the logger for the switch-account handler.
func WithSwitchLogger(l application.Logger) SwitchAccountOption {
	return func(c *switchAccountConfig) {
		if l != nil {
			c.logger = l
		}
	}
}

// WithSwitchCookieName sets the JWT cookie name. Defaults to "pericarp_token".
func WithSwitchCookieName(name string) SwitchAccountOption {
	return func(c *switchAccountConfig) {
		if name != "" {
			c.cookieName = name
		}
	}
}

// WithSwitchCookieMaxAge sets the MaxAge (in seconds) for the JWT cookie.
// Defaults to 900 (15 minutes).
func WithSwitchCookieMaxAge(maxAge int) SwitchAccountOption {
	return func(c *switchAccountConfig) {
		if maxAge > 0 {
			c.cookieMaxAge = maxAge
		}
	}
}

type switchAccountRequest struct {
	AccountID string `json:"account_id"`
}

// SwitchActiveAccountHandler returns an http.Handler that switches the active
// account in the JWT. It assumes RequireJWT middleware has already run and
// placed PericarpClaims in the request context.
//
// The accounts parameter is optional. When nil, membership is validated solely
// against the JWT's AccountIDs claim. When non-nil, an authoritative
// FindMemberRole check is performed.
//
// sessions and authService re-scope the caller's stored auth session so that
// session-authenticated routes follow the switch too. Pass both as nil only if
// the service serves no session-authenticated routes. A service that mounts
// RequireAuth and passes nil will keep serving those routes in the account the
// caller switched away from, while its RequireJWT routes act in the new one —
// the same agent in two tenants at once.
//
// Wire them together or not at all. authService must itself have been built
// with an account repository: membership cannot be verified without one, so
// re-scoping fails closed, and a caller holding a session cookie then gets 500
// on every switch — even where the JWT-only path would have succeeded.
func SwitchActiveAccountHandler(
	reissuer application.TokenReissuer,
	accounts repositories.AccountRepository,
	sessions session.SessionManager,
	authService application.AuthenticationService,
	opts ...SwitchAccountOption,
) http.Handler {
	cfg := switchAccountConfig{
		logger:       application.NoOpLogger{},
		cookieName:   "pericarp_token",
		cookieMaxAge: 900,
	}
	for _, o := range opts {
		o(&cfg)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		claims := GetJWTClaims(ctx)
		if claims == nil {
			writeJSONError(w, http.StatusUnauthorized, "not authenticated")
			return
		}

		var req switchAccountRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AccountID == "" {
			writeJSONError(w, http.StatusBadRequest, "account_id is required")
			return
		}

		// Check membership in JWT claims.
		found := false
		for _, id := range claims.AccountIDs {
			if id == req.AccountID {
				found = true
				break
			}
		}
		if !found {
			writeJSONError(w, http.StatusForbidden, "not a member of the requested account")
			return
		}

		// Optionally verify against the account repository.
		if accounts != nil {
			role, err := accounts.FindMemberRole(ctx, req.AccountID, claims.AgentID)
			if err != nil {
				cfg.logger.Error(ctx, "failed to verify membership",
					"account_id", req.AccountID, "agent_id", claims.AgentID, "error", err)
				writeJSONError(w, http.StatusInternalServerError, "failed to verify membership")
				return
			}
			if role == "" {
				writeJSONError(w, http.StatusForbidden, "not a member of the requested account")
				return
			}
		}

		// Re-scope the stored session before reissuing, so a failure here
		// cannot leave the caller holding a new token over a session still
		// pinned to the old account.
		if sessions != nil && authService != nil {
			sessionData, sessErr := sessions.GetHTTPSession(r)
			if sessErr == nil && sessionData != nil && sessionData.SessionID != "" {
				if scopeErr := authService.ScopeSessionToAccount(ctx, sessionData.SessionID, req.AccountID); scopeErr != nil {
					cfg.logger.Error(ctx, "failed to re-scope session",
						"session_id", sessionData.SessionID, "account_id", req.AccountID, "error", scopeErr)
					writeJSONError(w, http.StatusInternalServerError, "failed to switch account")
					return
				}
			}
		}

		tokenString, err := reissuer.ReissueToken(ctx, claims, req.AccountID)
		if err != nil {
			cfg.logger.Error(ctx, "failed to issue token", "error", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to issue token")
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     cfg.cookieName,
			Value:    tokenString,
			Path:     "/",
			MaxAge:   cfg.cookieMaxAge,
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		})

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"active_account_id": req.AccountID,
		})
	})
}
