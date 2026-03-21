package infrastructure

import (
	"encoding/json"
	"fmt"
)

// toAnyMap converts a value to map[string]any using JSON round-trip.
// Used by multiple EventStore implementations to normalize typed payloads.
func toAnyMap(v any) (map[string]any, error) {
	switch p := v.(type) {
	case map[string]any:
		return p, nil
	case nil:
		return map[string]any{}, nil
	default:
		data, err := json.Marshal(p)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal %T: %w", p, err)
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("failed to unmarshal %T to map: %w", p, err)
		}
		return m, nil
	}
}
