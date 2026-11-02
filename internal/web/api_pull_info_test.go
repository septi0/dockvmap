package web

import (
	"net/http/httptest"
	"testing"

	"github.com/septi0/dockvmap/internal/config"
)

func TestProxyAddress(t *testing.T) {
	cases := []struct {
		name           string
		publicHost     string
		requestHost    string
		wantHost       string
		wantPort       string
		wantConfigured bool
	}{
		{
			name:           "configured host takes the listen port",
			publicHost:     "registry.lan",
			requestHost:    "dockvmap.lan:8080",
			wantHost:       "registry.lan",
			wantPort:       "5000",
			wantConfigured: true,
		},
		{
			name:           "configured host with its own port",
			publicHost:     " registry.lan:5050 ",
			requestHost:    "dockvmap.lan:8080",
			wantHost:       "registry.lan",
			wantPort:       "5050",
			wantConfigured: true,
		},
		{
			name:        "unset uses the request host and the listen port",
			requestHost: "dockvmap.lan:8080",
			wantHost:    "dockvmap.lan",
			wantPort:    "5000",
		},
		{
			name:        "unset with a request host without a port",
			requestHost: "dockvmap.lan",
			wantHost:    "dockvmap.lan",
			wantPort:    "5000",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := &Web{cfg: &config.Config{ProxyServerListen: ":5000", ProxyPublicHost: tc.publicHost}}
			r := httptest.NewRequest("GET", "/api/proxy/pull-info", nil)
			r.Host = tc.requestHost

			host, port, configured := w.proxyAddress(r)

			if host != tc.wantHost || port != tc.wantPort || configured != tc.wantConfigured {
				t.Errorf("proxyAddress() = %q, %q, %v; want %q, %q, %v", host, port, configured, tc.wantHost, tc.wantPort, tc.wantConfigured)
			}
		})
	}
}
