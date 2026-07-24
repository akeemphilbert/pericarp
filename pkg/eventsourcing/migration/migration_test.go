package migration_test

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/migration"
)

// seedEvent appends one event for aggID at the given sequence number and returns
// the stored envelope. Calls interleaved across aggregates exercise the global,
// cross-aggregate ordering that export/import must preserve.
func seedEvent(t *testing.T, s domain.EventStore, aggID, eventType string, seq int, payload map[string]any) domain.EventEnvelope[any] {
	t.Helper()
	env := domain.NewEventEnvelope[any](payload, aggID, eventType, seq)
	if err := s.Append(context.Background(), aggID, seq-1, env); err != nil {
		t.Fatalf("seed append %s#%d: %v", aggID, seq, err)
	}
	return env
}

// interleavedFixture seeds a source store with events across three aggregates,
// interleaved so global position order differs from any single aggregate's
// order. Returns the source store and the events in global (append) order.
func interleavedFixture(t *testing.T) (*infrastructure.MemoryStore, []domain.EventEnvelope[any]) {
	t.Helper()
	src := infrastructure.NewMemoryStore()
	add := func(agg, typ string, seq int, v string) {
		seedEvent(t, src, agg, typ, seq, map[string]any{"v": v})
	}
	add("a", "a.created", 1, "a1")
	add("b", "b.created", 1, "b1")
	add("a", "a.updated", 2, "a2")
	add("c", "c.created", 1, "c1")
	add("b", "b.updated", 2, "b2")
	add("a", "a.updated", 3, "a3")
	add("c", "c.updated", 2, "c2")
	// Return the stored envelopes (in global order) so callers see the
	// store-assigned Position and the payload exactly as persisted.
	return src, globalOrder(t, src)
}

// globalOrder returns every event in s in ascending global position order.
func globalOrder(t *testing.T, s domain.EventStore) []domain.EventEnvelope[any] {
	t.Helper()
	all, err := s.ReadAfter(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("ReadAfter: %v", err)
	}
	return all
}

// assertSameEvent compares the identity/content fields that must survive a
// migration. Position is deliberately excluded: the destination reassigns it.
func assertSameEvent(t *testing.T, got, want domain.EventEnvelope[any], ctx string) {
	t.Helper()
	if got.ID != want.ID {
		t.Errorf("%s: ID = %q, want %q", ctx, got.ID, want.ID)
	}
	if got.AggregateID != want.AggregateID {
		t.Errorf("%s: AggregateID = %q, want %q", ctx, got.AggregateID, want.AggregateID)
	}
	if got.EventType != want.EventType {
		t.Errorf("%s: EventType = %q, want %q", ctx, got.EventType, want.EventType)
	}
	if got.SequenceNo != want.SequenceNo {
		t.Errorf("%s: SequenceNo = %d, want %d", ctx, got.SequenceNo, want.SequenceNo)
	}
	if !got.Created.Equal(want.Created) {
		t.Errorf("%s: Created = %v, want %v", ctx, got.Created, want.Created)
	}
	if !reflect.DeepEqual(got.Payload, want.Payload) {
		t.Errorf("%s: Payload = %#v, want %#v", ctx, got.Payload, want.Payload)
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	t.Parallel()

	// Small batch sizes force multiple ReadAfter calls, exercising the paging loop.
	for _, batch := range []int{0, 1, 3} {
		t.Run("batch="+strconv.Itoa(batch), func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			src, order := interleavedFixture(t)

			var buf bytes.Buffer
			exp, err := migration.Export(ctx, src, &buf, migration.ExportOptions{BatchSize: batch})
			if err != nil {
				t.Fatalf("Export: %v", err)
			}
			if exp.Count != int64(len(order)) {
				t.Fatalf("export Count = %d, want %d", exp.Count, len(order))
			}
			if exp.LastPosition != order[len(order)-1].Position {
				t.Errorf("export LastPosition = %d, want %d", exp.LastPosition, order[len(order)-1].Position)
			}

			dst := infrastructure.NewMemoryStore()
			imp, err := migration.Import(ctx, dst, bytes.NewReader(buf.Bytes()), migration.ImportOptions{})
			if err != nil {
				t.Fatalf("Import: %v", err)
			}
			if imp.Count != int64(len(order)) || imp.Skipped != 0 {
				t.Fatalf("import report = %+v, want Count=%d Skipped=0", imp, len(order))
			}

			// Global feed order is preserved and destination positions are dense/ascending.
			gotOrder := globalOrder(t, dst)
			if len(gotOrder) != len(order) {
				t.Fatalf("dst has %d events, want %d", len(gotOrder), len(order))
			}
			for i := range order {
				assertSameEvent(t, gotOrder[i], order[i], "global["+strconv.Itoa(i)+"]")
				if want := int64(i + 1); gotOrder[i].Position != want {
					t.Errorf("dst position[%d] = %d, want %d", i, gotOrder[i].Position, want)
				}
			}
		})
	}
}

