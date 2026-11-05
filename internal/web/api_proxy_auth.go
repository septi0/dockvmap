package web

import "net/http"

func (w *Web) apiProxyAuthStatus(rw http.ResponseWriter, r *http.Request) {
	apiJSON(rw, http.StatusOK, map[string]bool{
		"enabled":          w.cfg.ProxyAuth.Enabled,
		"internalRegistry": w.internalRegistry.Address() != "",
		"pushRestricted":   len(w.cfg.InternalRegistry.PushAllowedIPs) > 0,
	})
}
