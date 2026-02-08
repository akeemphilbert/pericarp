package cqrs_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/cqrs"
)

// --- Test-specific types ---

type CommandDispatcherTestCreateUser struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

type CommandDispatcherTestUpdateUser struct {
	UserID   string `json:"user_id"`
	NewEmail string `json:"new_email"`
}

type CommandDispatcherTestDeleteUser struct {
	UserID string `json:"user_id"`
}

type CommandDispatcherTestPlaceOrder struct {
	OrderID     string  `json:"order_id"`
	CustomerID  string  `json:"customer_id"`
	TotalAmount float64 `json:"total_amount"`
}

// --- Helpers ---

func makeEnvelope(commandType string, payload any) cqrs.CommandEnvelope[any] {
	env := cqrs.NewCommandEnvelope(payload, commandType)
	return cqrs.ToAnyCommandEnvelope(env)
}

// runForBothDispatchers runs a test function against both dispatcher types.
// The callback receives the dispatcher name, the dispatcher (as CommandDispatcher),
// and typed registration functions since RegisterReceiver needs the concrete type.
func runForBothDispatchers(t *testing.T, testFn func(t *testing.T, name string, d cqrs.CommandDispatcher, registerCreateUser func(commandType string, receiver cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, registerUpdateUser func(commandType string, receiver cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error)) {
	t.Helper()

	t.Run("AsyncCommandDispatcher", func(t *testing.T) {
		t.Parallel()
		ad := cqrs.NewAsyncCommandDispatcher()
		testFn(t, "AsyncCommandDispatcher", ad,
			func(ct string, r cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error {
				return cqrs.RegisterReceiver(ad, ct, r)
			},
			func(ct string, r cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error {
				return cqrs.RegisterReceiver(ad, ct, r)
			},
		)
	})

	t.Run("QueuedCommandDispatcher", func(t *testing.T) {
		t.Parallel()
		qd := cqrs.NewQueuedCommandDispatcher()
		testFn(t, "QueuedCommandDispatcher", qd,
			func(ct string, r cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error {
				return cqrs.RegisterReceiver(qd, ct, r)
			},
			func(ct string, r cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error {
				return cqrs.RegisterReceiver(qd, ct, r)
			},
		)
	})
}

// =============================================================================
// REQ-CD-001: CommandReceiver[T] type
// =============================================================================

func TestCommandReceiverType(t *testing.T) {
	t.Parallel()

	t.Run("CommandReceiver function type accepts context and typed envelope", func(t *testing.T) {
		t.Parallel()

		var receiver cqrs.CommandReceiver[CommandDispatcherTestCreateUser] = func(
			ctx context.Context,
			env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser],
		) (any, error) {
			return env.Payload.Email, nil
		}

		env := cqrs.NewCommandEnvelope(
			CommandDispatcherTestCreateUser{Email: "test@example.com", Name: "Test"},
			"user.create",
		)
		result, err := receiver(context.Background(), env)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result != "test@example.com" {
			t.Errorf("Expected 'test@example.com', got %v", result)
		}
	})
}

// =============================================================================
// REQ-CD-002: CommandEnvelope[T] struct with ID, CommandType, Payload, Created, Metadata
// =============================================================================

func TestCommandEnvelope(t *testing.T) {
	t.Parallel()

	t.Run("NewCommandEnvelope populates all fields", func(t *testing.T) {
		t.Parallel()

		before := time.Now()
		payload := CommandDispatcherTestCreateUser{Email: "a@b.com", Name: "Alice"}
		env := cqrs.NewCommandEnvelope(payload, "user.create")
		after := time.Now()

		if env.ID == "" {
			t.Error("Expected non-empty ID")
		}
		if env.CommandType != "user.create" {
			t.Errorf("Expected CommandType 'user.create', got %q", env.CommandType)
		}
		if env.Payload.Email != "a@b.com" {
			t.Errorf("Expected Payload.Email 'a@b.com', got %q", env.Payload.Email)
		}
		if env.Payload.Name != "Alice" {
			t.Errorf("Expected Payload.Name 'Alice', got %q", env.Payload.Name)
		}
		if env.Created.Before(before) || env.Created.After(after) {
			t.Errorf("Expected Created between %v and %v, got %v", before, after, env.Created)
		}
		if env.Metadata == nil {
			t.Error("Expected non-nil Metadata map")
		}
	})

	t.Run("ToAnyCommandEnvelope preserves all fields", func(t *testing.T) {
		t.Parallel()

		payload := CommandDispatcherTestCreateUser{Email: "b@c.com", Name: "Bob"}
		typed := cqrs.NewCommandEnvelope(payload, "user.create")
		typed.Metadata["key"] = "value"

		anyEnv := cqrs.ToAnyCommandEnvelope(typed)

		if anyEnv.ID != typed.ID {
			t.Errorf("Expected ID %q, got %q", typed.ID, anyEnv.ID)
		}
		if anyEnv.CommandType != typed.CommandType {
			t.Errorf("Expected CommandType %q, got %q", typed.CommandType, anyEnv.CommandType)
		}
		if anyEnv.Created != typed.Created {
			t.Errorf("Expected Created %v, got %v", typed.Created, anyEnv.Created)
		}
		if anyEnv.Metadata["key"] != "value" {
			t.Errorf("Expected Metadata['key'] = 'value', got %v", anyEnv.Metadata["key"])
		}
		p, ok := anyEnv.Payload.(CommandDispatcherTestCreateUser)
		if !ok {
			t.Fatalf("Expected payload type CommandDispatcherTestCreateUser, got %T", anyEnv.Payload)
		}
		if p.Email != "b@c.com" {
			t.Errorf("Expected Email 'b@c.com', got %q", p.Email)
		}
	})

	t.Run("unique IDs for different envelopes", func(t *testing.T) {
		t.Parallel()

		env1 := cqrs.NewCommandEnvelope("p1", "cmd.a")
		env2 := cqrs.NewCommandEnvelope("p2", "cmd.b")
		if env1.ID == env2.ID {
			t.Error("Expected unique IDs for different envelopes")
		}
	})
}

// =============================================================================
// REQ-CD-003: CommandResult with Value, Error, CommandType
// =============================================================================

func TestCommandResult(t *testing.T) {
	t.Parallel()

	t.Run("CommandResult holds value, error, and command type", func(t *testing.T) {
		t.Parallel()

		r := cqrs.CommandResult{
			Value:       "ok",
			Error:       nil,
			CommandType: "user.create",
		}
		if r.Value != "ok" {
			t.Errorf("Expected Value 'ok', got %v", r.Value)
		}
		if r.Error != nil {
			t.Errorf("Expected nil Error, got %v", r.Error)
		}
		if r.CommandType != "user.create" {
			t.Errorf("Expected CommandType 'user.create', got %q", r.CommandType)
		}
	})

	t.Run("CommandResult with error", func(t *testing.T) {
		t.Parallel()

		r := cqrs.CommandResult{
			Value:       nil,
			Error:       errors.New("fail"),
			CommandType: "order.place",
		}
		if r.Value != nil {
			t.Errorf("Expected nil Value, got %v", r.Value)
		}
		if r.Error == nil || r.Error.Error() != "fail" {
			t.Errorf("Expected error 'fail', got %v", r.Error)
		}
		if r.CommandType != "order.place" {
			t.Errorf("Expected CommandType 'order.place', got %q", r.CommandType)
		}
	})
}

// =============================================================================
// REQ-CD-004: Watchable type
// =============================================================================

func TestWatchableType(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, regCU func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()
		w := d.Dispatch(context.Background(), makeEnvelope("test.cmd", "payload"))
		if w == nil {
			t.Fatal("Expected non-nil Watchable")
		}
		w.Wait()
	})
}

// =============================================================================
// REQ-CD-010: RegisterReceiver[T] generic package-level function
// =============================================================================

func TestRegisterReceiver(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, regCU func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		receiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			return "user-created:" + env.Payload.Email, nil
		}

		err := regCU("user.create", receiver)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		w := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "test@x.com"}))
		results := w.Wait()
		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(results))
		}
		if results[0].Error != nil {
			t.Fatalf("Expected no error, got %v", results[0].Error)
		}
		if results[0].Value != "user-created:test@x.com" {
			t.Errorf("Expected 'user-created:test@x.com', got %v", results[0].Value)
		}
	})
}

