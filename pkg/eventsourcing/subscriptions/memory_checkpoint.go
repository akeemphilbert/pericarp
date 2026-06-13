package subscriptions

import (
	"context"
	"fmt"
	"sync"
)

// MemoryCheckpointStore is an in-memory CheckpointStore for tests and
// single-process development setups. It offers no transactional coupling with
// handler writes — handlers get at-least-once delivery.
type MemoryCheckpointStore struct {
	mu        sync.Mutex
	positions map[string]int64
	held      map[string]bool
}

var _ CheckpointStore = (*MemoryCheckpointStore)(nil)

// NewMemoryCheckpointStore creates an empty in-memory checkpoint store.
func NewMemoryCheckpointStore() *MemoryCheckpointStore {
	return &MemoryCheckpointStore{
		positions: make(map[string]int64),
		held:      make(map[string]bool),
	}
}

// Acquire begins a processing cycle; acquired is false while another batch
// for the same subscriber is active.
func (m *MemoryCheckpointStore) Acquire(ctx context.Context, subscriber string) (Batch, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.held[subscriber] {
		return nil, false, nil
	}
	m.held[subscriber] = true
	return &memoryBatch{store: m, subscriber: subscriber, position: m.positions[subscriber]}, true, nil
}

// Position returns the committed checkpoint for the subscriber.
func (m *MemoryCheckpointStore) Position(ctx context.Context, subscriber string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.positions[subscriber], nil
}

// Reset sets the committed checkpoint. It fails while a batch is in flight
// (an in-memory caller can simply stop the subscriber first).
func (m *MemoryCheckpointStore) Reset(ctx context.Context, subscriber string, position int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.held[subscriber] {
		return fmt.Errorf("subscriber %q has a batch in flight", subscriber)
	}
	m.positions[subscriber] = position
	return nil
}

type memoryBatch struct {
	store      *MemoryCheckpointStore
	subscriber string
	position   int64
	done       bool
}

func (b *memoryBatch) Position() int64 { return b.position }

func (b *memoryBatch) HandlerContext(ctx context.Context) context.Context { return ctx }

func (b *memoryBatch) Commit(ctx context.Context, position int64) error {
	b.store.mu.Lock()
	defer b.store.mu.Unlock()
	if b.done {
		return fmt.Errorf("batch for subscriber %q already finished", b.subscriber)
	}
	b.done = true
	b.store.positions[b.subscriber] = position
	delete(b.store.held, b.subscriber)
	return nil
}

func (b *memoryBatch) Rollback() error {
	b.store.mu.Lock()
	defer b.store.mu.Unlock()
	if b.done {
		return nil
	}
	b.done = true
	delete(b.store.held, b.subscriber)
	return nil
}

// Savepoint is a no-op: memory batches have no transaction to mark.
func (b *memoryBatch) Savepoint(ctx context.Context, name string) error { return nil }

// RollbackToSavepoint is a no-op: memory batches have no transaction.
func (b *memoryBatch) RollbackToSavepoint(ctx context.Context, name string) error { return nil }
