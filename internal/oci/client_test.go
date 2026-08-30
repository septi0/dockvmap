package oci

import (
	"net/url"
	"testing"
)

func TestNextPageURL(t *testing.T) {
	const baseRaw = "https://registry-1.docker.io/v2/library/nginx/tags/list?n=1000"

	base, err := url.Parse(baseRaw)
	if err != nil {
		t.Fatalf("parsing base URL: %v", err)
	}

	cases := []struct {
		name string
		link string
		want string
	}{
		{
			name: "empty Link header",
			link: "",
			want: "",
		},
		{
			name: "absolute path with rel=next",
			link: `</v2/library/nginx/tags/list?n=1000&last=1.25>; rel="next"`,
			want: "https://registry-1.docker.io/v2/library/nginx/tags/list?n=1000&last=1.25",
		},
		{
			name: "rel=prev is not followed",
			link: `</v2/library/nginx/tags/list?last=x>; rel="prev"`,
			want: "",
		},
		{
			name: "cross-host next is rejected (SSRF guard)",
			link: `<https://evil.example/v2/x/tags/list>; rel="next"`,
			want: "",
		},
		{
			name: "non-http(s) scheme is rejected",
			link: `<ftp://registry-1.docker.io/x>; rel="next"`,
			want: "",
		},
		{
			name: "picks the next entry out of several",
			link: `</a>; rel="prev", </v2/library/nginx/tags/list?last=y>; rel="next"`,
			want: "https://registry-1.docker.io/v2/library/nginx/tags/list?last=y",
		},
		{
			name: "comma inside angle brackets is not a value separator",
			link: `</v2/x?a=1,2>; rel="next"`,
			want: "https://registry-1.docker.io/v2/x?a=1,2",
		},
		{
			name: "relative reference resolves against the base path (last segment replaced)",
			link: `<tags/list?last=z>; rel="next"`,
			want: "https://registry-1.docker.io/v2/library/nginx/tags/tags/list?last=z",
		},
		{
			name: "protocol-relative reference inherits the base scheme",
			link: `<//registry-1.docker.io/v2/x>; rel="next"`,
			want: "https://registry-1.docker.io/v2/x",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextPageURL(base, tc.link); got != tc.want {
				t.Errorf("nextPageURL(%q) = %q; want %q", tc.link, got, tc.want)
			}
		})
	}
}
