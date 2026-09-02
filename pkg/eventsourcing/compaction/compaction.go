// Package compaction trades an event store's old history for one full-state
// event per surviving aggregate, and moves everything it retires into a
// portable archive file so nothing is actually lost.
//
// An event store that has run for a long time carries history nobody reads any
// more: a resource that changed five hundred times still replays five hundred
// events to answer one question. Compact collapses that history at or below a
// watermark into a single compaction event carrying the aggregate's full
// state, and writes the retired events to an archive in the same
// newline-delimited JSON format the migration package exports.
//
// # The order of work is the safety argument
//
// For each batch the archive is written and fsynced first, the compaction
// events are appended second, and only then is anything deleted. Every failure
// before the delete therefore leaves the store exactly as it was, and every
// delete happens with the archive already durable. Because deletes come last,
// positions are never reused: a retired position becomes a permanent gap in
// the global feed rather than a number a later event might claim.
//
// Deleting is transactional per batch, and each batch's archive segment is
// recorded as a manifest in the same transaction, so an interrupted run
// resumes at the first batch it never recorded instead of archiving and
// compacting the same history twice.
//
// # What stays out
//
// What to retain is policy, and this package only provides the hook (Retain)
// and honours it. What the state of an aggregate actually is belongs to the
// caller, which is why StateProvider is a parameter rather than something
// compaction works out for itself.
package compaction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/segmentio/ksuid"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/migration"
)

const (
	// MetadataCompactedFrom is the compaction event's metadata key holding the
	// lowest global position the event replaces for its own aggregate.
	MetadataCompactedFrom = "compacted_from"

	// MetadataCompactedTo is the compaction event's metadata key holding the
	// run's watermark — the global position the run compacted up to. It is the
	// same on every compaction event a run appends.
	MetadataCompactedTo = "compacted_to"

	// DefaultBatchSize is how many retiring events one batch archives, appends
	// for, and deletes when Options.BatchSize is not set.
	DefaultBatchSize = 500
)

var (
	// ErrArchiveNotSyncable is returned when the archive destination cannot be
	// fsynced. A plain writer would report a clean success while the archive
	// sat in a page cache, so compaction refuses it rather than deleting
	// behind it.
	ErrArchiveNotSyncable = errors.New("compaction archive cannot be fsynced")

	// ErrMissingArchive is returned when Options.Archive is nil.
	ErrMissingArchive = errors.New("compaction requires an archive destination")

	// ErrMissingEventType is returned when Options.EventType is empty. The
	// compaction event's type is the caller's to choose (e.g.
	// "Resource.Compacted") because it lands in the caller's own event stream.
	ErrMissingEventType = errors.New("compaction requires an event type")

	// ErrMissingStateProvider is returned when Options.State is nil.
	ErrMissingStateProvider = errors.New("compaction requires a state provider")
)

// StateProvider returns an aggregate's current full state — the payload the
// compaction event will carry.
//
// It is asked for the state as the aggregate stands now, including any events
// above the watermark. The compaction event is appended at sequence max + 1,
// so it is the last thing replay applies; a snapshot of the state as at the
// watermark would sit above the surviving events and silently undo them on
// every rehydration.
//
// An error aborts the whole run before anything is written or deleted.
type StateProvider func(ctx context.Context, aggregateID string) (any, error)

// Syncer is the archive's durability contract. Compact refuses an archive that
// does not implement it (*os.File does).
type Syncer interface {
	Sync() error
}

// Retain holds chosen events back from both compaction and the archive: a
// retained event is neither archived nor deleted, and stays in the live store
// below the compaction event. What to retain is policy set by the caller; this
// type is only the hook.
//
// The conditions are a union — an event is retained if any of them matches.
// The zero value retains nothing.
type Retain struct {
	// EventTypes retains events whose type is exactly one of these.
	EventTypes []string

	// EventTypeMatch retains events whose type the predicate accepts, for
	// policies a fixed list cannot express (a prefix, a suffix, a namespace).
	EventTypeMatch func(eventType string) bool

	// NotBefore retains events created at or after this time. The zero time
	// retains nothing on this condition.
	NotBefore time.Time
}

// Holds reports whether the retention policy keeps this event out of the run.
func (r Retain) Holds(event domain.EventEnvelope[any]) bool {
	for _, t := range r.EventTypes {
		if t == event.EventType {
			return true
		}
	}
	if r.EventTypeMatch != nil && r.EventTypeMatch(event.EventType) {
		return true
	}
	if !r.NotBefore.IsZero() && !event.Created.Before(r.NotBefore) {
		return true
	}
	return false
}