// =============================================================================
// REQ-CD-011: Empty command type returns error
// =============================================================================

func TestRegisterReceiverEmptyCommandType(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, regCU func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		receiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			return nil, nil
		}

		err := regCU("", receiver)
		if err == nil {
			t.Fatal("Expected error for empty command type")
		}
		if !strings.Contains(err.Error(), "command type cannot be empty") {
			t.Errorf("Expected error about empty command type, got %q", err.Error())
		}
	})
}

// =============================================================================
// REQ-CD-012: Nil receiver returns error
// =============================================================================

func TestRegisterReceiverNilReceiver(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, regCU func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		err := regCU("user.create", nil)
		if err == nil {
			t.Fatal("Expected error for nil receiver")
		}
		if !strings.Contains(err.Error(), "receiver cannot be nil") {
			t.Errorf("Expected error about nil receiver, got %q", err.Error())
		}
	})
}

// =============================================================================
// REQ-CD-013: Multiple receivers for same command type
// =============================================================================

func TestRegisterMultipleReceivers(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, regCU func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		var count int64

		receiver1 := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			atomic.AddInt64(&count, 1)
			return "r1", nil
		}
		receiver2 := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			atomic.AddInt64(&count, 1)
			return "r2", nil
		}

		if err := regCU("user.create", receiver1); err != nil {
			t.Fatalf("Failed to register receiver1: %v", err)
		}
		if err := regCU("user.create", receiver2); err != nil {
			t.Fatalf("Failed to register receiver2: %v", err)
		}

		w := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))
		results := w.Wait()

		if len(results) != 2 {
			t.Fatalf("Expected 2 results, got %d", len(results))
		}
		if atomic.LoadInt64(&count) != 2 {
			t.Errorf("Expected both receivers called, got count %d", count)
		}
	})
}

// =============================================================================
// REQ-CD-014: RegisterWildcardReceiver for catch-all
// =============================================================================

func TestRegisterWildcardReceiver(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, regCU func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		var count int64

		err := d.RegisterWildcardReceiver(func(ctx context.Context, env cqrs.CommandEnvelope[any]) (any, error) {
			atomic.AddInt64(&count, 1)
			return "wildcard", nil
		})
		if err != nil {
			t.Fatalf("Failed to register wildcard: %v", err)
		}

		w1 := d.Dispatch(context.Background(), makeEnvelope("user.create", "p1"))
		w1.Wait()
		w2 := d.Dispatch(context.Background(), makeEnvelope("order.place", "p2"))
		w2.Wait()

		if atomic.LoadInt64(&count) != 2 {
			t.Errorf("Expected wildcard called 2 times, got %d", count)
		}
	})
}

func TestRegisterWildcardReceiverNil(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		err := d.RegisterWildcardReceiver(nil)
		if err == nil {
			t.Fatal("Expected error for nil wildcard receiver")
		}
		if !strings.Contains(err.Error(), "receiver cannot be nil") {
			t.Errorf("Expected error about nil receiver, got %q", err.Error())
		}
	})
}

func TestRegisterWildcardReceiverCombinedWithSpecific(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, regCU func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		var specificCount, wildcardCount int64

		specificReceiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			atomic.AddInt64(&specificCount, 1)
			return "specific", nil
		}
		if err := regCU("user.create", specificReceiver); err != nil {
			t.Fatalf("Failed to register specific: %v", err)
		}

		wildcardReceiver := func(ctx context.Context, env cqrs.CommandEnvelope[any]) (any, error) {
			atomic.AddInt64(&wildcardCount, 1)
			return "wildcard", nil
		}
		if err := d.RegisterWildcardReceiver(wildcardReceiver); err != nil {
			t.Fatalf("Failed to register wildcard: %v", err)
		}

		w := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))
		results := w.Wait()

		if len(results) != 2 {
			t.Fatalf("Expected 2 results, got %d", len(results))
		}
		if atomic.LoadInt64(&specificCount) != 1 {
			t.Errorf("Expected specific receiver called once, got %d", specificCount)
		}
		if atomic.LoadInt64(&wildcardCount) != 1 {
			t.Errorf("Expected wildcard receiver called once, got %d", wildcardCount)
		}
	})
}

// =============================================================================
// REQ-CD-020: Dispatch accepts context and CommandEnvelope[any], returns Watchable
// =============================================================================

func TestDispatchSignature(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		ctx := context.Background()
		env := makeEnvelope("test.cmd", "payload")
		w := d.Dispatch(ctx, env)

		if w == nil {
			t.Fatal("Expected non-nil Watchable from Dispatch")
		}
		w.Wait()
	})
}

// =============================================================================
// REQ-CD-021: Pattern matching (exact, entity.*, *.action, *.*)
// =============================================================================

