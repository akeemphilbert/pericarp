package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// FileStore is a file-based implementation of EventStore.
// It stores events in JSON files, one file per aggregate.
// This is suitable for local development but not recommended for production.
type FileStore struct {
	baseDir string
	mu      sync.RWMutex
	// Cache for in-memory access
	cache map[string][]domain.EventEnvelope[any]
}

// NewFileStore creates a new file-based event store.
// The baseDir is the directory where event files will be stored.
func NewFileStore(baseDir string) (*FileStore, error) {
	if baseDir == "" {
		return nil, errors.New("base directory cannot be empty")
	}

	// Create base directory if it doesn't exist
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}

	store := &FileStore{
		baseDir: baseDir,
		cache:   make(map[string][]domain.EventEnvelope[any]),
	}

	// Load existing events from disk
	if err := store.loadAllFromDisk(); err != nil {
		return nil, fmt.Errorf("failed to load existing events: %w", err)
	}

	return store, nil
}

// getFilePath returns the file path for an aggregate's events.
func (f *FileStore) getFilePath(aggregateID string) string {
	// Sanitize aggregateID to be filesystem-safe
	safeID := filepath.Base(aggregateID)
	return filepath.Join(f.baseDir, safeID+".json")
}

// loadAllFromDisk loads all events from disk into the cache.
func (f *FileStore) loadAllFromDisk() error {
	entries, err := os.ReadDir(f.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Directory doesn't exist yet, that's okay
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		// Extract aggregate ID from filename (remove .json extension)
		aggregateID := entry.Name()[:len(entry.Name())-5]
		filePath := filepath.Join(f.baseDir, entry.Name())

		events, err := f.loadFromFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to load events from %s: %w", filePath, err)
		}

		f.cache[aggregateID] = events
	}

	return nil
}

// loadFromFile loads events from a single file.
func (f *FileStore) loadFromFile(filePath string) ([]domain.EventEnvelope[any], error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []domain.EventEnvelope[any]{}, nil
		}
		return nil, err
	}

	if len(data) == 0 {
		return []domain.EventEnvelope[any]{}, nil
	}

	var events []domain.EventEnvelope[any]
	if err := json.Unmarshal(data, &events); err != nil {
		return nil, fmt.Errorf("failed to unmarshal events: %w", err)
	}

	return events, nil
}

// saveToFile saves events to a file.
func (f *FileStore) saveToFile(aggregateID string, events []domain.EventEnvelope[any]) error {
	filePath := f.getFilePath(aggregateID)

	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal events: %w", err)
	}

	// Write to temporary file first, then rename (atomic write)
	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temporary file: %w", err)
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		os.Remove(tmpPath) // Clean up on error
		return fmt.Errorf("failed to rename temporary file: %w", err)
	}

	return nil
}

// Append appends events to the store for the given aggregate.
func (f *FileStore) Append(ctx context.Context, aggregateID string, expectedVersion int, events ...domain.EventEnvelope[any]) error {
	if len(events) == 0 {
		return nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	// Validate events
	for _, event := range events {
		if event.AggregateID != aggregateID {
			return fmt.Errorf("%w: aggregate ID mismatch", domain.ErrInvalidEvent)
		}
		if event.ID == "" {
			return fmt.Errorf("%w: event ID is required", domain.ErrInvalidEvent)
		}
	}

	// Load current events
	currentEvents := f.cache[aggregateID]
	if currentEvents == nil {
		// Try to load from disk
		filePath := f.getFilePath(aggregateID)
		loaded, err := f.loadFromFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to load existing events: %w", err)
		}
		currentEvents = loaded
		f.cache[aggregateID] = currentEvents
	}

	// Check current version
	currentVersion := 0
	if len(currentEvents) > 0 {
		currentVersion = currentEvents[len(currentEvents)-1].Version
	}

	if expectedVersion != -1 && currentVersion != expectedVersion {
		return fmt.Errorf("%w: expected version %d, got %d", domain.ErrConcurrencyConflict, expectedVersion, currentVersion)
	}

	// Append events with sequential versions
	nextVersion := currentVersion + 1
	for i, event := range events {
		event.Version = nextVersion + i
		currentEvents = append(currentEvents, event)
	}

	// Save to disk
	if err := f.saveToFile(aggregateID, currentEvents); err != nil {
		return err
	}

	// Update cache
	f.cache[aggregateID] = currentEvents

	return nil
}

