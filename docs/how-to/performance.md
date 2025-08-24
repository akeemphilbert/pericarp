# How to Optimize Performance

This guide provides practical techniques for optimizing the performance of your Pericarp applications.

## Problem

You need to optimize your Pericarp application for:
- High throughput (requests per second)
- Low latency (response times)
- Efficient memory usage
- Scalability under load

## Solution

### 1. Event Store Optimization

#### Batch Operations
```go
// Configure batch size for bulk operations
config := infrastructure.PerformanceConfig{
    EventStore: infrastructure.EventStoreConfig{
        BatchSize: 200, // Increase for bulk operations
    },
}

// Use batch operations when possible
events := make([]domain.Event, 0, 100)
for _, aggregate := range aggregates {
    events = append(events, aggregate.UncommittedEvents()...)
}

// Single batch save instead of multiple individual saves
envelopes, err := eventStore.Save(ctx, events)
```

#### Database Connection Optimization
```go
// Configure connection pool
db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
    PrepareStmt: true, // Enable prepared statements
})

sqlDB, err := db.DB()
sqlDB.SetMaxOpenConns(20)    // Maximum open connections
sqlDB.SetMaxIdleConns(10)    // Maximum idle connections
sqlDB.SetConnMaxLifetime(time.Hour) // Connection lifetime
```

#### Query Optimization
```go
// Use proper indexing
type EventRecord struct {
    ID          string    `gorm:"primaryKey"`
    AggregateID string    `gorm:"index:idx_aggregate_version"`
    Version     int       `gorm:"index:idx_aggregate_version"`
    EventType   string    `gorm:"index"`
    Timestamp   time.Time `gorm:"index"`
}

// Optimize queries with proper WHERE clauses
query := db.Where("aggregate_id = ? AND version > ?", aggregateID, version).
    Order("version ASC").
    Limit(1000) // Limit large result sets
```

### 2. Memory Optimization

#### Pre-allocate Slices
```go
// Bad - causes multiple allocations
var events []domain.Event
for _, item := range items {
    events = append(events, createEvent(item))
}

// Good - pre-allocate with known capacity
events := make([]domain.Event, 0, len(items))
for _, item := range items {
    events = append(events, createEvent(item))
}
```

#### Reuse Buffers
```go
type EventProcessor struct {
    jsonBuffer []byte // Reusable buffer
}

func (p *EventProcessor) ProcessEvent(event domain.Event) error {
    // Reuse buffer instead of allocating new one
    p.jsonBuffer = p.jsonBuffer[:0] // Reset length, keep capacity
    
    data, err := json.Marshal(event)
    if err != nil {
        return err
    }
    
    // Process data...
    return nil
}
```

#### Object Pooling
```go
var eventPool = sync.Pool{
    New: func() interface{} {
        return &EventEnvelope{}
    },
}

func ProcessEvent(event domain.Event) error {
    envelope := eventPool.Get().(*EventEnvelope)
    defer eventPool.Put(envelope)
    
    // Use envelope...
    envelope.Reset() // Reset state before returning to pool
    
    return nil
}
```

### 3. Middleware Optimization

#### Conditional Logging
```go
func OptimizedLoggingMiddleware[Req any, Res any](logLevel string) Middleware[Req, Res] {
    return func(next Handler[Req, Res]) Handler[Req, Res] {
        return func(ctx context.Context, log domain.Logger, p Payload[Req]) (Response[Res], error) {
            // Only measure time if logging is enabled
            var start time.Time
            if logLevel == "debug" || p.TraceID != "" {
                start = time.Now()
            }
            
            response, err := next(ctx, log, p)
            
            // Only log if necessary
            if err != nil || (logLevel == "debug" && !start.IsZero()) {
                duration := time.Since(start)
                if err != nil {
                    log.Error("Request failed", "duration", duration, "error", err)
                } else {
                    log.Debug("Request completed", "duration", duration)
                }
            }
            
            return response, err
        }
    }
}
```

#### Efficient Metrics Collection
```go
type FastMetricsCollector struct {
    counters map[string]*int64 // Use pointers for atomic operations
    mu       sync.RWMutex
}

func (m *FastMetricsCollector) IncrementCounter(name string) {
    m.mu.RLock()
    counter, exists := m.counters[name]
    m.mu.RUnlock()
    
    if exists {
        atomic.AddInt64(counter, 1)
        return
    }
    
    // Slow path - create new counter
    m.mu.Lock()
    if counter, exists := m.counters[name]; exists {
        m.mu.Unlock()
        atomic.AddInt64(counter, 1)
        return
    }
    
    newCounter := new(int64)
    m.counters[name] = newCounter
    m.mu.Unlock()
    
    atomic.AddInt64(newCounter, 1)
}
```