func TestCommandDispatcherPatternMatching(t *testing.T) {
	t.Parallel()

	patterns := []struct {
		name            string
		registerPattern string
		dispatchType    string
		shouldMatch     bool
	}{
		{"exact match", "user.create", "user.create", true},
		{"entity wildcard", "user.*", "user.create", true},
		{"action wildcard", "*.create", "user.create", true},
		{"full wildcard", "*.*", "user.create", true},
		{"no match entity", "order.*", "user.create", false},
		{"no match action", "*.delete", "user.create", false},
		{"no match exact", "user.delete", "user.create", false},
	}

	for _, tc := range patterns {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, regCU func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
				defer d.Close()

				var called int64
				receiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
					atomic.AddInt64(&called, 1)
					return "matched", nil
				}

				if err := regCU(tc.registerPattern, receiver); err != nil {
					t.Fatalf("Failed to register: %v", err)
				}

				w := d.Dispatch(context.Background(), makeEnvelope(tc.dispatchType, CommandDispatcherTestCreateUser{Email: "test@x.com"}))
				results := w.Wait()

				if tc.shouldMatch {
					if len(results) != 1 {
						t.Fatalf("Expected 1 result, got %d", len(results))
					}
					if atomic.LoadInt64(&called) != 1 {
						t.Errorf("Expected receiver called once, got %d", called)
					}
				} else {
					if len(results) != 0 {
						t.Fatalf("Expected 0 results, got %d", len(results))
					}
					if atomic.LoadInt64(&called) != 0 {
						t.Errorf("Expected receiver not called, got %d", called)
					}
				}
			})
		})
	}
}

func TestCommandDispatcherMultiplePatternsMatchSameCommand(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, regCU func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		var exactCount, entityCount, actionCount, allCount int64

		r1 := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			atomic.AddInt64(&exactCount, 1)
			return "exact", nil
		}
		r2 := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			atomic.AddInt64(&entityCount, 1)
			return "entity", nil
		}
		r3 := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			atomic.AddInt64(&actionCount, 1)
			return "action", nil
		}
		r4 := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			atomic.AddInt64(&allCount, 1)
			return "all", nil
		}

		if err := regCU("user.create", r1); err != nil {
			t.Fatalf("Failed to register exact: %v", err)
		}
		if err := regCU("user.*", r2); err != nil {
			t.Fatalf("Failed to register entity.*: %v", err)
		}
		if err := regCU("*.create", r3); err != nil {
			t.Fatalf("Failed to register *.create: %v", err)
		}
		if err := regCU("*.*", r4); err != nil {
			t.Fatalf("Failed to register *.*: %v", err)
		}

		w := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))
		results := w.Wait()

		if len(results) != 4 {
			t.Fatalf("Expected 4 results, got %d", len(results))
		}
		if atomic.LoadInt64(&exactCount) != 1 {
			t.Errorf("Expected exact receiver called once, got %d", exactCount)
		}
		if atomic.LoadInt64(&entityCount) != 1 {
			t.Errorf("Expected entity.* receiver called once, got %d", entityCount)
		}
		if atomic.LoadInt64(&actionCount) != 1 {
			t.Errorf("Expected *.create receiver called once, got %d", actionCount)
		}
		if atomic.LoadInt64(&allCount) != 1 {
			t.Errorf("Expected *.* receiver called once, got %d", allCount)
		}
	})
}

// =============================================================================
// REQ-CD-022: No receivers -> Watchable completes with zero results
// =============================================================================

func TestDispatchNoReceivers(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		w := d.Dispatch(context.Background(), makeEnvelope("nonexistent.command", "payload"))
		results := w.Wait()

		if len(results) != 0 {
			t.Errorf("Expected 0 results, got %d", len(results))
		}
	})
}

func TestDispatchNoReceiversChannelsClosed(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		w := d.Dispatch(context.Background(), makeEnvelope("nonexistent.command", "payload"))

		// Done channel should be closed
		select {
		case <-w.Done():
			// expected
		case <-time.After(time.Second):
			t.Fatal("Done channel not closed for zero receivers")
		}

		// Results channel should be closed
		_, ok := <-w.Results()
		if ok {
			t.Fatal("Expected Results channel to be closed")
		}
	})
}

// =============================================================================
// REQ-CD-030: Results() returns receive-only channel
// =============================================================================

func TestWatchableResults(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, regCU func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		receiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			return "result-value", nil
		}
		if err := regCU("user.create", receiver); err != nil {
			t.Fatalf("Failed to register: %v", err)
		}

		w := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))

		ch := w.Results()
		result, ok := <-ch
		if !ok {
			t.Fatal("Expected result from Results channel")
		}
		if result.Value != "result-value" {
			t.Errorf("Expected 'result-value', got %v", result.Value)
		}
	})
}

// =============================================================================
// REQ-CD-031: Results channel buffered to number of matched receivers
// =============================================================================

func TestWatchableResultsBufferSize(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, regCU func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		numReceivers := 3
		for i := 0; i < numReceivers; i++ {
			receiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
				return "ok", nil
			}
			if err := regCU("user.create", receiver); err != nil {
				t.Fatalf("Failed to register receiver %d: %v", i, err)
			}
		}

		w := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))

		// Wait for all to complete
		<-w.Done()

		// Since the buffer is sized to the number of receivers, all results
		// should be readable without blocking after Done
		for i := 0; i < numReceivers; i++ {
			select {
			case r, ok := <-w.Results():
				if !ok {
					t.Fatalf("Channel closed prematurely at result %d", i)
				}
				if r.Value != "ok" {
					t.Errorf("Expected 'ok', got %v", r.Value)
				}
			default:
				t.Fatalf("Expected result %d to be available without blocking", i)
			}
		}
	})
}

// =============================================================================
// REQ-CD-032: Results channel closed after all receivers complete
// =============================================================================

func TestWatchableResultsChannelClosed(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, regCU func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		receiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			return "done", nil
		}
		if err := regCU("user.create", receiver); err != nil {
			t.Fatalf("Failed to register: %v", err)
		}

		w := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))

		results := w.Wait()
		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(results))
		}

		// Channel should now be closed
		_, ok := <-w.Results()
		if ok {
			t.Error("Expected Results channel to be closed after Wait()")
		}
	})
}

// =============================================================================
// REQ-CD-033: Wait() blocks until all complete, returns all results
// =============================================================================

