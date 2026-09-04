package domain

import (
	"context"
	"errors"
	"time"
)

// ErrCompactionNotSupported is returned when compaction is asked of an event
// store that cannot retire events. Compaction deletes history, so a store that
// cannot delete transactionally — or cannot record that it did — must refuse
// rather than archive history it will never remove.
var ErrCompactionNotSupported = errors.New("event store does not support compaction")

// MetadataSnapshot is the event-metadata key an event sets to declare that
// its payload is the aggregate's complete state rather than a change to it.
// Compaction sets it on every compaction event.
//
// It is what lets replay accept such an event above the sequence number it was
// expecting. A snapshot's payload already folds in the effect of every event it
// replaced, so the numbers it skips carry no information replay still needs —
// which is not true of an ordinary event, where a skipped number means a lost
// one.
const MetadataSnapshot = "snapshot"

// IsSnapshot reports whether an event's metadata declares it a full-state
// snapshot. Metadata that has round-tripped through a SQL store arrives as
// decoded JSON, so the flag is read leniently.
func IsSnapshot(metadata map[string]any) bool {
	switch v := metadata[MetadataSnapshot].(type) {
	case bool:
		return v
	case string:
		return v == "true"
	default:
		return false
	}
}

// CompactionManifest describes one batch of events that a compaction run
// archived and then deleted. It is the record that makes a run resumable: a
// later run skips every position a recorded manifest already covers, so an
// interrupted run continues at the first batch it never recorded instead of
// archiving and compacting the same history twice.
//
// The manifest is written in the same transaction as the batch's delete, so
// there is never a recorded manifest whose events survived, nor a deleted
// batch with no record of where its events went.
type CompactionManifest struct {
	// ID uniquely identifies this manifest.
	ID string

	// FromPosition and ToPosition are the lowest and highest global positions
	// in the batch. Positions between them may belong to events the run
	// retained rather than retired, so the span is a range, not a count.
	FromPosition int64
	ToPosition   int64

	// Watermark is the global position the run was compacting up to. Every
	// event in the batch is at or below it.
	Watermark int64

	// EventCount is the number of events archived and deleted in this batch.
	EventCount int

	// Checksum is the hex-encoded SHA-256 of the batch's bytes in the archive
	// — the newline-delimited JSON lines for its events, each including its
	// trailing newline, and excluding the archive's version header. It lets an
	// operator verify a segment against the record before trusting a delete.
	Checksum string

	// CreatedAt is when the batch was retired.
	CreatedAt time.Time
}

// CompactableEventStore is the optional capability an EventStore implements to
// let compaction retire history. Stores that cannot delete events (the
// file-backed and DynamoDB stores) do not implement it, and compaction refuses
// them before reading anything.
type CompactableEventStore interface {
	EventStore

	// RetireEvents deletes the events with the given IDs and records manifest,
	// both in a single transaction. Either the whole batch is gone and the
	// manifest is recorded, or nothing changed.
	//
	// Deleting is what leaves permanent gaps in the global feed: positions are
	// never reused, so a retired position simply never appears again.
	// Implementations return an error rather than deleting a subset if any of
	// the IDs is not present.
	RetireEvents(ctx context.Context, eventIDs []string, manifest CompactionManifest) error

	// Compactions returns every recorded manifest in ascending ToPosition
	// order, or an empty slice when the store has never been compacted.
	Compactions(ctx context.Context) ([]CompactionManifest, error)
}
