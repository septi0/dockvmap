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

type recordedDigest struct {
	id          int64
	tag, digest string
}

type recordingUsage struct {
	pulls   []recordedPull
	digests []recordedDigest
}

func (r *recordingUsage) RecordImagePull(imageID int64, tag string) {
	r.pulls = append(r.pulls, recordedPull{imageID, tag})
}

func (r *recordingUsage) RecordTagDigest(imageID int64, tag, digest string) {
	r.digests = append(r.digests, recordedDigest{imageID, tag, digest})
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

func TestTagRequestsRecordTheServedDigest(t *testing.T) {
	stored := "sha256:" + strings.Repeat("a", 64)
	served := "sha256:" + strings.Repeat("b", 64)
	storedTag := "v2"
	img := &model.Image{ID: 7, Tag: "v2", CurrentDigest: &stored, CurrentDigestTag: &storedTag}

	index := "application/vnd.oci.image.index.v1+json"
	dockerAccept := "application/vnd.docker.distribution.manifest.v2+json, application/vnd.docker.distribution.manifest.list.v2+json;q=0.9"
	manifestOnly := "application/vnd.docker.distribution.manifest.v2+json"

	cases := []struct {
		name, method, ref, accept, digest string
		want                              bool
	}{
		{"head by tag with a new digest", http.MethodHead, "v2", index, served, true},
		{"get by tag with docker's accept list", http.MethodGet, "v2", dockerAccept, served, true},
		{"unchanged digest", http.MethodHead, "v2", index, stored, false},
		{"request by digest", http.MethodGet, served, index, served, false},
		{"another tag", http.MethodHead, "v1", index, served, false},
		{"client that can't take an index", http.MethodHead, "v2", manifestOnly, served, false},
		{"no digest from the registry", http.MethodHead, "v2", index, "", false},
	}

	for _, tc := range cases {
		usage := &recordingUsage{}
		p := &Proxy{usage: usage}

		r := httptest.NewRequest(tc.method, "/v2/app/manifests/current", nil)
		r.Header.Set("Accept", tc.accept)

		w := httptest.NewRecorder()

		if tc.digest != "" {
			w.Header().Set("Docker-Content-Digest", tc.digest)
		}

		p.recordTagDigest(w, r, img, tc.ref)

		want := []recordedDigest(nil)

		if tc.want {
			want = []recordedDigest{{7, "v2", tc.digest}}
		}

		if len(usage.digests) != len(want) || (len(want) == 1 && usage.digests[0] != want[0]) {
			t.Errorf("%s: digests = %+v, want %+v", tc.name, usage.digests, want)
		}
	}
}