func TestWatchableWait(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, regCU func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		numReceivers := 3
		for i := 0; i < numReceivers; i++ {
			idx := i
			receiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
				return fmt.Sprintf("result-%d", idx), nil
			}
			if err := regCU("user.create", receiver); err != nil {
				t.Fatalf("Failed to register receiver %d: %v", i, err)
			}
		}

		w := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))
		results := w.Wait()

		if len(results) != numReceivers {
			t.Fatalf("Expected %d results, got %d", numReceivers, len(results))
		}

		for _, r := range results {
			if r.Error != nil {
				t.Errorf("Unexpected error: %v", r.Error)
			}
			if r.Value == nil {
				t.Error("Expected non-nil value")
			}
			if r.CommandType != "user.create" {
				t.Errorf("Expected CommandType 'user.create', got %q", r.CommandType)
			}
		}
	})
}

func TestWatchableWaitBlocksUntilSlowReceiver(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, regCU func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		receiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			time.Sleep(50 * time.Millisecond)
			return "slow-result", nil
		}
		if err := regCU("user.create", receiver); err != nil {
			t.Fatalf("Failed to register: %v", err)
		}

		start := time.Now()
		w := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))
		results := w.Wait()
		elapsed := time.Since(start)

		if elapsed < 50*time.Millisecond {
			t.Errorf("Wait returned too quickly: %v", elapsed)
		}
		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(results))
		}
		if results[0].Value != "slow-result" {
			t.Errorf("Expected 'slow-result', got %v", results[0].Value)
		}
	})
}

// =============================================================================
// REQ-CD-034: First() blocks until first result, remaining continue
// =============================================================================

func TestWatchableFirst(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, regCU func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		receiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			return "first-result", nil
		}
		if err := regCU("user.create", receiver); err != nil {
			t.Fatalf("Failed to register: %v", err)
		}

		w := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))
		result, ok := w.First()

		if !ok {
			t.Fatal("Expected First() to return true")
		}
		if result.Value != "first-result" {
			t.Errorf("Expected 'first-result', got %v", result.Value)
		}
	})
}

func TestWatchableFirstMultipleReceivers(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, regCU func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		for i := 0; i < 3; i++ {
			idx := i
			receiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
				return fmt.Sprintf("r%d", idx), nil
			}
			if err := regCU("user.create", receiver); err != nil {
				t.Fatalf("Failed to register: %v", err)
			}
		}

		w := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))
		result, ok := w.First()

		if !ok {
			t.Fatal("Expected First() to return true")
		}
		if result.Value == nil {
			t.Error("Expected non-nil value from First()")
		}

		// Remaining results should still complete
		<-w.Done()
	})
}

// =============================================================================
// REQ-CD-035: First() with no receivers returns zero-value and false
// =============================================================================

func TestWatchableFirstNoReceivers(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		w := d.Dispatch(context.Background(), makeEnvelope("nonexistent.cmd", "p"))
		result, ok := w.First()

		if ok {
			t.Fatal("Expected First() to return false for no receivers")
		}
		if result.Value != nil {
			t.Errorf("Expected nil Value, got %v", result.Value)
		}
		if result.Error != nil {
			t.Errorf("Expected nil Error, got %v", result.Error)
		}
		if result.CommandType != "" {
			t.Errorf("Expected empty CommandType, got %q", result.CommandType)
		}
	})
}

// =============================================================================
// REQ-CD-036: Done() returns channel closed when all complete
// =============================================================================

func TestWatchableDone(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, regCU func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		receiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			time.Sleep(30 * time.Millisecond)
			return "done", nil
		}
		if err := regCU("user.create", receiver); err != nil {
			t.Fatalf("Failed to register: %v", err)
		}

		w := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))

		select {
		case <-w.Done():
			// expected
		case <-time.After(2 * time.Second):
			t.Fatal("Done channel not closed within timeout")
		}
	})
}

func TestWatchableDoneClosedImmediatelyForNoReceivers(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		w := d.Dispatch(context.Background(), makeEnvelope("nope", "p"))

		select {
		case <-w.Done():
			// expected
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Done channel should be closed immediately for no receivers")
		}
	})
}

func TestWatchableDoneUsableInSelect(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, regCU func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		receiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			return "ok", nil
		}
		if err := regCU("user.create", receiver); err != nil {
			t.Fatalf("Failed to register: %v", err)
		}

		w := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))

		timeout := time.After(2 * time.Second)
		select {
		case <-w.Done():
			results := make([]cqrs.CommandResult, 0)
			for r := range w.Results() {
				results = append(results, r)
			}
			if len(results) != 1 {
				t.Errorf("Expected 1 result after Done, got %d", len(results))
			}
		case <-timeout:
			t.Fatal("Timed out waiting for Done")
		}
	})
}

// =============================================================================
// REQ-CD-037: Context cancellation propagated to receivers
// =============================================================================

func TestContextCancellationPropagated(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, regCU func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		ctx, cancel := context.WithCancel(context.Background())

		var receivedCancelled int64
		receiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			cancel()
			<-ctx.Done()
			atomic.AddInt64(&receivedCancelled, 1)
			return nil, ctx.Err()
		}
		if err := regCU("user.create", receiver); err != nil {
			t.Fatalf("Failed to register: %v", err)
		}

		w := d.Dispatch(ctx, makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))
		results := w.Wait()

		if len(results) < 1 {
			t.Fatal("Expected at least 1 result")
		}
		if atomic.LoadInt64(&receivedCancelled) != 1 {
			t.Error("Expected receiver to see cancelled context")
		}
	})
}

func TestContextValuesPropagated(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, regCU func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		type ctxKey string
		var receivedValue any
		receiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			receivedValue = ctx.Value(ctxKey("test-key"))
			return nil, nil
		}
		if err := regCU("user.create", receiver); err != nil {
			t.Fatalf("Failed to register: %v", err)
		}

		ctx := context.WithValue(context.Background(), ctxKey("test-key"), "test-value")
		w := d.Dispatch(ctx, makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))
		w.Wait()

		if receivedValue != "test-value" {
			t.Errorf("Expected context value 'test-value', got %v", receivedValue)
		}
	})
}

// =============================================================================
// REQ-CD-040: Concurrent execution (AsyncCommandDispatcher)
// =============================================================================

