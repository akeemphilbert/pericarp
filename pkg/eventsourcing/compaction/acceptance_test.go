package compaction_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/glebarez/sqlite"
	"github.com/segmentio/ksuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/akeemphilbert/pericarp/pkg/ddd"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/compaction"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"
)

// TestAcceptance runs the Gherkin contract in features/ against the real
// compaction implementation and real event stores — once per store kind, so
// every scenario is proved against both the in-memory store and the SQLite
// store rather than against whichever one happened to be wired up. Nothing is
// stubbed except the failure injections the contract itself asks for (an
// archive that will not write, a store that will not delete).
func TestAcceptance(t *testing.T) {
	for _, kind := range []string{storeMemory, storeSQLite} {
		t.Run(kind, func(t *testing.T) {
			suite := godog.TestSuite{
				ScenarioInitializer: initializeScenario(kind),
				Options: &godog.Options{
					Format:   "pretty",
					Paths:    []string{"features"},
					Tags:     "@compaction",
					TestingT: t,
					Strict:   true,
				},
			}
			if suite.Run() != 0 {
				t.Fatalf("compaction acceptance suite failed for the %s store", kind)
			}
		})
	}
}

const (
	storeMemory = "memory"
	storeSQLite = "sqlite"
	storeFile   = "file-backed"
	storeDynamo = "DynamoDB"
)

// eventRow is one line of a scenario's "the event store holds" table, before
// it reaches a store. Rows are held here rather than written straight through
// because later Given steps still adjust them — a scenario may set an event's
// payload after the table that introduced it.
type eventRow struct {
	aggregate string
	eventType string
	position  int64
	created   time.Time
	payload   map[string]any
}

// world holds one scenario's state. A fresh one is built per scenario, over
// its own store and its own temporary directory, so scenarios cannot see each
// other's events or archives.
type world struct {
	kind string
	dir  string

	// store is what Compact is handed. For a compaction-capable kind it is an
	// observedStore wrapping the real one, so the contract's ordering and
	// "nothing was read" claims can be checked against what actually happened.
	store    domain.EventStore
	observed *observedStore
	memory   *infrastructure.MemoryStore
	db       *gorm.DB
	closers  []func()

	// pending holds seeded rows; seeded is how many of them have reached the
	// store. Seeding is deferred to the first step that needs the store so the
	// Givens that adjust a row still can.
	pending []eventRow
	seeded  int

	eventType      string
	stateOverrides map[string]map[string]any
	providerFails  map[string]bool

	archive      *archiveFile
	archivePath  string
	plainArchive io.Writer

	retain    compaction.Retain
	isDelete  func(domain.EventEnvelope[any]) bool
	batchSize int

	journal []string

	report      compaction.Report
	err         error
	errConsumed bool

	// lastFrom is the position the run under test resumed from, so an
	// assertion can tell what this run added to an archive that already held
	// history from an earlier one.
	lastFrom int64

	// referenced is the position a "the surviving X event is still at position
	// N" step just checked, so the step after it can say "above it".
	referenced int64

	rebuilt  *resource
	applyErr error
}

func initializeScenario(kind string) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) {
		w := &world{}

		sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
			return ctx, w.setup(kind)
		})
		sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
			defer w.teardown()
			// A compaction that failed where no step claimed a failure is a
			// silent red: several scenarios assert only that the store was
			// left alone, which an aborted run satisfies for the wrong reason.
			if err == nil && w.err != nil && !w.errConsumed {
				return ctx, fmt.Errorf("compaction failed but no step expected a failure: %w", w.err)
			}
			return ctx, nil
		})

		w.registerSteps(sc)
	}
}

func (w *world) setup(kind string) error {
	dir, err := os.MkdirTemp("", "pericarp-compaction-")
	if err != nil {
		return fmt.Errorf("create scenario directory: %w", err)
	}

	*w = world{
		kind:           kind,
		dir:            dir,
		eventType:      "Resource.Compacted",
		stateOverrides: map[string]map[string]any{},
		providerFails:  map[string]bool{},
	}
	w.closers = append(w.closers, func() { _ = os.RemoveAll(dir) })

	if err := w.useStore(context.Background(), kind); err != nil {
		return err
	}
	return w.useArchive("archive.jsonl")
}

func (w *world) teardown() {
	for i := len(w.closers) - 1; i >= 0; i-- {
		w.closers[i]()
	}
	w.closers = nil
}

