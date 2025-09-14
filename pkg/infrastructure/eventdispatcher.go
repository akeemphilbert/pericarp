package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/pubsub/gochannel"
	"github.com/akeemphilbert/pericarp/pkg/domain"
)

// WatermillEventDispatcher implements the EventDispatcher interface using Watermill
type WatermillEventDispatcher struct {
	pubSub     *gochannel.GoChannel
	logger     watermill.LoggerAdapter
	handlers   map[string][]domain.EventHandler
	handlersMu sync.RWMutex
	router     *message.Router
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewWatermillEventDispatcher creates a new Watermill-based event dispatcher
func NewWatermillEventDispatcher(logger watermill.LoggerAdapter) (*WatermillEventDispatcher, error) {
	if logger == nil {
		logger = watermill.NopLogger{}
	}

	pubSub := gochannel.NewGoChannel(
		gochannel.Config{
			OutputChannelBuffer: 64,
			Persistent:          false,
		},
		logger,
	)

	ctx, cancel := context.WithCancel(context.Background())

	router, err := message.NewRouter(message.RouterConfig{}, logger)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create message router: %w", err)
	}

	dispatcher := &WatermillEventDispatcher{
		pubSub:   pubSub,
		logger:   logger,
		handlers: make(map[string][]domain.EventHandler),
		router:   router,
		ctx:      ctx,
		cancel:   cancel,
	}

	// Start the router in a goroutine
	go func() {
		if err := router.Run(ctx); err != nil {
			logger.Error("Router stopped with error", err, nil)
		}
	}()

	return dispatcher, nil
}

// Dispatch sends envelopes to registered event handlers
func (d *WatermillEventDispatcher) Dispatch(ctx context.Context, envelopes []domain.Envelope) error {
	for _, envelope := range envelopes {
		if err := d.dispatchSingle(ctx, envelope); err != nil {
			return fmt.Errorf("failed to dispatch event %s: %w", envelope.EventID(), err)
		}
	}
	return nil
}

// dispatchSingle dispatches a single envelope to all registered handlers using wildcard topics
func (d *WatermillEventDispatcher) dispatchSingle(ctx context.Context, envelope domain.Envelope) error {
	event := envelope.Event()
	eventType := event.EventType()

	d.handlersMu.RLock()
	handlers := d.handlers[eventType]
	d.handlersMu.RUnlock()

	// If no handlers are registered for this event type, just log and continue
	if len(handlers) == 0 {
		d.logger.Debug("No handlers registered for event type", watermill.LogFields{
			"event_type": eventType,
			"event_id":   envelope.EventID(),
		})
		return nil
	}

	// Generate the 4 wildcard topic patterns
	wildcardTopics := d.generateWildcardTopics(eventType)

	// Create Watermill message with serialized Event object
	eventJSON, err := json.Marshal(envelope.Event())
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}
	msg := message.NewMessage(envelope.EventID(), eventJSON)

	msg.Metadata.Set("event_type", eventType)
	msg.Metadata.Set("aggregate_id", event.AggregateID())
	msg.Metadata.Set("sequence_no", fmt.Sprintf("%d", event.SequenceNo()))
	msg.Metadata.Set("occurred_at", event.CreatedAt().Format(time.RFC3339))
	msg.Metadata.Set("timestamp", envelope.Timestamp().Format(time.RFC3339))
	msg.Metadata.Set("metadata", fmt.Sprintf("%v", envelope.Metadata()))
	msg.Metadata.Set("event_data", fmt.Sprintf("%v", event))

	// Publish to all 4 wildcard topic patterns - the router will route these to handlers
	for _, topic := range wildcardTopics {
		if err := d.pubSub.Publish(topic, msg); err != nil {
			return fmt.Errorf("failed to publish message to wildcard topic %s: %w", topic, err)
		}
	}

	// Also publish to each handler's specific topic to ensure they receive the event
	for i := range handlers {
		handlerTopic := fmt.Sprintf("%s_handler_%d", eventType, i+1)
		if err := d.pubSub.Publish(handlerTopic, msg); err != nil {
			return fmt.Errorf("failed to publish message to handler topic %s: %w", handlerTopic, err)
		}
	}

	d.logger.Debug("Event dispatched to wildcard and handler topics", watermill.LogFields{
		"event_id":        envelope.EventID(),
		"event_type":      eventType,
		"aggregate_id":    event.AggregateID(),
		"wildcard_topics": wildcardTopics,
		"handler_count":   len(handlers),
	})

	return nil
}

// generateWildcardTopics generates the 4 wildcard topic patterns for an event type
// Event type format: "entityType.eventType" (e.g., "user.created")
// Returns: ["entityType.*", "*.eventType", "*.*", "entityType.eventType"]
func (d *WatermillEventDispatcher) generateWildcardTopics(eventType string) []string {
	// Split the event type into entity type and event type
	parts := strings.Split(eventType, ".")
	if len(parts) != 2 {
		// If the event type doesn't follow the expected format, return just the original
		return []string{eventType}
	}

	entityType := parts[0]
	eventTypeOnly := parts[1]

	return []string{
		fmt.Sprintf("%s.*", entityType),    // entityType.*
		fmt.Sprintf("*.%s", eventTypeOnly), // *.eventType
		"*.*",                              // *.*
		eventType,                          // entityType.eventType
	}
}