func TestAsyncConcurrentExecution(t *testing.T) {
	t.Parallel()

	t.Run("receivers execute concurrently", func(t *testing.T) {
		t.Parallel()
		d := cqrs.NewAsyncCommandDispatcher()
		defer d.Close()

		startBarrier := make(chan struct{})
		var running int64

		for i := 0; i < 3; i++ {
			receiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
				atomic.AddInt64(&running, 1)
				<-startBarrier
				return "ok", nil
			}
			if err := cqrs.RegisterReceiver(d, "user.create", receiver); err != nil {
				t.Fatalf("Failed to register: %v", err)
			}
		}

		w := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))

		// Wait for all 3 to be running concurrently
		deadline := time.After(2 * time.Second)
		for {
			if atomic.LoadInt64(&running) == 3 {
				break
			}
			select {
			case <-deadline:
				t.Fatalf("Timed out waiting for concurrent execution; only %d running", atomic.LoadInt64(&running))
			default:
				time.Sleep(time.Millisecond)
			}
		}

		close(startBarrier)
		results := w.Wait()

		if len(results) != 3 {
			t.Fatalf("Expected 3 results, got %d", len(results))
		}
	})
}

// =============================================================================
// REQ-CD-041: Results sent immediately as each receiver completes
// =============================================================================

func TestAsyncResultsSentImmediately(t *testing.T) {
	t.Parallel()

	t.Run("results available before all receivers complete", func(t *testing.T) {
		t.Parallel()
		d := cqrs.NewAsyncCommandDispatcher()
		defer d.Close()

		slowGate := make(chan struct{})

		fastReceiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			return "fast", nil
		}
		slowReceiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			<-slowGate
			return "slow", nil
		}

		if err := cqrs.RegisterReceiver(d, "user.create", fastReceiver); err != nil {
			t.Fatalf("Failed to register fast: %v", err)
		}
		if err := cqrs.RegisterReceiver(d, "user.create", slowReceiver); err != nil {
			t.Fatalf("Failed to register slow: %v", err)
		}

		w := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))

		// Should get at least one result before slow completes
		select {
		case <-w.Results():
			// Got a result while slow is still blocked
		case <-time.After(2 * time.Second):
			t.Fatal("Expected at least one result before timeout")
		}

		close(slowGate)
		<-w.Done()
	})
}

// =============================================================================
// REQ-CD-042: All receivers complete regardless of individual errors
// =============================================================================

func TestAsyncAllReceiversCompleteOnError(t *testing.T) {
	t.Parallel()

	t.Run("erroring receiver does not stop others", func(t *testing.T) {
		t.Parallel()
		d := cqrs.NewAsyncCommandDispatcher()
		defer d.Close()

		var count int64

		errorReceiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			atomic.AddInt64(&count, 1)
			return nil, errors.New("receiver error")
		}
		okReceiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			atomic.AddInt64(&count, 1)
			return "ok", nil
		}

		if err := cqrs.RegisterReceiver(d, "user.create", errorReceiver); err != nil {
			t.Fatalf("Failed to register error receiver: %v", err)
		}
		if err := cqrs.RegisterReceiver(d, "user.create", okReceiver); err != nil {
			t.Fatalf("Failed to register ok receiver: %v", err)
		}

		w := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))
		results := w.Wait()

		if len(results) != 2 {
			t.Fatalf("Expected 2 results, got %d", len(results))
		}
		if atomic.LoadInt64(&count) != 2 {
			t.Errorf("Expected both receivers called, got %d", count)
		}

		var errorCount, okCount int
		for _, r := range results {
			if r.Error != nil {
				errorCount++
			} else {
				okCount++
			}
		}
		if errorCount != 1 {
			t.Errorf("Expected 1 error result, got %d", errorCount)
		}
		if okCount != 1 {
			t.Errorf("Expected 1 ok result, got %d", okCount)
		}
	})
}

// =============================================================================
// REQ-CD-050: Sequential execution in registration order (QueuedCommandDispatcher)
// =============================================================================

func TestQueuedSequentialExecution(t *testing.T) {
	t.Parallel()

	t.Run("receivers execute in registration order", func(t *testing.T) {
		t.Parallel()
		d := cqrs.NewQueuedCommandDispatcher()
		defer d.Close()

		var mu sync.Mutex
		order := make([]int, 0, 3)

		for i := 0; i < 3; i++ {
			idx := i
			receiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
				mu.Lock()
				order = append(order, idx)
				mu.Unlock()
				return fmt.Sprintf("r%d", idx), nil
			}
			if err := cqrs.RegisterReceiver(d, "user.create", receiver); err != nil {
				t.Fatalf("Failed to register receiver %d: %v", i, err)
			}
		}

		w := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))
		results := w.Wait()

		if len(results) != 3 {
			t.Fatalf("Expected 3 results, got %d", len(results))
		}

		mu.Lock()
		defer mu.Unlock()
		for i, v := range order {
			if v != i {
				t.Errorf("Expected order[%d] = %d, got %d; full order: %v", i, i, v, order)
			}
		}
	})
}

// =============================================================================
// REQ-CD-051: Result sent before invoking next receiver
// =============================================================================

func TestQueuedResultSentBeforeNext(t *testing.T) {
	t.Parallel()

	t.Run("result available before next receiver executes", func(t *testing.T) {
		t.Parallel()
		d := cqrs.NewQueuedCommandDispatcher()
		defer d.Close()

		secondStarted := make(chan struct{})

		receiver1 := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			return "first", nil
		}
		receiver2 := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			close(secondStarted)
			return "second", nil
		}

		if err := cqrs.RegisterReceiver(d, "user.create", receiver1); err != nil {
			t.Fatalf("Failed to register receiver1: %v", err)
		}
		if err := cqrs.RegisterReceiver(d, "user.create", receiver2); err != nil {
			t.Fatalf("Failed to register receiver2: %v", err)
		}

		w := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))

		// Read first result
		select {
		case r := <-w.Results():
			if r.Value != "first" {
				t.Errorf("Expected first result 'first', got %v", r.Value)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Timed out waiting for first result")
		}

		// Second should eventually start and complete
		select {
		case <-secondStarted:
			// expected
		case <-time.After(2 * time.Second):
			t.Fatal("Second receiver never started")
		}

		// Read second result
		select {
		case r := <-w.Results():
			if r.Value != "second" {
				t.Errorf("Expected second result 'second', got %v", r.Value)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Timed out waiting for second result")
		}

		<-w.Done()
	})
}

// =============================================================================
// REQ-CD-052: Error in one receiver doesn't stop others
// =============================================================================

