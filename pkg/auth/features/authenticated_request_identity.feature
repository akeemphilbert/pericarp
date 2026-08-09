@auth @session-scoping
Feature: An authenticated request carries the caller's active account

  Session-authenticated requests build their identity from the stored session.
  Whatever the session is scoped to has to arrive intact at the handler, because
  handlers derive resource ownership (tenant + creator) from that identity.

  Requests are strict about the account where sign-in is permissive. Sign-in
  lets an agent through with an unscoped session so a missing account cannot
  break the login callback itself (see session_account_scoping.feature); a
  request refuses that same session outright. The two halves differ on purpose:
  a session with no account cannot own resources, so admitting the request only
  defers the failure to a handler that has no way to explain it. Rejecting at
  the door reports the problem once, at the boundary, in terms the caller can
  act on — sign in again. The cost is deliberate and is stated in the scenario
  below: an agent with no account can sign in and still do nothing.

  These scenarios drive the real authentication service backed by real
  repositories. Existing middleware tests stub the authentication service and
  hand it an account that the real service never produces, which is why the
  empty active account survived those tests.

  Background:
    Given an active agent "ada" with a password credential for "ada@example.com"
    And "ada" owns an active personal account "ada-personal"
    And a protected endpoint mounted behind session authentication

  Rule: The identity on a request reflects the session's account

    Scenario: A signed-in agent's request carries a non-empty active account
      Given "ada" has signed in and holds a session cookie
      When "ada" calls the protected endpoint
      Then the request succeeds
      And the identity on the request has agent "ada"
      And the identity on the request has active account "ada-personal"

    Scenario: A handler can derive resource ownership on an authenticated request
      # This is the reported symptom: handlers tagging new resources with the
      # caller's tenant fail with "empty ActiveAccountID".
      Given "ada" has signed in and holds a session cookie
      When "ada" calls the protected endpoint
      Then resource ownership derived from the request is account "ada-personal" created by "ada"

    Scenario: The accounts listed on the identity never include an empty account
      Given "ada" has signed in and holds a session cookie
      When "ada" calls the protected endpoint
      Then the accounts listed on the identity do not contain an empty value
      And the accounts listed on the identity include "ada-personal"

    @decision
    Scenario: The accounts listed on the identity cover every account the agent belongs to
      # DECISION: session auth currently lists only the session's own account,
      # while JWT auth lists every membership. Recorded here as "both list every
      # membership" so an account switcher works the same under either
      # middleware; the cost is a membership lookup per validated session.
      Given "ada" is also a "member" of the organization account "acme"
      And "ada" has signed in and holds a session cookie
      When "ada" calls the protected endpoint
      Then the accounts listed on the identity are "ada-personal" and "acme"
      And the identity on the request has active account "ada-personal"

    Scenario: Re-scoping a session changes the active account on later requests
      Given "ada" is also a "member" of the organization account "acme"
      And "ada" has signed in and holds a session cookie
      When the session is scoped to account "acme"
      And "ada" calls the protected endpoint
      Then the identity on the request has active account "acme"

  Rule: A request that cannot produce an account-scoped identity is not authenticated

    @decision
    Scenario: A stored session with no account is refused
      # DECISION: every session written before this change has an empty account,
      # and there is no backfill. Those sessions are refused and their owners
      # sign in again. The alternative — admitting the request unscoped — keeps
      # the original defect alive under a new name.
      Given "ada" holds a session cookie for a stored session with no account
      When "ada" calls the protected endpoint
      Then the request is rejected as unauthenticated
      And no identity is attached to the request

    @decision
    Scenario: An agent who signed in without an account is refused on every request
      # The consequence of a permissive sign-in meeting a strict request: this
      # agent holds a valid, active, unexpired session and still cannot make a
      # single authenticated call. Specified rather than emergent, so nobody
      # reads the two rules as a contradiction and "fixes" one of them.
      Given an active agent "orphan" with a password credential and no account membership
      And "orphan" has signed in and holds a session cookie
      When "orphan" calls the protected endpoint
      Then the request is rejected as unauthenticated
      And no identity is attached to the request
      And the stored session for "orphan" is still active

    Scenario: A revoked session yields no identity
      Given "ada" has signed in and holds a session cookie
      And that session has been revoked
      When "ada" calls the protected endpoint
      Then the request is rejected as unauthenticated
      And no identity is attached to the request

    Scenario: An expired session yields no identity
      Given "ada" has signed in and holds a session cookie
      And that session has expired
      When "ada" calls the protected endpoint
      Then the request is rejected as unauthenticated
      And no identity is attached to the request
