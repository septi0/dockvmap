package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/septi0/dockvmap/internal/config"
)

type fakeInternalRegistry struct {
	address string
}

func (f fakeInternalRegistry) Address() string { return f.address }

func TestProxyAuthStatusReportsTheInternalRegistry(t *testing.T) {
	for _, address := range []string{"", "registry:5000"} {
		cfg := &config.Config{}
		cfg.ProxyAuth.Enabled = true
		w := &Web{cfg: cfg, internalRegistry: fakeInternalRegistry{address: address}}
		rec := httptest.NewRecorder()

		w.apiProxyAuthStatus(rec, httptest.NewRequest(http.MethodGet, "/api/proxy-auth", nil))

		var status map[string]bool

		if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
			t.Fatalf("body %q: %v", rec.Body.String(), err)
		}

		if !status["enabled"] || status["internalRegistry"] != (address != "") {
			t.Errorf("address %q: status = %v, want enabled and internalRegistry %v", address, status, address != "")
		}
	}
}
