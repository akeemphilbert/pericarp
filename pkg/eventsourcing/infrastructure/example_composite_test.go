package infrastructure_test

import (
	"context"
	"fmt"
	"os"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"
)

// ExampleCompositeEventStore shows an in-memory primary mirrored to a
// file-based secondary. Secondary writes happen asynchronously and never
// block the caller's commit path; errors are delivered to the handler.
func ExampleCompositeEventStore() {
	ctx := context.Background()

	primary := infrastructure.NewMemoryStore()

	backupDir, err := os.MkdirTemp("", "composite-backup-*")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(backupDir) }()

	backup, err := infrastructure.NewFileStore(backupDir)
	if err != nil {
		panic(err)
	}

	composite := infrastructure.NewCompositeEventStore(
		primary,
		[]domain.EventStore{backup},
		infrastructure.WithErrorHandler(func(idx int, err error, envelopes []domain.EventEnvelope[any]) {
			// In real code, log or emit a metric here.
			fmt.Printf("secondary[%d] failed on %d events: %v\n", idx, len(envelopes), err)
		}),
	)
	defer func() { _ = composite.Close() }()

	envelope := domain.ToAnyEnvelope(
		domain.NewEventEnvelope(map[string]any{"name": "Ada"}, "user-42", "user.created", 1),
	)
	if err := composite.Append(ctx, "user-42", -1, envelope); err != nil {
		panic(err)
	}

	events, _ := composite.GetEvents(ctx, "user-42")
	fmt.Println(len(events), events[0].EventType)

	// Output:
	// 1 user.created
}
