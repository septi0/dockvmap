package web

import (
	"regexp"
	"strings"
)

var (
	lookupHost = regexp.MustCompile(`lookup ([^\s:]+)`)
	dialHost   = regexp.MustCompile(`dial tcp ([^\s]+?): `)
	urlHost    = regexp.MustCompile(`https?://([^/"\s]+)`)
	httpStatus = regexp.MustCompile(`returned (\d{3}) `)
)

type failureCause struct {
	host   string
	phrase string
	hint   string
}

func recognizeFailure(raw string) (failureCause, bool) {
	cause := failureCause{host: failureHost(raw)}

	switch {
	case strings.Contains(raw, "no such host"):
		cause.phrase, cause.hint = "couldn't be found", "Check its address."
	case strings.Contains(raw, "server misbehaving"):
		cause.phrase, cause.hint = "couldn't be looked up", "Check the DNS server."
	case strings.Contains(raw, "connection refused"):
		cause.phrase, cause.hint = "refused the connection", "Check that it's running."
	case containsAny(raw, "i/o timeout", "deadline exceeded", "Client.Timeout", "TLS handshake timeout"):
		cause.phrase, cause.hint = "didn't answer", "Check that it's running and reachable."
	case containsAny(raw, "x509:", "failed to verify certificate"):
		cause.phrase, cause.hint = "has a certificate DockVMap doesn't accept", ""
	case strings.HasPrefix(raw, "authenticating:"):
		cause.phrase, cause.hint = "refused the login", "Check the smtp settings."
	default:
		status := httpStatus.FindStringSubmatch(raw)

		if status == nil {
			return failureCause{}, false
		}

		switch status[1] {
		case "401", "403":
			cause.phrase, cause.hint = "refused the login", "Check the login."
		case "429":
			cause.phrase, cause.hint = "is limiting how often DockVMap can ask", "It clears by itself."
		default:
			return failureCause{}, false
		}
	}

	return cause, true
}

func failureHost(raw string) string {
	for _, pattern := range []*regexp.Regexp{lookupHost, urlHost, dialHost} {
		if match := pattern.FindStringSubmatch(raw); match != nil {
			return match[1]
		}
	}

	return ""
}

func (c failureCause) sentence(fallbackHost string) string {
	host := c.host

	if host == "" {
		host = fallbackHost
	}

	return strings.TrimSpace(host + " " + c.phrase + ". " + c.hint)
}

func containsAny(s string, parts ...string) bool {
	for _, part := range parts {
		if strings.Contains(s, part) {
			return true
		}
	}

	return false
}
