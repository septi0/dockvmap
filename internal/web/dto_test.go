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
