package authhttp

import (
	"crypto/tls"
	"net/http"
	"testing"
)

func TestBuildRedirectURI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		host      string
		tls       bool
		headers   map[string]string
		cfg       RedirectURIConfig
		want      string
		wantErr   error
		wantNoErr bool
	}{
		{
			name: "basic HTTP request",
			host: "example.com",
			cfg:  RedirectURIConfig{CallbackPath: "/callback"},
			want: "http://example.com/callback",
		},
		{
			name: "TLS request produces https",
			host: "example.com",
			tls:  true,
			cfg:  RedirectURIConfig{CallbackPath: "/callback"},
			want: "https://example.com/callback",
		},
		{
			name: "ForceTLS overrides to https",
			host: "example.com",
			cfg:  RedirectURIConfig{CallbackPath: "/callback", ForceTLS: true},
			want: "https://example.com/callback",
		},
		{
			name:    "X-Forwarded-Proto upgrades to https",
			host:    "example.com",
			headers: map[string]string{"X-Forwarded-Proto": "https"},
			cfg:     RedirectURIConfig{CallbackPath: "/callback"},
			want:    "https://example.com/callback",
		},
		{
			name:    "X-Forwarded-Proto cannot downgrade HTTPS to HTTP",
			host:    "example.com",
			tls:     true,
			headers: map[string]string{"X-Forwarded-Proto": "http"},
			cfg:     RedirectURIConfig{CallbackPath: "/callback"},
			want:    "https://example.com/callback",
		},
		{
			name:    "X-Forwarded-Proto rejects invalid scheme",
			host:    "example.com",
			headers: map[string]string{"X-Forwarded-Proto": "javascript"},
			cfg:     RedirectURIConfig{CallbackPath: "/callback"},
			want:    "http://example.com/callback",
		},
		{
			name: "host in AllowedHosts succeeds",
			host: "example.com",
			cfg:  RedirectURIConfig{CallbackPath: "/cb", AllowedHosts: []string{"example.com"}},
			want: "http://example.com/cb",
		},
		{
			name:    "host not in AllowedHosts returns error",
			host:    "evil.com",
			cfg:     RedirectURIConfig{CallbackPath: "/cb", AllowedHosts: []string{"example.com"}},
			wantErr: ErrHostNotAllowed,
		},
		{
			name:    "X-Forwarded-Host honored when in AllowedHosts",
			host:    "proxy.internal",
			headers: map[string]string{"X-Forwarded-Host": "example.com"},
			cfg:     RedirectURIConfig{CallbackPath: "/cb", AllowedHosts: []string{"example.com"}},
			want:    "http://example.com/cb",
		},
		{
			name:    "X-Forwarded-Host ignored when AllowedHosts is empty",
			host:    "example.com",
			headers: map[string]string{"X-Forwarded-Host": "evil.com"},
			cfg:     RedirectURIConfig{CallbackPath: "/cb"},
			want:    "http://example.com/cb",
		},
		{
			name:    "X-Forwarded-Host rejected when not in AllowedHosts",
			host:    "example.com",
			headers: map[string]string{"X-Forwarded-Host": "evil.com"},
			cfg:     RedirectURIConfig{CallbackPath: "/cb", AllowedHosts: []string{"example.com"}},
			want:    "http://example.com/cb",
		},
		{
			name: "port stripping: host with port matches allowed without port",
			host: "example.com:8080",
			cfg:  RedirectURIConfig{CallbackPath: "/cb", AllowedHosts: []string{"example.com"}},
			want: "http://example.com:8080/cb",
		},
		{
			name: "port stripping: allowed with port matches host without port",
			host: "example.com",
			cfg:  RedirectURIConfig{CallbackPath: "/cb", AllowedHosts: []string{"example.com:8080"}},
			want: "http://example.com/cb",
		},
		{
			name: "case-insensitive host comparison",
			host: "EXAMPLE.COM",
			cfg:  RedirectURIConfig{CallbackPath: "/cb", AllowedHosts: []string{"example.com"}},
			want: "http://EXAMPLE.COM/cb",
		},
		{
			name: "IPv6 host with port in AllowedHosts",
			host: "[::1]:8080",
			cfg:  RedirectURIConfig{CallbackPath: "/cb", AllowedHosts: []string{"[::1]"}},
			want: "http://[::1]:8080/cb",
		},
		{
			name:    "IPv6 host not in AllowedHosts",
			host:    "[::1]:8080",
			cfg:     RedirectURIConfig{CallbackPath: "/cb", AllowedHosts: []string{"example.com"}},
			wantErr: ErrHostNotAllowed,
		},
		{
			name:      "empty AllowedHosts uses r.Host directly",
			host:      "anything.example.com",
			cfg:       RedirectURIConfig{CallbackPath: "/cb"},
			want:      "http://anything.example.com/cb",
			wantNoErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, _ := http.NewRequest("GET", "/", nil)
			r.Host = tt.host
			if tt.tls {
				r.TLS = &tls.ConnectionState{}
			}
			for k, v := range tt.headers {
				r.Header.Set(k, v)
			}

			got, err := BuildRedirectURI(r, tt.cfg)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tt.wantErr)
				}
				if err != tt.wantErr {
					t.Fatalf("error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("BuildRedirectURI() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsValidRedirectPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want bool
	}{
		{"/dashboard", true},
		{"/foo/bar", true},
		{"/", true},
		{"/valid/path?foo=bar", true},
		{"", false},
		{"//evil.com", false},
		{"https://evil.com", false},
		{"http://evil.com", false},
		{"relative/path", false},
		{"javascript:alert(1)", false},
		{"/valid/but//double-slash-inside", true}, // only leading // is blocked
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			got := isValidRedirectPath(tt.path)
			if got != tt.want {
				t.Errorf("isValidRedirectPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestRealIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		want       string
	}{
		{
			name:       "X-Real-Ip preferred",
			remoteAddr: "10.0.0.1:1234",
			headers:    map[string]string{"X-Real-Ip": "203.0.113.50"},
			want:       "203.0.113.50",
		},
		{
			name:       "X-Forwarded-For single IP",
			remoteAddr: "10.0.0.1:1234",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.50"},
			want:       "203.0.113.50",
		},
		{
			name:       "X-Forwarded-For chain takes first",
			remoteAddr: "10.0.0.1:1234",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.50, 70.41.3.18, 150.172.238.178"},
			want:       "203.0.113.50",
		},
		{
			name:       "RemoteAddr IPv4 with port",
			remoteAddr: "192.168.1.1:12345",
			want:       "192.168.1.1",
		},
		{
			name:       "RemoteAddr IPv6 with port",
			remoteAddr: "[::1]:54321",
			want:       "::1",
		},
		{
			name:       "RemoteAddr without port",
			remoteAddr: "192.168.1.1",
			want:       "192.168.1.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r, _ := http.NewRequest("GET", "/", nil)
			r.RemoteAddr = tt.remoteAddr
			for k, v := range tt.headers {
				r.Header.Set(k, v)
			}
			got := realIP(r)
			if got != tt.want {
				t.Errorf("realIP() = %q, want %q", got, tt.want)
			}
		})
	}
}
