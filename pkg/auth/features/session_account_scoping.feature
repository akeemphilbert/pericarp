@wip @auth @session-scoping
Feature: An auth session is scoped to an account when the agent signs in

  Every authenticated request needs to know which account (tenant) the caller is
  acting in. That account is resolved during sign-in, when the agent, the
  credential and the account are all already in hand. The session is the only
  thing that survives sign-in, so the account has to be recorded on the session
  or it is lost.

  A session whose account is empty is not a usable session: downstream code that
  tags new resources with the caller's tenant (ResourceOwnershipFromCtx) rejects
  an identity with no active account, so the failure surfaces far away from the
  sign-in that caused it.

  Sign-in and request handling deliberately treat that case differently. Sign-in
  is permissive: an agent whose account cannot be resolved still completes
  sign-in and gets a stored session with no account, so a data anomaly can never
  break the OAuth callback or the password login itself. Request handling is
  strict: an unscoped session is refused, because letting the request through
  only moves the failure somewhere harder to read. The consequence — an agent
  who can sign in but cannot make a single authenticated request — is specified
  in authenticated_request_identity.feature rather than left to emerge.

  Background:
    Given an active agent "ada" with a password credential for "ada@example.com"
    And "ada" owns an active personal account "ada-personal"

  Rule: Signing in records the account that the sign-in resolved

    Scenario: An agent with a personal account gets a session scoped to it
      When "ada" signs in
      Then the new session is scoped to account "ada-personal"

    Scenario: A first-time agent is scoped to the personal account created during sign-in
      Given an agent "grace" who has never signed in before
      When "grace" signs in for the first time with a new credential
      Then an active personal account is created for "grace"
      And the new session is scoped to that newly created account

    Scenario: The account resolved by the sign-in wins over any default lookup
      # An invited agent joins somebody else's organization account and has no
      # personal account of their own. Resolving a "default" account inside
      # session creation would find nothing and leave the session unscoped, so
      # the account the sign-in path resolved must be the account recorded.
      Given an organization account "acme" exists
      And an agent "linus" was invited to "acme" as "member" and has no personal account
      When "linus" signs in by accepting the invite
      Then the new session is scoped to account "acme"

    @decision
    Scenario: An agent who belongs to several accounts is scoped to their personal account
      # DECISION: which account wins when there are several? Recorded here as
      # "personal account first", matching how FindOrCreateAgent already picks a
      # default. The alternative is "the account most recently used", which
      # nothing currently persists.
      Given "ada" is also a "member" of the organization account "acme"
      When "ada" signs in
      Then the new session is scoped to account "ada-personal"

    @decision
    Scenario: An agent with no account at all still signs in, with an unscoped session
      # DECISION: sign-in stays permissive. A missing account is a data anomaly,
      # and failing the sign-in itself would break the login callback for it.
      # The refusal happens on the next request instead — see
      # authenticated_request_identity.feature, "An agent who signed in without
      # an account is refused on every request".
      Given an active agent "orphan" with a password credential and no account membership
      When "orphan" signs in
      Then sign-in succeeds
      And a session is stored for "orphan"
      And that session is not scoped to any account

    @decision
    Scenario: A deactivated account is passed over in favour of an active one
      # DECISION: account lookups do not filter on active today, so a
      # deactivated account can be chosen and the agent lands in a dead tenant.
      # Recorded here as "skip deactivated accounts".
      Given the account "ada-personal" is deactivated
      And "ada" is an active "member" of the organization account "acme"
      When "ada" signs in
      Then the new session is scoped to account "acme"

    @decision
    Scenario: An agent whose only account is deactivated gets an unscoped session
      # Skipping the deactivated account leaves nothing to resolve, so this
      # falls through to the same permissive rule as an agent with no account
      # at all: sign-in succeeds, the session is unscoped, and the refusal
      # happens on the next request.
      Given the account "ada-personal" is deactivated
      And "ada" is not a member of any other account
      When "ada" signs in
      Then sign-in succeeds
      And that session is not scoped to any account

    @security
    Scenario: A session is never scoped to an account the agent does not belong to
      Given an organization account "acme" exists
      And "ada" is not a member of "acme"
      When a session is requested for "ada" scoped to account "acme"
      Then the request is rejected because "ada" is not a member of "acme"
      And no session is stored for "ada"

  Rule: The account survives every way a session is read back

    Scenario: Reloading a stored session keeps its account
      When "ada" signs in
      And the session is reloaded from the session repository
      Then the reloaded session is scoped to account "ada-personal"

    Scenario: Rebuilding a session from its recorded events keeps its account
      # Guards against the account being assigned as a plain field with no event
      # recorded — the stored row would be right and the event stream wrong.
      When "ada" signs in
      And the session is rebuilt from its recorded events
      Then the rebuilt session is scoped to account "ada-personal"

    Scenario: Validating a session reports the account it is scoped to
      When "ada" signs in
      And the session is validated
      Then the session info reports account "ada-personal"

  Rule: An existing session changes account only through an explicit scope change

    Scenario: Re-scoping a session to another account the agent belongs to
      Given "ada" is also a "member" of the organization account "acme"
      And "ada" has signed in and holds a session scoped to "ada-personal"
      When the session is scoped to account "acme"
      And the session is validated
      Then the session info reports account "acme"

    @security
    Scenario: Re-scoping a session to an account the agent does not belong to is refused
      Given an organization account "globex" exists
      And "ada" is not a member of "globex"
      And "ada" has signed in and holds a session scoped to "ada-personal"
      When the session is scoped to account "globex"
      Then the request is rejected because "ada" is not a member of "globex"
      And the session is still scoped to account "ada-personal"

    @decision
    Scenario: Switching the active account also re-scopes the stored session
      # DECISION: the switch-account endpoint currently reissues the JWT only.
      # Requests served by RequireAuth read the account from the session, so
      # after a switch those requests still act in the old account. Recorded
      # here as "the session follows the switch"; the alternative is to state
      # that account switching is a JWT-only feature and say so in the docs.
      Given "ada" is also a "member" of the organization account "acme"
      And "ada" has signed in and holds a session scoped to "ada-personal"
      When "ada" switches the active account to "acme"
      And the session is validated
      Then the session info reports account "acme"
