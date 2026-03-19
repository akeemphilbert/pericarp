package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/sessions"
)

const (
	// flowSessionSuffix is appended to the session name for flow data storage.
	flowSessionSuffix = "-flow"

	// flowMaxAge is the maximum age for flow data sessions (10 minutes).
	flowMaxAge = 600

	// Session keys for stored values.
	keySessionID = "session_id"
	keyAgentID   = "agent_id"
	keyAccountID = "account_id"
	keyCreatedAt = "created_at"
	keyExpiresAt = "expires_at"

	keyState        = "state"
	keyCodeVerifier = "code_verifier"
	keyNonce        = "nonce"
	keyProvider     = "provider"
	keyRedirectURI  = "redirect_uri"
	keyFlowCreated  = "flow_created_at"
	keyFlowMetadata = "flow_metadata"
	keyInviteToken  = "invite_token"
)

var (
	// ErrSessionNotFound is returned when no session exists in the request.
	ErrSessionNotFound = errors.New("session: not found")
	// ErrFlowDataNotFound is returned when no flow data exists in the session.
	ErrFlowDataNotFound = errors.New("session: flow data not found")
)

// SessionOptions configures HTTP session cookie behavior.
type SessionOptions struct {
	MaxAge   int           // seconds
	Domain   string        // cookie domain
	Path     string        // cookie path
	HttpOnly bool          // default: true
	Secure   bool          // default: true
	SameSite http.SameSite // default: Lax
}

