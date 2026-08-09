package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/glebarez/sqlite"
	"github.com/gorilla/sessions"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/akeemphilbert/pericarp/pkg/auth"
	"github.com/akeemphilbert/pericarp/pkg/auth/application"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/repositories"
	gorminfra "github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/database/gorm"
	authhttp "github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/http"
	authjwt "github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/jwt"
	"github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/session"
	esdomain "github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// TestAcceptance runs the Gherkin acceptance contract in features/ against the
// real authentication service, the real GORM repositories over sqlite, and the
// real RequireAuth middleware over an httptest server. Nothing here is stubbed:
// a stubbed AuthenticationService is what let sessions ship unscoped in the
// first place, since the stub returned an account the real service never set.
func TestAcceptance(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("acceptance suite failed")
	}
}

const (
	testPassword  = "correct-horse-battery-staple"
	testIPAddress = "192.0.2.10"
	testUserAgent = "godog-acceptance/1.0"
	jwtCookieName = "pericarp_token"
)

// world holds one scenario's state. A fresh one is built per scenario, over
// its own in-memory database, so scenarios cannot see each other's rows.
type world struct {
	db          *gorm.DB
	agents      repositories.AgentRepository
	accounts    repositories.AccountRepository
	credentials repositories.CredentialRepository
	sessions    repositories.AuthSessionRepository
	passwords   repositories.PasswordCredentialRepository
	invites     repositories.InviteRepository

	svc       *application.DefaultAuthenticationService
	inviteSvc *application.InviteService
	jwtSvc    *authjwt.RSAJWTService
	sm        session.SessionManager
	server    *httptest.Server

	// provider is the stand-in identity provider. signerProfile is the profile
	// it hands back at code exchange — whoever the scenario is signing in.
	provider      *stubOAuthProvider
	signerProfile application.UserInfo
	inviteToken   string

	callbackStatus  int
	callbackCookies []*http.Cookie

	// emails maps a scenario-friendly agent name to its login email. Agent and
	// account IDs are the friendly names themselves, so assertions read the
	// same way the feature file does.
	emails map[string]string

	sess          *entities.AuthSession
	sessionEvents []esdomain.EventEnvelope[any]
	reloaded      *entities.AuthSession
	rebuilt       *entities.AuthSession
	info          *application.SessionInfo
	createdPerson *entities.Account

	cookies    []*http.Cookie
	lastErr    error
	lastStatus int
	lastBody   map[string]string

	identity     *auth.Identity
	ownership    auth.ResourceOwnership
	ownershipErr error
}

