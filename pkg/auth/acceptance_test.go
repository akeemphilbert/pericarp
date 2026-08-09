package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net/http"
	"net/http/httptest"
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

	svc    *application.DefaultAuthenticationService
	jwtSvc *authjwt.RSAJWTService
	sm     session.SessionManager
	server *httptest.Server

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

	w.svc = application.NewDefaultAuthenticationService(
		application.OAuthProviderRegistry{},
		w.agents, w.credentials, w.sessions, w.accounts,
		application.WithPasswordCredentialRepository(w.passwords),
		application.WithJWTService(w.jwtSvc),
		application.WithBcryptCost(bcrypt.MinCost),
	)

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
	w.server = httptest.NewServer(mux)
	return nil
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

func isNotMemberError(err error) bool {
	return err != nil && strings.Contains(err.Error(), application.ErrAccountNotMember.Error())
}
