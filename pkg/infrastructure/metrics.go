package infrastructure

import (
	"sync"
	"time"

	"github.com/example/pericarp/pkg/application"
	"github.com/example/pericarp/pkg/domain"
)

// simpleMetricsCollector implements the MetricsCollector interface
type simpleMetricsCollector struct {
	logger domain.Logger
	mu     sync.RWMutex
	
	// Simple in-memory metrics storage
	commandDurations map[string][]time.Duration
	queryDurations   map[string][]time.Duration
	commandErrors    map[string]int
	queryErrors      map[string]int
}

// NewSimpleMetricsCollector creates a new simple metrics collector
func NewSimpleMetricsCollector(logger domain.Logger) application.MetricsCollector {
	return &simpleMetricsCollector{
		logger:           logger,
		commandDurations: make(map[string][]time.Duration),
		queryDurations:   make(map[string][]time.Duration),
		commandErrors:    make(map[string]int),
		queryErrors:      make(map[string]int),
	}
}

// RecordCommandDuration records the duration of a command execution
func (m *simpleMetricsCollector) RecordCommandDuration(commandType string, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.commandDurations[commandType] = append(m.commandDurations[commandType], duration)
	m.logger.Debug("Command duration recorded", "command_type", commandType, "duration", duration)
}

// RecordQueryDuration records the duration of a query execution
func (m *simpleMetricsCollector) RecordQueryDuration(queryType string, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.queryDurations[queryType] = append(m.queryDurations[queryType], duration)
	m.logger.Debug("Query duration recorded", "query_type", queryType, "duration", duration)
}

// IncrementCommandErrors increments the error count for a command type
func (m *simpleMetricsCollector) IncrementCommandErrors(commandType string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.commandErrors[commandType]++
	m.logger.Debug("Command error count incremented", "command_type", commandType, "total_errors", m.commandErrors[commandType])
}

// IncrementQueryErrors increments the error count for a query type
func (m *simpleMetricsCollector) IncrementQueryErrors(queryType string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.queryErrors[queryType]++
	m.logger.Debug("Query error count incremented", "query_type", queryType, "total_errors", m.queryErrors[queryType])
}

// GetCommandMetrics returns command metrics for a specific type (for testing/monitoring)
func (m *simpleMetricsCollector) GetCommandMetrics(commandType string) (durations []time.Duration, errors int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	durations = make([]time.Duration, len(m.commandDurations[commandType]))
	copy(durations, m.commandDurations[commandType])
	errors = m.commandErrors[commandType]
	
	return durations, errors
}

// GetQueryMetrics returns query metrics for a specific type (for testing/monitoring)
func (m *simpleMetricsCollector) GetQueryMetrics(queryType string) (durations []time.Duration, errors int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	durations = make([]time.Duration, len(m.queryDurations[queryType]))
	copy(durations, m.queryDurations[queryType])
	errors = m.queryErrors[queryType]
	
	return durations, errors
}

// GetAllCommandTypes returns all command types that have metrics
func (m *simpleMetricsCollector) GetAllCommandTypes() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	types := make([]string, 0, len(m.commandDurations))
	for cmdType := range m.commandDurations {
		types = append(types, cmdType)
	}
	
	return types
}

// GetAllQueryTypes returns all query types that have metrics
func (m *simpleMetricsCollector) GetAllQueryTypes() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	types := make([]string, 0, len(m.queryDurations))
	for queryType := range m.queryDurations {
		types = append(types, queryType)
	}
	
	return types
}