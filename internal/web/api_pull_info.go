package web

import (
	"net"
	"net/http"
	"strings"
)

func (w *Web) apiProxyPullInfo(rw http.ResponseWriter, r *http.Request) {
	configuredHost := strings.TrimSpace(w.cfg.ProxyPublicHost)
	hostConfigured := configuredHost != ""

	var host, port string

	if hostConfigured {
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

	apiJSON(rw, http.StatusOK, pullInfoResponse{
		Host:           host,
		Port:           port,
		VirtualTag:     w.cfg.VirtualTag,
		HostConfigured: hostConfigured,
	})
}
