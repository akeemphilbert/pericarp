package ddd

import (
	"context"
	"errors"
	"testing"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// TestApplyEventSnapshotStart pins the rule a compacted store depends on: an
// aggregate with no state yet accepts whatever sequence number its first
// surviving event carries, because on a compacted store that event is the
// compaction event and its sequence number is the retired history's maximum
// plus one. The relaxation must stop there — a gap after state exists still
// means a lost event, which is the failure event sourcing exists to prevent.
func TestApplyEventSnapshotStart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	event := func(sequenceNo int) domain.EventEnvelope[any] {
		return domain.ToAnyEnvelope(domain.NewEventEnvelope("payload", "resource-1", "Resource.Compacted", sequenceNo))
	}

	t.Run("a fresh entity accepts a snapshot at any sequence number", func(t *testing.T) {
		t.Parallel()
		entity := NewBaseEntity("resource-1")

		if err := entity.ApplyEvent(ctx, event(7)); err != nil {
			t.Fatalf("expected a snapshot start to be accepted, got %v", err)
		}
		if got := entity.GetSequenceNo(); got != 7 {
			t.Fatalf("expected the entity at version 7, got %d", got)
		}
	})

	t.Run("the event after a snapshot must still follow it", func(t *testing.T) {
		t.Parallel()
		entity := NewBaseEntity("resource-1")
		if err := entity.ApplyEvent(ctx, event(4)); err != nil {
			t.Fatalf("apply the snapshot: %v", err)
		}

		if err := entity.ApplyEvent(ctx, event(6)); !errors.Is(err, ErrInvalidEventSequenceNo) {
			t.Fatalf("expected a skipped sequence number to be refused, got %v", err)
		}
		if err := entity.ApplyEvent(ctx, event(5)); err != nil {
			t.Fatalf("expected the next sequence number to be accepted, got %v", err)
		}
	})

	t.Run("a sequence number below one is still invalid", func(t *testing.T) {
		t.Parallel()
		entity := NewBaseEntity("resource-1")

		if err := entity.ApplyEvent(ctx, event(0)); !errors.Is(err, ErrInvalidEventSequenceNo) {
			t.Fatalf("expected sequence number 0 to be refused, got %v", err)
		}
	})

	t.Run("a restored entity keeps strict ordering", func(t *testing.T) {
		t.Parallel()
		// RestoreBaseEntity's sequence number came from the store, so the
		// entity does have state and a snapshot start would be a lost event.
		entity := RestoreBaseEntity("resource-1", 3)

		if err := entity.ApplyEvent(ctx, event(9)); !errors.Is(err, ErrInvalidEventSequenceNo) {
			t.Fatalf("expected a restored entity to refuse a jump, got %v", err)
		}
		if err := entity.ApplyEvent(ctx, event(4)); err != nil {
			t.Fatalf("expected the next sequence number to be accepted, got %v", err)
		}
	})

	t.Run("recording still starts a fresh entity at one", func(t *testing.T) {
		t.Parallel()
		entity := NewBaseEntity("resource-1")

		if err := entity.RecordEvent("payload", "Resource.Created"); err != nil {
			t.Fatalf("record: %v", err)
		}
		if got := entity.GetUncommittedEvents()[0].SequenceNo; got != 1 {
			t.Fatalf("expected the first recorded event at sequence 1, got %d", got)
		}
	})
}
