package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/bigtable"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// BigtableColumnFamily is the single column family used by BigtableEventStore.
// The table must be provisioned with this family before the store is used.
const BigtableColumnFamily = "e"

// Row-key prefixes and columns.
const (
	btEventPrefix = "e#"
	btIDPrefix    = "id#"
	btTxPrefix    = "tx#"

	btColID       = "id"
	btColType     = "type"
	btColSeq      = "seq"
	btColTx       = "tx"
	btColPayload  = "payload"
	btColMetadata = "metadata"
	btColCreated  = "created"
	btColAgg      = "agg"

	btSeqWidth = 20 // zero-padded digits so lex sort = numeric sort
)

var _ domain.EventStore = (*BigtableEventStore)(nil)

// BigtableEventStore persists events to Google Cloud Bigtable using a single
// table with three row-key spaces:
//
//   - "e#<aggregateID>#<seq:20>"              — authoritative event row
//   - "id#<eventID>"                          — secondary index for GetEventByID
//   - "tx#<txID>#<aggregateID>#<seq:20>"      — secondary index for GetEventsByTransactionID
//
// Because `#` is the field separator, Append rejects aggregate IDs and
// transaction IDs containing `#` or NUL — callers would otherwise corrupt
// neighbouring key spaces (e.g., agg "user#admin" would alias with agg "user").
//
// Concurrency limitation (read this before adopting): Append performs a
// non-atomic read-then-write version check (GetCurrentVersion followed by
// ApplyBulk). Two concurrent writers passing the same expectedVersion can
// both observe the pre-write state and both succeed, producing duplicate
// sequence numbers. The store therefore does NOT provide strict optimistic
// concurrency — implementing that on Bigtable requires CheckAndMutateRow
// against a per-aggregate version marker, which this store deliberately
// does not yet do. Acceptable for low-contention workloads (single-writer
// per aggregate, idempotent producers, append-only feeds). High-contention
// callers must serialize writes per aggregate (e.g., an actor mailbox) or
// use a different EventStore.
//
// All reads apply a LatestNFilter(1): the column family is expected to retain
// only the most recent version per cell. If duplicate appends (per the
// concurrency disclaimer above) produce multiple versions, reads still
// return a consistent envelope drawn from the most recent write, not a
// merge across writes.
type BigtableEventStore struct {
	client *bigtable.Client
	table  *bigtable.Table
}

// NewBigtableEventStore wraps a *bigtable.Client and the name of a
// pre-provisioned table that has column family BigtableColumnFamily.
// Panics with clear messages on nil/empty required inputs.
//
// The client is caller-owned; Close() does NOT close it. This matches the
// BigQuery and Dynamo peer stores so callers can safely share one client
// across multiple stores or other subsystems.
func NewBigtableEventStore(client *bigtable.Client, tableName string) *BigtableEventStore {
	if client == nil {
		panic("bigtable: client must not be nil")
	}
	if tableName == "" {
		panic("bigtable: table name must not be empty")
	}
	return &BigtableEventStore{
		client: client,
		table:  client.Open(tableName),
	}
}

func eventRowKey(aggregateID string, seq int) string {
	return btEventPrefix + aggregateID + "#" + padSeq(seq)
}

func eventPrefixRange(aggregateID string) bigtable.RowRange {
	return bigtable.PrefixRange(btEventPrefix + aggregateID + "#")
}

// eventPrefixEnd returns the exclusive upper bound for rows under the
// aggregate's event prefix: "#" (0x23) is bumped to "$" (0x24), which is
// safe because aggregate IDs are validated to contain neither.
func eventPrefixEnd(aggregateID string) string {
	return btEventPrefix + aggregateID + "$"
}

// eventRangeFrom returns the half-open range [padSeq(fromVersion), end-of-prefix)
// so Bigtable can seek directly to the requested seq rather than scanning the
// full aggregate history.
func eventRangeFrom(aggregateID string, fromVersion int) bigtable.RowRange {
	if fromVersion < 1 {
		fromVersion = 1
	}
	return bigtable.NewRange(eventRowKey(aggregateID, fromVersion), eventPrefixEnd(aggregateID))
}

