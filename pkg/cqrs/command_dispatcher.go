package cqrs

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/segmentio/ksuid"
)

// CommandReceiver is a type-safe receiver function for processing commands.
// The type parameter T represents the strongly-typed command payload.
// REQ-CD-001
type CommandReceiver[T any] func(ctx context.Context, env CommandEnvelope[T]) (any, error)

// receiverFunc is the internal representation of a receiver that accepts CommandEnvelope[any].
type receiverFunc func(ctx context.Context, env CommandEnvelope[any]) (any, error)

// CommandEnvelope wraps a command payload with metadata fields.
// REQ-CD-002
type CommandEnvelope[T any] struct {
	ID          string         `json:"id"`
	CommandType string         `json:"command_type"`
	Payload     T              `json:"payload"`
	Created     time.Time      `json:"timestamp"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// NewCommandEnvelope creates a new CommandEnvelope with the given payload and command type.
func NewCommandEnvelope[T any](payload T, commandType string) CommandEnvelope[T] {
	return CommandEnvelope[T]{
		ID:          ksuid.New().String(),
		CommandType: commandType,
		Payload:     payload,
		Created:     time.Now(),
		Metadata:    make(map[string]any),
	}
}

// ToAnyCommandEnvelope converts a typed CommandEnvelope to CommandEnvelope[any].
func ToAnyCommandEnvelope[T any](env CommandEnvelope[T]) CommandEnvelope[any] {
	return CommandEnvelope[any]{
		ID:          env.ID,
		CommandType: env.CommandType,
		Payload:     env.Payload,
		Created:     env.Created,
		Metadata:    env.Metadata,
	}
}

// CommandResult contains the result from a single receiver execution.
// REQ-CD-003
type CommandResult struct {
	Value       any
	Error       error
	CommandType string
}

// Watchable allows the caller to observe results from receivers incrementally as they complete.
// REQ-CD-004
type Watchable struct {
	results chan CommandResult
	done    chan struct{}
}

func newWatchable(bufferSize int) *Watchable {
	if bufferSize < 0 {
		bufferSize = 0
	}
	return &Watchable{
		results: make(chan CommandResult, bufferSize),
		done:    make(chan struct{}),
	}
}

// Results returns a receive-only channel of CommandResult values, allowing the caller
// to iterate over results as each receiver completes.
// REQ-CD-030
func (w *Watchable) Results() <-chan CommandResult {
	return w.results
}

// Wait blocks until all receivers have completed and returns a slice of all CommandResult values.
// REQ-CD-033
func (w *Watchable) Wait() []CommandResult {
	var results []CommandResult
	for r := range w.results {
		results = append(results, r)
	}
	return results
}

// First blocks until the first result arrives from any receiver and returns that single CommandResult.
// Remaining receivers continue executing in the background and their results are buffered.
// Returns false if no receivers are registered.
// REQ-CD-034, REQ-CD-035
func (w *Watchable) First() (CommandResult, bool) {
	r, ok := <-w.results
	return r, ok
}

// Done returns a receive-only channel closed when all receivers complete, for use in select statements.
// REQ-CD-036
func (w *Watchable) Done() <-chan struct{} {
	return w.done
}

// CommandDispatcher is the common interface for async and queued dispatchers.
// REQ-CD-080
type CommandDispatcher interface {
	Dispatch(ctx context.Context, envelope CommandEnvelope[any]) *Watchable
	RegisterWildcardReceiver(receiver func(context.Context, CommandEnvelope[any]) (any, error)) error
	Close() error
}

// receiverRegistrar is an internal interface for generic receiver registration.
type receiverRegistrar interface {
	addReceiver(commandType string, fn receiverFunc) error
}

// commandRegistry provides shared registration and resolution logic for dispatchers.
type commandRegistry struct {
	mu                sync.RWMutex
	receivers         map[string][]receiverFunc
	wildcardReceivers []receiverFunc
}

func newCommandRegistry() commandRegistry {
	return commandRegistry{
		receivers:         make(map[string][]receiverFunc),
		wildcardReceivers: make([]receiverFunc, 0),
	}
}

// addReceiver stores a receiver function for a command type.
func (r *commandRegistry) addReceiver(commandType string, fn receiverFunc) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.receivers[commandType] = append(r.receivers[commandType], fn)
	return nil
}

// RegisterWildcardReceiver registers a catch-all receiver invoked for all command types.
// REQ-CD-014
func (r *commandRegistry) RegisterWildcardReceiver(receiver func(context.Context, CommandEnvelope[any]) (any, error)) error {
	if receiver == nil {
		return fmt.Errorf("receiver cannot be nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.wildcardReceivers = append(r.wildcardReceivers, receiver)
	return nil
}

// resolveReceivers returns all receivers matching the command type using dot-separated pattern matching.
// The lock is acquired during resolution and released before returning, so receivers execute lock-free.
// REQ-CD-021, REQ-CD-071
func (r *commandRegistry) resolveReceivers(commandType string) []receiverFunc {
	r.mu.RLock()
	defer r.mu.RUnlock()

	patterns := getMatchingPatterns(commandType)
	var all []receiverFunc
	for _, p := range patterns {
		all = append(all, r.receivers[p]...)
	}
	all = append(all, r.wildcardReceivers...)
	return all
}

// RegisterReceiver registers a typed CommandReceiver for a specific command type string.
// This is a generic package-level function because Go doesn't support generic methods on non-generic types.
// It accepts the CommandDispatcher interface and internally type-asserts to access registration.
// REQ-CD-010, REQ-CD-011, REQ-CD-012, REQ-CD-013
func RegisterReceiver[T any](d CommandDispatcher, commandType string, receiver CommandReceiver[T]) error {
	if commandType == "" {
		return fmt.Errorf("command type cannot be empty")
	}
	if receiver == nil {
		return fmt.Errorf("receiver cannot be nil")
	}

	reg, ok := d.(receiverRegistrar)
	if !ok {
		return fmt.Errorf("dispatcher does not support receiver registration")
	}

	wrapped := func(ctx context.Context, env CommandEnvelope[any]) (any, error) {
		payload, ok := env.Payload.(T)
		if !ok {
			// REQ-CD-062
			return nil, fmt.Errorf("type assertion failed: expected %T, got %T for command type %q", *new(T), env.Payload, commandType)
		}

		typedEnv := CommandEnvelope[T]{
			ID:          env.ID,
			CommandType: env.CommandType,
			Payload:     payload,
			Created:     env.Created,
			Metadata:    env.Metadata,
		}

		return receiver(ctx, typedEnv)
	}

	return reg.addReceiver(commandType, wrapped)
}

// executeReceiver invokes a receiver with panic recovery and sends the result to the Watchable.
// REQ-CD-060, REQ-CD-061
func executeReceiver(fn receiverFunc, ctx context.Context, envelope CommandEnvelope[any], w *Watchable) {
	defer func() {
		if r := recover(); r != nil {
			w.results <- CommandResult{
				Error:       fmt.Errorf("receiver panicked: %v", r),
				CommandType: envelope.CommandType,
			}
		}
	}()

	value, err := fn(ctx, envelope)
	result := CommandResult{CommandType: envelope.CommandType}
	if err != nil {
		result.Error = err
	} else {
		result.Value = value
	}
	w.results <- result
}

// --- Async Command Dispatcher ---

// AsyncCommandDispatcher executes all matched receivers concurrently using goroutines.
// REQ-CD-040
type AsyncCommandDispatcher struct {
	commandRegistry
}

// NewAsyncCommandDispatcher creates a new AsyncCommandDispatcher.
func NewAsyncCommandDispatcher() *AsyncCommandDispatcher {
	return &AsyncCommandDispatcher{
		commandRegistry: newCommandRegistry(),
	}
}

// Dispatch dispatches a command to all matching receivers concurrently and returns a Watchable.
// REQ-CD-020, REQ-CD-040, REQ-CD-041, REQ-CD-042
func (d *AsyncCommandDispatcher) Dispatch(ctx context.Context, envelope CommandEnvelope[any]) *Watchable {
	receivers := d.resolveReceivers(envelope.CommandType)
	w := newWatchable(len(receivers))

	// REQ-CD-022: no receivers match -> immediately complete
	if len(receivers) == 0 {
		close(w.results)
		close(w.done)
		return w
	}

	var wg sync.WaitGroup
	wg.Add(len(receivers))

	for i := range receivers {
		go func(fn receiverFunc) {
			defer wg.Done()
			executeReceiver(fn, ctx, envelope, w)
		}(receivers[i])
	}

	// REQ-CD-032: close results channel after all receivers complete
	go func() {
		wg.Wait()
		close(w.results)
		close(w.done)
	}()

	return w
}

// Close releases resources held by the dispatcher.
func (d *AsyncCommandDispatcher) Close() error {
	return nil
}

// --- Queued Command Dispatcher ---

// QueuedCommandDispatcher executes matched receivers sequentially in registration order.
// REQ-CD-050
type QueuedCommandDispatcher struct {
	commandRegistry
}

// NewQueuedCommandDispatcher creates a new QueuedCommandDispatcher.
func NewQueuedCommandDispatcher() *QueuedCommandDispatcher {
	return &QueuedCommandDispatcher{
		commandRegistry: newCommandRegistry(),
	}
}

// Dispatch dispatches a command to all matching receivers sequentially and returns a Watchable.
// REQ-CD-020, REQ-CD-050, REQ-CD-051, REQ-CD-052, REQ-CD-053
func (d *QueuedCommandDispatcher) Dispatch(ctx context.Context, envelope CommandEnvelope[any]) *Watchable {
	receivers := d.resolveReceivers(envelope.CommandType)
	w := newWatchable(len(receivers))

	// REQ-CD-022: no receivers match -> immediately complete
	if len(receivers) == 0 {
		close(w.results)
		close(w.done)
		return w
	}

	go func() {
		defer close(w.results)
		defer close(w.done)

		for _, fn := range receivers {
			// REQ-CD-053: check context cancellation between receivers
			select {
			case <-ctx.Done():
				return
			default:
			}

			// REQ-CD-051: send result before invoking next receiver
			executeReceiver(fn, ctx, envelope, w)
		}
	}()

	return w
}

// Close releases resources held by the dispatcher.
func (d *QueuedCommandDispatcher) Close() error {
	return nil
}

// --- Pattern matching utilities ---

// getMatchingPatterns returns all patterns that match the given type string.
// For "user.create", it returns: ["user.create", "user.*", "*.create", "*.*"]
func getMatchingPatterns(typeName string) []string {
	parts := splitType(typeName)
	if len(parts) == 0 {
		return []string{typeName}
	}

	patterns := []string{
		typeName, // Exact match
	}

	if len(parts) == 1 {
		patterns = append(patterns, "*")
	} else if len(parts) == 2 {
		patterns = append(patterns,
			parts[0]+".*",
			"*."+parts[1],
			"*.*",
		)
	} else {
		for i := 0; i < len(parts); i++ {
			wildcardParts := make([]string, len(parts))
			copy(wildcardParts, parts)
			wildcardParts[i] = "*"
			patterns = append(patterns, joinParts(wildcardParts))
		}
		allWildcards := make([]string, len(parts))
		for i := range allWildcards {
			allWildcards[i] = "*"
		}
		patterns = append(patterns, joinParts(allWildcards))
	}

	return patterns
}

// splitType splits a dot-separated type string, filtering empty parts.
func splitType(typeName string) []string {
	if typeName == "" {
		return []string{}
	}
	parts := strings.Split(typeName, ".")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

// joinParts joins type parts with dots.
func joinParts(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += "." + parts[i]
	}
	return result
}
