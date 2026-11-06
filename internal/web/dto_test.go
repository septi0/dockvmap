package web

import (
	"testing"
	"time"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/service"
)

func TestImageResponseShowsDigestOnlyForTheCurrentTag(t *testing.T) {
	digest := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	since := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	current, previous := "v2.7", "v2.6"

	cases := []struct {
		name      string
		digestTag *string
		want      bool
	}{
		{name: "current tag", digestTag: &current, want: true},
		{name: "previous tag", digestTag: &previous, want: false},
		{name: "never looked up", digestTag: nil, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			image := model.Image{Tag: "v2.7", CurrentDigest: &digest, CurrentDigestTag: tc.digestTag, CurrentDigestSince: &since}
			response := newImageResponse(image)

			if got := response.CurrentDigest != nil && response.CurrentDigestSince != nil; got != tc.want {
				t.Fatalf("digest shown = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFailureMessageNamesTheImageForDigestLookups(t *testing.T) {
	failure := service.Failure{Source: service.FailureSourceDigest, Detail: "web", Error: "registry returned 404 Not Found"}

	if got := failureMessage(failure); got != `Image "web" digest lookup failed: registry returned 404 Not Found` {
		t.Fatalf("message = %q", got)
	}
}

func TestFailureMessagesSayTheCauseInPlainWords(t *testing.T) {
	cases := []struct {
		name    string
		failure service.Failure
		want    string
	}{
		{"unknown host", service.Failure{Source: service.FailureSourceRefresh, Detail: "myapp", Error: `Get "https://registry.example.com/v2/apps/myapp/tags/list?n=1000": dial tcp: lookup registry.example.com: no such host`},
			"myapp: registry.example.com couldn't be found. Check its address."},
		{"smtp timeout", service.Failure{Source: service.FailureSourceEmail, Error: "dialing smtp server 192.0.2.25:587: dial tcp 192.0.2.25:587: i/o timeout"},
			"Email not sent: the SMTP server 192.0.2.25:587 didn't answer. Check that it's running and reachable."},
		{"smtp login", service.Failure{Source: service.FailureSourceEmail, Error: "authenticating: 535 5.7.8 bad credentials"},
			"Email not sent: the SMTP server refused the login. Check the smtp settings."},
		{"webhook refused", service.Failure{Source: service.FailureSourceWebhook, Detail: "https://hooks.example.com/x", Error: `sending webhook: Post "https://hooks.example.com/x": dial tcp 10.0.0.9:443: connect: connection refused`},
			"Webhook not sent: hooks.example.com refused the connection. Check that it's running."},
		{"registry login", service.Failure{Source: service.FailureSourceDiscovery, Detail: "team/web", Error: "registry returned 401 Unauthorized"},
			"Tag discovery for team/web: the registry refused the login. Check the login."},
		{"unknown cause keeps the raw text", service.Failure{Source: service.FailureSourceDigest, Detail: "web", Error: "registry returned 404 Not Found"},
			`Image "web" digest lookup failed: registry returned 404 Not Found`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := failureMessage(tc.failure); got != tc.want {
				t.Errorf("message = %q, want %q", got, tc.want)
			}
		})
	}
}