// Options configures one compaction run.
type Options struct {
	// Watermark is the global position the run compacts up to. Every event at
	// or below it is a candidate; everything above it is left alone.
	Watermark int64

	// FromPosition resumes an archive that already holds the history up to
	// this position: the run starts above it and, because the archive is being
	// appended to rather than started, writes no version header. Leave it zero
	// for a fresh archive.
	//
	// It is a floor, not the whole resume story: recorded manifests raise the
	// starting position further, so an interrupted run resumes correctly even
	// when the caller passes nothing.
	FromPosition int64

	// EventType is the type given to every compaction event the run appends,
	// e.g. "Resource.Compacted". Required.
	EventType string

	// State supplies the full state each compaction event carries. Required.
	State StateProvider

	// Archive is where retired events are written, as newline-delimited JSON
	// in the same shape migration.Export produces. It must implement Syncer.
	// Required.
	Archive io.Writer

	// Retain holds chosen events back from both compaction and the archive.
	Retain Retain

	// IsDelete decides whether an event retires its aggregate. An aggregate
	// whose last event is a delete gets no compaction event — its history goes
	// to the archive only. Defaults to DefaultIsDelete.
	IsDelete func(event domain.EventEnvelope[any]) bool

	// BatchSize is how many retiring events one batch handles. Values <= 0 use
	// DefaultBatchSize.
	BatchSize int
}

// Report summarizes a completed — or partially completed — run.
type Report struct {
	// Archived is the number of events written to the archive and deleted.
	Archived int

	// CompactionEvents is the number of compaction events appended.
	CompactionEvents int

	// Manifests are the batch records this run wrote, in the order it wrote
	// them. On a failure it holds the batches that did complete.
	Manifests []domain.CompactionManifest

	// LastPosition is the highest global position the run retired, or the
	// position it resumed from when it retired nothing.
	LastPosition int64
}

// DefaultIsDelete recognises an event as retiring its aggregate when its type
// ends in "Deleted", the convention the library's own event types follow. A
// caller whose domain retires things differently supplies its own predicate
// through Options.IsDelete rather than renaming its events to suit compaction.
func DefaultIsDelete(event domain.EventEnvelope[any]) bool {
	return strings.HasSuffix(event.EventType, "Deleted")
}

// Compact collapses store's history at or below opts.Watermark into one
// full-state event per surviving aggregate and moves the retired events to
// opts.Archive.
//
// store must implement domain.CompactableEventStore; anything else is refused
// with domain.ErrCompactionNotSupported before a single event is read, since
// archiving history a store can never delete would be pure loss of time and
// disk. An archive that cannot be fsynced is refused just as early.
//
// The returned Report describes what the run did, and is meaningful even when
// the run fails partway: every batch it lists was archived, appended for, and
// deleted, and every batch it does not list left the store untouched.
func Compact(ctx context.Context, store domain.EventStore, opts Options) (Report, error) {
	var report Report

	compactable, ok := store.(domain.CompactableEventStore)
	if !ok {
		return report, fmt.Errorf("%w: %T cannot retire events", domain.ErrCompactionNotSupported, store)
	}
	if opts.EventType == "" {
		return report, ErrMissingEventType
	}
	if opts.State == nil {
		return report, ErrMissingStateProvider
	}
	if opts.Archive == nil {
		return report, ErrMissingArchive
	}
	syncer, ok := opts.Archive.(Syncer)
	if !ok {
		return report, fmt.Errorf("%w: %T has no Sync method", ErrArchiveNotSyncable, opts.Archive)
	}

	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}
	isDelete := opts.IsDelete
	if isDelete == nil {
		isDelete = DefaultIsDelete
	}

	// Recorded manifests, not the caller's cursor, are the authority on what a
	// previous run already retired: a run interrupted after recording a batch
	// must not archive that batch again.
	recorded, err := compactable.Compactions(ctx)
	if err != nil {
		return report, fmt.Errorf("read recorded compactions: %w", err)
	}
	from := opts.FromPosition
	for _, m := range recorded {
		if m.ToPosition > from {
			from = m.ToPosition
		}
	}
	report.LastPosition = from

	if opts.Watermark <= from {
		return report, nil
	}

	retiring, err := collect(ctx, compactable, from, opts.Watermark, opts.Retain, batchSize)
	if err != nil {
		return report, err
	}
	retiring = dropAlreadyCompacted(retiring, opts.EventType)
	if len(retiring) == 0 {
		return report, nil
	}

	// The whole plan — including every state-provider call — is built before
	// anything is written, so a provider that fails for one aggregate leaves
	// the store and the archive untouched rather than half compacted.
	plans, err := planAggregates(ctx, compactable, retiring, opts, isDelete)
	if err != nil {
		return report, err
	}

	writeHeader := opts.FromPosition == 0
	for start := 0; start < len(retiring); start += batchSize {
		end := min(start+batchSize, len(retiring))
		batch := retiring[start:end]

		segment, err := archiveBatch(opts.Archive, syncer, batch, writeHeader)
		if err != nil {
			return report, err
		}
		writeHeader = false

		if err := appendCompactionEvents(ctx, compactable, batch, plans, opts); err != nil {
			return report, err
		}
		report.CompactionEvents = countAppended(plans)

		ids := make([]string, len(batch))
		for i, event := range batch {
			ids[i] = event.ID
		}
		manifest := domain.CompactionManifest{
			ID:           ksuid.New().String(),
			FromPosition: batch[0].Position,
			ToPosition:   batch[len(batch)-1].Position,
			Watermark:    opts.Watermark,
			EventCount:   len(batch),
			Checksum:     hex.EncodeToString(segment[:]),
			CreatedAt:    time.Now().UTC(),
		}
		if err := compactable.RetireEvents(ctx, ids, manifest); err != nil {
			return report, fmt.Errorf("retire events at positions %d-%d: %w",
				manifest.FromPosition, manifest.ToPosition, err)
		}

		report.Manifests = append(report.Manifests, manifest)
		report.Archived += len(batch)
		report.LastPosition = manifest.ToPosition
	}

	return report, nil
}

