// Package migration moves a pericarp event feed between event stores.
//
// A pericarp app's event store is its only source of truth — read models and
// projections are derived from the feed. Migrating an app's data therefore
// means moving its events; the destination app rebuilds its own projections
// when it next runs.
//
// Migration is modelled as export → portable file → import so the source and
// destination never need to be reachable at the same time, and so the two ends
// may run on different backends (SQLite, Postgres, DynamoDB — anything that
// implements domain.EventStore). The portable format is newline-delimited JSON
// (one EventEnvelope per line, optionally preceded by a version header line).
//
// Ordering: a store assigns each event a global Position on append, and the
// destination reassigns Position when the event is re-appended. Export reads in
// ascending source Position order (EventStore.ReadAfter) and Import appends in
// file order, so the destination's append order — and thus its global feed
// order — matches the source. Payloads are copied as opaque JSON, so no payload
// type registration is required and the copy is faithful; event schemas are not
// upcast (transform in the import loop if versions differ).
package migration

const (
	// FormatVersion is written into an export file's header line and is the
	// highest format version Import understands.
	FormatVersion = 1

	// DefaultBatchSize is the number of events Export requests per ReadAfter
	// call when ExportOptions.BatchSize is not set.
	DefaultBatchSize = 500
)

// header is the optional first line of an export file. Its presence lets Import
// detect and version the format; an export without it is still importable.
type header struct {
	PericarpExport int `json:"pericarp_export"`
}

// ExportOptions configures Export.
type ExportOptions struct {
	// FromPosition resumes the export after this global position; 0 exports
	// from the beginning of the feed. Use a previous run's
	// ExportReport.LastPosition to continue an interrupted export.
	FromPosition int64

	// BatchSize is how many events are fetched per ReadAfter call. Values <= 0
	// use DefaultBatchSize.
	BatchSize int

	// Progress, if set, is called after each batch with the cumulative report
	// so far. It lets long-running exports surface progress (e.g. an async job
	// endpoint polling its state). It must not block or panic.
	Progress func(ExportReport)
}

// ExportReport summarizes a completed export.
type ExportReport struct {
	// Count is the number of events written (excluding the header line).
	Count int64
	// LastPosition is the resume cursor. It starts at ExportOptions.FromPosition
	// and advances to each written event's Position, so it is the highest
	// position written — or equal to FromPosition when nothing was written after
	// it (Count == 0). Pass it back as ExportOptions.FromPosition to continue an
	// interrupted export.
	LastPosition int64
}

// ImportOptions configures Import.
type ImportOptions struct {
	// SkipExisting checks the destination for each event ID before appending
	// and skips events already present, making a re-run idempotent. Without it
	// a re-run against a destination that already has some events fails on the
	// first duplicate (event ID is the store's primary key). Costs one extra
	// read per event.
	SkipExisting bool

	// Progress, if set, is called roughly every 1000 events with the
	// cumulative report so far. It must not block or panic.
	Progress func(ImportReport)
}

// ImportReport summarizes a completed import.
type ImportReport struct {
	// Count is the number of events appended to the destination.
	Count int64
	// Skipped is the number of events skipped because they already existed
	// (only non-zero when ImportOptions.SkipExisting is set).
	Skipped int64
}
