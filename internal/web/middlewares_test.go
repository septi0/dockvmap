package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireAPIHeaderRefusesWritesWithoutIt(t *testing.T) {
	handler := requireAPIHeader(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusNoContent)
	}))

	cases := []struct {
		method string
		header bool
		want   int
	}{
		{http.MethodGet, false, http.StatusNoContent},
		{http.MethodPost, false, http.StatusForbidden},
		{http.MethodPut, false, http.StatusForbidden},
		{http.MethodDelete, false, http.StatusForbidden},
		{http.MethodPost, true, http.StatusNoContent},
		{http.MethodDelete, true, http.StatusNoContent},
	}

	for _, c := range cases {
		r := httptest.NewRequest(c.method, "/images", nil)

		if c.header {
			r.Header.Set(apiRequestHeader, "1")
		}

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, r)

		if rec.Code != c.want {
			t.Errorf("%s with header=%v: status %d, want %d", c.method, c.header, rec.Code, c.want)
		}
	}
}
