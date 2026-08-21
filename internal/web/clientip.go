package web

import (
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/septi0/dockvmap/internal/ipmatch"
)

// resolveClientIP returns the real client IP for a request. X-Forwarded-For
// is only honored when the immediate connecting peer is a trusted proxy -
// otherwise it's client-supplied and trivially spoofable. When trusted, it
// walks the header from the right, skipping further trusted-proxy hops,
// until it finds the first address that isn't itself a trusted proxy.
func resolveClientIP(r *http.Request, trusted ipmatch.Set) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)

	if err != nil {
		host = r.RemoteAddr
	}

	if trusted.Empty() {
		return host
	}

	peer, err := netip.ParseAddr(host)

	if err != nil || !trusted.Contains(peer) {
		return host
	}

	forwardedFor := r.Header.Get("X-Forwarded-For")

	if forwardedFor == "" {
		return host
	}

	hops := strings.Split(forwardedFor, ",")

	for i := len(hops) - 1; i >= 0; i-- {
		candidate := strings.TrimSpace(hops[i])

		addr, err := netip.ParseAddr(candidate)

		if err != nil {
			break
		}

		if !trusted.Contains(addr) {
			return candidate
		}
	}

	return host
}