func TestQueuedErrorDoesNotStopOthers(t *testing.T) {
	t.Parallel()

	t.Run("error in first receiver does not prevent second", func(t *testing.T) {
		t.Parallel()
		d := cqrs.NewQueuedCommandDispatcher()
		defer d.Close()

		var secondCalled int64

		errorReceiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			return nil, errors.New("first receiver error")
		}
		okReceiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			atomic.AddInt64(&secondCalled, 1)
			return "ok", nil
		}

		if err := cqrs.RegisterReceiver(d, "user.create", errorReceiver); err != nil {
			t.Fatalf("Failed to register error receiver: %v", err)
		}
		if err := cqrs.RegisterReceiver(d, "user.create", okReceiver); err != nil {
			t.Fatalf("Failed to register ok receiver: %v", err)
		}

		w := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))
		results := w.Wait()

		if len(results) != 2 {
			t.Fatalf("Expected 2 results, got %d", len(results))
		}
		if atomic.LoadInt64(&secondCalled) != 1 {
			t.Error("Expected second receiver to be called despite first error")
		}

		// Verify ordering: first has error, second has value
		if results[0].Error == nil {
			t.Error("Expected first result to have error")
		}
		if results[1].Error != nil {
			t.Errorf("Expected second result to have no error, got %v", results[1].Error)
		}
		if results[1].Value != "ok" {
			t.Errorf("Expected second result value 'ok', got %v", results[1].Value)
		}
	})
}

// =============================================================================
// REQ-CD-053: Context cancellation stops subsequent receivers (Queued only)
// =============================================================================

func TestQueuedContextCancellationStopsSubsequent(t *testing.T) {
	t.Parallel()

	t.Run("context cancellation stops subsequent receivers", func(t *testing.T) {
		t.Parallel()
		d := cqrs.NewQueuedCommandDispatcher()
		defer d.Close()

		ctx, cancel := context.WithCancel(context.Background())

		var secondCalled int64

		firstReceiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			cancel()
			return "first", nil
		}
		secondReceiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			atomic.AddInt64(&secondCalled, 1)
			return "second", nil
		}

		if err := cqrs.RegisterReceiver(d, "user.create", firstReceiver); err != nil {
			t.Fatalf("Failed to register first: %v", err)
		}
		if err := cqrs.RegisterReceiver(d, "user.create", secondReceiver); err != nil {
			t.Fatalf("Failed to register second: %v", err)
		}

		w := d.Dispatch(ctx, makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))
		results := w.Wait()

		// First receiver completes, second is skipped due to context cancellation
		if len(results) != 1 {
			t.Fatalf("Expected 1 result (second skipped due to cancellation), got %d", len(results))
		}
		if results[0].Value != "first" {
			t.Errorf("Expected first result 'first', got %v", results[0].Value)
		}
		if atomic.LoadInt64(&secondCalled) != 0 {
			t.Error("Expected second receiver not to be called after cancellation")
		}
	})
}

// =============================================================================
// REQ-CD-060: Receiver error -> Value nil in CommandResult
// =============================================================================

func TestReceiverErrorValueNil(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, regCU func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		receiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			return nil, errors.New("something went wrong")
		}
		if err := regCU("user.create", receiver); err != nil {
			t.Fatalf("Failed to register: %v", err)
		}

		w := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))
		results := w.Wait()

		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(results))
		}
		if results[0].Error == nil {
			t.Fatal("Expected error in result")
		}
		if results[0].Value != nil {
			t.Errorf("Expected nil Value when error, got %v", results[0].Value)
		}
		if results[0].CommandType != "user.create" {
			t.Errorf("Expected CommandType 'user.create', got %q", results[0].CommandType)
		}
	})
}

// =============================================================================
// REQ-CD-061: Receiver panic -> recovered, wrapped as error in CommandResult
// =============================================================================

func TestReceiverPanicRecovery(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, regCU func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		receiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			panic("something terrible happened")
		}
		if err := regCU("user.create", receiver); err != nil {
			t.Fatalf("Failed to register: %v", err)
		}

		w := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))
		results := w.Wait()

		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(results))
		}
		if results[0].Error == nil {
			t.Fatal("Expected error from panic recovery")
		}
		if !strings.Contains(results[0].Error.Error(), "panicked") {
			t.Errorf("Expected panic error message, got %q", results[0].Error.Error())
		}
		if !strings.Contains(results[0].Error.Error(), "something terrible happened") {
			t.Errorf("Expected panic message in error, got %q", results[0].Error.Error())
		}
		if results[0].Value != nil {
			t.Errorf("Expected nil Value on panic, got %v", results[0].Value)
		}
	})
}

func TestReceiverPanicDoesNotPreventOthers(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, regCU func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		var okCalled int64

		panicReceiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			panic("boom")
		}
		okReceiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			atomic.AddInt64(&okCalled, 1)
			return "ok", nil
		}

		if err := regCU("user.create", panicReceiver); err != nil {
			t.Fatalf("Failed to register panic receiver: %v", err)
		}
		if err := regCU("user.create", okReceiver); err != nil {
			t.Fatalf("Failed to register ok receiver: %v", err)
		}

		w := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))
		results := w.Wait()

		if len(results) != 2 {
			t.Fatalf("Expected 2 results, got %d", len(results))
		}
		if atomic.LoadInt64(&okCalled) != 1 {
			t.Error("Expected ok receiver to be called despite panic in other")
		}
	})
}

// =============================================================================
// REQ-CD-062: Type assertion failure -> descriptive error in CommandResult
// =============================================================================

func TestTypeAssertionFailure(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, regCU func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		receiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			return env.Payload.Email, nil
		}
		if err := regCU("user.create", receiver); err != nil {
			t.Fatalf("Failed to register: %v", err)
		}

		// Dispatch with wrong payload type
		wrongPayload := CommandDispatcherTestPlaceOrder{OrderID: "order-123", CustomerID: "c-1", TotalAmount: 99.99}
		w := d.Dispatch(context.Background(), makeEnvelope("user.create", wrongPayload))
		results := w.Wait()

		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(results))
		}
		if results[0].Error == nil {
			t.Fatal("Expected type assertion error")
		}
		errMsg := results[0].Error.Error()
		if !strings.Contains(errMsg, "type assertion failed") {
			t.Errorf("Expected 'type assertion failed' in error, got %q", errMsg)
		}
		if results[0].Value != nil {
			t.Errorf("Expected nil Value on type assertion failure, got %v", results[0].Value)
		}
	})
}

// =============================================================================
// REQ-CD-070: Concurrent registration and dispatch safe
// =============================================================================

