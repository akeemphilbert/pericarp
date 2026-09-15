package application_test

import (
	"testing"

	"github.com/akeemphilbert/pericarp/pkg/auth/application"
)

func TestUserInfo_HasUnverifiedEmail(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		info application.UserInfo
		want bool
	}{
		{
			name: "google email not verified",
			info: application.UserInfo{Provider: "google", Email: "user@example.com"},
			want: true,
		},
		{
			name: "apple email not verified",
			info: application.UserInfo{Provider: "apple", Email: "user@example.com"},
			want: true,
		},
		{
			name: "google email verified",
			info: application.UserInfo{Provider: "google", Email: "user@example.com", EmailVerified: true},
			want: false,
		},
		{
			name: "apple email verified",
			info: application.UserInfo{Provider: "apple", Email: "user@example.com", EmailVerified: true},
			want: false,
		},
		{
			// NetSuite sends no email_verified claim, so its false says nothing
			// and must not refuse a sign-in that works today.
			name: "netsuite sends no claim",
			info: application.UserInfo{Provider: "netsuite", Email: "user@example.com"},
			want: false,
		},
		{
			name: "github sends no claim",
			info: application.UserInfo{Provider: "github", Email: "user@example.com"},
			want: false,
		},
		{
			// No address, nothing to vouch for: a credential without an email
			// cannot be bound by one.
			name: "google with no email",
			info: application.UserInfo{Provider: "google"},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.info.HasUnverifiedEmail(); got != tc.want {
				t.Errorf("HasUnverifiedEmail() = %v, want %v", got, tc.want)
			}
		})
	}
}
