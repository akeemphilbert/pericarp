package infrastructure

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/domain"
)

// PerformanceMonitor provides performance monitoring capabilities
type PerformanceMonitor struct {
	logger domain.Logger
	mu     sync.RWMutex

	// Metrics
	requestCounts    map[string]int64
	requestDurations map[string][]time.Duration
	errorCounts      map[string]int64

	// System metrics
	startTime  time.Time
	lastGCTime time.Time
	gcCount    uint32

	// Configuration
	maxDurationSamples int
	sampleInterval     time.Duration
}

// NewPerformanceMonitor creates a new performance monitor
func NewPerformanceMonitor(logger domain.Logger) *PerformanceMonitor {
	monitor := &PerformanceMonitor{
		logger:             logger,
		requestCounts:      make(map[string]int64),
		requestDurations:   make(map[string][]time.Duration),
		errorCounts:        make(map[string]int64),
		startTime:          time.Now(),
		maxDurationSamples: 1000,
		sampleInterval:     time.Minute,
	}

	// Start background monitoring
	go monitor.backgroundMonitoring()

	return monitor
}

// RecordRequest records a request with its duration and outcome
func (pm *PerformanceMonitor) RecordRequest(requestType string, duration time.Duration, success bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Increment request count
	pm.requestCounts[requestType]++

	// Record duration (with circular buffer to prevent memory leaks)
	durations := pm.requestDurations[requestType]
	if len(durations) >= pm.maxDurationSamples {
		// Remove oldest sample
		copy(durations, durations[1:])
		durations = durations[:len(durations)-1]
	}
	pm.requestDurations[requestType] = append(durations, duration)

	// Record errors
	if !success {
		pm.errorCounts[requestType]++
	}
}

// GetStats returns current performance statistics
func (pm *PerformanceMonitor) GetStats() PerformanceStats {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	stats := PerformanceStats{
		Uptime:       time.Since(pm.startTime),
		RequestStats: make(map[string]RequestStats),
		SystemStats:  pm.getSystemStats(),
	}

	// Calculate request statistics
	for requestType, count := range pm.requestCounts {
		durations := pm.requestDurations[requestType]
		errorCount := pm.errorCounts[requestType]

		requestStats := RequestStats{
			Count:      count,
			ErrorCount: errorCount,
			ErrorRate:  float64(errorCount) / float64(count),
		}

		if len(durations) > 0 {
			requestStats.AvgDuration = pm.calculateAverage(durations)
			requestStats.MinDuration = pm.calculateMin(durations)
			requestStats.MaxDuration = pm.calculateMax(durations)
			requestStats.P95Duration = pm.calculatePercentile(durations, 0.95)
			requestStats.P99Duration = pm.calculatePercentile(durations, 0.99)
		}

		stats.RequestStats[requestType] = requestStats
	}

	return stats
}

// LogStats logs current performance statistics
func (pm *PerformanceMonitor) LogStats() {
	stats := pm.GetStats()

	pm.logger.Info("Performance Statistics",
		"uptime", stats.Uptime,
		"memory_mb", stats.SystemStats.MemoryUsageMB,
		"goroutines", stats.SystemStats.GoroutineCount,
		"gc_count", stats.SystemStats.GCCount,
	)

	for requestType, reqStats := range stats.RequestStats {
		pm.logger.Info("Request Statistics",
			"type", requestType,
			"count", reqStats.Count,
			"error_rate", reqStats.ErrorRate,
			"avg_duration", reqStats.AvgDuration,
			"p95_duration", reqStats.P95Duration,
			"p99_duration", reqStats.P99Duration,
		)
	}
}

// backgroundMonitoring runs periodic monitoring tasks
func (pm *PerformanceMonitor) backgroundMonitoring() {
	ticker := time.NewTicker(pm.sampleInterval)
	defer ticker.Stop()

	for range ticker.C {
		pm.collectSystemMetrics()
		pm.LogStats()
	}
}

// collectSystemMetrics collects system-level performance metrics
func (pm *PerformanceMonitor) collectSystemMetrics() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Check for GC activity
	if memStats.NumGC > pm.gcCount {
		pm.lastGCTime = time.Now()
		pm.gcCount = memStats.NumGC
	}
}

// getSystemStats returns current system statistics
func (pm *PerformanceMonitor) getSystemStats() SystemStats {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return SystemStats{
		MemoryUsageMB:  float64(memStats.Alloc) / 1024 / 1024,
		GoroutineCount: runtime.NumGoroutine(),
		GCCount:        memStats.NumGC,
		LastGCTime:     pm.lastGCTime,
	}
}

// Helper functions for statistical calculations
func (pm *PerformanceMonitor) calculateAverage(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	var total time.Duration
	for _, d := range durations {
		total += d
	}
	return total / time.Duration(len(durations))
}

func (pm *PerformanceMonitor) calculateMin(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	min := durations[0]
	for _, d := range durations[1:] {
		if d < min {
			min = d
		}
	}
	return min
}

func (pm *PerformanceMonitor) calculateMax(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	max := durations[0]
	for _, d := range durations[1:] {
		if d > max {
			max = d
		}
	}
	return max
}

func (pm *PerformanceMonitor) calculatePercentile(durations []time.Duration, percentile float64) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	// Simple percentile calculation (for production, consider using a proper algorithm)
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)

	// Simple bubble sort (for small datasets)
	for i := 0; i < len(sorted); i++ {
		for j := 0; j < len(sorted)-1-i; j++ {
			if sorted[j] > sorted[j+1] {
				sorted[j], sorted[j+1] = sorted[j+1], sorted[j]
			}
		}
	}

	index := int(float64(len(sorted)) * percentile)
	if index >= len(sorted) {
		index = len(sorted) - 1
	}

	return sorted[index]
}

// PerformanceStats contains performance statistics
type PerformanceStats struct {
	Uptime       time.Duration
	RequestStats map[string]RequestStats
	SystemStats  SystemStats
}

// RequestStats contains statistics for a specific request type
type RequestStats struct {
	Count       int64
	ErrorCount  int64
	ErrorRate   float64
	AvgDuration time.Duration
	MinDuration time.Duration
	MaxDuration time.Duration
	P95Duration time.Duration
	P99Duration time.Duration
}

// SystemStats contains system-level statistics
type SystemStats struct {
	MemoryUsageMB  float64
	GoroutineCount int
	GCCount        uint32
	LastGCTime     time.Time
}

// PerformanceMiddleware creates middleware that integrates with the performance monitor
func PerformanceMiddleware(monitor *PerformanceMonitor) func(next func(ctx context.Context) error) func(ctx context.Context) error {
	return func(next func(ctx context.Context) error) func(ctx context.Context) error {
		return func(ctx context.Context) error {
			start := time.Now()

			err := next(ctx)

			duration := time.Since(start)
			success := err == nil

			// Extract request type from context if available
			requestType := "unknown"
			if rt := ctx.Value("request_type"); rt != nil {
				if rtStr, ok := rt.(string); ok {
					requestType = rtStr
				}
			}

			monitor.RecordRequest(requestType, duration, success)

			return err
		}
	}
}
