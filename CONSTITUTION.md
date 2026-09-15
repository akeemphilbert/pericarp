# Pericarp Constitution

**Version:** 1.2.0 &nbsp;|&nbsp; **Ratified:** 2026-09-03 &nbsp;|&nbsp; **Last amended:** 2026-09-03

This document states the non-negotiable rules for `pericarp` — the Event Sourcing
and DDD library the vine-os services build on. Every contributor is bound by it,
human or agent. Where an article conflicts with `CLAUDE.md`, `CONTRIBUTING.md`,
`docs/`, a skill, or a habit, the article wins. Change an article only through the
amendment procedure in [Governance](#governance).

Each article gives the rule, the reason, and how the rule is enforced. The gate is
named in the first sentence of *How this is enforced*: a test suite, a CI job, a
linter, or `Review`. An article marked **(ASPIRATIONAL)** is binding on reviewers
but has no automated gate yet.

Pericarp is a library, not a service. Its rules are shaped by that: an exported
name is a promise to a consumer, a dependency added here is a dependency every
consumer inherits, and a defect in the append contract is a defect in every service
that stores events through it.

---

## Article I — The event store is the source of truth

**Rule.** State changes are recorded as events. An aggregate calls
`BaseEntity.RecordEvent(payload, eventType)`; the events reach storage through
`SimpleUnitOfWork.Track` and `Commit`, or through a direct `EventStore.Append`.
An envelope is immutable once created. Projections and event handlers are
idempotent and survive replay.

**Reason.** The store holds the history; everything else is a view of it. A view
written directly disagrees with the history, and the next replay produces a
different system than the one that ran. Compaction (`pkg/eventsourcing/compaction`)
rewrites history on purpose, under its own contract; nothing else does.

**How this is enforced.** Review. Mutating a persisted envelope, or writing a
projection row that no event produced, is a violation.

---

## Article II — Dependencies point inward

**Rule.** Application → Domain ← Infrastructure. `pkg/eventsourcing/domain` and
`pkg/ddd` import the standard library and the two core dependencies
(`segmentio/ksuid`, `golang.org/x/sync`) only. They do not import
`application`, `infrastructure`, `compaction`, `subscriptions`, GORM, pgx, or the
AWS SDK. Infrastructure implements the interfaces the domain declares.

**Reason.** The domain must stay testable with no database, no container, and no
network. An inward-pointing import graph is what keeps that true, and it is what
lets a consumer depend on the event contract without inheriting a driver.

**How this is enforced.** The `depguard` linter, through its
`article-ii-domain-points-inward` rule in `.golangci.yml`, run by `make lint` and
the CI linter step. A file under `pkg/eventsourcing/domain/` or `pkg/ddd/` may
import the standard library, those two packages, `segmentio/ksuid`, and
`golang.org/x/sync`. Every other import fails, and the likeliest mistakes — the
outer layers, GORM, pgx, the AWS SDK — fail with a message naming this article.

Test files are exempt, with a reason: `package domain_test` is a separate,
external package that consumes the domain, and exercising the `EventStore`
contract against `MemoryStore` is what it is for. The rule binds the compiled
domain package, which is the non-test files.

---

## Article III — Sequence numbers and optimistic concurrency are the append contract

**Rule.** A new aggregate starts at sequence 0. Its first event is sequence 1.
Sequence numbers are strictly ordered with no gaps. `expectedVersion` is the
aggregate's sequence number before the appended events; `-1` skips the check. A
mismatch returns `domain.ErrConcurrencyConflict`. Every `EventStore` implementation
obeys this identically.

**Reason.** Consumers rebuild aggregates by replaying in sequence order and detect
lost updates by the conflict error. A store that numbers differently, or that
accepts a stale `expectedVersion`, corrupts state in a way that only appears under
concurrency, in production.

**How this is enforced.** The table-driven suites in
`pkg/eventsourcing/infrastructure/` — `eventstore_test.go`,
`eventstore_range_test.go`, `eventstore_readafter_test.go` — run the same
assertions against every store, under `make test`.

---

## Article IV — Optional capabilities are declared, and refused loudly

**Rule.** A capability that not every store can offer is a separate interface, and
a store that lacks it fails with a named sentinel rather than degrading. Global
ordering (`ReadAfter`, `HeadPosition`) returns
`domain.ErrGlobalOrderingNotSupported`. Compaction is
`domain.CompactableEventStore`, and a store outside it returns
`domain.ErrCompactionNotSupported`. A new optional capability follows the same
shape: interface, sentinel, refusal.

**Reason.** A store that quietly returns an empty feed instead of refusing looks
like "caught up" to a subscriber, and a compaction that quietly skips looks like
success. Both are silent data loss. `FileStore` and `DynamoEventStore` refuse
compaction today; that refusal is the feature.

**How this is enforced.** The sentinel errors in
`pkg/eventsourcing/domain/eventstore.go` and `compaction.go`, and the per-store
tests that assert the refusal — `compaction_store_test.go`, the DynamoDB support
tests in `pkg/eventsourcing/compaction/`.

---

## Article V — Errors are sentinels, and they are wrapped

**Rule.** A condition a caller can act on has an exported `Err*` sentinel in
`domain/`. Implementations return that sentinel, wrapped with `%w` when adding
context. Do not invent a new error string for a condition that already has one, and
do not return a bare string where a caller needs `errors.Is`.

**Reason.** Consumers branch on `errors.Is(err, domain.ErrConcurrencyConflict)` to
retry, and on `ErrEventNotFound` to treat a miss as empty. A store that returns an
unmatched string turns a retryable condition into an outage.

**How this is enforced.** Review, and the store conformance suites, which assert
the sentinel rather than the message.

---

## Article VI — No silent failures

**Rule.** Handle every error, surface it, or log it with a reason. A deliberately
discarded error carries a comment saying why discarding it is safe.

```go
// Best-effort archive fsync on close; the batch already fsynced its data.
_ = f.Close()
```

**Reason.** A swallowed error in an event store turns a loud write failure into a
quiet missing event, and a missing event is a wrong answer forever after.

**How this is enforced.** The `errcheck` linter, through `make lint` and the CI
linter step. Review catches the uncommented `_ =`. At amendment 1.1.0 the tree
carried three `//nolint` directives and three `#nosec` annotations, each naming its
check and its reason — a snapshot for the record, not a live count. `nolintlint`
is the gate on the `//nolint` form; review is the gate on `#nosec`.

---

## Article VII — A store implementation joins the shared suite, and its tests do not silently skip

**Rule.** A new `EventStore` implementation is added to the table-driven suites in
`pkg/eventsourcing/infrastructure/` rather than tested on its own terms. A test
that needs a container skips only when Docker is genuinely absent, and fails
instead of skipping when `PERICARP_REQUIRE_DOCKER_TESTS` is set.

**Reason.** A store tested by its own bespoke assertions passes its own
expectations, not the contract in Article III. And a container test that skips
silently in CI reports green for a store nobody exercised — the DynamoDB and
Postgres stores are only ever covered by container-backed runs.

**How this is enforced.** The CI `Test` job, which sets
`PERICARP_REQUIRE_DOCKER_TESTS: "1"` in `.github/workflows/ci.yml`, and the
`os.Getenv("PERICARP_REQUIRE_DOCKER_TESTS")` guards in `dynamo_store_test.go`,
`dynamo_support_test.go`, and `postgres_subscriptions_test.go`.

---

## Article VIII — Tests are colocated, table-driven, parallel, and race-checked

**Rule.** Tests live beside the code they cover. New tests are table-driven and
call `t.Parallel()` unless the case genuinely cannot run in parallel. Everything
runs under `-race`.

**Reason.** This library's defects are concurrency defects — a store appending from
two goroutines, a dispatcher fanning out through `errgroup`, a subscriber advancing
a checkpoint. Only `-race` and parallel tests find them before a service does.

**How this is enforced.** `make test`, which runs `go test -v -race
-coverprofile=coverage.out ./...`, and the CI `Test` job that runs it. The target
covered `./pkg/...` only until amendment 1.1.0, which left the tests under
`cmd/pericarp/` and `examples/` in no gate; it now covers the whole module.

---

## Article IX — The acceptance contract comes before the implementation

**Rule.** A feature that has user-visible behavior gets its Gherkin scenarios
first, in a `features/` directory beside the package that implements it
(`pkg/eventsourcing/compaction/features/`, `pkg/auth/features/`). Get the scenarios
confirmed, then write the code that satisfies them. Scenarios are declarative:
domain-level steps, observable `Then` outcomes, one behavior per scenario.

**Reason.** Scenarios written after the code describe what was built, not what was
asked for. Confirming the contract first is where a misread requirement is cheap to
fix.

**How this is enforced. (ASPIRATIONAL)** Review, backed by the godog runners
(`pkg/eventsourcing/compaction/acceptance_test.go`, `pkg/auth/acceptance_test.go`)
which run under `make test`. Nothing checks the ordering of the commits; the pull
request shows the `.feature` file first, or explains why not.

---

## Article X — The exported surface is a contract

**Rule.** An exported type, function, method signature, or interface in `pkg/` is a
promise to every consumer. Changing one — including adding a method to an interface
consumers implement — is a breaking change. A breaking change is deliberate: it is
named in the pull request body, recorded in an ADR under `docs/decisions/`
(Article XII), and released under a version bump that says so.

**Reason.** vine-os services and outside consumers `go get` this module by tag. A
signature changed without a bump breaks their build with no warning and no way back
except pinning an older tag. The module is at `v1.0.0-beta.*`; pre-1.0 permits the
break, it does not excuse hiding it.

**How this is enforced. (ASPIRATIONAL)** Review, and the release gate in
`.github/workflows/release.yml`, which builds and tests the tagged tree. Nothing
diffs the exported API between tags; a reviewer reading the diff is the gate.

---

## Article XI — The library stays cheap to import

**Rule.** A new third-party dependency enters at the edge that needs it —
`infrastructure/`, `subscriptions/`, `auth/` — never in `domain/` or `ddd/`
(Article II). Adding one to `go.mod` is justified in the pull request body.
`go.mod` never requires a private module.

**Reason.** Every dependency here is inherited by every service that imports
pericarp, along with its transitive tree, its CVEs, and its upgrade schedule.
Deleting the BigQuery store on 2026-05-30 dropped roughly twenty transitive
dependencies in one commit — that is the size of what a single leaf import costs.
A private requirement makes the public module unbuildable for anyone outside the
organisation.

**How this is enforced.** The CI `Build` job, which runs `make build` with no
private credentials, and review of the `go.mod` diff.

---

## Article XII — An architectural decision is recorded as an ADR

**Rule.** A new package, a change to the event or store contract, a new store
capability, a swapped foundational dependency, a breaking change to the exported
surface, or a reversed decision gets a record in `docs/decisions/` and a row in
`docs/decisions/index.md`, in the same pull request. The record follows
`0000-adr-template.md`: the problem, the drivers, every option actually weighed,
the outcome, and its consequences. An accepted record is never edited; a change
of mind is a new record that supersedes it. Routine fixes, test additions, and
minor refactors get no record.

**Reason.** The reasoning behind a decision decays faster than the code. Six
months on, the record is the only thing that says which options were rejected and
why, and a reader who disagrees with a decision needs the drivers it answered
before they can argue with it. The append-only journal this replaced recorded
what changed; it did not reliably record what else was considered.

**How this is enforced. (ASPIRATIONAL)** Review. The reviewer asks for the record
on any pull request that meets the rule, and refuses a record with one considered
option. The former journal at `.claude/journal.md` is frozen; a new entry there is
a violation.

---

## Article XIII — Code is formatted and lints clean

**Rule.** Code passes `gofmt` and `golangci-lint run` with no findings. The
enabled set is the ten linters in `.golangci.yml`:

| Linter | Catches |
|---|---|
| `errcheck` | Unhandled errors |
| `govet` | Suspect constructs the compiler allows |
| `ineffassign` | Assignments nothing reads |
| `staticcheck` | Bugs, dead code, and misuse |
| `unused` | Unreachable identifiers |
| `misspell` | Misspellings (US locale) |
| `gocritic` | Diagnostic and style defects |
| `gosec` | Weak permissions, unsafe paths, hardcoded secrets |
| `depguard` | Article II — imports that point outward from `domain/` and `ddd/` |
| `nolintlint` | A `//nolint` that names no linter or gives no reason |

A `//nolint` directive names its linter and carries a comment giving the reason.
An exclusion rule in `.golangci.yml` carries its reason in the config, next to the
rule.

**Reason.** A consistent format removes formatting from review. A lint finding is a
concrete, reproducible line a reviewer can point at, which is the difference
between grounded feedback and opinion.

**How this is enforced.** `make lint` and the CI linter step in
`.github/workflows/ci.yml`, both running golangci-lint at the one version the
Makefile pins (`GOLANGCI_LINT_VERSION`) — so a new upstream check cannot turn the
build red without a deliberate bump. Bump the Makefile and the workflow in the same
commit. `make lint` fails, with the install command, when the binary is missing; it
no longer exits 0 having linted nothing. Findings are uncapped, so a clean run means
every instance is clean, not the first three.

Four exclusions exist, each with its reason in `.golangci.yml`: `gosec` and
`depguard` are relaxed in `_test.go` files, and `gosec`'s `G101` is relaxed in
`pkg/auth/domain/entities/ontology.go`, `pkg/auth/infrastructure/providers/`, and
`examples/`, where it matches vocabulary constants, OAuth endpoint URLs, and
placeholder credentials on the word "credential" or "token". Do not add an
exclusion without its reason.

---

## Article XIV — The gates are green before merge

**Rule.** Both CI jobs pass before a pull request merges: `Test` and `Build` in
`.github/workflows/ci.yml`. Do not merge on red. Do not disable a job to go green —
fix the cause, or amend this constitution.

**Reason.** A red gate that merges anyway trains everyone to ignore the gate.

**How this is enforced.** `.github/workflows/ci.yml`, which runs both jobs on every
pull request whose base is `main` or `develop`. A stacked pull request based on
another feature branch gets no run of its own; ask for one on its branch with
`gh workflow run ci.yml --ref <branch>` and take the verdict from that run.

---

## Article XV — Work lands through a reviewed pull request

**Rule.** Branch from `main` and open the pull request against `main`. In a stack,
the bottom layer does that and each layer above branches from, and opens against,
the layer below it. Never push directly to `main` or `develop`. A reviewer who
finds a violation names the article; an author who must violate one says which and
why, in the pull request body.

**Reason.** Review is where an article of this constitution gets caught before it is
broken in the default branch — and for the articles enforced by `Review`, it is the
only gate there is.

**How this is enforced. (ASPIRATIONAL)** Review. No branch ruleset blocks a direct
push today, so this article rests on review and habit until one is configured.

---

## Governance

**Precedence.** On conflict, an article of this constitution outranks `CLAUDE.md`,
`CONTRIBUTING.md`, `docs/` (the ADRs included), the frozen journal, and any skill
or agent instruction.

**Amendment.** Amend an article through a pull request that edits this file and
bumps the version in the same commit. State the reason for the amendment in the
pull request body.

**Versioning.** This document uses semantic versioning, independently of the Go
module's version.

| Bump | When |
|------|------|
| MAJOR | An article is removed, or redefined in a way that makes previously compliant code non-compliant |
| MINOR | An article is added, or materially expanded |
| PATCH | Wording, formatting, or a corrected reference only |

**Compliance review.** A reviewer who finds a violation names the article. An author
who must violate an article says which one and why, in the pull request body. A
repeated, justified violation is a signal to amend the article rather than to keep
granting exceptions.

**Aspirational articles.** An article marked **(ASPIRATIONAL)** is binding on review
but has no automated gate. Remove the marker when a gate lands, in the same pull
request that lands it.

### Amendment history

| Version | Date | Change |
|---|---|---|
| 1.0.0 | 2026-09-03 | Ratified |
| 1.1.0 | 2026-09-03 | Article II gains its machine gate: `.golangci.yml` lands with a `depguard` rule that names the article in its finding. Article XIII gains the pinned linter version, the ten-linter table, and a `make lint` that fails when the binary is missing. Article VIII's target now covers the whole module. The aspirational marker on Article II is removed. |
| 1.2.0 | 2026-09-03 | Article XII redefined: the decision record moves from the append-only journal to MADR records in `docs/decisions/`, with the eleven load-bearing decisions reconstructed as 0001–0011 and the journal frozen. Article X's breaking-change record now points at an ADR. |
