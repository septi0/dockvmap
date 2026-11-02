package agentapi

import "testing"

const digest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestParseImageRef(t *testing.T) {
	cases := []struct {
		ref  string
		want ImageRef
	}{
		{"docker-registry.uscrib.ro:5050/nextcloud_collabora:current", ImageRef{Host: "docker-registry.uscrib.ro:5050", Repository: "nextcloud_collabora", Tag: "current"}},
		{"Registry.Example.com/team/web:v2.7", ImageRef{Host: "registry.example.com", Repository: "team/web", Tag: "v2.7"}},
		{"localhost/web", ImageRef{Host: "localhost", Repository: "web", Tag: "latest"}},
		{"127.0.0.1:5000/web@" + digest, ImageRef{Host: "127.0.0.1:5000", Repository: "web", Digest: digest}},
		{"127.0.0.1:5000/web:v1@" + digest, ImageRef{Host: "127.0.0.1:5000", Repository: "web", Tag: "v1", Digest: digest}},
		{"nginx", ImageRef{Host: "docker.io", Repository: "library/nginx", Tag: "latest"}},
		{"nginx:1.27", ImageRef{Host: "docker.io", Repository: "library/nginx", Tag: "1.27"}},
		{"portainer/portainer-ce:latest", ImageRef{Host: "docker.io", Repository: "portainer/portainer-ce", Tag: "latest"}},
		{digest, ImageRef{}},
		{"", ImageRef{}},
	}

	for _, tc := range cases {
		if got := ParseImageRef(tc.ref); got != tc.want {
			t.Errorf("ParseImageRef(%q) = %+v, want %+v", tc.ref, got, tc.want)
		}
	}
}

func TestImageRefOnHost(t *testing.T) {
	cases := []struct {
		ref  string
		host string
		want bool
	}{
		{"docker-registry.uscrib.ro:5050/web:current", "docker-registry.uscrib.ro:5050", true},
		{"docker-registry.uscrib.ro:5050/web:current", "Docker-Registry.uscrib.ro:5050", true},
		{"docker-registry.uscrib.ro:5050/web:current", "docker-registry.uscrib.ro:5000", false},
		{"registry.example.com/web:current", "registry.example.com:443", true},
		{"registry.example.com:443/web:current", "registry.example.com:443", true},
		{"registry.example.com/web:current", "registry.example.com:5000", false},
		{"web:current", "docker.io", true},
		{"web:current", "", false},
		{digest, "docker.io", false},
	}

	for _, tc := range cases {
		if got := ParseImageRef(tc.ref).OnHost(tc.host); got != tc.want {
			t.Errorf("%q on %q = %v, want %v", tc.ref, tc.host, got, tc.want)
		}
	}
}
