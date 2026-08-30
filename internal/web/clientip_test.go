package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/septi0/dockvmap/internal/ipmatch"
)

func TestResolveClientIP(t *testing.T) {
	cases := []struct {
		name       string
		remoteAddr string
		trusted    []string
		xff        string
		want       string
	}{
		{
			name:       "no trusted proxies configured, XFF ignored",
			remoteAddr: "10.0.0.5:1234",
			xff:        "1.2.3.4",
			want:       "10.0.0.5",
		},
		{
			name:       "trusted peer, no XFF",
			remoteAddr: "10.0.0.5:1234",
			trusted:    []string{"10.0.0.0/8"},
			want:       "10.0.0.5",
		},
		{
			name:       "trusted peer, single-hop XFF",
			remoteAddr: "10.0.0.5:1234",
			trusted:    []string{"10.0.0.0/8"},
			xff:        "1.2.3.4",
			want:       "1.2.3.4",
		},
		{
			name:       "walks right-to-left skipping trusted hops",
			remoteAddr: "10.0.0.5:1234",
			trusted:    []string{"10.0.0.0/8"},
			xff:        "1.2.3.4, 10.0.0.9",
			want:       "1.2.3.4",
		},
		{
			name:       "returns first untrusted hop from the right",
			remoteAddr: "10.0.0.5:1234",
			trusted:    []string{"10.0.0.0/8"},
			xff:        "1.2.3.4, 9.9.9.9",
			want:       "9.9.9.9",
		},
		{
			name:       "untrusted peer, self-supplied XFF rejected",
			remoteAddr: "9.9.9.9:1234",
			trusted:    []string{"10.0.0.0/8"},
			xff:        "1.2.3.4",
			want:       "9.9.9.9",
		},
		{
			name:       "unparseable hop to the left of a good one is fine",
			remoteAddr: "10.0.0.5:1234",
			trusted:    []string{"10.0.0.0/8"},
			xff:        "bad, 1.2.3.4",
			want:       "1.2.3.4",
		},
		{
			name:       "unparseable rightmost hop falls back to the peer",
			remoteAddr: "10.0.0.5:1234",
			trusted:    []string{"10.0.0.0/8"},
			xff:        "1.2.3.4, bad",
			want:       "10.0.0.5",
		},
		{
			name:       "all hops trusted falls back to the peer",
			remoteAddr: "10.0.0.5:1234",
			trusted:    []string{"10.0.0.0/8"},
			xff:        "10.0.0.1, 10.0.0.2",
			want:       "10.0.0.5",
		},
		{
			name:       "RemoteAddr without a port",
			remoteAddr: "10.0.0.5",
			trusted:    []string{"10.0.0.0/8"},
			xff:        "1.2.3.4",
			want:       "1.2.3.4",
		},
		{
			name:       "IPv6 peer and trusted range",
			remoteAddr: "[2001:db8::1]:443",
			trusted:    []string{"2001:db8::/32"},
			xff:        "1.2.3.4",
			want:       "1.2.3.4",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trusted, err := ipmatch.Parse(tc.trusted)
			if err != nil {
				t.Fatalf("ipmatch.Parse(%v): %v", tc.trusted, err)
			}

			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.remoteAddr
			if tc.xff != "" {
				r.Header.Set("X-Forwarded-For", tc.xff)
			}

			if got := resolveClientIP(r, trusted); got != tc.want {
				t.Errorf("resolveClientIP(remote=%q, xff=%q) = %q; want %q",
					tc.remoteAddr, tc.xff, got, tc.want)
			}
		})
	}
}