// eventRangeBetween returns [padSeq(fromVersion), padSeq(toVersion+1)), or
// the unbounded-right variant when toVersion == -1.
func eventRangeBetween(aggregateID string, fromVersion, toVersion int) bigtable.RowRange {
	if fromVersion < 1 {
		fromVersion = 1
	}
	if toVersion == -1 {
		return bigtable.NewRange(eventRowKey(aggregateID, fromVersion), eventPrefixEnd(aggregateID))
	}
	return bigtable.NewRange(eventRowKey(aggregateID, fromVersion), eventRowKey(aggregateID, toVersion+1))
}

func padSeq(seq int) string {
	return fmt.Sprintf("%0*d", btSeqWidth, seq)
}

func parseSeq(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("bigtable: invalid seq %q: %w", s, err)
	}
	return n, nil
}

// validateIDSeparators rejects IDs containing `#` (the row-key separator) or
// NUL. Without this, "a#b" and "a" become indistinguishable key-spaces.
func validateIDSeparators(kind, id string) error {
	if strings.ContainsAny(id, "#\x00") {
		return fmt.Errorf("%w: %s must not contain '#' or NUL", domain.ErrInvalidEvent, kind)
	}
	return nil
}

// latestVersionRead is the canonical ReadOption bundle for every read path:
// keep multi-version rows from corrupting decoded envelopes.
func latestVersionRead() bigtable.ReadOption {
	return bigtable.RowFilter(bigtable.LatestNFilter(1))
}

// Append writes events to the primary event rows plus the ID and TX indexes.
// If expectedVersion != -1 the current version is read first and a mismatch
// yields ErrConcurrencyConflict.
func (s *BigtableEventStore) Append(ctx context.Context, aggregateID string, expectedVersion int, events ...domain.EventEnvelope[any]) error {
	if len(events) == 0 {
		return nil
	}
	if err := validateIDSeparators("aggregate ID", aggregateID); err != nil {
		return err
	}
	for _, event := range events {
		if event.AggregateID != aggregateID {
			return fmt.Errorf("%w: aggregate ID mismatch", domain.ErrInvalidEvent)
		}
		if event.ID == "" {
			return fmt.Errorf("%w: event ID is required", domain.ErrInvalidEvent)
		}
		if event.EventType == "" {
			return fmt.Errorf("%w: event type is required", domain.ErrInvalidEvent)
		}
		if event.TransactionID != "" {
			if err := validateIDSeparators("transaction ID", event.TransactionID); err != nil {
				return err
			}
		}
	}

	if expectedVersion != -1 {
		current, err := s.GetCurrentVersion(ctx, aggregateID)
		if err != nil {
			return fmt.Errorf("bigtable: version check read failed: %w", err)
		}
		if current != expectedVersion {
			return fmt.Errorf("%w: expected version %d, got %d", domain.ErrConcurrencyConflict, expectedVersion, current)
		}
	}

	rowKeys := make([]string, 0, len(events)*3)
	muts := make([]*bigtable.Mutation, 0, len(events)*3)

	for _, event := range events {
		payloadJSON, metadataJSON, err := marshalPayloadAndMetadata(event)
		if err != nil {
			return fmt.Errorf("%w: %v", domain.ErrInvalidEvent, err)
		}

		evKey := eventRowKey(aggregateID, event.SequenceNo)
		evMut := bigtable.NewMutation()
		evMut.Set(BigtableColumnFamily, btColID, bigtable.ServerTime, []byte(event.ID))
		evMut.Set(BigtableColumnFamily, btColAgg, bigtable.ServerTime, []byte(aggregateID))
		evMut.Set(BigtableColumnFamily, btColType, bigtable.ServerTime, []byte(event.EventType))
		evMut.Set(BigtableColumnFamily, btColSeq, bigtable.ServerTime, []byte(padSeq(event.SequenceNo)))
		if event.TransactionID != "" {
			evMut.Set(BigtableColumnFamily, btColTx, bigtable.ServerTime, []byte(event.TransactionID))
		}
		evMut.Set(BigtableColumnFamily, btColPayload, bigtable.ServerTime, []byte(payloadJSON))
		evMut.Set(BigtableColumnFamily, btColMetadata, bigtable.ServerTime, []byte(metadataJSON))
		evMut.Set(BigtableColumnFamily, btColCreated, bigtable.ServerTime, []byte(event.Created.UTC().Format(time.RFC3339Nano)))
		rowKeys = append(rowKeys, evKey)
		muts = append(muts, evMut)

		idKey := btIDPrefix + event.ID
		idMut := bigtable.NewMutation()
		idMut.Set(BigtableColumnFamily, btColAgg, bigtable.ServerTime, []byte(aggregateID))
		idMut.Set(BigtableColumnFamily, btColSeq, bigtable.ServerTime, []byte(padSeq(event.SequenceNo)))
		rowKeys = append(rowKeys, idKey)
		muts = append(muts, idMut)

		if event.TransactionID != "" {
			txKey := btTxPrefix + event.TransactionID + "#" + aggregateID + "#" + padSeq(event.SequenceNo)
			txMut := bigtable.NewMutation()
			txMut.Set(BigtableColumnFamily, btColAgg, bigtable.ServerTime, []byte(aggregateID))
			txMut.Set(BigtableColumnFamily, btColSeq, bigtable.ServerTime, []byte(padSeq(event.SequenceNo)))
			rowKeys = append(rowKeys, txKey)
			muts = append(muts, txMut)
		}
	}

	errs, err := s.table.ApplyBulk(ctx, rowKeys, muts)
	if err != nil {
		return fmt.Errorf("bigtable: ApplyBulk failed: %w", err)
	}
	var rowErrs []error
	for i, rowErr := range errs {
		if rowErr != nil {
			rowErrs = append(rowErrs, fmt.Errorf("bigtable: row %q failed: %w", rowKeys[i], rowErr))
		}
	}
	return errors.Join(rowErrs...)
}

