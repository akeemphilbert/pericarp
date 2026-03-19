package authhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/application"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/repositories"
	"github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/session"
)

const defaultSessionDuration = 24 * time.Hour

// InviteAcceptor accepts an invite on behalf of a user during the OAuth callback.
type InviteAcceptor interface {
	AcceptInvite(ctx context.Context, token string, userInfo application.UserInfo) (
		*entities.Agent, *entities.Credential, *entities.Account, error)
}

// HandlerConfig configures the reference auth HTTP handlers.
type HandlerConfig struct {
	AuthService     application.AuthenticationService
	SessionManager  session.SessionManager
	Credentials     repositories.CredentialRepository
	RedirectURI     RedirectURIConfig
	DefaultProvider string
	SessionDuration time.Duration
	// FrontendURL is prepended to post-login redirect paths for cross-origin
	// redirects. When empty, post-login redirects use relative paths.
	FrontendURL string
	Logger      application.Logger
	// JWTCookieName is the cookie name for the issued JWT. Defaults to "pericarp_token".
	JWTCookieName string
	// JWTCookieMaxAge is the MaxAge (in seconds) for the JWT cookie. When zero,
	// it defaults to 900 (15 minutes) to match the default token TTL.
	JWTCookieMaxAge int
	// InviteAcceptor is optional. When set and the login flow carries an invite
	// token, the callback handler calls AcceptInvite instead of FindOrCreateAgent.
	// Flows without an invite token use the normal FindOrCreateAgent path regardless.
	InviteAcceptor InviteAcceptor
}

// AuthHandlers provides standard HTTP handlers for OAuth login, callback, me, and logout.
// They use net/http directly and work with any router (Echo, Chi, Gin, stdlib).
type AuthHandlers struct {
	cfg HandlerConfig
}

// NewAuthHandlers creates AuthHandlers from the given configuration.
func NewAuthHandlers(cfg HandlerConfig) *AuthHandlers {
	if cfg.SessionDuration == 0 {
		cfg.SessionDuration = defaultSessionDuration
	}
	if cfg.DefaultProvider == "" {
		cfg.DefaultProvider = "google"
	}
	if cfg.Logger == nil {
		cfg.Logger = application.NoOpLogger{}
	}
	if cfg.JWTCookieMaxAge == 0 {
		cfg.JWTCookieMaxAge = 900 // 15 minutes, matching default token TTL
	}
	return &AuthHandlers{cfg: cfg}
}

// Login initiates the OAuth authorization flow. It reads an optional ?redirect= query
// parameter and stores it in FlowData.Metadata for use after callback.
func (h *AuthHandlers) Login(w http.ResponseWriter, r *http.Request) {
	redirectURI, err := BuildRedirectURI(r, h.cfg.RedirectURI)
	if err != nil {
		h.cfg.Logger.Warn(r.Context(), "failed to build redirect URI", "error", err)
		h.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid host"})
		return
	}

	provider := r.URL.Query().Get("provider")
	if provider == "" {
		provider = h.cfg.DefaultProvider
	}

	authReq, err := h.cfg.AuthService.InitiateAuthFlow(r.Context(), provider, redirectURI)
	if err != nil {
		h.cfg.Logger.Error(r.Context(), "failed to initiate auth flow", "error", err)
		h.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to initiate auth flow"})
		return
	}

	flowData := session.FlowData{
		State:        authReq.State,
		CodeVerifier: authReq.CodeVerifier,
		Nonce:        authReq.Nonce,
		Provider:     authReq.Provider,
		RedirectURI:  redirectURI,
		CreatedAt:    time.Now(),
	}

	if redirectPath := r.URL.Query().Get("redirect"); redirectPath != "" && isValidRedirectPath(redirectPath) {
		flowData.Metadata = map[string]string{
			"post_login_redirect": redirectPath,
		}
	}

	if inviteToken := r.URL.Query().Get("invite_token"); inviteToken != "" {
		flowData.InviteToken = inviteToken
	}

	if err := h.cfg.SessionManager.SetFlowData(w, r, flowData); err != nil {
		h.cfg.Logger.Error(r.Context(), "failed to store flow data", "error", err)
		h.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to store flow data"})
		return
	}

	http.Redirect(w, r, authReq.AuthURL, http.StatusFound)
}

