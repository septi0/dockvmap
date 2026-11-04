package proxy

import "testing"

func TestParseRegistryPath(t *testing.T) {
	cases := []struct {
		path string
		want registryPath
		ok   bool
	}{
		{"/v2/web/manifests/current", registryPath{name: "web", kind: pathManifest, ref: "current"}, true},
		{"/v2/team/sub/web/manifests/sha256:abc", registryPath{name: "team/sub/web", kind: pathManifest, ref: "sha256:abc"}, true},
		{"/v2/team/web/blobs/sha256:abc", registryPath{name: "team/web", kind: pathBlob, ref: "sha256:abc"}, true},
		{"/v2/team/web/blobs/uploads/", registryPath{name: "team/web", kind: pathUploads}, true},
		{"/v2/team/web/blobs/uploads/0f1e-uuid", registryPath{name: "team/web", kind: pathUpload, ref: "0f1e-uuid"}, true},
		{"/v2/team/web/tags/list", registryPath{name: "team/web", kind: pathTags}, true},
		{"/v2/team/web/referrers/sha256:abc", registryPath{name: "team/web", kind: pathReferrers, ref: "sha256:abc"}, true},
		{"/v2/_catalog", registryPath{kind: pathCatalog}, true},
		{"/v2/", registryPath{}, false},
		{"/v2/web", registryPath{}, false},
		{"/v2/web/manifests/", registryPath{}, false},
		{"/v2/manifests/current", registryPath{}, false},
		{"/v2/blobs/uploads/", registryPath{}, false},
		{"/v2/team/web/unknown/thing", registryPath{}, false},
		{"/v2/team/web/tags/other", registryPath{}, false},
		{"/debug/vars", registryPath{}, false},
	}

	for _, tc := range cases {
		got, ok := parseRegistryPath(tc.path)

		if ok != tc.ok || got != tc.want {
			t.Errorf("parseRegistryPath(%q) = %+v, %v; want %+v, %v", tc.path, got, ok, tc.want, tc.ok)
		}
	}
}