func (s *BigtableEventStore) GetEvents(ctx context.Context, aggregateID string) ([]domain.EventEnvelope[any], error) {
	return s.readEventRange(ctx, eventPrefixRange(aggregateID))
}

func (s *BigtableEventStore) GetEventsFromVersion(ctx context.Context, aggregateID string, fromVersion int) ([]domain.EventEnvelope[any], error) {
	return s.readEventRange(ctx, eventRangeFrom(aggregateID, fromVersion))
}

// GetEventsRange returns events whose seq is within [fromVersion, toVersion].
// -1 sentinels match the EventStore interface contract.
func (s *BigtableEventStore) GetEventsRange(ctx context.Context, aggregateID string, fromVersion, toVersion int) ([]domain.EventEnvelope[any], error) {
	return s.readEventRange(ctx, eventRangeBetween(aggregateID, fromVersion, toVersion))
}

func (s *BigtableEventStore) GetEventByID(ctx context.Context, eventID string) (domain.EventEnvelope[any], error) {
	idRow, err := s.table.ReadRow(ctx, btIDPrefix+eventID, latestVersionRead())
	if err != nil {
		return domain.EventEnvelope[any]{}, fmt.Errorf("bigtable: id-index read failed: %w", err)
	}
	if len(idRow) == 0 {
		return domain.EventEnvelope[any]{}, domain.ErrEventNotFound
	}
	agg, seq, ok, err := extractAggSeq(idRow)
	if err != nil {
		return domain.EventEnvelope[any]{}, err
	}
	if !ok {
		return domain.EventEnvelope[any]{}, fmt.Errorf("%w: id-index row missing agg/seq columns", domain.ErrInvalidEvent)
	}
	evRow, err := s.table.ReadRow(ctx, eventRowKey(agg, seq), latestVersionRead())
	if err != nil {
		return domain.EventEnvelope[any]{}, fmt.Errorf("bigtable: event read failed: %w", err)
	}
	if len(evRow) == 0 {
		return domain.EventEnvelope[any]{}, domain.ErrEventNotFound
	}
	return decodeEventRow(evRow)
}