// Callback handles the OAuth provider callback, exchanges the code for tokens,
// finds or creates the agent, creates a session, and redirects to the frontend.
func (h *AuthHandlers) Callback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	flowData, err := h.cfg.SessionManager.GetFlowData(w, r)
	if err != nil {
		h.cfg.Logger.Warn(ctx, "OAuth callback: missing or expired flow data", "error", err)
		h.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing or expired flow data"})
		return
	}

	receivedState := r.URL.Query().Get("state")
	if err := h.cfg.AuthService.ValidateState(ctx, receivedState, flowData.State); err != nil {
		h.cfg.Logger.Warn(ctx, "OAuth callback: invalid state parameter", "error", err,
			"remote_addr", realIP(r))
		h.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid state parameter"})
		return
	}

	code := r.URL.Query().Get("code")
	authResult, err := h.cfg.AuthService.ExchangeCode(ctx, code, flowData.CodeVerifier, flowData.Provider, flowData.RedirectURI)
	if err != nil {
		h.cfg.Logger.Error(ctx, "code exchange failed", "error", err)
		h.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to exchange authorization code"})
		return
	}

	var agent *entities.Agent
	var credential *entities.Credential
	var account *entities.Account

	if flowData.InviteToken != "" {
		if h.cfg.InviteAcceptor == nil {
			h.cfg.Logger.Error(ctx, "invite token present but InviteAcceptor not configured")
			h.writeJSON(w, http.StatusInternalServerError,
				map[string]string{"error": "invite system not configured"})
			return
		}
		agent, credential, account, err = h.cfg.InviteAcceptor.AcceptInvite(
			ctx, flowData.InviteToken, authResult.UserInfo)
		if err != nil {
			h.cfg.Logger.Error(ctx, "invite acceptance failed", "error", err)
			status := http.StatusInternalServerError
			msg := "failed to accept invite"
			if errors.Is(err, application.ErrInviteNotFound) ||
				errors.Is(err, application.ErrInviteNotPending) ||
				errors.Is(err, application.ErrInviteExpired) ||
				errors.Is(err, application.ErrInviteTokenInvalid) ||
				errors.Is(err, application.ErrInviteEmailMismatch) {
				status = http.StatusBadRequest
				msg = "invalid or expired invite"
			}
			h.writeJSON(w, status, map[string]string{"error": msg})
			return
		}
	} else {
		agent, credential, account, err = h.cfg.AuthService.FindOrCreateAgent(ctx, authResult.UserInfo)
		if err != nil {
			h.cfg.Logger.Error(ctx, "find or create agent failed", "error", err)
			h.writeJSON(w, http.StatusInternalServerError,
				map[string]string{"error": "failed to find or create agent"})
			return
		}
	}

	ipAddress := realIP(r)
	userAgent := r.UserAgent()
	authSession, err := h.cfg.AuthService.CreateSession(
		ctx, agent.GetID(), credential.GetID(),
		ipAddress, userAgent, h.cfg.SessionDuration,
	)
	if err != nil {
		h.cfg.Logger.Error(ctx, "session creation failed", "error", err)
		h.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create session"})
		return
	}

	sessionData := session.SessionData{
		SessionID: authSession.GetID(),
		AgentID:   agent.GetID(),
		CreatedAt: time.Now(),
		ExpiresAt: authSession.ExpiresAt(),
	}
	if account != nil {
		sessionData.AccountID = account.GetID()
	}
	if err := h.cfg.SessionManager.CreateHTTPSession(w, r, sessionData); err != nil {
		h.cfg.Logger.Error(ctx, "HTTP session creation failed", "error", err)
		h.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create HTTP session"})
		return
	}

	// Issue identity token if the AuthService supports it (non-fatal on failure).
	activeAccountID := ""
	if account != nil {
		activeAccountID = account.GetID()
	}
	tokenString, issueErr := h.cfg.AuthService.IssueIdentityToken(ctx, agent, activeAccountID)
	if issueErr != nil {
		h.cfg.Logger.Warn(ctx, "failed to issue identity token", "error", issueErr)
	} else if tokenString != "" {
		cookieName := h.cfg.JWTCookieName
		if cookieName == "" {
			cookieName = "pericarp_token"
		}
		http.SetCookie(w, &http.Cookie{
			Name:     cookieName,
			Value:    tokenString,
			Path:     "/",
			MaxAge:   h.cfg.JWTCookieMaxAge,
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		})
	}

	// Determine post-login redirect path from flow metadata.
	redirectPath := "/"
	if flowData.Metadata != nil {
		if path, ok := flowData.Metadata["post_login_redirect"]; ok && isValidRedirectPath(path) {
			redirectPath = path
		}
	}

	finalRedirect := redirectPath
	if h.cfg.FrontendURL != "" {
		finalRedirect = strings.TrimRight(h.cfg.FrontendURL, "/") + redirectPath
	}

	http.Redirect(w, r, finalRedirect, http.StatusFound)
}