// useStore builds the store of the given kind and makes it the one under test.
// A scenario may call it after setup to name a kind of its own ("a SQLite
// backed event store", the unsupported-store outline); it always starts from
// an empty store, which is why the contract names the kind before any events.
func (w *world) useStore(ctx context.Context, kind string) error {
	w.kind = kind
	w.memory = nil
	w.db = nil
	w.observed = nil

	switch kind {
	case storeMemory:
		w.memory = infrastructure.NewMemoryStore()
		w.observed = &observedStore{CompactableEventStore: w.memory, world: w}
		w.store = w.observed

	case storeSQLite:
		db, err := gorm.Open(sqlite.Open(filepath.Join(w.dir, "events.db")), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if err != nil {
			return fmt.Errorf("open sqlite: %w", err)
		}
		store, err := infrastructure.NewGormEventStore(db)
		if err != nil {
			return fmt.Errorf("build sqlite event store: %w", err)
		}
		w.db = db
		w.observed = &observedStore{CompactableEventStore: store, world: w}
		w.store = w.observed

	case storeFile:
		store, err := infrastructure.NewFileStore(filepath.Join(w.dir, "events"))
		if err != nil {
			return fmt.Errorf("build file event store: %w", err)
		}
		w.closers = append(w.closers, func() { _ = store.Close() })
		w.store = store

	case storeDynamo:
		store, cleanup, err := newDynamoStore(ctx)
		if err != nil {
			return err
		}
		w.closers = append(w.closers, cleanup)
		w.store = store

	default:
		return fmt.Errorf("unknown store kind %q", kind)
	}

	return nil
}

// useArchive points the run at a fresh archive file under the scenario's
// directory. Every compaction run gets its own archive, which is how the
// contract can say a second run wrote "nothing further".
func (w *world) useArchive(name string) error {
	path := filepath.Join(w.dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open archive %s: %w", name, err)
	}
	w.closers = append(w.closers, func() { _ = f.Close() })

	w.archive = &archiveFile{file: f, world: w}
	w.archivePath = path
	w.plainArchive = nil
	return nil
}

// ---------------------------------------------------------------------------
// Store and archive doubles
// ---------------------------------------------------------------------------

// observedStore wraps a real compaction-capable store so a scenario can watch
// what compaction did to it — how often it read the feed, and in what order it
// appended and deleted relative to the archive's writes and syncs — and so a
// scenario can make one delete fail without touching the store's own code.
type observedStore struct {
	domain.CompactableEventStore
	world *world

	reads        int
	deletes      int
	failDeleteOn int
}

var errDeleteFailed = errors.New("simulated delete failure")

func (o *observedStore) ReadAfter(ctx context.Context, afterPosition int64, limit int) ([]domain.EventEnvelope[any], error) {
	o.reads++
	return o.CompactableEventStore.ReadAfter(ctx, afterPosition, limit)
}

func (o *observedStore) Append(ctx context.Context, aggregateID string, expectedVersion int, events ...domain.EventEnvelope[any]) error {
	err := o.CompactableEventStore.Append(ctx, aggregateID, expectedVersion, events...)
	if err == nil {
		o.world.record("append")
	}
	return err
}

func (o *observedStore) RetireEvents(ctx context.Context, eventIDs []string, manifest domain.CompactionManifest) error {
	o.deletes++
	if o.failDeleteOn == o.deletes {
		return errDeleteFailed
	}
	if err := o.CompactableEventStore.RetireEvents(ctx, eventIDs, manifest); err != nil {
		return err
	}
	o.world.record("delete")
	return nil
}

// archiveFile is a real file the run writes through, with hooks for the two
// failures the contract requires: a write that fails and an fsync that fails.
type archiveFile struct {
	file  *os.File
	world *world

	writes      int
	failWriteOn int
	failSync    bool
}

var errArchiveIO = errors.New("simulated archive failure")

func (a *archiveFile) Write(p []byte) (int, error) {
	a.writes++
	if a.failWriteOn == a.writes {
		return 0, errArchiveIO
	}
	n, err := a.file.Write(p)
	if err == nil {
		a.world.record("write")
	}
	return n, err
}

func (a *archiveFile) Sync() error {
	if a.failSync {
		return errArchiveIO
	}
	if err := a.file.Sync(); err != nil {
		return err
	}
	a.world.record("sync")
	return nil
}

// plainWriter is an archive destination with no Sync method at all — the case
// compaction has to refuse rather than delete behind a page cache.
type plainWriter struct {
	w io.Writer
}

func (p *plainWriter) Write(b []byte) (int, error) { return p.w.Write(b) }

// record appends to the ordering journal, but only for the scenario that asked
// for one; the rest of the suite pays nothing for it.
func (w *world) record(what string) {
	if w.journal == nil {
		return
	}
	w.journal = append(w.journal, what)
}

// ---------------------------------------------------------------------------
// The aggregate the scenarios compact
// ---------------------------------------------------------------------------

// resource is the aggregate the contract talks about. It exists so a scenario
// can prove that a compacted aggregate rehydrates from its compaction event
// alone — which needs a real BaseEntity, not a hand-read of the payload.
type resource struct {
	*ddd.BaseEntity
	name string
	tag  string
}

func newResource(id string) *resource {
	return &resource{BaseEntity: ddd.NewBaseEntity(id)}
}

func (r *resource) Apply(ctx context.Context, event domain.EventEnvelope[any]) error {
	if err := r.ApplyEvent(ctx, event); err != nil {
		return err
	}
	if payload, ok := event.Payload.(map[string]any); ok {
		if v, ok := payload["name"].(string); ok {
			r.name = v
		}
		if v, ok := payload["tag"].(string); ok {
			r.tag = v
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Seeding
// ---------------------------------------------------------------------------

// ensureSeeded writes any rows a scenario has declared but not yet stored.
// Seeding is lazy so that a Given which adjusts a declared row still can, and
// incremental so that a scenario can declare more rows after a compaction has
// already run ("the event store then holds").
func (w *world) ensureSeeded(ctx context.Context) error {
	rows := w.pending[w.seeded:]
	if len(rows) == 0 {
		return nil
	}
	w.seeded = len(w.pending)

	versions := map[string]int{}
	envelopes := make([]domain.EventEnvelope[any], 0, len(rows))
	for _, row := range rows {
		version, ok := versions[row.aggregate]
		if !ok {
			current, err := w.base().GetCurrentVersion(ctx, row.aggregate)
			if err != nil {
				return fmt.Errorf("read current version of %s: %w", row.aggregate, err)
			}
			version = current
		}
		version++
		versions[row.aggregate] = version

		envelopes = append(envelopes, domain.EventEnvelope[any]{
			ID:          ksuid.New().String(),
			AggregateID: row.aggregate,
			EventType:   row.eventType,
			Payload:     row.payload,
			Created:     row.created,
			SequenceNo:  version,
			Metadata:    map[string]any{},
			Position:    row.position,
		})
	}

	switch {
	case w.memory != nil:
		return w.memory.SeedEvents(ctx, envelopes...)
	case w.db != nil:
		return w.seedSQLite(ctx, envelopes)
	default:
		// The file-backed and DynamoDB stores only appear in the
		// unsupported-store outline, whose single event needs no gap, so a
		// plain append reproduces the table exactly.
		for _, envelope := range envelopes {
			if err := w.base().Append(ctx, envelope.AggregateID, envelope.SequenceNo-1, envelope); err != nil {
				return fmt.Errorf("seed %s: %w", envelope.AggregateID, err)
			}
		}
		return nil
	}
}

// seedSQLite appends the events and then moves each row to the position the
// scenario asked for. Append always allocates the next position, so the gaps a
// compacted feed has can only be produced by writing the positions afterwards;
// rows are moved highest-first so no update ever collides with a position a
// later row still holds.
func (w *world) seedSQLite(ctx context.Context, envelopes []domain.EventEnvelope[any]) error {
	for _, envelope := range envelopes {
		if err := w.base().Append(ctx, envelope.AggregateID, envelope.SequenceNo-1, envelope); err != nil {
			return fmt.Errorf("seed %s: %w", envelope.AggregateID, err)
		}
	}
	for i := len(envelopes) - 1; i >= 0; i-- {
		if err := w.db.WithContext(ctx).
			Model(&infrastructure.GormEventModel{}).
			Where("id = ?", envelopes[i].ID).
			Update("position", envelopes[i].Position).Error; err != nil {
			return fmt.Errorf("place event %s at position %d: %w", envelopes[i].ID, envelopes[i].Position, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Running compaction
// ---------------------------------------------------------------------------

// stateOf is the state provider the contract's Background describes: it reads
// the aggregate as it currently stands in the store and folds its events into
// one full state. Scenarios override what it reports, or make it fail.
func (w *world) stateOf(ctx context.Context, aggregateID string) (any, error) {
	if w.providerFails[aggregateID] {
		return nil, fmt.Errorf("no state available for %s", aggregateID)
	}

	events, err := w.store.GetEvents(ctx, aggregateID)
	if err != nil {
		return nil, err
	}

	state := map[string]any{"id": aggregateID}
	for _, event := range events {
		payload, ok := event.Payload.(map[string]any)
		if !ok {
			continue
		}
		for k, v := range payload {
			state[k] = v
		}
	}
	for k, v := range w.stateOverrides[aggregateID] {
		state[k] = v
	}
	return state, nil
}

func (w *world) options(watermark, from int64) compaction.Options {
	var archive io.Writer = w.archive
	if w.plainArchive != nil {
		archive = w.plainArchive
	}
	return compaction.Options{
		Watermark:    watermark,
		FromPosition: from,
		EventType:    w.eventType,
		State:        w.stateOf,
		Archive:      archive,
		Retain:       w.retain,
		IsDelete:     w.isDelete,
		BatchSize:    w.batchSize,
	}
}

// compact runs compaction and keeps both halves of the outcome: several
// scenarios assert on the error, and the After hook fails the scenario if one
// appears that no step claimed.
func (w *world) compact(ctx context.Context, watermark, from int64) error {
	if err := w.ensureSeeded(ctx); err != nil {
		return err
	}
	w.lastFrom = from
	w.report, w.err = compaction.Compact(ctx, w.store, w.options(watermark, from))
	w.errConsumed = false
	return nil
}
