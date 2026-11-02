package web

import (
	"net"
	"net/http"
	"strings"

	"github.com/septi0/dockvmap/internal/service"
)

func (w *Web) apiProxyPullInfo(rw http.ResponseWriter, r *http.Request) {
	host, port, hostConfigured := w.proxyAddress(r)

	apiJSON(rw, http.StatusOK, pullInfoResponse{
		Host:           host,
		Port:           port,
		VirtualTag:     w.cfg.VirtualTag,
		HostConfigured: hostConfigured,
	})
}

func (w *Web) proxyTarget(r *http.Request) service.ProxyTarget {
	host, port, _ := w.proxyAddress(r)

	return service.ProxyTarget{Host: net.JoinHostPort(host, port), VirtualTag: w.cfg.VirtualTag}
}

func (w *Web) proxyAddress(r *http.Request) (host, port string, configured bool) {
	configuredHost := strings.TrimSpace(w.cfg.ProxyPublicHost)
	configured = configuredHost != ""

	if configured {
		if h, p, err := net.SplitHostPort(configuredHost); err == nil {
			host, port = h, p
		} else {
			host = configuredHost
		}
	} else {
		var err error

		host, _, err = net.SplitHostPort(r.Host)

		if err != nil {
			host = r.Host
		}
	}

	if port == "" {
		_, p, err := net.SplitHostPort(w.cfg.ProxyServerListen)

		if err != nil {
			port = w.cfg.ProxyServerListen
		} else {
			port = p
		}
	}

	return host, port, configured
}
