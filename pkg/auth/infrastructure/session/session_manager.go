package session

import (
	"net/http"
	"time"
)

// SessionData represents the data stored in an HTTP session cookie.
// Only the opaque session ID is stored in the cookie; all real data
// lives in the domain AuthSession aggregate.
type SessionData struct {
	// SessionID maps to the AuthSession aggregate ID.
	SessionID string
	// AgentID is the authenticated agent's ID.
	AgentID string
	// AccountID is the account the session is scoped to. Empty if not scoped.
	AccountID string
	// CreatedAt is when the session was created.
	CreatedAt time.Time
	// ExpiresAt is when the session expires.
	ExpiresAt time.Time
}

// FlowData represents temporary OAuth flow data stored server-side during
// the authorization code flow. This data is short-lived and cleared after use.
type FlowData struct {
	// State is the OAuth CSRF protection parameter.
	State string
	// CodeVerifier is the PKCE code verifier for the current flow.
	CodeVerifier string
	// Nonce is the OpenID Connect nonce for ID token validation.
	Nonce string
	// Provider is the identity provider for this flow.
	Provider string
	// RedirectURI is the callback URI for this flow.
	RedirectURI string
	// CreatedAt is when the flow was initiated.
	CreatedAt time.Time
}

// SessionManager defines the interface for HTTP session management.
// Implementations handle cookie management, session creation/destruction,
// and temporary OAuth flow data storage.
type SessionManager interface {
	// CreateHTTPSession creates a new HTTP session and sets the cookie.
	CreateHTTPSession(w http.ResponseWriter, r *http.Request, sessionInfo SessionData) error

	// GetHTTPSession retrieves the session data from the request cookie.
	GetHTTPSession(r *http.Request) (*SessionData, error)

	// DestroyHTTPSession destroys the HTTP session and clears the cookie.
	DestroyHTTPSession(w http.ResponseWriter, r *http.Request) error

	// SetFlowData stores temporary OAuth flow data in the session.
	SetFlowData(w http.ResponseWriter, r *http.Request, data FlowData) error

	// GetFlowData retrieves and clears OAuth flow data from the session.
	GetFlowData(w http.ResponseWriter, r *http.Request) (*FlowData, error)
}
