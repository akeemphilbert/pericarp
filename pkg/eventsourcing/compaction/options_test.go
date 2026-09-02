package compaction_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/compaction"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"
)

func TestRetainHolds(t *testing.T) {
	t.Parallel()

	event := func(eventType string, created time.Time) domain.EventEnvelope[any] {
		return domain.EventEnvelope[any]{EventType: eventType, Created: created}
	}
	june := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	august := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		retain compaction.Retain
		event  domain.EventEnvelope[any]
		want   bool
	}{
		{
			name:  "the zero policy retains nothing",
			event: event("Resource.Created", june),
		},
		{
			name:   "a listed type is retained",
			retain: compaction.Retain{EventTypes: []string{"Resource.Audited"}},
			event:  event("Resource.Audited", june),
			want:   true,
		},
		{
			name:   "an unlisted type is not",
			retain: compaction.Retain{EventTypes: []string{"Resource.Audited"}},
			event:  event("Resource.Created", june),
		},
		{
			name:   "a matching predicate retains",
			retain: compaction.Retain{EventTypeMatch: func(t string) bool { return strings.HasSuffix(t, "Audited") }},
			event:  event("Resource.Audited", june),
			want:   true,
		},
		{
			name:   "an event on the not-before boundary is retained",
			retain: compaction.Retain{NotBefore: august},
			event:  event("Resource.Created", august),
			want:   true,
		},
		{
			name:   "an event before the not-before time is not",
			retain: compaction.Retain{NotBefore: august},
			event:  event("Resource.Created", june),
		},
		{
			name: "the conditions are a union",
			retain: compaction.Retain{
				EventTypes: []string{"Resource.Audited"},
				NotBefore:  august,
			},
			event: event("Resource.Created", august),
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.retain.Holds(tt.event); got != tt.want {
				t.Fatalf("expected Holds to be %v, got %v", tt.want, got)
			}
		})
	}
}

func TestDefaultIsDelete(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		"Resource.Deleted":     true,
		"AccountDeleted":       true,
		"Resource.Retired":     false,
		"Resource.DeletedItem": false,
		"Resource.Created":     false,
	}

	for eventType, want := range tests {
		t.Run(eventType, func(t *testing.T) {
			t.Parallel()
			got := compaction.DefaultIsDelete(domain.EventEnvelope[any]{EventType: eventType})
			if got != want {
				t.Fatalf("expected DefaultIsDelete(%q) to be %v, got %v", eventType, want, got)
			}
		})
	}
}

// TestCompactRefusesIncompleteOptions checks the guards that run before
// anything is read, so a misconfigured call cannot leave a store half
// compacted or an archive half written.
func TestCompactRefusesIncompleteOptions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	archive, err := os.CreateTemp(t.TempDir(), "archive-*.jsonl")
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	t.Cleanup(func() { _ = archive.Close() })

	state := func(context.Context, string) (any, error) { return map[string]any{}, nil }

	tests := []struct {
		name string
		opts compaction.Options
		want error
	}{
		{
			name: "no event type",
			opts: compaction.Options{Watermark: 1, State: state, Archive: archive},
			want: compaction.ErrMissingEventType,
		},
		{
			name: "no state provider",
			opts: compaction.Options{Watermark: 1, EventType: "Resource.Compacted", Archive: archive},
			want: compaction.ErrMissingStateProvider,
		},
		{
			name: "no archive",
			opts: compaction.Options{Watermark: 1, EventType: "Resource.Compacted", State: state},
			want: compaction.ErrMissingArchive,
		},
		{
			name: "an archive that cannot be fsynced",
			opts: compaction.Options{
				Watermark: 1, EventType: "Resource.Compacted", State: state,
				Archive: &plainWriter{w: archive},
			},
			want: compaction.ErrArchiveNotSyncable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := infrastructure.NewMemoryStore()
			if _, err := compaction.Compact(ctx, store, tt.opts); !errors.Is(err, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, err)
			}
		})
	}
}