func InitializeScenario(sc *godog.ScenarioContext) {
	w := &world{}

	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, w.setup()
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		w.teardown()
		return ctx, nil
	})

	// Background / Given — world state
	sc.Step(`^an active agent "([^"]*)" with a password credential for "([^"]*)"$`, w.seedPasswordAgent)
	sc.Step(`^an active agent "([^"]*)" with a password credential and no account membership$`, w.seedAgentWithoutAccount)
	sc.Step(`^"([^"]*)" owns an active personal account "([^"]*)"$`, w.ownsPersonalAccount)
	sc.Step(`^a protected endpoint mounted behind session authentication$`, w.protectedEndpointMounted)
	sc.Step(`^an organization account "([^"]*)" exists$`, w.organizationAccountExists)
	sc.Step(`^"([^"]*)" is also a "([^"]*)" of the organization account "([^"]*)"$`, w.isAlsoMemberOfOrg)
	sc.Step(`^"([^"]*)" is an active "([^"]*)" of the organization account "([^"]*)"$`, w.isActiveMemberOfOrg)
	sc.Step(`^"([^"]*)" is not a member of "([^"]*)"$`, w.isNotAMemberOf)
	sc.Step(`^"([^"]*)" is not a member of any other account$`, w.isNotAMemberOfAnyOther)
	sc.Step(`^the account "([^"]*)" is deactivated$`, w.accountIsDeactivated)
	sc.Step(`^an agent "([^"]*)" who has never signed in before$`, w.agentNeverSignedIn)
	sc.Step(`^an agent "([^"]*)" was invited to "([^"]*)" as "([^"]*)" and has no personal account$`, w.invitedAgentWithoutPersonalAccount)
	sc.Step(`^"([^"]*)" has signed in and holds a session cookie$`, w.hasSignedInWithCookie)
	sc.Step(`^"([^"]*)" has signed in and holds a session scoped to "([^"]*)"$`, w.hasSignedInScopedTo)
	sc.Step(`^"([^"]*)" holds a session cookie for a stored session with no account$`, w.holdsCookieForUnscopedSession)
	sc.Step(`^that session has been revoked$`, w.sessionHasBeenRevoked)
	sc.Step(`^that session has expired$`, w.sessionHasExpired)

	// Given — sign-in callback fixtures
	sc.Step(`^an identity provider "([^"]*)" that returns the profile of whoever is signing in$`, w.identityProviderConfigured)
	sc.Step(`^a sign-in callback mounted for "([^"]*)"$`, w.callbackMounted)
	sc.Step(`^an active agent "([^"]*)" known to "([^"]*)" with email "([^"]*)"$`, w.agentKnownToProvider)
	sc.Step(`^an agent "([^"]*)" not yet known to "([^"]*)"$`, w.agentNotYetKnownToProvider)
	sc.Step(`^an organization account "([^"]*)" owned by "([^"]*)"$`, w.orgAccountOwnedBy)
	sc.Step(`^"([^"]*)" holds a pending invite to "([^"]*)" as "([^"]*)"$`, w.holdsPendingInvite)
	sc.Step(`^"([^"]*)" is not a member of any account$`, w.isNotAMemberOfAnyAccount)

	// When — sign-in callback
	sc.Step(`^"([^"]*)" completes the sign-in callback$`, w.completesCallback)
	sc.Step(`^"([^"]*)" completes the sign-in callback with the invite$`, w.completesCallbackWithInvite)

	// Then — sign-in callback
	sc.Step(`^the callback completes successfully$`, w.callbackCompletes)
	sc.Step(`^the session stored by the callback is scoped to account "([^"]*)"$`, w.callbackSessionScopedTo)
	sc.Step(`^the session stored by the callback is not scoped to any account$`, w.callbackSessionUnscoped)
	sc.Step(`^the callback creates an active personal account for "([^"]*)"$`, w.callbackCreatedPersonalAccount)
	sc.Step(`^the session stored by the callback is scoped to that new account$`, w.callbackSessionScopedToNewAccount)
	sc.Step(`^"([^"]*)" owns no personal account$`, w.ownsNoPersonalAccount)
	sc.Step(`^the callback issues an identity token whose active account is "([^"]*)"$`, w.callbackTokenActiveAccount)
	sc.Step(`^the callback issues no identity token$`, w.callbackIssuesNoToken)

	// When — actions
	sc.Step(`^"([^"]*)" signs in$`, w.signsIn)
	sc.Step(`^"([^"]*)" signs in by accepting the invite$`, w.signsIn)
	sc.Step(`^"([^"]*)" signs in for the first time with a new credential$`, w.signsInFirstTime)
	sc.Step(`^"([^"]*)" calls the protected endpoint$`, w.callsProtectedEndpoint)
	sc.Step(`^"([^"]*)" switches the active account to "([^"]*)"$`, w.switchesActiveAccount)
	sc.Step(`^a session is requested for "([^"]*)" scoped to account "([^"]*)"$`, w.sessionRequestedScopedTo)
	sc.Step(`^the session is scoped to account "([^"]*)"$`, w.sessionIsScopedTo)
	sc.Step(`^the session is validated$`, w.sessionIsValidated)
	sc.Step(`^the session is reloaded from the session repository$`, w.sessionIsReloaded)
	sc.Step(`^the session is rebuilt from its recorded events$`, w.sessionIsRebuiltFromEvents)

	// Then — assertions
	sc.Step(`^sign-in succeeds$`, w.signInSucceeds)
	sc.Step(`^the new session is scoped to account "([^"]*)"$`, w.newSessionScopedTo)
	sc.Step(`^the new session is scoped to that newly created account$`, w.newSessionScopedToCreatedAccount)
	sc.Step(`^an active personal account is created for "([^"]*)"$`, w.personalAccountCreatedFor)
	sc.Step(`^a session is stored for "([^"]*)"$`, w.sessionStoredFor)
	sc.Step(`^no session is stored for "([^"]*)"$`, w.noSessionStoredFor)
	sc.Step(`^that session is not scoped to any account$`, w.sessionNotScopedToAnyAccount)
	sc.Step(`^the stored session for "([^"]*)" is still active$`, w.storedSessionStillActive)
	sc.Step(`^the reloaded session is scoped to account "([^"]*)"$`, w.reloadedSessionScopedTo)
	sc.Step(`^the rebuilt session is scoped to account "([^"]*)"$`, w.rebuiltSessionScopedTo)
	sc.Step(`^the session info reports account "([^"]*)"$`, w.sessionInfoReportsAccount)
	sc.Step(`^the session is still scoped to account "([^"]*)"$`, w.storedSessionStillScopedTo)
	sc.Step(`^the request is rejected because "([^"]*)" is not a member of "([^"]*)"$`, w.rejectedAsNotMember)
	sc.Step(`^the request succeeds$`, w.requestSucceeds)
	sc.Step(`^the request is rejected as unauthenticated$`, w.requestRejectedUnauthenticated)
	sc.Step(`^the refusal is coded "([^"]*)"$`, w.refusalIsCoded)
	sc.Step(`^no identity is attached to the request$`, w.noIdentityAttached)
	sc.Step(`^the identity on the request has agent "([^"]*)"$`, w.identityHasAgent)
	sc.Step(`^the identity on the request has active account "([^"]*)"$`, w.identityHasActiveAccount)
	sc.Step(`^resource ownership derived from the request is account "([^"]*)" created by "([^"]*)"$`, w.ownershipIs)
	sc.Step(`^the accounts listed on the identity do not contain an empty value$`, w.accountsHaveNoEmptyValue)
	sc.Step(`^the accounts listed on the identity include "([^"]*)"$`, w.accountsInclude)
	sc.Step(`^the accounts listed on the identity are "([^"]*)" and "([^"]*)"$`, w.accountsAreExactly)
}

// ---------------------------------------------------------------- world setup