// GetEventsByTransactionID scans the tx-index prefix, dereferences each hit to
// the authoritative event row, and returns results sorted by (agg, seq).
func (s *BigtableEventStore) GetEventsByTransactionID(ctx context.Context, transactionID string) ([]domain.EventEnvelope[any], error) {
	if transactionID == "" {
		return nil, fmt.Errorf("%w: transaction ID must not be empty", domain.ErrInvalidEvent)
	}
	if err := validateIDSeparators("transaction ID", transactionID); err != nil {
		return nil, err
	}

	type hit struct {
		agg string
		seq int
	}
	var hits []hit
	var scanErr error
	err := s.table.ReadRows(ctx, bigtable.PrefixRange(btTxPrefix+transactionID+"#"), func(row bigtable.Row) bool {
		agg, seq, ok, parseErr := extractAggSeq(row)
		if parseErr != nil {
			scanErr = fmt.Errorf("tx-index row %q: %w", row.Key(), parseErr)
			return false
		}
		if !ok {
			scanErr = fmt.Errorf("%w: tx-index row %q missing agg/seq columns", domain.ErrInvalidEvent, row.Key())
			return false
		}
		hits = append(hits, hit{agg: agg, seq: seq})
		return true
	}, latestVersionRead())
	if err != nil {
		return nil, fmt.Errorf("bigtable: tx-index scan failed: %w", err)
	}
	if scanErr != nil {
		return nil, scanErr
	}
	if len(hits) == 0 {
		return []domain.EventEnvelope[any]{}, nil
	}

	sort.Slice(hits, func(i, j int) bool {
		if hits[i].agg != hits[j].agg {
			return hits[i].agg < hits[j].agg
		}
		return hits[i].seq < hits[j].seq
	})

	// Batch the dereference into a single RPC; collecting N separate ReadRows
	// would be O(N) round trips for a single logical transaction lookup.
	keys := make(bigtable.RowList, 0, len(hits))
	for _, h := range hits {
		keys = append(keys, eventRowKey(h.agg, h.seq))
	}
	found := make(map[string]domain.EventEnvelope[any], len(hits))
	var decodeErr error
	err = s.table.ReadRows(ctx, keys, func(row bigtable.Row) bool {
		env, derr := decodeEventRow(row)
		if derr != nil {
			decodeErr = derr
			return false
		}
		found[row.Key()] = env
		return true
	}, latestVersionRead())
	if err != nil {
		return nil, fmt.Errorf("bigtable: tx-ref batch read failed: %w", err)
	}
	if decodeErr != nil {
		return nil, decodeErr
	}

	out := make([]domain.EventEnvelope[any], 0, len(hits))
	for _, h := range hits {
		env, ok := found[eventRowKey(h.agg, h.seq)]
		if !ok {
			// Orphan index row (Append partial failure or out-of-band deletion).
			// Surface rather than silently drop; callers can decide to reindex.
			return nil, fmt.Errorf("%w: tx-index references missing event %s#%d", domain.ErrInvalidEvent, h.agg, h.seq)
		}
		out = append(out, env)
	}
	return out, nil
}

// GetCurrentVersion returns the seq number of the most recent event for the
// aggregate. Uses a reverse scan limited to one row — a single bounded row
// read, independent of history length.
func (s *BigtableEventStore) GetCurrentVersion(ctx context.Context, aggregateID string) (int, error) {
	var latest int
	var found bool
	var decodeErr error
	err := s.table.ReadRows(ctx, eventPrefixRange(aggregateID), func(row bigtable.Row) bool {
		_, seq, ok, parseErr := extractAggSeq(row)
		if parseErr != nil {
			// A malformed seq column is row corruption, not a missing column —
			// surface it rather than silently falling back.
			decodeErr = parseErr
			return false
		}
		if !ok {
			// Fall back to parsing the row key; if that also fails, surface an
			// error rather than returning 0 (which Append would read as
			// "brand-new aggregate").
			seq, decodeErr = seqFromRowKey(row.Key(), aggregateID)
			if decodeErr != nil {
				return false
			}
		}
		latest = seq
		found = true
		return false
	}, bigtable.ReverseScan(), bigtable.LimitRows(1), latestVersionRead())
	if err != nil {
		return 0, fmt.Errorf("bigtable: current-version read failed: %w", err)
	}
	if decodeErr != nil {
		return 0, decodeErr
	}
	if !found {
		return 0, nil
	}
	return latest, nil
}

// Close is a no-op. The client is caller-owned (matches BigQuery/Dynamo peers)
// so it can safely be shared across stores or long-lived subsystems.
func (s *BigtableEventStore) Close() error {
	return nil
}

// readEventRange materialises every row in r, decodes it, and returns the
// slice in ascending seq order (Bigtable row ranges are already lex-sorted,
// which equals numeric for zero-padded seq).
func (s *BigtableEventStore) readEventRange(ctx context.Context, r bigtable.RowRange) ([]domain.EventEnvelope[any], error) {
	var out []domain.EventEnvelope[any]
	var decodeErr error
	err := s.table.ReadRows(ctx, r, func(row bigtable.Row) bool {
		env, err := decodeEventRow(row)
		if err != nil {
			decodeErr = err
			return false
		}
		out = append(out, env)
		return true
	}, latestVersionRead())
	if err != nil {
		return nil, fmt.Errorf("bigtable: range scan failed: %w", err)
	}
	if decodeErr != nil {
		return nil, decodeErr
	}
	if out == nil {
		return []domain.EventEnvelope[any]{}, nil
	}
	return out, nil
}

