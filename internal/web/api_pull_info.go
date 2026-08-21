package web

import (
	"net"
	"net/http"
)

func (w *Web) apiProxyPullInfo(rw http.ResponseWriter, r *http.Request) {
	host, _, err := net.SplitHostPort(r.Host)

	if err != nil {
		host = r.Host
	}

	_, port, err := net.SplitHostPort(w.cfg.ProxyServerListen)

	if err != nil {
		port = w.cfg.ProxyServerListen
	}

	apiJSON(rw, http.StatusOK, pullInfoResponse{
		Host:       host,
		Port:       port,
		VirtualTag: w.cfg.VirtualTag,
	})
}
