# Pericarp Journal

Append-only log of major changes to Pericarp. Each entry records what changed,
why, and any key design decisions. Claude Code reads this at the start of major
tasks to maintain context across sessions. Entries are never edited or removed.

---

### 2026-03-17: Multi-tenant auth foundations

- Added `pkg/auth` package with `Identity`, context helpers (`AgentFromCtx`, `ContextWithAgent`)
- Added JWT-based authentication service with RSA key support
- Added invite system (`InviteService`, `InviteTokenService`) for account onboarding
- Added `ResourceOwnership` helpers for tenant-scoped resource creation and access verification
- Added account switching support via session management
- **Why:** Building toward a multi-tenant IAM layer where agents belong to accounts, resources are tenant-scoped, and authentication flows support invite-based onboarding

---

### 2026-03-21: BigQuery EventStore implementation

- Added `BigQueryEventStore` in `pkg/eventsourcing/infrastructure/bigquery_store.go` — 5th EventStore implementation
- Uses BigQuery DML (INSERT via `client.Query()`) for strong read-after-write consistency
- Optimistic concurrency via BigQuery scripting blocks (DECLARE + IF/ELSE) — DML table-level locks serialize concurrent writes
- Payload/metadata stored as JSON STRING columns (schemaless, like GORM's JSONB approach)
- `GetEventByID` performs full table scan since `id` is not in the clustering key — documented trade-off
- Integration tests use `ghcr.io/goccy/bigquery-emulator` via testcontainers; skip gracefully without Docker
- Added BigQuery variants to shared table-driven tests in `eventstore_test.go` and `eventstore_range_test.go`
- **Why:** BigQuery is append-optimized and enables powerful analytics over event streams — natural fit for event sourcing at scale

---

### 2026-04-04: Transaction ID for unit of work correlation

- Added `TransactionID` field to `EventEnvelope[T]` — a KSUID string that correlates all events committed in the same unit of work
- `SimpleUnitOfWork.Commit()` generates a single transaction ID and stamps it on every event before persisting
- Updated `ToAnyEnvelope` to copy the `TransactionID` field
- Field is `omitempty` in JSON so existing events without a transaction ID remain backward-compatible
- Added tests: same-commit events share a transaction ID, different commits get different IDs, dispatched events carry the transaction ID
- **Why:** Enables correlating events across multiple aggregates that were committed together, useful for auditing, debugging, and cross-aggregate consistency tracking

---

### 2026-04-19: CompositeEventStore for primary-sync + secondaries-async replication

- Added `CompositeEventStore` in `pkg/eventsourcing/infrastructure/composite_store.go` — 6th EventStore implementation; wraps a primary store plus zero or more secondaries
- Primary `Append` is synchronous; each secondary gets a dedicated goroutine + buffered channel (default 1024) so secondary latency/failures never block the caller
- All read methods forward to the primary — secondaries are write-only replicas in v1
- Optional `WithErrorHandler` functional option receives failed secondary appends; no `Logger` interface added to pericarp per existing "callers decide logging strategy" convention
- `Close()` is idempotent (`sync.Once`), drains each secondary's queue, then closes underlying stores; returns the first non-nil close error wrapped with which store failed
- Handler panics are recovered so a misbehaving handler can't stall replication
- **Why:** Lets callers attach backup/replica stores (e.g., FileStore mirror of a primary memory/Postgres store) without the replica's latency showing up on the request path

---

### 2026-04-20: Bigtable EventStore implementation

- Added `BigtableEventStore` in `pkg/eventsourcing/infrastructure/bigtable_store.go` — 6th EventStore implementation (after Memory, File, GORM, BigQuery, Dynamo, Composite)
- Single-table design with three row-key spaces: `e#<agg>#<seq:20>` (events), `id#<eventID>` (index for GetEventByID), `tx#<txID>#<agg>#<seq:20>` (index for GetEventsByTransactionID)
- Zero-padded 20-digit sequence numbers so lexicographic row order equals numeric order — enables `bigtable.NewRange` scans for `GetEventsRange` / `GetEventsFromVersion`
- `GetCurrentVersion` uses `ReverseScan()` + `LimitRows(1)` — O(1) regardless of history length, unlike BigQuery's MAX(seq) aggregation
- `GetEventByID` is a two-phase read (id-index row → event row) to avoid the full-table scan BigQuery needs
- Append uses `ApplyBulk` to write event row + ID index + optional TX index in one RPC
- Concurrency: matches BigQuery's documented read-then-write model — weak consistency acceptable for low-contention workloads; high-contention callers should serialize via a per-aggregate actor
- Integration tests in `bigtable_store_test.go` against `gcr.io/google.com/cloudsdktool/cloud-sdk:emulators` via testcontainers; skip cleanly without Docker
- Adds `cloud.google.com/go/bigtable v1.46.0` as a direct dependency
- Pairs naturally with `CompositeEventStore` as a cloud-NoSQL primary
- **Why:** Bigtable offers single-digit-ms point reads + strong row-level consistency — the missing piece for high-throughput event-sourced workloads on GCP where BigQuery's analytics focus isn't a fit