// GetEvents retrieves all events for the given aggregate ID.
func (f *FileStore) GetEvents(ctx context.Context, aggregateID string) ([]domain.EventEnvelope[any], error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Check cache first
	if events, exists := f.cache[aggregateID]; exists {
		return events, nil
	}

	// Load from disk
	filePath := f.getFilePath(aggregateID)
	events, err := f.loadFromFile(filePath)
	if err != nil {
		return nil, err
	}

	// Cache the result
	f.cache[aggregateID] = events

	return events, nil
}

// GetEventsFromVersion retrieves events starting from the specified version.
func (f *FileStore) GetEventsFromVersion(ctx context.Context, aggregateID string, fromVersion int) ([]domain.EventEnvelope[any], error) {
	events, err := f.GetEvents(ctx, aggregateID)
	if err != nil {
		return nil, err
	}

	result := make([]domain.EventEnvelope[any], 0)
	for _, event := range events {
		if event.Version >= fromVersion {
			result = append(result, event)
		}
	}

	return result, nil
}

// GetEventsRange retrieves events within a version range.
func (f *FileStore) GetEventsRange(ctx context.Context, aggregateID string, fromVersion, toVersion int) ([]domain.EventEnvelope[any], error) {
	events, err := f.GetEvents(ctx, aggregateID)
	if err != nil {
		return nil, err
	}

	// Default fromVersion to 1 if -1
	if fromVersion == -1 {
		fromVersion = 1
	}

	result := make([]domain.EventEnvelope[any], 0)
	for _, event := range events {
		if event.Version < fromVersion {
			continue
		}
		// If toVersion is -1, include all events from fromVersion onwards
		if toVersion != -1 && event.Version > toVersion {
			break
		}
		result = append(result, event)
	}

	return result, nil
}

// GetEventByID retrieves a specific event by its ID.
func (f *FileStore) GetEventByID(ctx context.Context, eventID string) (domain.EventEnvelope[any], error) {
	f.mu.RLock()
	// Search through all cached aggregates
	for _, events := range f.cache {
		for _, event := range events {
			if event.ID == eventID {
				return event, nil
			}
		}
	}
	f.mu.RUnlock()

	// If not in cache, search all files on disk
	entries, err := os.ReadDir(f.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return domain.EventEnvelope[any]{}, domain.ErrEventNotFound
		}
		return domain.EventEnvelope[any]{}, err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		filePath := filepath.Join(f.baseDir, entry.Name())
		events, err := f.loadFromFile(filePath)
		if err != nil {
			continue // Skip files that can't be read
		}

		for _, event := range events {
			if event.ID == eventID {
				// Cache the aggregate's events for future use
				aggregateID := entry.Name()[:len(entry.Name())-5]
				f.mu.Lock()
				f.cache[aggregateID] = events
				f.mu.Unlock()

				return event, nil
			}
		}
	}

	return domain.EventEnvelope[any]{}, domain.ErrEventNotFound
}

// GetCurrentVersion returns the current version for the aggregate.
func (f *FileStore) GetCurrentVersion(ctx context.Context, aggregateID string) (int, error) {
	events, err := f.GetEvents(ctx, aggregateID)
	if err != nil {
		return 0, err
	}

	if len(events) == 0 {
		return 0, nil
	}

	return events[len(events)-1].Version, nil
}

// Close closes the file store and releases resources.
func (f *FileStore) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Clear cache
	f.cache = make(map[string][]domain.EventEnvelope[any])
	return nil
}