func TestExportResumeFromPosition(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	src, order := interleavedFixture(t)

	// Resume after the 4th event; only later events should be exported.
	from := order[3].Position
	var buf bytes.Buffer
	exp, err := migration.Export(ctx, src, &buf, migration.ExportOptions{FromPosition: from})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	want := order[4:]
	if exp.Count != int64(len(want)) {
		t.Fatalf("resumed export Count = %d, want %d", exp.Count, len(want))
	}

	dst := infrastructure.NewMemoryStore()
	if _, err := migration.Import(ctx, dst, &buf, migration.ImportOptions{}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	got := globalOrder(t, dst)
	if len(got) != len(want) {
		t.Fatalf("dst has %d events, want %d", len(got), len(want))
	}
	for i := range want {
		assertSameEvent(t, got[i], want[i], "resumed["+strconv.Itoa(i)+"]")
	}
}

func TestImportSkipExistingIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	src, order := interleavedFixture(t)

	var buf bytes.Buffer
	if _, err := migration.Export(ctx, src, &buf, migration.ExportOptions{}); err != nil {
		t.Fatalf("Export: %v", err)
	}
	data := buf.Bytes()

	dst := infrastructure.NewMemoryStore()
	if imp, err := migration.Import(ctx, dst, bytes.NewReader(data), migration.ImportOptions{}); err != nil {
		t.Fatalf("first Import: %v", err)
	} else if imp.Count != int64(len(order)) {
		t.Fatalf("first import Count = %d, want %d", imp.Count, len(order))
	}

	// Re-running with SkipExisting is a no-op: every event is found on the
	// destination and skipped, so no duplicates are appended. (Stores with a
	// unique event-ID key — GORM/Dynamo — would additionally reject a plain
	// re-import; MemoryStore does not, so that path is covered by the CLI
	// end-to-end test against SQLite rather than here.)
	imp, err := migration.Import(ctx, dst, bytes.NewReader(data), migration.ImportOptions{SkipExisting: true})
	if err != nil {
		t.Fatalf("re-import with SkipExisting: %v", err)
	}
	if imp.Count != 0 || imp.Skipped != int64(len(order)) {
		t.Fatalf("skip-existing report = %+v, want Count=0 Skipped=%d", imp, len(order))
	}
	if got := globalOrder(t, dst); len(got) != len(order) {
		t.Fatalf("dst has %d events after re-import, want %d", len(got), len(order))
	}
}

func TestImportWithoutHeader(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	src, order := interleavedFixture(t)

	var buf bytes.Buffer
	if _, err := migration.Export(ctx, src, &buf, migration.ExportOptions{}); err != nil {
		t.Fatalf("Export: %v", err)
	}
	// Strip the header line — a headerless file must still import.
	lines := strings.SplitN(buf.String(), "\n", 2)
	if len(lines) != 2 {
		t.Fatalf("export produced no body")
	}
	dst := infrastructure.NewMemoryStore()
	imp, err := migration.Import(ctx, dst, strings.NewReader(lines[1]), migration.ImportOptions{})
	if err != nil {
		t.Fatalf("Import headerless: %v", err)
	}
	if imp.Count != int64(len(order)) {
		t.Fatalf("headerless import Count = %d, want %d", imp.Count, len(order))
	}
}

func TestImportUnsupportedVersionRejected(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dst := infrastructure.NewMemoryStore()
	_, err := migration.Import(ctx, dst, strings.NewReader(`{"pericarp_export":999}`+"\n"), migration.ImportOptions{})
	if err == nil || !strings.Contains(err.Error(), "unsupported export format version") {
		t.Fatalf("expected unsupported-version error, got %v", err)
	}
}

func TestImportMalformedLineErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dst := infrastructure.NewMemoryStore()
	// Header, then a valid-JSON-but-empty event (missing id/aggregate_id).
	in := `{"pericarp_export":1}` + "\n" + `{"event_type":"x"}` + "\n"
	_, err := migration.Import(ctx, dst, strings.NewReader(in), migration.ImportOptions{})
	if err == nil || !strings.Contains(err.Error(), "missing id or aggregate_id") {
		t.Fatalf("expected missing-id error, got %v", err)
	}
}

func TestExportEmptyStore(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	src := infrastructure.NewMemoryStore()

	var buf bytes.Buffer
	exp, err := migration.Export(ctx, src, &buf, migration.ExportOptions{})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if exp.Count != 0 || exp.LastPosition != 0 {
		t.Fatalf("empty export report = %+v, want zero", exp)
	}

	dst := infrastructure.NewMemoryStore()
	imp, err := migration.Import(ctx, dst, &buf, migration.ImportOptions{})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if imp.Count != 0 {
		t.Fatalf("empty import Count = %d, want 0", imp.Count)
	}
}

func TestExportContextCancelled(t *testing.T) {
	t.Parallel()
	src, _ := interleavedFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := migration.Export(ctx, src, &bytes.Buffer{}, migration.ExportOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Export with cancelled ctx: err = %v, want context.Canceled", err)
	}
}