### 4. Concurrency Optimization

#### Use Read-Write Mutexes
```go
type OptimizedCache struct {
    data map[string]interface{}
    mu   sync.RWMutex // Use RWMutex for read-heavy workloads
}

func (c *OptimizedCache) Get(key string) (interface{}, bool) {
    c.mu.RLock()         // Read lock
    defer c.mu.RUnlock()
    
    value, exists := c.data[key]
    return value, exists
}

func (c *OptimizedCache) Set(key string, value interface{}) {
    c.mu.Lock()         // Write lock
    defer c.mu.Unlock()
    
    c.data[key] = value
}
```

#### Worker Pool Pattern
```go
type EventProcessor struct {
    workers   int
    eventChan chan domain.Event
    wg        sync.WaitGroup
}

func NewEventProcessor(workers int) *EventProcessor {
    p := &EventProcessor{
        workers:   workers,
        eventChan: make(chan domain.Event, workers*2), // Buffered channel
    }
    
    // Start worker goroutines
    for i := 0; i < workers; i++ {
        p.wg.Add(1)
        go p.worker()
    }
    
    return p
}

func (p *EventProcessor) worker() {
    defer p.wg.Done()
    
    for event := range p.eventChan {
        p.processEvent(event)
    }
}

func (p *EventProcessor) ProcessAsync(event domain.Event) {
    select {
    case p.eventChan <- event:
        // Event queued successfully
    default:
        // Channel full, handle backpressure
        p.processEvent(event) // Process synchronously as fallback
    }
}
```

### 5. JSON Optimization

#### Use Streaming for Large Payloads
```go
func StreamEvents(w io.Writer, events []domain.Event) error {
    encoder := json.NewEncoder(w)
    
    for _, event := range events {
        if err := encoder.Encode(event); err != nil {
            return err
        }
    }
    
    return nil
}
```

#### Custom JSON Marshaling
```go
type OptimizedEvent struct {
    Type        string    `json:"type"`
    AggregateID string    `json:"aggregate_id"`
    Version     int       `json:"version"`
    OccurredAt  time.Time `json:"occurred_at"`
    Data        json.RawMessage `json:"data"` // Avoid double marshaling
}

func (e *OptimizedEvent) MarshalJSON() ([]byte, error) {
    // Custom marshaling logic for better performance
    return json.Marshal(struct {
        Type        string          `json:"type"`
        AggregateID string          `json:"aggregate_id"`
        Version     int             `json:"version"`
        OccurredAt  int64           `json:"occurred_at"` // Unix timestamp
        Data        json.RawMessage `json:"data"`
    }{
        Type:        e.Type,
        AggregateID: e.AggregateID,
        Version:     e.Version,
        OccurredAt:  e.OccurredAt.Unix(),
        Data:        e.Data,
    })
}
```

### 6. Database Optimization

#### Use Prepared Statements
```go
// Enable prepared statements in GORM
db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
    PrepareStmt: true,
})

// Or use raw SQL with prepared statements
stmt, err := db.Prepare("SELECT * FROM events WHERE aggregate_id = ? AND version > ? ORDER BY version")
defer stmt.Close()

rows, err := stmt.Query(aggregateID, version)
```

#### Optimize Indexes
```sql
-- Composite index for common query patterns
CREATE INDEX idx_events_aggregate_version ON events(aggregate_id, version);

-- Partial index for recent events
CREATE INDEX idx_events_recent ON events(timestamp) WHERE timestamp > NOW() - INTERVAL '1 day';

-- Index for event type queries
CREATE INDEX idx_events_type ON events(event_type);
```

### 7. Caching Strategies

#### Multi-Level Caching
```go
type MultiLevelCache struct {
    l1 *sync.Map           // In-memory cache
    l2 *redis.Client       // Redis cache
    l3 ReadModelRepository // Database
}

func (c *MultiLevelCache) Get(ctx context.Context, key string) (interface{}, error) {
    // L1 - Memory cache
    if value, ok := c.l1.Load(key); ok {
        return value, nil
    }
    
    // L2 - Redis cache
    if c.l2 != nil {
        value, err := c.l2.Get(ctx, key).Result()
        if err == nil {
            c.l1.Store(key, value) // Populate L1
            return value, nil
        }
    }
    
    // L3 - Database
    value, err := c.l3.Get(ctx, key)
    if err != nil {
        return nil, err
    }
    
    // Populate caches
    c.l1.Store(key, value)
    if c.l2 != nil {
        c.l2.Set(ctx, key, value, time.Hour)
    }
    
    return value, nil
}
```

