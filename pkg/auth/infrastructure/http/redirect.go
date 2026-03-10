package authhttp

import (
	"errors"
	"net"
	"net/http"
	"strings"
)

var (
	// ErrHostNotAllowed is returned when the resolved host is not in the AllowedHosts whitelist.
	ErrHostNotAllowed = errors.New("authhttp: host not in allowed list")
)

// RedirectURIConfig configures how OAuth callback redirect URIs are built.
type RedirectURIConfig struct {
	// CallbackPath is the OAuth callback endpoint path (e.g., "/api/auth/callback").
	CallbackPath string
	// AllowedHosts is a whitelist of permitted hostnames. When empty, only r.Host is trusted.
	// Port numbers are ignored during comparison: "example.com:8080" matches "example.com".
	AllowedHosts []string
	// ForceTLS forces the scheme to "https" regardless of the request.
	ForceTLS bool
}

// BuildRedirectURI constructs a safe OAuth callback redirect URI from the request.
//
// Security rules:
//   - Never trusts the Origin header (trivially forged on GET navigations).
//   - Only honours X-Forwarded-Host when it matches AllowedHosts.
//   - X-Forwarded-Proto is only trusted to upgrade the scheme to "https";
//     it cannot downgrade an HTTPS connection to HTTP. Use ForceTLS=true
//     in production to ensure HTTPS regardless of headers.
//   - Empty AllowedHosts means only r.Host is used (safe default).
//   - Returns ErrHostNotAllowed if the resolved host doesn't match the whitelist.
func BuildRedirectURI(r *http.Request, cfg RedirectURIConfig) (string, error) {
	host := r.Host

	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		if len(cfg.AllowedHosts) > 0 && isAllowedHost(fwdHost, cfg.AllowedHosts) {
			host = fwdHost
		}
		// If AllowedHosts is empty, ignore X-Forwarded-Host and stick with r.Host.
	}

	if len(cfg.AllowedHosts) > 0 && !isAllowedHost(host, cfg.AllowedHosts) {
		return "", ErrHostNotAllowed
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	// Only allow X-Forwarded-Proto to upgrade the scheme, never downgrade.
	if fwdProto := r.Header.Get("X-Forwarded-Proto"); fwdProto == "https" {
		scheme = "https"
	}
	if cfg.ForceTLS {
		scheme = "https"
	}

	return scheme + "://" + host + cfg.CallbackPath, nil
}

func isAllowedHost(host string, allowed []string) bool {
	h := stripPort(host)
	for _, a := range allowed {
		if strings.EqualFold(h, stripPort(a)) {
			return true
		}
	}
	return false
}

// stripPort removes the port from a host string, correctly handling IPv6 addresses.
func stripPort(hostport string) string {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		// No port present; strip brackets from bare IPv6 like "[::1]"
		return strings.TrimSuffix(strings.TrimPrefix(hostport, "["), "]")
	}
	return host
}