// Subscribe registers an event handler for specific event types
func (d *WatermillEventDispatcher) Subscribe(eventType string, handler domain.EventHandler) error {
	d.handlersMu.Lock()
	defer d.handlersMu.Unlock()

	// Add handler to the registry
	d.handlers[eventType] = append(d.handlers[eventType], handler)

	// Create a unique handler name for this subscription
	handlerIndex := len(d.handlers[eventType])
	handlerName := fmt.Sprintf("%s_handler_%d", eventType, handlerIndex)

	// Subscribe to the specific event type topic
	// Each handler gets its own unique topic to avoid conflicts
	handlerTopic := fmt.Sprintf("%s_handler_%d", eventType, handlerIndex)
	d.router.AddNoPublisherHandler(
		handlerName,
		handlerTopic,
		d.pubSub,
		func(msg *message.Message) error {
			return d.handleMessage(msg, handler)
		},
	)

	d.logger.Info("Event handler subscribed", watermill.LogFields{
		"event_type":   eventType,
		"handler_name": handlerName,
		"topic":        handlerTopic,
	})

	return nil
}

// handleMessage processes a Watermill message using the domain event handler
func (d *WatermillEventDispatcher) handleMessage(msg *message.Message, handler domain.EventHandler) error {
	// Reconstruct the envelope from message metadata and payload
	envelope, err := d.reconstructEnvelopeFromMessage(msg)
	if err != nil {
		return fmt.Errorf("failed to reconstruct envelope: %w", err)
	}

	// Handle the event
	if err := handler.Handle(context.Background(), envelope); err != nil {
		d.logger.Error("Event handler failed", err, watermill.LogFields{
			"event_id":   envelope.EventID(),
			"event_type": envelope.Event().EventType(),
			"handler":    fmt.Sprintf("%T", handler),
		})
		return fmt.Errorf("event handler failed: %w", err)
	}

	d.logger.Debug("Event handled successfully", watermill.LogFields{
		"event_id":   envelope.EventID(),
		"event_type": envelope.Event().EventType(),
		"handler":    fmt.Sprintf("%T", handler),
	})

	return nil
}

// reconstructEnvelopeFromMessage reconstructs a domain.Envelope from watermill message metadata and payload
func (d *WatermillEventDispatcher) reconstructEnvelopeFromMessage(msg *message.Message) (domain.Envelope, error) {
	// Extract data from message metadata
	sequenceNoStr := msg.Metadata.Get("sequence_no")
	timestampStr := msg.Metadata.Get("timestamp")
	metadataStr := msg.Metadata.Get("metadata")

	// Parse sequence number
	var seqNo int64
	if _, err := fmt.Sscanf(sequenceNoStr, "%d", &seqNo); err != nil {
		return nil, fmt.Errorf("invalid sequence_no: %s", sequenceNoStr)
	}

	timestamp, err := time.Parse(time.RFC3339, timestampStr)
	if err != nil {
		return nil, fmt.Errorf("invalid timestamp: %w", err)
	}

	// Parse metadata
	var metadata map[string]interface{}
	if metadataStr != "" {
		if err := json.Unmarshal([]byte(metadataStr), &metadata); err != nil {
			metadata = make(map[string]interface{})
		}
	} else {
		metadata = make(map[string]interface{})
	}

	// Deserialize the complete event from the message payload
	var event domain.EntityEvent
	if err := json.Unmarshal(msg.Payload, &event); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event from payload: %w", err)
	}

	// Create envelope
	envelope := &eventEnvelope{
		event:     &event,
		metadata:  metadata,
		eventID:   msg.UUID,
		timestamp: timestamp,
	}

	return envelope, nil
}

// reconstructEnvelope reconstructs a domain.Envelope from serialized data
func (d *WatermillEventDispatcher) reconstructEnvelope(data map[string]interface{}) (domain.Envelope, error) {
	eventID, ok := data["event_id"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid event_id")
	}

	eventType, ok := data["event_type"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid event_type")
	}

	aggregateID, ok := data["aggregate_id"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid aggregate_id")
	}

	sequenceNo, ok := data["sequence_no"].(float64) // JSON numbers are float64
	if !ok {
		return nil, fmt.Errorf("missing or invalid sequence_no")
	}

	// Extract metadata
	metadata, ok := data["metadata"].(map[string]interface{})
	if !ok {
		metadata = make(map[string]interface{})
	}

	// Parse entity type and event type from the combined event type
	// EntityEvent combines them as "EntityType.EventType"
	entityType := "Generic"
	eventTypeOnly := eventType
	if dotIndex := strings.LastIndex(eventType, "."); dotIndex != -1 {
		entityType = eventType[:dotIndex]
		eventTypeOnly = eventType[dotIndex+1:]
	}

	// Create a generic event (in a real implementation, you'd use an event registry)
	rawData, err := json.Marshal(data["event_data"])
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event_data: %w", err)
	}
	event := &domain.EntityEvent{
		EntityType:  entityType,
		Type:        eventTypeOnly,
		AggregateId: aggregateID,
		SequenceNum: int64(sequenceNo),
		CreatedTime: time.Now(), // We don't have the original timestamp
		UserId:      "",
		AccountId:   "",
		PayloadData: rawData, // Preserve the original event data
	}

	// Create envelope
	envelope := &eventEnvelope{
		event:    event,
		metadata: metadata,
		eventID:  eventID,
		// timestamp would be reconstructed from data["timestamp"]
	}

	return envelope, nil
}

// Close shuts down the event dispatcher
func (d *WatermillEventDispatcher) Close() error {
	d.cancel()
	return d.router.Close()
}

// GetHandlers returns the registered handlers for testing purposes
func (d *WatermillEventDispatcher) GetHandlers(eventType string) []domain.EventHandler {
	d.handlersMu.RLock()
	defer d.handlersMu.RUnlock()

	handlers := make([]domain.EventHandler, len(d.handlers[eventType]))
	copy(handlers, d.handlers[eventType])
	return handlers
}