// DefaultSessionOptions returns secure default session options.
func DefaultSessionOptions() SessionOptions {
	return SessionOptions{
		MaxAge:   86400, // 24 hours
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
}

// GorillaSessionManager implements SessionManager using gorilla/sessions.
type GorillaSessionManager struct {
	sessionName string
	store       sessions.Store
	options     SessionOptions
}

// NewGorillaSessionManager creates a new GorillaSessionManager.
func NewGorillaSessionManager(sessionName string, store sessions.Store, options SessionOptions) *GorillaSessionManager {
	return &GorillaSessionManager{
		sessionName: sessionName,
		store:       store,
		options:     options,
	}
}

// CreateHTTPSession creates a new HTTP session and sets the cookie.
func (m *GorillaSessionManager) CreateHTTPSession(w http.ResponseWriter, r *http.Request, sessionInfo SessionData) error {
	sess, err := m.store.Get(r, m.sessionName)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	sess.Options = m.cookieOptions()
	sess.Values[keySessionID] = sessionInfo.SessionID
	sess.Values[keyAgentID] = sessionInfo.AgentID
	sess.Values[keyAccountID] = sessionInfo.AccountID
	sess.Values[keyCreatedAt] = sessionInfo.CreatedAt.Unix()
	sess.Values[keyExpiresAt] = sessionInfo.ExpiresAt.Unix()

	return sess.Save(r, w)
}

// GetHTTPSession retrieves the session data from the request cookie.
func (m *GorillaSessionManager) GetHTTPSession(r *http.Request) (*SessionData, error) {
	sess, err := m.store.Get(r, m.sessionName)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	sessionID, ok := sess.Values[keySessionID].(string)
	if !ok || sessionID == "" {
		return nil, ErrSessionNotFound
	}

	agentID, _ := sess.Values[keyAgentID].(string)
	accountID, _ := sess.Values[keyAccountID].(string)
	createdAt, _ := sess.Values[keyCreatedAt].(int64)
	expiresAt, _ := sess.Values[keyExpiresAt].(int64)

	return &SessionData{
		SessionID: sessionID,
		AgentID:   agentID,
		AccountID: accountID,
		CreatedAt: time.Unix(createdAt, 0),
		ExpiresAt: time.Unix(expiresAt, 0),
	}, nil
}

// DestroyHTTPSession destroys the HTTP session and clears the cookie.
func (m *GorillaSessionManager) DestroyHTTPSession(w http.ResponseWriter, r *http.Request) error {
	sess, err := m.store.Get(r, m.sessionName)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	sess.Options = m.cookieOptions()
	sess.Options.MaxAge = -1

	for key := range sess.Values {
		delete(sess.Values, key)
	}

	return sess.Save(r, w)
}

// SetFlowData stores temporary OAuth flow data in a separate short-lived session.
func (m *GorillaSessionManager) SetFlowData(w http.ResponseWriter, r *http.Request, data FlowData) error {
	flowName := m.sessionName + flowSessionSuffix
	sess, err := m.store.Get(r, flowName)
	if err != nil {
		return fmt.Errorf("failed to get flow session: %w", err)
	}

	sess.Options = m.flowCookieOptions()
	sess.Values[keyState] = data.State
	sess.Values[keyCodeVerifier] = data.CodeVerifier
	sess.Values[keyNonce] = data.Nonce
	sess.Values[keyProvider] = data.Provider
	sess.Values[keyRedirectURI] = data.RedirectURI
	sess.Values[keyFlowCreated] = data.CreatedAt.Unix()

	if len(data.Metadata) > 0 {
		raw, err := json.Marshal(data.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal flow metadata: %w", err)
		}
		sess.Values[keyFlowMetadata] = string(raw)
	}

	if data.InviteToken != "" {
		sess.Values[keyInviteToken] = data.InviteToken
	}

	return sess.Save(r, w)
}

// GetFlowData retrieves and clears OAuth flow data from the session.
func (m *GorillaSessionManager) GetFlowData(w http.ResponseWriter, r *http.Request) (*FlowData, error) {
	flowName := m.sessionName + flowSessionSuffix
	sess, err := m.store.Get(r, flowName)
	if err != nil {
		return nil, fmt.Errorf("failed to get flow session: %w", err)
	}

	state, ok := sess.Values[keyState].(string)
	if !ok || state == "" {
		return nil, ErrFlowDataNotFound
	}

	codeVerifier, _ := sess.Values[keyCodeVerifier].(string)
	nonce, _ := sess.Values[keyNonce].(string)
	provider, _ := sess.Values[keyProvider].(string)
	redirectURI, _ := sess.Values[keyRedirectURI].(string)
	flowCreated, _ := sess.Values[keyFlowCreated].(int64)

	var metadata map[string]string
	if raw, ok := sess.Values[keyFlowMetadata].(string); ok && raw != "" {
		if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal flow metadata: %w", err)
		}
	}

	inviteToken, _ := sess.Values[keyInviteToken].(string)

	data := &FlowData{
		State:        state,
		CodeVerifier: codeVerifier,
		Nonce:        nonce,
		Provider:     provider,
		RedirectURI:  redirectURI,
		CreatedAt:    time.Unix(flowCreated, 0),
		Metadata:     metadata,
		InviteToken:  inviteToken,
	}

	// Clear flow data after retrieval (one-time use)
	sess.Options = m.flowCookieOptions()
	sess.Options.MaxAge = -1
	for key := range sess.Values {
		delete(sess.Values, key)
	}
	if saveErr := sess.Save(r, w); saveErr != nil {
		return nil, fmt.Errorf("failed to clear flow session: %w", saveErr)
	}

	return data, nil
}

// cookieOptions returns gorilla session options for the main session cookie.
func (m *GorillaSessionManager) cookieOptions() *sessions.Options {
	return &sessions.Options{
		MaxAge:   m.options.MaxAge,
		Domain:   m.options.Domain,
		Path:     m.options.Path,
		HttpOnly: m.options.HttpOnly,
		Secure:   m.options.Secure,
		SameSite: m.options.SameSite,
	}
}

// flowCookieOptions returns gorilla session options for the short-lived flow session.
func (m *GorillaSessionManager) flowCookieOptions() *sessions.Options {
	return &sessions.Options{
		MaxAge:   flowMaxAge,
		Domain:   m.options.Domain,
		Path:     m.options.Path,
		HttpOnly: true,
		Secure:   m.options.Secure,
		SameSite: http.SameSiteLaxMode,
	}
}