// collect reads the events the run will retire: everything above from and at
// or below watermark that the retention policy does not hold back, in
// ascending position order.
func collect(
	ctx context.Context,
	store domain.EventStore,
	from, watermark int64,
	retain Retain,
	batchSize int,
) ([]domain.EventEnvelope[any], error) {
	var retiring []domain.EventEnvelope[any]

	cursor := from
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		page, err := store.ReadAfter(ctx, cursor, batchSize)
		if err != nil {
			return nil, fmt.Errorf("read after position %d: %w", cursor, err)
		}
		if len(page) == 0 {
			return retiring, nil
		}

		for _, event := range page {
			if event.Position > watermark {
				return retiring, nil
			}
			cursor = event.Position
			if retain.Holds(event) {
				continue
			}
			retiring = append(retiring, event)
		}
	}
}

// dropAlreadyCompacted removes the events of every aggregate whose only
// candidates are compaction events an earlier run appended. Such an aggregate
// has no history left to collapse: its snapshot already stands for everything
// below the watermark, and retiring it only to write an equivalent one back
// would archive a duplicate and churn the feed for nothing.
//
// An aggregate that does have real history among its candidates keeps them all,
// its stale snapshot included — that one is genuinely superseded by the fresh
// snapshot replacing it, so snapshots never pile up.
func dropAlreadyCompacted(retiring []domain.EventEnvelope[any], eventType string) []domain.EventEnvelope[any] {
	hasHistory := make(map[string]bool)
	for _, event := range retiring {
		if event.EventType != eventType {
			hasHistory[event.AggregateID] = true
		}
	}

	kept := retiring[:0]
	for _, event := range retiring {
		if hasHistory[event.AggregateID] {
			kept = append(kept, event)
		}
	}
	return kept
}

// aggregatePlan is what the run decided to do about one aggregate, worked out
// before any writing starts.
type aggregatePlan struct {
	// lowest is the lowest position being retired for this aggregate — the
	// compaction event's compacted_from.
	lowest int64

	// currentVersion is the aggregate's sequence number now; the compaction
	// event gets currentVersion + 1 and is appended against it.
	currentVersion int

	// state is the payload the compaction event will carry. Unset when skip.
	state any

	// skip is set when no compaction event is due: the aggregate's last event
	// is a delete, so its history goes to the archive only, or a previous run
	// already compacted it to this watermark.
	skip bool

	// appended records that this run has already appended the aggregate's
	// compaction event, so a second batch touching the same aggregate does not
	// append another.
	appended bool
}

// planAggregates decides, for every aggregate with retiring events, whether a
// compaction event is due and what state it carries. It calls the state
// provider for all of them up front so a provider failure aborts before any
// archive write or delete.
func planAggregates(
	ctx context.Context,
	store domain.CompactableEventStore,
	retiring []domain.EventEnvelope[any],
	opts Options,
	isDelete func(domain.EventEnvelope[any]) bool,
) (map[string]*aggregatePlan, error) {
	plans := make(map[string]*aggregatePlan)
	order := make([]string, 0)

	for _, event := range retiring {
		if plan, ok := plans[event.AggregateID]; ok {
			if event.Position < plan.lowest {
				plan.lowest = event.Position
			}
			continue
		}
		plans[event.AggregateID] = &aggregatePlan{lowest: event.Position}
		order = append(order, event.AggregateID)
	}

	for _, aggregateID := range order {
		plan := plans[aggregateID]

		history, err := store.GetEvents(ctx, aggregateID)
		if err != nil {
			return nil, fmt.Errorf("read history of %s: %w", aggregateID, err)
		}
		if len(history) == 0 {
			return nil, fmt.Errorf("%w: aggregate %s has retiring events but no history",
				domain.ErrEventNotFound, aggregateID)
		}

		last := history[0]
		for _, event := range history {
			if event.SequenceNo > last.SequenceNo {
				last = event
			}
		}
		plan.currentVersion = last.SequenceNo

		// An aggregate whose last event is a delete leaves nothing behind but
		// its archive: a snapshot of a deleted thing is a resurrection.
		if isDelete(last) {
			plan.skip = true
			continue
		}
		// A run that was interrupted after appending a compaction event but
		// before its delete committed already left the snapshot in place;
		// appending a second one for the same watermark would stack snapshots.
		if alreadyCompactedTo(last, opts.EventType, opts.Watermark) {
			plan.skip = true
			continue
		}

		state, err := opts.State(ctx, aggregateID)
		if err != nil {
			return nil, fmt.Errorf("state provider for %s: %w", aggregateID, err)
		}
		plan.state = state
	}

	return plans, nil
}