func TestConcurrentRegistrationAndDispatch(t *testing.T) {
	t.Parallel()

	t.Run("AsyncCommandDispatcher concurrent registration and dispatch", func(t *testing.T) {
		t.Parallel()
		d := cqrs.NewAsyncCommandDispatcher()
		defer d.Close()

		var wg sync.WaitGroup
		concurrency := 10

		wg.Add(concurrency)
		for i := 0; i < concurrency; i++ {
			go func(idx int) {
				defer wg.Done()
				receiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
					return fmt.Sprintf("r%d", idx), nil
				}
				if err := cqrs.RegisterReceiver(d, "user.create", receiver); err != nil {
					t.Errorf("Failed to register receiver %d: %v", idx, err)
				}
			}(i)
		}

		wg.Add(concurrency)
		for i := 0; i < concurrency; i++ {
			go func() {
				defer wg.Done()
				w := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))
				w.Wait()
			}()
		}

		wg.Wait()
	})

	t.Run("QueuedCommandDispatcher concurrent registration and dispatch", func(t *testing.T) {
		t.Parallel()
		d := cqrs.NewQueuedCommandDispatcher()
		defer d.Close()

		var wg sync.WaitGroup
		concurrency := 10

		wg.Add(concurrency)
		for i := 0; i < concurrency; i++ {
			go func(idx int) {
				defer wg.Done()
				receiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
					return fmt.Sprintf("r%d", idx), nil
				}
				if err := cqrs.RegisterReceiver(d, "user.create", receiver); err != nil {
					t.Errorf("Failed to register receiver %d: %v", idx, err)
				}
			}(i)
		}

		wg.Add(concurrency)
		for i := 0; i < concurrency; i++ {
			go func() {
				defer wg.Done()
				w := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))
				w.Wait()
			}()
		}

		wg.Wait()
	})
}

func TestConcurrentWildcardRegistrationAndDispatch(t *testing.T) {
	t.Parallel()

	t.Run("AsyncCommandDispatcher concurrent wildcard", func(t *testing.T) {
		t.Parallel()
		d := cqrs.NewAsyncCommandDispatcher()
		defer d.Close()

		var wg sync.WaitGroup
		concurrency := 5

		wg.Add(concurrency)
		for i := 0; i < concurrency; i++ {
			go func() {
				defer wg.Done()
				err := d.RegisterWildcardReceiver(func(ctx context.Context, env cqrs.CommandEnvelope[any]) (any, error) {
					return "wildcard", nil
				})
				if err != nil {
					t.Errorf("Failed to register wildcard: %v", err)
				}
			}()
		}

		wg.Add(concurrency)
		for i := 0; i < concurrency; i++ {
			go func() {
				defer wg.Done()
				w := d.Dispatch(context.Background(), makeEnvelope("any.cmd", "p"))
				w.Wait()
			}()
		}

		wg.Wait()
	})

	t.Run("QueuedCommandDispatcher concurrent wildcard", func(t *testing.T) {
		t.Parallel()
		d := cqrs.NewQueuedCommandDispatcher()
		defer d.Close()

		var wg sync.WaitGroup
		concurrency := 5

		wg.Add(concurrency)
		for i := 0; i < concurrency; i++ {
			go func() {
				defer wg.Done()
				err := d.RegisterWildcardReceiver(func(ctx context.Context, env cqrs.CommandEnvelope[any]) (any, error) {
					return "wildcard", nil
				})
				if err != nil {
					t.Errorf("Failed to register wildcard: %v", err)
				}
			}()
		}

		wg.Add(concurrency)
		for i := 0; i < concurrency; i++ {
			go func() {
				defer wg.Done()
				w := d.Dispatch(context.Background(), makeEnvelope("any.cmd", "p"))
				w.Wait()
			}()
		}

		wg.Wait()
	})
}

// =============================================================================
// REQ-CD-071: Lock released before invoking receivers
// =============================================================================

func TestLockReleasedBeforeReceiverInvocation(t *testing.T) {
	t.Parallel()

	t.Run("AsyncCommandDispatcher receiver can register without deadlock", func(t *testing.T) {
		t.Parallel()
		d := cqrs.NewAsyncCommandDispatcher()
		defer d.Close()

		done := make(chan struct{})

		receiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			newReceiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestUpdateUser]) (any, error) {
				return "nested", nil
			}
			err := cqrs.RegisterReceiver(d, "user.update", newReceiver)
			close(done)
			return "ok", err
		}
		if err := cqrs.RegisterReceiver(d, "user.create", receiver); err != nil {
			t.Fatalf("Failed to register: %v", err)
		}

		w := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))

		select {
		case <-done:
			// No deadlock
		case <-time.After(2 * time.Second):
			t.Fatal("Deadlock detected: lock not released before receiver invocation")
		}

		results := w.Wait()
		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(results))
		}
		if results[0].Error != nil {
			t.Errorf("Expected no error from nested registration, got %v", results[0].Error)
		}
	})

	t.Run("QueuedCommandDispatcher receiver can register without deadlock", func(t *testing.T) {
		t.Parallel()
		d := cqrs.NewQueuedCommandDispatcher()
		defer d.Close()

		done := make(chan struct{})

		receiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			newReceiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestUpdateUser]) (any, error) {
				return "nested", nil
			}
			err := cqrs.RegisterReceiver(d, "user.update", newReceiver)
			close(done)
			return "ok", err
		}
		if err := cqrs.RegisterReceiver(d, "user.create", receiver); err != nil {
			t.Fatalf("Failed to register: %v", err)
		}

		w := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))

		select {
		case <-done:
			// No deadlock
		case <-time.After(2 * time.Second):
			t.Fatal("Deadlock detected: lock not released before receiver invocation")
		}

		results := w.Wait()
		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(results))
		}
		if results[0].Error != nil {
			t.Errorf("Expected no error from nested registration, got %v", results[0].Error)
		}
	})
}

// =============================================================================
// REQ-CD-072: Watchable safe for concurrent reads
// =============================================================================

func TestWatchableConcurrentReads(t *testing.T) {
	t.Parallel()

	t.Run("AsyncCommandDispatcher concurrent reads", func(t *testing.T) {
		t.Parallel()
		d := cqrs.NewAsyncCommandDispatcher()
		defer d.Close()

		for i := 0; i < 5; i++ {
			idx := i
			receiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
				return fmt.Sprintf("r%d", idx), nil
			}
			if err := cqrs.RegisterReceiver(d, "user.create", receiver); err != nil {
				t.Fatalf("Failed to register: %v", err)
			}
		}

		w := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))

		var wg sync.WaitGroup

		// Reader 1: drain results
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range w.Results() {
			}
		}()

		// Reader 2: wait on Done
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-w.Done()
		}()

		wg.Wait()
	})

	t.Run("QueuedCommandDispatcher concurrent reads", func(t *testing.T) {
		t.Parallel()
		d := cqrs.NewQueuedCommandDispatcher()
		defer d.Close()

		for i := 0; i < 5; i++ {
			idx := i
			receiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
				return fmt.Sprintf("r%d", idx), nil
			}
			if err := cqrs.RegisterReceiver(d, "user.create", receiver); err != nil {
				t.Fatalf("Failed to register: %v", err)
			}
		}

		w := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))

		var wg sync.WaitGroup

		// Reader 1: drain results
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range w.Results() {
			}
		}()

		// Reader 2: wait on Done
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-w.Done()
		}()

		wg.Wait()
	})
}

