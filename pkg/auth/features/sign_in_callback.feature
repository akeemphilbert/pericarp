@auth @session-scoping
Feature: The sign-in callback carries the resolved account into the session

  The callback is where sign-in resolves an account, and it is the line the
  original defect lived on: the handler held the account, spent it on the cookie
  and the identity token, and passed nothing to session creation. Testing the
  service alone cannot catch a regression there — the service can be perfectly
  correct while the caller drops the account on the floor.

  So these scenarios drive the callback itself: a stub identity provider, the
  real login and callback handlers over HTTP, the real authentication service,
  the real repositories, and the real invite acceptance. Only the provider is
  a stand-in, because there is no Google to call.

  Background:
    Given an identity provider "google" that returns the profile of whoever is signing in
    And a sign-in callback mounted for "google"

  Rule: The callback records the account the sign-in resolved

    Scenario: A returning agent is scoped to their personal account
      Given an active agent "ada" known to "google" with email "ada@example.com"
      And "ada" owns an active personal account "ada-personal"
      When "ada" completes the sign-in callback
      Then the callback completes successfully
      And the session stored by the callback is scoped to account "ada-personal"

    Scenario: A first-time agent is scoped to the account the callback creates
      Given an agent "grace" not yet known to "google"
      When "grace" completes the sign-in callback
      Then the callback completes successfully
      And the callback creates an active personal account for "grace"
      And the session stored by the callback is scoped to that new account

    Scenario: An invited agent is scoped to the account that invited them
      # The case the whole fix turns on. An invited agent owns no personal
      # account, so an account resolved inside session creation would find
      # nothing and leave exactly these agents unscoped — a fix that looks
      # complete until an invited user tries to do anything.
      Given an organization account "acme" owned by "owner"
      And "linus" holds a pending invite to "acme" as "member"
      When "linus" completes the sign-in callback with the invite
      Then the callback completes successfully
      And the session stored by the callback is scoped to account "acme"
      And "linus" owns no personal account

  Rule: The identity token issued at the callback agrees with the session

    Scenario: The callback issues a token for the account the session is scoped to
      Given an active agent "ada" known to "google" with email "ada@example.com"
      And "ada" owns an active personal account "ada-personal"
      When "ada" completes the sign-in callback
      Then the callback issues an identity token whose active account is "ada-personal"

    @decision
    Scenario: An agent with no resolvable account gets no identity token
      # The JWT-side mirror of the unscoped session. Sign-in still succeeds, but
      # a token carrying an empty active account would reproduce this very defect
      # on the JWT routes, which admit what the session routes now refuse. So the
      # callback issues none: this agent leaves sign-in holding a session that
      # every authenticated request will turn away, and no token either.
      Given an active agent "orphan" known to "google" with email "orphan@example.com"
      And "orphan" is not a member of any account
      When "orphan" completes the sign-in callback
      Then the callback completes successfully
      And the session stored by the callback is not scoped to any account
      And the callback issues no identity token
