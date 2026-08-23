package web

import "net/http"

func (w *Web) apiSMTPStatus(rw http.ResponseWriter, r *http.Request) {
	apiJSON(rw, http.StatusOK, map[string]bool{"enabled": w.cfg.SMTP.Enabled})
}
