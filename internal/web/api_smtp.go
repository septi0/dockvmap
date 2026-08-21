package web

import "net/http"

func (w *Web) apiSMTPStatus(rw http.ResponseWriter, r *http.Request) {
	enabled := w.cfg.SMTP.Enabled && w.cfg.SMTP.Host != ""

	apiJSON(rw, http.StatusOK, map[string]bool{"enabled": enabled})
}