func (w *world) setup() error {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	if err := gorminfra.AutoMigrate(db); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("generate signing key: %w", err)
	}

	w.db = db
	w.agents = gorminfra.NewAgentRepository(db)
	w.accounts = gorminfra.NewAccountRepository(db)
	w.credentials = gorminfra.NewCredentialRepository(db)
	w.sessions = gorminfra.NewAuthSessionRepository(db)
	w.passwords = gorminfra.NewPasswordCredentialRepository(db)
	w.jwtSvc = authjwt.NewRSAJWTService(authjwt.WithSigningKey(key))
	w.emails = map[string]string{}
	w.cookies = nil
	w.lastErr = nil
	w.lastStatus = 0
	w.identity = nil
	w.ownership = auth.ResourceOwnership{}
	w.ownershipErr = nil
	w.sess = nil
	w.sessionEvents = nil
	w.info = nil
	w.reloaded = nil
	w.rebuilt = nil
	w.createdPerson = nil

	w.invites = gorminfra.NewInviteRepository(db)
	w.provider = &stubOAuthProvider{world: w}
	w.signerProfile = application.UserInfo{}
	w.inviteToken = ""
	w.callbackStatus = 0
	w.callbackCookies = nil
	w.lastBody = nil

	w.svc = application.NewDefaultAuthenticationService(
		application.OAuthProviderRegistry{"google": w.provider},
		w.agents, w.credentials, w.sessions, w.accounts,
		application.WithPasswordCredentialRepository(w.passwords),
		application.WithJWTService(w.jwtSvc),
		application.WithBcryptCost(bcrypt.MinCost),
	)

	// The real invite service, acting as the callback's InviteAcceptor, so the
	// invited-agent scenario exercises actual invite acceptance rather than a
	// stand-in that returns whatever the test wants.
	w.inviteSvc = application.NewInviteService(
		w.invites, w.agents, w.accounts, w.credentials, w.jwtSvc)

	w.sm = session.NewGorillaSessionManager(
		"acceptance-session",
		sessions.NewCookieStore([]byte("acceptance-test-key-32-bytes-ok!")),
		session.DefaultSessionOptions(),
	)

	mux := http.NewServeMux()
	mux.Handle("/protected", authhttp.RequireAuth(w.sm, w.svc)(http.HandlerFunc(
		func(rw http.ResponseWriter, r *http.Request) {
			w.identity = auth.AgentFromCtx(r.Context())
			w.ownership, w.ownershipErr = auth.ResourceOwnershipFromCtx(r.Context())
			rw.WriteHeader(http.StatusOK)
		})))
	mux.Handle("/switch", authhttp.RequireJWT(w.jwtSvc, jwtCookieName)(
		authhttp.SwitchActiveAccountHandler(w.jwtSvc, w.accounts, w.sm, w.svc)))

	handlers := authhttp.NewAuthHandlers(authhttp.HandlerConfig{
		AuthService:     w.svc,
		SessionManager:  w.sm,
		Credentials:     w.credentials,
		RedirectURI:     authhttp.RedirectURIConfig{CallbackPath: "/auth/callback"},
		DefaultProvider: "google",
		SessionDuration: 24 * time.Hour,
		JWTCookieName:   jwtCookieName,
		InviteAcceptor:  w.inviteSvc,
	})
	mux.HandleFunc("/auth/login", handlers.Login)
	mux.HandleFunc("/auth/callback", handlers.Callback)

	w.server = httptest.NewServer(mux)
	return nil
}

// stubOAuthProvider stands in for the identity provider. Only the code
// exchange matters here: it returns the profile of whoever the scenario is
// signing in, which is what a real provider would hand back.
type stubOAuthProvider struct {
	world *world
}

func (p *stubOAuthProvider) Name() string { return "google" }

func (p *stubOAuthProvider) AuthCodeURL(state, codeChallenge, nonce, redirectURI string) string {
	return "https://provider.test/authorize?state=" + url.QueryEscape(state)
}

func (p *stubOAuthProvider) Exchange(_ context.Context, _, _, _ string) (*application.AuthResult, error) {
	return &application.AuthResult{
		AccessToken: "stub-access-token",
		TokenType:   "Bearer",
		ExpiresIn:   3600,
		UserInfo:    p.world.signerProfile,
	}, nil
}

func (p *stubOAuthProvider) RefreshToken(_ context.Context, _ string) (*application.AuthResult, error) {
	return nil, fmt.Errorf("stub provider: refresh not supported")
}

func (p *stubOAuthProvider) RevokeToken(_ context.Context, _ string) error { return nil }

func (p *stubOAuthProvider) ValidateIDToken(_ context.Context, _, _ string) (*application.UserInfo, error) {
	profile := p.world.signerProfile
	return &profile, nil
}