// alreadyCompactedTo reports whether event is a compaction event of this run's
// type that already covers this run's watermark.
func alreadyCompactedTo(event domain.EventEnvelope[any], eventType string, watermark int64) bool {
	if event.EventType != eventType {
		return false
	}
	to, ok := metadataInt64(event.Metadata, MetadataCompactedTo)
	return ok && to >= watermark
}

// metadataInt64 reads a numeric metadata value. Envelopes come back from a SQL
// store through JSON, where an integer arrives as a float64 or a json.Number,
// so the concrete type depends on which store answered.
func metadataInt64(metadata map[string]any, key string) (int64, bool) {
	switch v := metadata[key].(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case float64:
		return int64(v), true
	case json.Number:
		n, err := v.Int64()
		return n, err == nil
	case string:
		n, err := strconv.ParseInt(v, 10, 64)
		return n, err == nil
	default:
		return 0, false
	}
}

// archiveBatch writes the batch to the archive and fsyncs it, returning the
// SHA-256 of the batch's bytes for the manifest. The whole batch — header
// included, when this is the first write to a fresh archive — goes out in one
// Write so a partially written batch cannot be mistaken for a whole one.
//
// The checksum covers only the event lines, not the header, so it identifies
// the segment wherever in an archive that segment sits.
func archiveBatch(w io.Writer, syncer Syncer, batch []domain.EventEnvelope[any], writeHeader bool) ([sha256.Size]byte, error) {
	var digest [sha256.Size]byte

	var events []byte
	for i := range batch {
		line, err := json.Marshal(&batch[i])
		if err != nil {
			return digest, fmt.Errorf("archive: marshal event %s: %w", batch[i].ID, err)
		}
		events = append(events, line...)
		events = append(events, '\n')
	}
	digest = sha256.Sum256(events)

	out := events
	if writeHeader {
		header, err := json.Marshal(map[string]int{"pericarp_export": migration.FormatVersion})
		if err != nil {
			return digest, fmt.Errorf("archive: marshal header: %w", err)
		}
		out = make([]byte, 0, len(header)+1+len(events))
		out = append(out, header...)
		out = append(out, '\n')
		out = append(out, events...)
	}

	if _, err := w.Write(out); err != nil {
		return digest, fmt.Errorf("archive: write segment at positions %d-%d: %w",
			batch[0].Position, batch[len(batch)-1].Position, err)
	}
	if err := syncer.Sync(); err != nil {
		return digest, fmt.Errorf("archive: fsync segment at positions %d-%d: %w",
			batch[0].Position, batch[len(batch)-1].Position, err)
	}

	return digest, nil
}

// appendCompactionEvents appends the compaction event for every aggregate this
// batch retires from that does not have one yet. It runs after the archive is
// durable and before anything is deleted, so the snapshot is in the store
// before the history it replaces leaves it.
func appendCompactionEvents(
	ctx context.Context,
	store domain.CompactableEventStore,
	batch []domain.EventEnvelope[any],
	plans map[string]*aggregatePlan,
	opts Options,
) error {
	for _, event := range batch {
		plan := plans[event.AggregateID]
		if plan.skip || plan.appended {
			continue
		}

		envelope := domain.NewEventEnvelope[any](plan.state, event.AggregateID, opts.EventType, plan.currentVersion+1)
		envelope.Metadata[MetadataCompactedFrom] = plan.lowest
		envelope.Metadata[MetadataCompactedTo] = opts.Watermark

		if err := store.Append(ctx, event.AggregateID, plan.currentVersion, envelope); err != nil {
			return fmt.Errorf("append compaction event for %s: %w", event.AggregateID, err)
		}
		plan.appended = true
	}
	return nil
}

func countAppended(plans map[string]*aggregatePlan) int {
	n := 0
	for _, plan := range plans {
		if plan.appended {
			n++
		}
	}
	return n
}
