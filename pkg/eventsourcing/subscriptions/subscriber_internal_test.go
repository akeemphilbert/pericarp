package subscriptions

import (
	"math"
	"testing"
	"time"
)

func TestBackoffDelay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		initial time.Duration
		maximum time.Duration
		attempt int
		want    time.Duration
	}{
		{"first attempt uses initial", 100 * time.Millisecond, 5 * time.Second, 0, 100 * time.Millisecond},
		{"doubles per attempt", 100 * time.Millisecond, 5 * time.Second, 3, 800 * time.Millisecond},
		{"caps at maximum", 100 * time.Millisecond, 5 * time.Second, 10, 5 * time.Second},
		{"huge attempt counts cannot overflow into a hot loop", time.Second, time.Duration(math.MaxInt64), 200, time.Duration(math.MaxInt64)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := &Subscriber{retryBackoff: tt.initial, maxBackoff: tt.maximum}
			got := s.backoffDelay(tt.attempt)
			if got != tt.want {
				t.Fatalf("backoffDelay(%d) = %v, want %v", tt.attempt, got, tt.want)
			}
			if got <= 0 {
				t.Fatalf("backoffDelay(%d) = %v, must always be positive", tt.attempt, got)
			}
		})
	}
}