func (w *world) teardown() {
	if w.server != nil {
		w.server.Close()
	}
	if w.db != nil {
		if sqlDB, err := w.db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
}

// ------------------------------------------------------------- Given: fixtures

// seedPasswordAgent writes an agent, a password-provider credential and its
// bcrypt hash straight through the real repositories. Registration is not used
// because it always mints a personal account, and several scenarios need an
// agent without one.
func (w *world) seedPasswordAgent(agentID, email string) error {
	ctx := context.Background()
	normalized := strings.ToLower(strings.TrimSpace(email))

	agent, err := new(entities.Agent).With(agentID, agentID, entities.AgentTypePerson)
	if err != nil {
		return fmt.Errorf("build agent %s: %w", agentID, err)
	}
	if err := w.agents.Save(ctx, agent); err != nil {
		return fmt.Errorf("save agent %s: %w", agentID, err)
	}

	credential, err := new(entities.Credential).With(
		agentID+"-cred", agentID, entities.ProviderPassword, normalized, normalized, agentID)
	if err != nil {
		return fmt.Errorf("build credential for %s: %w", agentID, err)
	}
	if err := w.credentials.Save(ctx, credential); err != nil {
		return fmt.Errorf("save credential for %s: %w", agentID, err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	if err != nil {
		return fmt.Errorf("hash password for %s: %w", agentID, err)
	}
	pc, err := new(entities.PasswordCredential).With(
		agentID+"-pw", credential.GetID(), entities.PasswordAlgorithmBcrypt, string(hash))
	if err != nil {
		return fmt.Errorf("build password credential for %s: %w", agentID, err)
	}
	if err := w.passwords.Save(ctx, pc); err != nil {
		return fmt.Errorf("save password credential for %s: %w", agentID, err)
	}

	w.emails[agentID] = normalized
	return nil
}

func (w *world) seedAgentWithoutAccount(agentID string) error {
	return w.seedPasswordAgent(agentID, agentID+"@example.com")
}

func (w *world) createAccount(accountID, accountType string) (*entities.Account, error) {
	ctx := context.Background()
	existing, err := w.accounts.FindByID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("look up account %s: %w", accountID, err)
	}
	if existing != nil {
		return existing, nil
	}
	account, err := new(entities.Account).With(accountID, accountID, accountType)
	if err != nil {
		return nil, fmt.Errorf("build account %s: %w", accountID, err)
	}
	if err := w.accounts.Save(ctx, account); err != nil {
		return nil, fmt.Errorf("save account %s: %w", accountID, err)
	}
	return account, nil
}

func (w *world) addMember(accountID, agentID, role string) error {
	if err := w.accounts.SaveMember(context.Background(), accountID, agentID, role); err != nil {
		return fmt.Errorf("save membership %s/%s: %w", accountID, agentID, err)
	}
	return nil
}

func (w *world) ownsPersonalAccount(agentID, accountID string) error {
	if _, err := w.createAccount(accountID, entities.AccountTypePersonal); err != nil {
		return err
	}
	return w.addMember(accountID, agentID, entities.RoleOwner)
}

func (w *world) organizationAccountExists(accountID string) error {
	_, err := w.createAccount(accountID, entities.AccountTypeOrganization)
	return err
}

func (w *world) isAlsoMemberOfOrg(agentID, role, accountID string) error {
	if _, err := w.createAccount(accountID, entities.AccountTypeOrganization); err != nil {
		return err
	}
	return w.addMember(accountID, agentID, role)
}

// isActiveMemberOfOrg is the "is an active <role> of" phrasing, used where the
// scenario contrasts a live membership against a deactivated account. The
// account is created active, so it is the same fixture as isAlsoMemberOfOrg
// with the activeness stated explicitly; assert it rather than assume it.
func (w *world) isActiveMemberOfOrg(agentID, role, accountID string) error {
	if err := w.isAlsoMemberOfOrg(agentID, role, accountID); err != nil {
		return err
	}
	account, err := w.accounts.FindByID(context.Background(), accountID)
	if err != nil {
		return fmt.Errorf("load account %s: %w", accountID, err)
	}
	if account == nil || !account.Active() {
		return fmt.Errorf("account %s is not active", accountID)
	}
	return nil
}

func (w *world) isNotAMemberOf(agentID, accountID string) error {
	role, err := w.accounts.FindMemberRole(context.Background(), accountID, agentID)
	if err != nil {
		return fmt.Errorf("check membership: %w", err)
	}
	if role != "" {
		return fmt.Errorf("precondition failed: %s holds role %q in %s", agentID, role, accountID)
	}
	return nil
}

func (w *world) isNotAMemberOfAnyOther(agentID string) error {
	accounts, err := w.accounts.FindByMember(context.Background(), agentID)
	if err != nil {
		return fmt.Errorf("list memberships: %w", err)
	}
	if len(accounts) > 1 {
		return fmt.Errorf("precondition failed: %s belongs to %d accounts, want at most 1", agentID, len(accounts))
	}
	return nil
}

func (w *world) accountIsDeactivated(accountID string) error {
	ctx := context.Background()
	account, err := w.accounts.FindByID(ctx, accountID)
	if err != nil {
		return fmt.Errorf("load account %s: %w", accountID, err)
	}
	if account == nil {
		return fmt.Errorf("account %s not found", accountID)
	}
	if err := account.Deactivate(); err != nil {
		return fmt.Errorf("deactivate %s: %w", accountID, err)
	}
	if err := w.accounts.Save(ctx, account); err != nil {
		return fmt.Errorf("save deactivated %s: %w", accountID, err)
	}
	return nil
}

func (w *world) agentNeverSignedIn(agentID string) error {
	credential, err := w.credentials.FindByProvider(context.Background(), "google", agentID+"-oauth")
	if err != nil {
		return fmt.Errorf("check credential: %w", err)
	}
	if credential != nil {
		return fmt.Errorf("precondition failed: %s already has a credential", agentID)
	}
	w.emails[agentID] = agentID + "@example.com"
	return nil
}

// invitedAgentWithoutPersonalAccount seeds the state an invite acceptance
// leaves behind: an activated agent holding a membership in somebody else's
// account and owning no personal account of their own.
func (w *world) invitedAgentWithoutPersonalAccount(agentID, accountID, role string) error {
	if err := w.seedPasswordAgent(agentID, agentID+"@example.com"); err != nil {
		return err
	}
	if _, err := w.createAccount(accountID, entities.AccountTypeOrganization); err != nil {
		return err
	}
	if err := w.addMember(accountID, agentID, role); err != nil {
		return err
	}
	personal, err := w.accounts.FindPersonalByMember(context.Background(), agentID)
	if err != nil {
		return fmt.Errorf("check personal account: %w", err)
	}
	if personal != nil {
		return fmt.Errorf("precondition failed: %s has personal account %s", agentID, personal.GetID())
	}
	return nil
}

func (w *world) protectedEndpointMounted() error {
	if w.server == nil {
		return fmt.Errorf("no server mounted")
	}
	return nil
}

// -------------------------------------------------------------- When: actions

// signIn runs the production sign-in sequence: verify the password, then create
// a session scoped to the account the verification resolved.
func (w *world) signsIn(agentID string) error {
	ctx := context.Background()
	email, ok := w.emails[agentID]
	if !ok {
		return fmt.Errorf("no email seeded for %s", agentID)
	}

	agent, credential, account, err := w.svc.VerifyPassword(ctx, email, testPassword)
	if err != nil {
		w.lastErr = err
		return nil
	}

	accountID := ""
	if account != nil {
		accountID = account.GetID()
	}
	sess, err := w.svc.CreateSession(
		ctx, agent.GetID(), accountID, credential.GetID(), testIPAddress, testUserAgent, 24*time.Hour)
	w.lastErr = err
	if err != nil {
		return nil
	}
	w.sess = sess
	w.sessionEvents = sess.GetUncommittedEvents()
	return nil
}

func (w *world) signsInFirstTime(agentID string) error {
	ctx := context.Background()
	agent, credential, account, err := w.svc.FindOrCreateAgent(ctx, application.UserInfo{
		ProviderUserID: agentID + "-oauth",
		Email:          agentID + "@example.com",
		DisplayName:    agentID,
		Provider:       "google",
	})
	if err != nil {
		w.lastErr = err
		return nil
	}

	accountID := ""
	if account != nil {
		accountID = account.GetID()
		w.createdPerson = account
	}
	sess, err := w.svc.CreateSession(
		ctx, agent.GetID(), accountID, credential.GetID(), testIPAddress, testUserAgent, 24*time.Hour)
	w.lastErr = err
	if err != nil {
		return nil
	}
	w.sess = sess
	w.sessionEvents = sess.GetUncommittedEvents()
	w.emails[agentID] = agent.GetID()
	return nil
}

func (w *world) hasSignedInWithCookie(agentID string) error {
	if err := w.signsIn(agentID); err != nil {
		return err
	}
	if w.lastErr != nil {
		return fmt.Errorf("sign-in failed: %w", w.lastErr)
	}
	return w.issueCookies(w.sess)
}

func (w *world) hasSignedInScopedTo(agentID, accountID string) error {
	if err := w.hasSignedInWithCookie(agentID); err != nil {
		return err
	}
	if got := w.sess.AccountID(); got != accountID {
		return fmt.Errorf("session scoped to %q, want %q", got, accountID)
	}
	return nil
}

func (w *world) holdsCookieForUnscopedSession(agentID string) error {
	sess, err := w.svc.CreateSession(
		context.Background(), agentID, "", agentID+"-cred", testIPAddress, testUserAgent, 24*time.Hour)
	if err != nil {
		return fmt.Errorf("create unscoped session: %w", err)
	}
	w.sess = sess
	w.sessionEvents = sess.GetUncommittedEvents()
	return w.issueCookies(sess)
}

// issueCookies mints the HTTP session cookie the same way the login handler
// does, and keeps it for subsequent requests.
func (w *world) issueCookies(sess *entities.AuthSession) error {
	if sess == nil {
		return fmt.Errorf("no session to put in a cookie")
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	err := w.sm.CreateHTTPSession(rec, req, session.SessionData{
		SessionID: sess.GetID(),
		AgentID:   sess.AgentID(),
		AccountID: sess.AccountID(),
		CreatedAt: time.Now(),
		ExpiresAt: sess.ExpiresAt(),
	})
	if err != nil {
		return fmt.Errorf("create http session: %w", err)
	}
	w.cookies = rec.Result().Cookies()
	return nil
}

func (w *world) sessionHasBeenRevoked() error {
	if err := w.svc.RevokeSession(context.Background(), w.sess.GetID()); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func (w *world) sessionHasExpired() error {
	res := w.db.Exec("UPDATE auth_sessions SET expires_at = ? WHERE id = ?",
		time.Now().Add(-time.Hour), w.sess.GetID())
	if res.Error != nil {
		return fmt.Errorf("expire session: %w", res.Error)
	}
	if res.RowsAffected != 1 {
		return fmt.Errorf("expire session: %d rows affected, want 1", res.RowsAffected)
	}
	return nil
}

func (w *world) callsProtectedEndpoint(_ string) error {
	w.identity = nil
	w.ownership = auth.ResourceOwnership{}
	w.ownershipErr = nil

	req, err := http.NewRequest(http.MethodGet, w.server.URL+"/protected", nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	for _, c := range w.cookies {
		req.AddCookie(c)
	}
	resp, err := w.server.Client().Do(req)
	if err != nil {
		return fmt.Errorf("call protected endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	w.lastStatus = resp.StatusCode
	w.lastBody = nil
	body := map[string]string{}
	if decodeErr := json.NewDecoder(resp.Body).Decode(&body); decodeErr == nil {
		w.lastBody = body
	}
	return nil
}

func (w *world) switchesActiveAccount(agentID, accountID string) error {
	ctx := context.Background()
	agent, err := w.agents.FindByID(ctx, agentID)
	if err != nil {
		return fmt.Errorf("load agent %s: %w", agentID, err)
	}
	token, err := w.svc.IssueIdentityToken(ctx, agent, w.sess.AccountID())
	if err != nil {
		return fmt.Errorf("issue token: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, w.server.URL+"/switch",
		strings.NewReader(fmt.Sprintf(`{"account_id":%q}`, accountID)))
	if err != nil {
		return fmt.Errorf("build switch request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for _, c := range w.cookies {
		req.AddCookie(c)
	}
	req.AddCookie(&http.Cookie{Name: jwtCookieName, Value: token})

	resp, err := w.server.Client().Do(req)
	if err != nil {
		return fmt.Errorf("call switch endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	w.lastStatus = resp.StatusCode
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("switch account returned %d, want 200", resp.StatusCode)
	}
	return nil
}

func (w *world) sessionRequestedScopedTo(agentID, accountID string) error {
	sess, err := w.svc.CreateSession(
		context.Background(), agentID, accountID, agentID+"-cred", testIPAddress, testUserAgent, 24*time.Hour)
	w.lastErr = err
	if err == nil {
		w.sess = sess
	}
	return nil
}

func (w *world) sessionIsScopedTo(accountID string) error {
	w.lastErr = w.svc.ScopeSessionToAccount(context.Background(), w.sess.GetID(), accountID)
	return nil
}

func (w *world) sessionIsValidated() error {
	info, err := w.svc.ValidateSession(context.Background(), w.sess.GetID())
	w.lastErr = err
	if err != nil {
		return fmt.Errorf("validate session: %w", err)
	}
	w.info = info
	return nil
}

func (w *world) sessionIsReloaded() error {
	reloaded, err := w.sessions.FindByID(context.Background(), w.sess.GetID())
	if err != nil {
		return fmt.Errorf("reload session: %w", err)
	}
	if reloaded == nil {
		return fmt.Errorf("session %s not found in repository", w.sess.GetID())
	}
	w.reloaded = reloaded
	return nil
}

// sessionIsRebuiltFromEvents replays the events the aggregate recorded, so a
// field assigned without an event shows up as a rebuild that lost the account.
func (w *world) sessionIsRebuiltFromEvents() error {
	ctx := context.Background()
	if len(w.sessionEvents) == 0 {
		return fmt.Errorf("session recorded no events to rebuild from")
	}
	rebuilt := &entities.AuthSession{}
	if err := rebuilt.Restore(
		w.sess.GetID(), "placeholder", "", "", "", "", false,
		time.Time{}, time.Time{}, time.Time{}); err != nil {
		return fmt.Errorf("prepare rebuild: %w", err)
	}
	for i, event := range w.sessionEvents {
		if err := rebuilt.ApplyEvent(ctx, event); err != nil {
			return fmt.Errorf("apply event %d (%s): %w", i, event.EventType, err)
		}
	}
	w.rebuilt = rebuilt
	return nil
}

// ---------------------------------------------------------- Then: assertions

func (w *world) signInSucceeds() error {
	if w.lastErr != nil {
		return fmt.Errorf("sign-in failed: %w", w.lastErr)
	}
	if w.sess == nil {
		return fmt.Errorf("sign-in produced no session")
	}
	return nil
}

func (w *world) newSessionScopedTo(accountID string) error {
	if w.lastErr != nil {
		return fmt.Errorf("sign-in failed: %w", w.lastErr)
	}
	if w.sess == nil {
		return fmt.Errorf("no session was created")
	}
	if got := w.sess.AccountID(); got != accountID {
		return fmt.Errorf("session scoped to %q, want %q", got, accountID)
	}
	return nil
}

func (w *world) newSessionScopedToCreatedAccount() error {
	if w.createdPerson == nil {
		return fmt.Errorf("no account was created during sign-in")
	}
	return w.newSessionScopedTo(w.createdPerson.GetID())
}

func (w *world) personalAccountCreatedFor(agentID string) error {
	if w.lastErr != nil {
		return fmt.Errorf("sign-in failed: %w", w.lastErr)
	}
	if w.createdPerson == nil {
		return fmt.Errorf("sign-in created no account for %s", agentID)
	}
	personal, err := w.accounts.FindPersonalByMember(context.Background(), w.sess.AgentID())
	if err != nil {
		return fmt.Errorf("look up personal account: %w", err)
	}
	if personal == nil {
		return fmt.Errorf("no personal account stored for %s", agentID)
	}
	if !personal.Active() {
		return fmt.Errorf("personal account %s is not active", personal.GetID())
	}
	return nil
}

func (w *world) sessionStoredFor(agentID string) error {
	count, err := w.storedSessionCount(agentID)
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("no session stored for %s", agentID)
	}
	return nil
}

func (w *world) noSessionStoredFor(agentID string) error {
	count, err := w.storedSessionCount(agentID)
	if err != nil {
		return err
	}
	if count != 0 {
		return fmt.Errorf("%d sessions stored for %s, want none", count, agentID)
	}
	return nil
}

func (w *world) storedSessionCount(agentID string) (int64, error) {
	var count int64
	if err := w.db.Raw("SELECT COUNT(*) FROM auth_sessions WHERE agent_id = ?", agentID).
		Scan(&count).Error; err != nil {
		return 0, fmt.Errorf("count sessions: %w", err)
	}
	return count, nil
}

func (w *world) sessionNotScopedToAnyAccount() error {
	if w.sess == nil {
		return fmt.Errorf("no session was created")
	}
	if got := w.sess.AccountID(); got != "" {
		return fmt.Errorf("session scoped to %q, want no account", got)
	}
	return nil
}

func (w *world) storedSessionStillActive(agentID string) error {
	stored, err := w.sessions.FindByID(context.Background(), w.sess.GetID())
	if err != nil {
		return fmt.Errorf("reload session: %w", err)
	}
	if stored == nil {
		return fmt.Errorf("no stored session for %s", agentID)
	}
	if !stored.Active() {
		return fmt.Errorf("stored session for %s is not active", agentID)
	}
	return nil
}

func (w *world) reloadedSessionScopedTo(accountID string) error {
	if w.reloaded == nil {
		return fmt.Errorf("session was not reloaded")
	}
	if got := w.reloaded.AccountID(); got != accountID {
		return fmt.Errorf("reloaded session scoped to %q, want %q", got, accountID)
	}
	return nil
}

func (w *world) rebuiltSessionScopedTo(accountID string) error {
	if w.rebuilt == nil {
		return fmt.Errorf("session was not rebuilt")
	}
	if got := w.rebuilt.AccountID(); got != accountID {
		return fmt.Errorf("rebuilt session scoped to %q, want %q", got, accountID)
	}
	return nil
}

func (w *world) sessionInfoReportsAccount(accountID string) error {
	if w.info == nil {
		return fmt.Errorf("session was not validated")
	}
	if got := w.info.AccountID; got != accountID {
		return fmt.Errorf("session info reports account %q, want %q", got, accountID)
	}
	return nil
}

func (w *world) storedSessionStillScopedTo(accountID string) error {
	stored, err := w.sessions.FindByID(context.Background(), w.sess.GetID())
	if err != nil {
		return fmt.Errorf("reload session: %w", err)
	}
	if stored == nil {
		return fmt.Errorf("session %s not found", w.sess.GetID())
	}
	if got := stored.AccountID(); got != accountID {
		return fmt.Errorf("stored session scoped to %q, want %q", got, accountID)
	}
	return nil
}

func (w *world) rejectedAsNotMember(agentID, accountID string) error {
	if w.lastErr == nil {
		return fmt.Errorf("expected %s to be refused for %s, got no error", agentID, accountID)
	}
	if !isNotMemberError(w.lastErr) {
		return fmt.Errorf("got error %v, want ErrAccountNotMember", w.lastErr)
	}
	return nil
}

func (w *world) requestSucceeds() error {
	if w.lastStatus != http.StatusOK {
		return fmt.Errorf("status %d, want 200", w.lastStatus)
	}
	return nil
}

func (w *world) requestRejectedUnauthenticated() error {
	if w.lastStatus != http.StatusUnauthorized {
		return fmt.Errorf("status %d, want 401", w.lastStatus)
	}
	return nil
}

func (w *world) noIdentityAttached() error {
	if w.identity != nil {
		return fmt.Errorf("identity %+v reached the handler, want none", w.identity)
	}
	return nil
}

func (w *world) identityHasAgent(agentID string) error {
	if w.identity == nil {
		return fmt.Errorf("no identity on the request")
	}
	if w.identity.AgentID != agentID {
		return fmt.Errorf("identity agent %q, want %q", w.identity.AgentID, agentID)
	}
	return nil
}

func (w *world) identityHasActiveAccount(accountID string) error {
	if w.identity == nil {
		return fmt.Errorf("no identity on the request")
	}
	if w.identity.ActiveAccountID != accountID {
		return fmt.Errorf("identity active account %q, want %q", w.identity.ActiveAccountID, accountID)
	}
	return nil
}

func (w *world) ownershipIs(accountID, agentID string) error {
	if w.ownershipErr != nil {
		return fmt.Errorf("deriving resource ownership failed: %w", w.ownershipErr)
	}
	if w.ownership.AccountID != accountID {
		return fmt.Errorf("ownership account %q, want %q", w.ownership.AccountID, accountID)
	}
	if w.ownership.CreatedByAgentID != agentID {
		return fmt.Errorf("ownership creator %q, want %q", w.ownership.CreatedByAgentID, agentID)
	}
	return nil
}

func (w *world) accountsHaveNoEmptyValue() error {
	if w.identity == nil {
		return fmt.Errorf("no identity on the request")
	}
	for i, id := range w.identity.AccountIDs {
		if id == "" {
			return fmt.Errorf("AccountIDs[%d] is empty: %#v", i, w.identity.AccountIDs)
		}
	}
	return nil
}

func (w *world) accountsInclude(accountID string) error {
	if w.identity == nil {
		return fmt.Errorf("no identity on the request")
	}
	for _, id := range w.identity.AccountIDs {
		if id == accountID {
			return nil
		}
	}
	return fmt.Errorf("AccountIDs %v does not include %q", w.identity.AccountIDs, accountID)
}

func (w *world) accountsAreExactly(first, second string) error {
	if w.identity == nil {
		return fmt.Errorf("no identity on the request")
	}
	want := map[string]bool{first: false, second: false}
	for _, id := range w.identity.AccountIDs {
		seen, expected := want[id]
		if !expected {
			return fmt.Errorf("AccountIDs %v contains unexpected %q", w.identity.AccountIDs, id)
		}
		if seen {
			return fmt.Errorf("AccountIDs %v lists %q twice", w.identity.AccountIDs, id)
		}
		want[id] = true
	}
	for id, seen := range want {
		if !seen {
			return fmt.Errorf("AccountIDs %v is missing %q", w.identity.AccountIDs, id)
		}
	}
	return nil
}

// ------------------------------------------------- sign-in callback: fixtures

func (w *world) identityProviderConfigured(name string) error {
	if w.provider == nil || w.provider.Name() != name {
		return fmt.Errorf("no identity provider %q configured", name)
	}
	return nil
}

func (w *world) callbackMounted(name string) error {
	if w.server == nil {
		return fmt.Errorf("no callback mounted for %q", name)
	}
	return nil
}

// agentKnownToProvider seeds an agent the provider already knows: an agent row
// plus the provider credential a previous sign-in would have left behind.
func (w *world) agentKnownToProvider(agentID, provider, email string) error {
	ctx := context.Background()
	agent, err := new(entities.Agent).With(agentID, agentID, entities.AgentTypePerson)
	if err != nil {
		return fmt.Errorf("build agent %s: %w", agentID, err)
	}
	if err := w.agents.Save(ctx, agent); err != nil {
		return fmt.Errorf("save agent %s: %w", agentID, err)
	}

	providerUserID := agentID + "-oauth"
	credential, err := new(entities.Credential).With(
		agentID+"-cred", agentID, provider, providerUserID, email, agentID)
	if err != nil {
		return fmt.Errorf("build credential for %s: %w", agentID, err)
	}
	if err := w.credentials.Save(ctx, credential); err != nil {
		return fmt.Errorf("save credential for %s: %w", agentID, err)
	}

	w.signerProfile = application.UserInfo{
		ProviderUserID: providerUserID,
		Email:          email,
		DisplayName:    agentID,
		Provider:       provider,
	}
	return nil
}

func (w *world) agentNotYetKnownToProvider(agentID, provider string) error {
	providerUserID := agentID + "-oauth"
	existing, err := w.credentials.FindByProvider(context.Background(), provider, providerUserID)
	if err != nil {
		return fmt.Errorf("check credential: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("precondition failed: %s is already known to %s", agentID, provider)
	}
	w.signerProfile = application.UserInfo{
		ProviderUserID: providerUserID,
		Email:          agentID + "@example.com",
		DisplayName:    agentID,
		Provider:       provider,
	}
	return nil
}

func (w *world) orgAccountOwnedBy(accountID, ownerID string) error {
	ctx := context.Background()
	owner, err := new(entities.Agent).With(ownerID, ownerID, entities.AgentTypePerson)
	if err != nil {
		return fmt.Errorf("build owner %s: %w", ownerID, err)
	}
	if err := w.agents.Save(ctx, owner); err != nil {
		return fmt.Errorf("save owner %s: %w", ownerID, err)
	}
	if _, err := w.createAccount(accountID, entities.AccountTypeOrganization); err != nil {
		return err
	}
	return w.addMember(accountID, ownerID, entities.RoleOwner)
}

// holdsPendingInvite issues a real invite through the real InviteService, so
// the callback later accepts a genuine token rather than a fabricated one.
func (w *world) holdsPendingInvite(agentID, accountID, role string) error {
	email := agentID + "@example.com"
	_, token, err := w.inviteSvc.CreateInvite(context.Background(), accountID, email, role, "owner")
	if err != nil {
		return fmt.Errorf("create invite for %s: %w", agentID, err)
	}
	w.inviteToken = token
	w.signerProfile = application.UserInfo{
		ProviderUserID: agentID + "-oauth",
		Email:          email,
		DisplayName:    agentID,
		Provider:       "google",
	}
	return nil
}

func (w *world) isNotAMemberOfAnyAccount(agentID string) error {
	accounts, err := w.accounts.FindByMember(context.Background(), agentID)
	if err != nil {
		return fmt.Errorf("list memberships: %w", err)
	}
	if len(accounts) != 0 {
		return fmt.Errorf("precondition failed: %s belongs to %d accounts, want none", agentID, len(accounts))
	}
	return nil
}

// --------------------------------------------------- sign-in callback: action

func (w *world) completesCallback(_ string) error {
	return w.runCallback("")
}

func (w *world) completesCallbackWithInvite(_ string) error {
	if w.inviteToken == "" {
		return fmt.Errorf("no invite token was issued")
	}
	return w.runCallback(w.inviteToken)
}

// runCallback drives the real login and callback handlers over HTTP: start the
// flow, take the state out of the provider redirect, come back with it.
func (w *world) runCallback(inviteToken string) error {
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	loginURL := w.server.URL + "/auth/login"
	if inviteToken != "" {
		loginURL += "?invite_token=" + url.QueryEscape(inviteToken)
	}
	loginResp, err := client.Get(loginURL)
	if err != nil {
		return fmt.Errorf("start login flow: %w", err)
	}
	defer func() { _ = loginResp.Body.Close() }()
	if loginResp.StatusCode != http.StatusFound {
		return fmt.Errorf("login returned %d, want 302", loginResp.StatusCode)
	}

	authorizeURL, err := url.Parse(loginResp.Header.Get("Location"))
	if err != nil {
		return fmt.Errorf("parse provider redirect: %w", err)
	}
	state := authorizeURL.Query().Get("state")
	if state == "" {
		return fmt.Errorf("provider redirect carried no state: %s", loginResp.Header.Get("Location"))
	}

	callbackURL := fmt.Sprintf("%s/auth/callback?state=%s&code=stub-auth-code",
		w.server.URL, url.QueryEscape(state))
	req, err := http.NewRequest(http.MethodGet, callbackURL, nil)
	if err != nil {
		return fmt.Errorf("build callback request: %w", err)
	}
	for _, c := range loginResp.Cookies() {
		req.AddCookie(c)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("call callback: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	w.callbackStatus = resp.StatusCode
	w.callbackCookies = resp.Cookies()
	w.cookies = resp.Cookies()
	return nil
}

// ----------------------------------------------- sign-in callback: assertions

func (w *world) callbackCompletes() error {
	if w.callbackStatus != http.StatusFound {
		return fmt.Errorf("callback returned %d, want 302", w.callbackStatus)
	}
	return nil
}

// callbackSession reads the session the callback stored, reached the way a
// browser would: through the cookie it set.
func (w *world) callbackSession() (*entities.AuthSession, error) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	for _, c := range w.callbackCookies {
		req.AddCookie(c)
	}
	data, err := w.sm.GetHTTPSession(req)
	if err != nil {
		return nil, fmt.Errorf("read session cookie set by the callback: %w", err)
	}
	if data == nil || data.SessionID == "" {
		return nil, fmt.Errorf("callback set no session cookie")
	}
	stored, err := w.sessions.FindByID(context.Background(), data.SessionID)
	if err != nil {
		return nil, fmt.Errorf("load stored session: %w", err)
	}
	if stored == nil {
		return nil, fmt.Errorf("session %s was not stored", data.SessionID)
	}
	w.sess = stored
	return stored, nil
}

func (w *world) callbackSessionScopedTo(accountID string) error {
	stored, err := w.callbackSession()
	if err != nil {
		return err
	}
	if got := stored.AccountID(); got != accountID {
		return fmt.Errorf("session stored by the callback is scoped to %q, want %q", got, accountID)
	}
	return nil
}

func (w *world) callbackSessionUnscoped() error {
	stored, err := w.callbackSession()
	if err != nil {
		return err
	}
	if got := stored.AccountID(); got != "" {
		return fmt.Errorf("session stored by the callback is scoped to %q, want no account", got)
	}
	return nil
}

func (w *world) callbackCreatedPersonalAccount(agentID string) error {
	stored, err := w.callbackSession()
	if err != nil {
		return err
	}
	personal, err := w.accounts.FindPersonalByMember(context.Background(), stored.AgentID())
	if err != nil {
		return fmt.Errorf("look up personal account: %w", err)
	}
	if personal == nil {
		return fmt.Errorf("callback created no personal account for %s", agentID)
	}
	if !personal.Active() {
		return fmt.Errorf("personal account %s for %s is not active", personal.GetID(), agentID)
	}
	w.createdPerson = personal
	return nil
}

func (w *world) callbackSessionScopedToNewAccount() error {
	if w.createdPerson == nil {
		return fmt.Errorf("no account was created by the callback")
	}
	return w.callbackSessionScopedTo(w.createdPerson.GetID())
}

func (w *world) ownsNoPersonalAccount(agentID string) error {
	stored, err := w.callbackSession()
	if err != nil {
		return err
	}
	personal, err := w.accounts.FindPersonalByMember(context.Background(), stored.AgentID())
	if err != nil {
		return fmt.Errorf("look up personal account: %w", err)
	}
	if personal != nil {
		return fmt.Errorf("%s owns personal account %s, want none", agentID, personal.GetID())
	}
	return nil
}

func (w *world) callbackToken() string {
	for _, c := range w.callbackCookies {
		if c.Name == jwtCookieName {
			return c.Value
		}
	}
	return ""
}

func (w *world) callbackTokenActiveAccount(accountID string) error {
	token := w.callbackToken()
	if token == "" {
		return fmt.Errorf("callback issued no identity token")
	}
	claims, err := w.jwtSvc.ValidateToken(context.Background(), token)
	if err != nil {
		return fmt.Errorf("validate issued token: %w", err)
	}
	if claims.ActiveAccountID != accountID {
		return fmt.Errorf("token active account %q, want %q", claims.ActiveAccountID, accountID)
	}
	return nil
}

func (w *world) callbackIssuesNoToken() error {
	if token := w.callbackToken(); token != "" {
		return fmt.Errorf("callback issued an identity token, want none")
	}
	return nil
}

func (w *world) refusalIsCoded(code string) error {
	if w.lastBody == nil {
		return fmt.Errorf("refusal carried no JSON body")
	}
	if got := w.lastBody["code"]; got != code {
		return fmt.Errorf("refusal coded %q, want %q (body: %v)", got, code, w.lastBody)
	}
	return nil
}

func isNotMemberError(err error) bool {
	return err != nil && strings.Contains(err.Error(), application.ErrAccountNotMember.Error())
}
