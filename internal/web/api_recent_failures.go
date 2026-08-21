package web

import "net/http"

func (w *Web) apiRecentFailures(rw http.ResponseWriter, r *http.Request) {
	failures := w.failures.Recent()

	responses := make([]recentFailureResponse, 0, len(failures))

	for _, failure := range failures {
		responses = append(responses, newRecentFailureResponse(failure))
	}

	apiJSON(rw, http.StatusOK, responses)
}
