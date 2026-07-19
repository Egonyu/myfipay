package middleware

import (
	"net"
	"net/http"
	"strings"
)

// ClientIP returns the trusted client address. nginx always sets X-Real-IP to
// $remote_addr (never forwarded from the client), so it is the only header we
// trust. X-Forwarded-For is client-appendable ($proxy_add_x_forwarded_for) and
// chi's RealIP honors a client-supplied True-Client-IP, so neither is safe for
// security decisions. Without the header (direct loopback access) we fall back
// to the socket address.
func ClientIP(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ParseIPList parses a comma-separated list of IPs and CIDRs into networks.
// Bare IPs become /32 (or /128) networks. Empty entries are skipped.
func ParseIPList(s string) ([]*net.IPNet, error) {
	var nets []*net.IPNet
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !strings.Contains(part, "/") {
			ip := net.ParseIP(part)
			if ip == nil {
				return nil, &net.ParseError{Type: "IP address", Text: part}
			}
			bits := 32
			if ip.To4() == nil {
				bits = 128
			}
			nets = append(nets, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			continue
		}
		_, n, err := net.ParseCIDR(part)
		if err != nil {
			return nil, err
		}
		nets = append(nets, n)
	}
	return nets, nil
}

// IPAllowed reports whether ip is inside any of the networks.
func IPAllowed(ipStr string, nets []*net.IPNet) bool {
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return false
	}
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// RequireSourceIP rejects requests whose trusted client IP is not in the
// allowlist. An empty allowlist disables the check (sandbox mode — same
// pattern as the empty webhook HMAC secret).
func RequireSourceIP(nets []*net.IPNet) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(nets) > 0 && !IPAllowed(ClientIP(r), nets) {
				http.Error(w, `{"error":{"code":"FORBIDDEN","message":"source not allowed"}}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
