package oci

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
)

type staticCredentials struct{ username, password string }

func (c staticCredentials) GetRegistryCredentials(context.Context, string) (*Credentials, error) {
	return &Credentials{Username: c.username, Credential: c.password}, nil
}

func catalogPages(t *testing.T, pages [][]string) (*Client, string, *[]string) {
	t.Helper()

	var requests []string

	client, host := digestServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RequestURI())

		page := 0
		fmt.Sscanf(r.URL.Query().Get("page"), "%d", &page)

		if page+1 < len(pages) {
			w.Header().Set("Link", fmt.Sprintf(`</v2/_catalog?n=%s&page=%d>; rel="next"`, r.URL.Query().Get("n"), page+1))
		}

		fmt.Fprintf(w, `{"repositories":["%s"]}`, strings.Join(pages[page], `","`))
	})

	return client, host, &requests
}

func TestListRepositoriesFollowsPagesUpToTheLimit(t *testing.T) {
	pages := [][]string{{"a/one", "a/two"}, {"b/one", "b/two"}, {"c/one"}}

	client, host, requests := catalogPages(t, pages)
	got, more, err := client.ListRepositories(context.Background(), host, 100)

	if err != nil || more || !slices.Equal(got, []string{"a/one", "a/two", "b/one", "b/two", "c/one"}) {
		t.Fatalf("all pages: got %v, more %v, err %v", got, more, err)
	}

	if (*requests)[0] != "/v2/_catalog?n=100" {
		t.Errorf("first request = %q, want n=100", (*requests)[0])
	}

	client, host, requests = catalogPages(t, pages)
	got, more, err = client.ListRepositories(context.Background(), host, 3)

	if err != nil || !more || !slices.Equal(got, []string{"a/one", "a/two", "b/one"}) {
		t.Fatalf("limit 3: got %v, more %v, err %v", got, more, err)
	}

	if len(*requests) != 2 {
		t.Errorf("limit 3 made %d requests, want 2", len(*requests))
	}

	client, host, _ = catalogPages(t, [][]string{{"a/one", "a/two"}})

	if got, more, err := client.ListRepositories(context.Background(), host, 2); err != nil || more || len(got) != 2 {
		t.Errorf("exactly the limit: got %v, more %v, err %v", got, more, err)
	}
}

func TestListRepositoriesLogsInWithTheCatalogScope(t *testing.T) {
	var host string

	client, address := digestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			user, password, ok := r.BasicAuth()

			if !ok || user != "admin" || password != "secret" || r.URL.Query().Get("scope") != "registry:catalog:*" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			io.WriteString(w, `{"token":"catalog-token"}`)

		case "/v2/_catalog":
			if r.Header.Get("Authorization") != "Bearer catalog-token" {
				w.Header().Set("Www-Authenticate", fmt.Sprintf(`Bearer realm="http://%s/token",service="registry",scope="registry:catalog:*"`, host))
				w.WriteHeader(http.StatusUnauthorized)

				return
			}

			io.WriteString(w, `{"repositories":["team/web"]}`)
		}
	})

	host = address
	client.credentials = staticCredentials{"admin", "secret"}

	got, _, err := client.ListRepositories(context.Background(), host, 1000)

	if err != nil || !slices.Equal(got, []string{"team/web"}) {
		t.Fatalf("got %v, err %v; want team/web after logging in", got, err)
	}
}

func TestListRepositoriesReportsARefusal(t *testing.T) {
	client, host := digestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_, _, err := client.ListRepositories(context.Background(), host, 1)

	var registryErr *Error

	if !errors.As(err, &registryErr) || registryErr.StatusCode != http.StatusNotFound {
		t.Errorf("error = %v, want a registry error with 404", err)
	}
}
