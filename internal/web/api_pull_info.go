package web

import (
	"net"
	"net/http"

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
	if address := w.cfg.ProxyPublicAddress(); address != "" {
		host, port, _ = net.SplitHostPort(address)

		return host, port, true
	}

	host, _, err := net.SplitHostPort(r.Host)

	if err != nil {
		host = r.Host
	}

	return host, w.cfg.ProxyListenPort(), false
}