// =============================================================================
// REQ-CD-080: Both dispatchers implement CommandDispatcher interface
// =============================================================================

func TestInterfaceCompliance(t *testing.T) {
	t.Parallel()

	t.Run("AsyncCommandDispatcher implements CommandDispatcher", func(t *testing.T) {
		t.Parallel()
		var d cqrs.CommandDispatcher = cqrs.NewAsyncCommandDispatcher()
		if d == nil {
			t.Fatal("Expected non-nil CommandDispatcher")
		}
		if err := d.Close(); err != nil {
			t.Errorf("Close returned error: %v", err)
		}
	})

	t.Run("QueuedCommandDispatcher implements CommandDispatcher", func(t *testing.T) {
		t.Parallel()
		var d cqrs.CommandDispatcher = cqrs.NewQueuedCommandDispatcher()
		if d == nil {
			t.Fatal("Expected non-nil CommandDispatcher")
		}
		if err := d.Close(); err != nil {
			t.Errorf("Close returned error: %v", err)
		}
	})

	t.Run("both dispatchers callable via CommandDispatcher interface", func(t *testing.T) {
		t.Parallel()

		dispatchers := []cqrs.CommandDispatcher{
			cqrs.NewAsyncCommandDispatcher(),
			cqrs.NewQueuedCommandDispatcher(),
		}

		for _, d := range dispatchers {
			// Dispatch and RegisterWildcardReceiver work via the interface
			err := d.RegisterWildcardReceiver(func(ctx context.Context, env cqrs.CommandEnvelope[any]) (any, error) {
				return "ok", nil
			})
			if err != nil {
				t.Fatalf("RegisterWildcardReceiver failed: %v", err)
			}

			w := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))
			results := w.Wait()
			if len(results) != 1 {
				t.Errorf("Expected 1 result, got %d", len(results))
			}
			d.Close()
		}
	})
}

// =============================================================================
// Additional edge case tests
// =============================================================================

func TestCommandDispatcherEdgeCases(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, regCU func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		t.Run("dispatch preserves command type in result", func(t *testing.T) {
			t.Parallel()

			// Create a fresh dispatcher for this subtest
			var dd cqrs.CommandDispatcher
			var reg func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error
			if name == "AsyncCommandDispatcher" {
				ad := cqrs.NewAsyncCommandDispatcher()
				dd = ad
				reg = func(ct string, r cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error {
					return cqrs.RegisterReceiver(ad, ct, r)
				}
			} else {
				qd := cqrs.NewQueuedCommandDispatcher()
				dd = qd
				reg = func(ct string, r cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error {
					return cqrs.RegisterReceiver(qd, ct, r)
				}
			}
			defer dd.Close()

			receiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
				return nil, nil
			}
			if err := reg("user.create", receiver); err != nil {
				t.Fatalf("Failed to register: %v", err)
			}

			w := dd.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "a@b.com"}))
			results := w.Wait()

			if len(results) != 1 {
				t.Fatalf("Expected 1 result, got %d", len(results))
			}
			if results[0].CommandType != "user.create" {
				t.Errorf("Expected CommandType 'user.create', got %q", results[0].CommandType)
			}
		})
	})
}

func TestCommandDispatcherMetadataPassed(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, regCU func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		var receivedMeta map[string]any
		receiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			receivedMeta = env.Metadata
			return nil, nil
		}
		if err := regCU("user.create", receiver); err != nil {
			t.Fatalf("Failed to register: %v", err)
		}

		env := cqrs.NewCommandEnvelope(
			CommandDispatcherTestCreateUser{Email: "a@b.com"},
			"user.create",
		)
		env.Metadata["correlation-id"] = "abc-123"
		anyEnv := cqrs.ToAnyCommandEnvelope(env)

		w := d.Dispatch(context.Background(), anyEnv)
		w.Wait()

		if receivedMeta == nil {
			t.Fatal("Expected metadata to be passed to receiver")
		}
		if receivedMeta["correlation-id"] != "abc-123" {
			t.Errorf("Expected correlation-id 'abc-123', got %v", receivedMeta["correlation-id"])
		}
	})
}

func TestCommandDispatcherCloseReturnsNil(t *testing.T) {
	t.Parallel()

	t.Run("AsyncCommandDispatcher Close returns nil", func(t *testing.T) {
		t.Parallel()
		d := cqrs.NewAsyncCommandDispatcher()
		if err := d.Close(); err != nil {
			t.Errorf("Expected nil from Close(), got %v", err)
		}
	})

	t.Run("QueuedCommandDispatcher Close returns nil", func(t *testing.T) {
		t.Parallel()
		d := cqrs.NewQueuedCommandDispatcher()
		if err := d.Close(); err != nil {
			t.Errorf("Expected nil from Close(), got %v", err)
		}
	})
}

func TestCommandDispatcherMultipleDispatchesIndependent(t *testing.T) {
	t.Parallel()

	runForBothDispatchers(t, func(t *testing.T, name string, d cqrs.CommandDispatcher, regCU func(string, cqrs.CommandReceiver[CommandDispatcherTestCreateUser]) error, _ func(string, cqrs.CommandReceiver[CommandDispatcherTestUpdateUser]) error) {
		defer d.Close()

		var count int64
		receiver := func(ctx context.Context, env cqrs.CommandEnvelope[CommandDispatcherTestCreateUser]) (any, error) {
			atomic.AddInt64(&count, 1)
			return env.Payload.Email, nil
		}
		if err := regCU("user.create", receiver); err != nil {
			t.Fatalf("Failed to register: %v", err)
		}

		w1 := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "first@x.com"}))
		w2 := d.Dispatch(context.Background(), makeEnvelope("user.create", CommandDispatcherTestCreateUser{Email: "second@x.com"}))

		r1 := w1.Wait()
		r2 := w2.Wait()

		if len(r1) != 1 || len(r2) != 1 {
			t.Fatalf("Expected 1 result each, got %d and %d", len(r1), len(r2))
		}
		if atomic.LoadInt64(&count) != 2 {
			t.Errorf("Expected receiver called twice, got %d", count)
		}
	})
}