// extractAggSeq pulls the "agg" and "seq" columns out of a Row (as written by
// id-index, tx-index, or event rows). seq is always a zero-padded decimal.
//
// Three outcomes:
//   - all required columns present and valid: (agg, seq, true, nil)
//   - columns missing: (agg-if-seen, 0, false, nil) — caller decides the fallback
//   - column present but malformed (seq can't parse): ("", 0, false, err) — caller
//     should surface the parse error rather than treat the row as "missing"
func extractAggSeq(row bigtable.Row) (string, int, bool, error) {
	items := row[BigtableColumnFamily]
	var agg string
	var seq int
	var seenAgg, seenSeq bool
	for _, it := range items {
		col := stripFamily(it.Column)
		switch col {
		case btColAgg:
			agg = string(it.Value)
			seenAgg = true
		case btColSeq:
			n, err := parseSeq(string(it.Value))
			if err != nil {
				return "", 0, false, fmt.Errorf("%w: %v", domain.ErrInvalidEvent, err)
			}
			seq = n
			seenSeq = true
		}
	}
	return agg, seq, seenAgg && seenSeq, nil
}

func seqFromRowKey(rowKey, aggregateID string) (int, error) {
	prefix := btEventPrefix + aggregateID + "#"
	if !strings.HasPrefix(rowKey, prefix) {
		return 0, fmt.Errorf("%w: row key %q does not match aggregate %q", domain.ErrInvalidEvent, rowKey, aggregateID)
	}
	return parseSeq(rowKey[len(prefix):])
}

// stripFamily removes the "<family>:" prefix that bigtable.ReadItem.Column carries.
func stripFamily(qualified string) string {
	if i := strings.IndexByte(qualified, ':'); i >= 0 {
		return qualified[i+1:]
	}
	return qualified
}

func decodeEventRow(row bigtable.Row) (domain.EventEnvelope[any], error) {
	items := row[BigtableColumnFamily]
	var (
		id, agg, typ, txID   string
		seq                  int
		payloadRaw, metaRaw  []byte
		created              time.Time
		haveSeq, haveCreated bool
	)
	for _, it := range items {
		switch stripFamily(it.Column) {
		case btColID:
			id = string(it.Value)
		case btColAgg:
			agg = string(it.Value)
		case btColType:
			typ = string(it.Value)
		case btColTx:
			txID = string(it.Value)
		case btColSeq:
			n, err := parseSeq(string(it.Value))
			if err != nil {
				return domain.EventEnvelope[any]{}, fmt.Errorf("%w: %v", domain.ErrInvalidEvent, err)
			}
			seq = n
			haveSeq = true
		case btColPayload:
			payloadRaw = it.Value
		case btColMetadata:
			metaRaw = it.Value
		case btColCreated:
			t, err := time.Parse(time.RFC3339Nano, string(it.Value))
			if err != nil {
				return domain.EventEnvelope[any]{}, fmt.Errorf("%w: created: %v", domain.ErrInvalidEvent, err)
			}
			created = t
			haveCreated = true
		}
	}
	if id == "" || agg == "" || typ == "" || !haveSeq || !haveCreated {
		return domain.EventEnvelope[any]{}, fmt.Errorf("%w: event row missing required fields", domain.ErrInvalidEvent)
	}

	payload := map[string]any{}
	if len(payloadRaw) > 0 {
		if err := json.Unmarshal(payloadRaw, &payload); err != nil {
			return domain.EventEnvelope[any]{}, fmt.Errorf("%w: payload: %v", domain.ErrInvalidEvent, err)
		}
	}
	metadata := map[string]any{}
	if len(metaRaw) > 0 {
		if err := json.Unmarshal(metaRaw, &metadata); err != nil {
			return domain.EventEnvelope[any]{}, fmt.Errorf("%w: metadata: %v", domain.ErrInvalidEvent, err)
		}
	}

	return domain.EventEnvelope[any]{
		ID:            id,
		AggregateID:   agg,
		EventType:     typ,
		Payload:       payload,
		Created:       created,
		SequenceNo:    seq,
		TransactionID: txID,
		Metadata:      metadata,
	}, nil
}
