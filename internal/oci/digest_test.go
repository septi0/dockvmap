package oci

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

type noCredentials struct{}

func (noCredentials) GetRegistryCredentials(context.Context, string) (*Credentials, error) {
	return nil, nil
}

type plainHTTP struct{}

func (plainHTTP) GetRegistryOptions(context.Context, string) (*RegistryOptions, error) {
	return &RegistryOptions{Insecure: true}, nil
}

const indexDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func digestServer(t *testing.T, handler http.HandlerFunc) (*Client, string) {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return NewClient(nil, noCredentials{}, plainHTTP{}), strings.TrimPrefix(server.URL, "http://")
}

func TestManifestDigestSendsHeadWithIndexMediaTypes(t *testing.T) {
	var method, path string
	var accept []string

	client, registry := digestServer(t, func(w http.ResponseWriter, r *http.Request) {
		method, path, accept = r.Method, r.URL.Path, r.Header.Values("Accept")
		w.Header().Set("Docker-Content-Digest", indexDigest)
	})

	digest, err := client.ManifestDigest(context.Background(), registry, "stack/web", "v2.7")

	if err != nil || digest != indexDigest {
		t.Fatalf("digest = %q, err = %v", digest, err)
	}

	if method != http.MethodHead || path != "/v2/stack/web/manifests/v2.7" {
		t.Fatalf("request = %s %s", method, path)
	}

	for _, mediaType := range []string{
		"application/vnd.oci.image.index.v1+json",
		"application/vnd.docker.distribution.manifest.list.v2+json",
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
	} {
		if !slices.Contains(accept, mediaType) {
			t.Errorf("Accept is missing %s: %v", mediaType, accept)
		}
	}
}

func TestManifestDigestResults(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		header    string
		want      string
		wantErr   bool
		errStatus int
	}{
		{name: "sha256", status: http.StatusOK, header: indexDigest, want: indexDigest},
		{name: "sha512 is not accepted", status: http.StatusOK, header: "sha512:" + strings.Repeat("ab", 64), wantErr: true},
		{name: "no header is unknown", status: http.StatusOK},
		{name: "malformed", status: http.StatusOK, header: "sha256:xyz", wantErr: true},
		{name: "uppercase hex", status: http.StatusOK, header: strings.ToUpper(indexDigest), wantErr: true},
		{name: "injection attempt", status: http.StatusOK, header: indexDigest + "\"><script>", wantErr: true},
		{name: "tag gone", status: http.StatusNotFound, wantErr: true, errStatus: http.StatusNotFound},
		{name: "forbidden", status: http.StatusForbidden, wantErr: true, errStatus: http.StatusForbidden},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, registry := digestServer(t, func(w http.ResponseWriter, r *http.Request) {
				if tc.header != "" {
					w.Header().Set("Docker-Content-Digest", tc.header)
				}

				w.WriteHeader(tc.status)
			})

			digest, err := client.ManifestDigest(context.Background(), registry, "stack/web", "latest")

			if (err != nil) != tc.wantErr || digest != tc.want {
				t.Fatalf("digest = %q, err = %v", digest, err)
			}

			var registryErr *Error

			if tc.errStatus != 0 && (!errors.As(err, &registryErr) || registryErr.StatusCode != tc.errStatus) {
				t.Fatalf("err = %v, want registry status %d", err, tc.errStatus)
			}
		})
	}
}

func TestManifestDigestEscapesReference(t *testing.T) {
	var rawPath string

	client, registry := digestServer(t, func(w http.ResponseWriter, r *http.Request) {
		rawPath = r.URL.EscapedPath()
		w.Header().Set("Docker-Content-Digest", indexDigest)
	})

	if _, err := client.ManifestDigest(context.Background(), registry, "stack/web", "../../other"); err != nil {
		t.Fatalf("lookup: %v", err)
	}

	if rawPath != "/v2/stack/web/manifests/..%2F..%2Fother" {
		t.Fatalf("path = %s", rawPath)
	}
}

func TestIsDigest(t *testing.T) {
	cases := map[string]bool{
		indexDigest:                          true,
		strings.ToUpper(indexDigest):         false,
		"sha256:" + indexDigest[7:70]:        false,
		"sha256:" + strings.Repeat("g", 64):  false,
		"sha512:" + strings.Repeat("a", 128): false,
		indexDigest[7:]:                      false,
		"":                                   false,
	}

	for digest, want := range cases {
		if got := IsDigest(digest); got != want {
			t.Errorf("IsDigest(%q) = %v, want %v", digest, got, want)
		}
	}
}
