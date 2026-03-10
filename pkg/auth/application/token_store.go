package application

import (
	"context"
	"time"
)

// noOpTokenStore is the default TokenStore that does nothing.
// It is used when consumers don't need server-side token storage.
type noOpTokenStore struct{}

func (noOpTokenStore) StoreTokens(_ context.Context, _, _, _, _ string, _ time.Time) error {
	return nil
}

func (noOpTokenStore) GetTokens(_ context.Context, _ string) (string, string, time.Time, error) {
	return "", "", time.Time{}, nil
}

func (noOpTokenStore) DeleteTokens(_ context.Context, _ string) error {
	return nil
}

func (noOpTokenStore) NeedsRefresh(_ context.Context, _ string) (bool, error) {
	return false, nil
}
