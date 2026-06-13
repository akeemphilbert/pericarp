package subscriptions

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

// defaultPostgresNotifyChannel mirrors infrastructure.PostgresNotifyChannel,
// the channel the Postgres event store NOTIFYs on commit. Duplicated by value
// so importing this package does not drag the infrastructure package (and its
// store dependencies) into consumer builds.
const defaultPostgresNotifyChannel = "pericarp_events"

// DefaultListenerReconnectDelay is how long PostgresListener waits before
// re-establishing a failed LISTEN connection.
const DefaultListenerReconnectDelay = 5 * time.Second

// PostgresListener holds a dedicated Postgres connection LISTENing for the
// event store's commit notifications and turns them into subscriber wake
// signals (WithWakeSignal). Notifications are never load-bearing: while the
// connection is down — or if a NOTIFY is missed entirely — subscribers still
// make progress through their poll interval.
type PostgresListener struct {
	dsn            string
	channel        string
	logger         *slog.Logger
	reconnectDelay time.Duration
	wake           chan struct{}
}

// PostgresListenerOption configures a PostgresListener.
type PostgresListenerOption func(*PostgresListener)

// WithListenerChannel overrides the NOTIFY channel (default
// infrastructure.PostgresNotifyChannel, "pericarp_events").
func WithListenerChannel(channel string) PostgresListenerOption {
	return func(l *PostgresListener) { l.channel = channel }
}

// WithListenerLogger sets the logger for connection failures (default
// slog.Default()).
func WithListenerLogger(logger *slog.Logger) PostgresListenerOption {
	return func(l *PostgresListener) { l.logger = logger }
}

// WithListenerReconnectDelay sets the wait between reconnect attempts
// (default DefaultListenerReconnectDelay).
func WithListenerReconnectDelay(d time.Duration) PostgresListenerOption {
	return func(l *PostgresListener) { l.reconnectDelay = d }
}

// NewPostgresListener creates a listener for the given Postgres DSN. Call Run
// to start it and pass Wake() to subscribers via WithWakeSignal.
func NewPostgresListener(dsn string, opts ...PostgresListenerOption) (*PostgresListener, error) {
	if dsn == "" {
		return nil, errors.New("postgres listener DSN must not be empty")
	}
	l := &PostgresListener{
		dsn:            dsn,
		channel:        defaultPostgresNotifyChannel,
		logger:         slog.Default(),
		reconnectDelay: DefaultListenerReconnectDelay,
		wake:           make(chan struct{}, 1),
	}
	for _, opt := range opts {
		opt(l)
	}
	if l.channel == "" {
		return nil, errors.New("postgres listener channel must not be empty")
	}
	if l.reconnectDelay <= 0 {
		return nil, fmt.Errorf("reconnect delay must be positive, got %v", l.reconnectDelay)
	}
	return l, nil
}

// Wake returns the channel that receives after each notification. Pass it to
// subscribers via WithWakeSignal; it is buffered, and sends never block.
func (l *PostgresListener) Wake() <-chan struct{} {
	return l.wake
}

// Run maintains the LISTEN connection until ctx is cancelled, reconnecting
// with a delay on any failure. It returns nil on cancellation; connection
// errors are logged, never fatal — polling subscribers keep making progress
// while the listener is down.
func (l *PostgresListener) Run(ctx context.Context) error {
	for {
		if err := l.listen(ctx); err != nil && ctx.Err() == nil {
			l.logger.Error("postgres listener connection failed; reconnecting",
				"channel", l.channel, "delay", l.reconnectDelay, "error", err)
		}
		if ctx.Err() != nil {
			return nil
		}

		timer := time.NewTimer(l.reconnectDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

// listen connects, LISTENs, and forwards notifications until the connection
// or ctx dies.
func (l *PostgresListener) listen(ctx context.Context) error {
	conn, err := pgx.Connect(ctx, l.dsn)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = conn.Close(closeCtx)
	}()

	if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{l.channel}.Sanitize()); err != nil {
		return fmt.Errorf("failed to LISTEN on %q: %w", l.channel, err)
	}

	for {
		if _, err := conn.WaitForNotification(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("failed waiting for notification: %w", err)
		}
		select {
		case l.wake <- struct{}{}:
		default: // a wake is already pending; one is enough
		}
	}
}
