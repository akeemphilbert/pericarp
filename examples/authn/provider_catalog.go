package main

import (
	"github.com/akeemphilbert/pericarp/pkg/auth/application"
	"github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/providers"
)

// BuildProviderRegistry assembles the OAuth provider catalog used by this
// example service. A real downstream service would copy this shape and
// populate the configs from env / Viper / Vault, dropping providers it does
// not need.
//
// The mock provider is what RunAuthenticationFlow drives end-to-end; the real
// providers are registered here to demonstrate the wiring shape — none of
// their constructors make network calls, so they are safe to construct with
// placeholder credentials in this example.
func BuildProviderRegistry() application.OAuthProviderRegistry {
	mock := NewMockOAuthProvider("mock-idp")

	netsuite := providers.NewNetSuite(providers.NetSuiteConfig{
		ClientID:     "your-netsuite-client-id",
		ClientSecret: "your-netsuite-client-secret",
		AccountID:    "1234567", // sandbox: "1234567_SB1" — auto-normalized to "1234567-sb1" in URLs
		// AuthEndpoint / TokenEndpoint / RevokeEndpoint / UserInfoEndpoint
		// can be set to point at a non-standard NetSuite host (e.g. a corporate
		// proxy or a future endpoint change). Each takes precedence over the
		// AccountID-derived URL when set.
	})

	return application.OAuthProviderRegistry{
		"mock-idp": mock,
		"netsuite": netsuite,
	}
}