#### Cache Warming
```go
func (s *UserService) WarmCache(ctx context.Context) error {
    // Pre-load frequently accessed data
    popularUsers, err := s.repo.GetPopularUsers(ctx, 100)
    if err != nil {
        return err
    }
    
    for _, user := range popularUsers {
        s.cache.Set(user.ID(), user)
    }
    
    return nil
}
```

## Performance Testing

### Benchmark Your Code
```go
func BenchmarkEventSave(b *testing.B) {
    eventStore := setupEventStore()
    events := generateTestEvents(100)
    
    b.ResetTimer()
    b.ReportAllocs()
    
    for i := 0; i < b.N; i++ {
        _, err := eventStore.Save(context.Background(), events)
        if err != nil {
            b.Fatal(err)
        }
    }
}

func BenchmarkAggregateReconstruction(b *testing.B) {
    events := generateTestEvents(1000)
    
    b.ResetTimer()
    b.ReportAllocs()
    
    for i := 0; i < b.N; i++ {
        aggregate := &User{}
        aggregate.LoadFromHistory(events)
    }
}
```

### Load Testing
```go
func TestConcurrentLoad(t *testing.T) {
    const (
        numGoroutines = 100
        requestsPerGoroutine = 100
    )
    
    var wg sync.WaitGroup
    start := time.Now()
    
    for i := 0; i < numGoroutines; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            
            for j := 0; j < requestsPerGoroutine; j++ {
                err := performRequest()
                if err != nil {
                    t.Errorf("Request failed: %v", err)
                }
            }
        }()
    }
    
    wg.Wait()
    
    duration := time.Since(start)
    totalRequests := numGoroutines * requestsPerGoroutine
    rps := float64(totalRequests) / duration.Seconds()
    
    t.Logf("Processed %d requests in %v (%.2f RPS)", totalRequests, duration, rps)
    
    // Assert performance requirements
    if rps < 1000 {
        t.Errorf("Performance below threshold: %.2f RPS (expected: 1000+)", rps)
    }
}
```

## Monitoring and Profiling

### Enable Profiling
```go
import _ "net/http/pprof"

func main() {
    // Enable pprof endpoint
    go func() {
        log.Println(http.ListenAndServe("localhost:6060", nil))
    }()
    
    // Your application code...
}
```

### Custom Metrics
```go
type PerformanceMetrics struct {
    RequestDuration prometheus.HistogramVec
    RequestCount    prometheus.CounterVec
    ErrorCount      prometheus.CounterVec
}

func (m *PerformanceMetrics) RecordRequest(requestType string, duration time.Duration, success bool) {
    m.RequestDuration.WithLabelValues(requestType).Observe(duration.Seconds())
    m.RequestCount.WithLabelValues(requestType).Inc()
    
    if !success {
        m.ErrorCount.WithLabelValues(requestType).Inc()
    }
}
```

## Production Checklist

- [ ] **Database**: Connection pooling configured
- [ ] **Indexes**: Proper indexes on aggregate_id, version, event_type
- [ ] **Caching**: Multi-level caching implemented
- [ ] **Monitoring**: Performance metrics and alerting
- [ ] **Profiling**: pprof enabled for production debugging
- [ ] **Load Testing**: Performance validated under expected load
- [ ] **Memory**: Memory usage patterns analyzed
- [ ] **Concurrency**: Proper synchronization and deadlock prevention

## Common Performance Pitfalls

1. **N+1 Queries**: Load related data in batches
2. **Memory Leaks**: Use object pooling and proper cleanup
3. **Blocking Operations**: Use async processing where appropriate
4. **Large Transactions**: Break down into smaller chunks
5. **Inefficient Serialization**: Use streaming for large payloads
6. **Missing Indexes**: Ensure proper database indexing
7. **Excessive Logging**: Use appropriate log levels in production

## Related Guides

- [Database Configuration](database-setup.md) - Optimize database settings
- [Caching Strategies](caching.md) - Implement effective caching
- [Monitoring & Observability](monitoring.md) - Monitor performance in production
- [Testing Strategies](testing-strategies.md) - Performance testing approaches