// Me returns the authenticated user's basic profile as JSON.
func (h *AuthHandlers) Me(w http.ResponseWriter, r *http.Request) {
	sessionData, err := h.cfg.SessionManager.GetHTTPSession(r)
	if err != nil {
		h.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	ctx := r.Context()
	sessionInfo, err := h.cfg.AuthService.ValidateSession(ctx, sessionData.SessionID)
	if err != nil {
		h.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "session invalid"})
		return
	}

	creds, err := h.cfg.Credentials.FindByAgent(ctx, sessionInfo.AgentID)
	if err != nil {
		h.cfg.Logger.Error(ctx, "failed to fetch credentials for agent",
			"agent_id", sessionInfo.AgentID, "error", err)
		h.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load profile"})
		return
	}
	if len(creds) == 0 {
		h.writeJSON(w, http.StatusOK, map[string]string{"id": sessionInfo.AgentID, "name": ""})
		return
	}

	cred := creds[0]
	h.writeJSON(w, http.StatusOK, map[string]string{
		"id":    sessionInfo.AgentID,
		"name":  cred.DisplayName(),
		"email": cred.Email(),
	})
}

// Logout revokes the domain session and destroys the HTTP session cookie.
func (h *AuthHandlers) Logout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	sessionData, err := h.cfg.SessionManager.GetHTTPSession(r)
	if err == nil && sessionData != nil {
		if revokeErr := h.cfg.AuthService.RevokeSession(ctx, sessionData.SessionID); revokeErr != nil {
			h.cfg.Logger.Warn(ctx, "failed to revoke domain session during logout",
				"session_id", sessionData.SessionID, "error", revokeErr)
		}
	}

	if err := h.cfg.SessionManager.DestroyHTTPSession(w, r); err != nil {
		h.cfg.Logger.Error(ctx, "failed to destroy HTTP session", "error", err)
		h.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to destroy session"})
		return
	}

	// Clear the JWT cookie if one was configured.
	cookieName := h.cfg.JWTCookieName
	if cookieName == "" {
		cookieName = "pericarp_token"
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	h.writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

// isValidRedirectPath checks that the path is a safe relative path.
func isValidRedirectPath(path string) bool {
	return strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "//") && !strings.Contains(path, "://")
}

// realIP extracts the client IP, preferring X-Real-Ip and X-Forwarded-For headers.
// These headers should only be trusted when the application runs behind a trusted reverse proxy.
func realIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-Ip"); ip != "" {
		return ip
	}
	if ff := r.Header.Get("X-Forwarded-For"); ff != "" {
		if idx := strings.Index(ff, ","); idx != -1 {
			return strings.TrimSpace(ff[:idx])
		}
		return ff
	}
	// Use net.SplitHostPort for correct IPv6 handling.
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (h *AuthHandlers) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		h.cfg.Logger.Error(nil, "failed to write JSON response", "error", err)
	}
}
