package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/septi0/dockvmap/internal/model"
)

type recordedPull struct {
	id  int64
	tag string
}

type recordingUsage struct{ pulls []recordedPull }

func (r *recordingUsage) RecordImagePull(imageID int64, tag string) {
	r.pulls = append(r.pulls, recordedPull{imageID, tag})
}

func TestOnlyManifestGetsCountAsPulls(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	img := &model.Image{ID: 7, Tag: "v2"}

	cases := []struct {
		name, method, ref string
		want              []recordedPull
	}{
		{"get by tag", http.MethodGet, "v2.1", []recordedPull{{7, "v2.1"}}},
		{"get by digest records the current tag", http.MethodGet, digest, []recordedPull{{7, "v2"}}},
		{"head is only a check", http.MethodHead, "v2.1", nil},
	}

	for _, tc := range cases {
		usage := &recordingUsage{}
		p := &Proxy{usage: usage}

		p.recordPull(httptest.NewRequest(tc.method, "/v2/app/manifests/current", nil), img, tc.ref)

		if len(usage.pulls) != len(tc.want) {
			t.Fatalf("%s: pulls = %+v, want %+v", tc.name, usage.pulls, tc.want)
		}

		for i := range tc.want {
			if usage.pulls[i] != tc.want[i] {
				t.Fatalf("%s: pulls = %+v, want %+v", tc.name, usage.pulls, tc.want)
			}
		}
	}
}
