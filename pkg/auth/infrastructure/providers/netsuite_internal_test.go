package providers

import "testing"

// TestNetSuiteURLBuilders_Derived locks down the per-account URL templating
// for token, revoke, and userinfo. The exported AuthCodeURL test covers the
// auth template; this file covers the three other builders that are otherwise
// only exercised through httptest with overrides set.
func TestNetSuiteURLBuilders_Derived(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		accountID      string
		wantToken      string
		wantRevoke     string
		wantUserInfo   string
	}{
		{
			name:         "production account",
			accountID:    "1234567",
			wantToken:    "https://1234567.suitetalk.api.netsuite.com/services/rest/auth/oauth2/v1/token",
			wantRevoke:   "https://1234567.suitetalk.api.netsuite.com/services/rest/auth/oauth2/v1/revoke",
			wantUserInfo: "https://1234567.suitetalk.api.netsuite.com/services/rest/auth/oauth2/v1/userinfo",
		},
		{
			name:         "sandbox account normalized",
			accountID:    "1234567_SB1",
			wantToken:    "https://1234567-sb1.suitetalk.api.netsuite.com/services/rest/auth/oauth2/v1/token",
			wantRevoke:   "https://1234567-sb1.suitetalk.api.netsuite.com/services/rest/auth/oauth2/v1/revoke",
			wantUserInfo: "https://1234567-sb1.suitetalk.api.netsuite.com/services/rest/auth/oauth2/v1/userinfo",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			n := NewNetSuite(NetSuiteConfig{AccountID: tt.accountID})
			if got := n.tokenURL(); got != tt.wantToken {
				t.Errorf("tokenURL() = %q, want %q", got, tt.wantToken)
			}
			if got := n.revokeURL(); got != tt.wantRevoke {
				t.Errorf("revokeURL() = %q, want %q", got, tt.wantRevoke)
			}
			if got := n.userInfoURL(); got != tt.wantUserInfo {
				t.Errorf("userInfoURL() = %q, want %q", got, tt.wantUserInfo)
			}
		})
	}
}

// TestNetSuiteURLBuilders_OverrideWins asserts the override-wins-over-derived
// rule for every endpoint, including revoke and userinfo where the exported
// surface didn't otherwise give us a way to assert it directly.
func TestNetSuiteURLBuilders_OverrideWins(t *testing.T) {
	t.Parallel()

	n := NewNetSuite(NetSuiteConfig{
		AccountID:        "1234567",
		AuthEndpoint:     "https://override.example.com/auth",
		TokenEndpoint:    "https://override.example.com/token",
		RevokeEndpoint:   "https://override.example.com/revoke",
		UserInfoEndpoint: "https://override.example.com/userinfo",
	})

	if got := n.authURL(); got != "https://override.example.com/auth" {
		t.Errorf("authURL() = %q, want override", got)
	}
	if got := n.tokenURL(); got != "https://override.example.com/token" {
		t.Errorf("tokenURL() = %q, want override", got)
	}
	if got := n.revokeURL(); got != "https://override.example.com/revoke" {
		t.Errorf("revokeURL() = %q, want override", got)
	}
	if got := n.userInfoURL(); got != "https://override.example.com/userinfo" {
		t.Errorf("userInfoURL() = %q, want override", got)
	}
}
