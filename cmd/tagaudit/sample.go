package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type repository struct {
	Registry string
	Name     string
}

func (r repository) String() string { return r.Registry + "/" + r.Name }

// deliberately broad and arbitrary — an unbiased sample, not a curated one
var searchTerms = []string{
	"server", "api", "database", "proxy", "agent", "worker", "gateway", "monitor",
	"cache", "queue", "search", "auth", "backup", "build", "cli", "dashboard",
	"exporter", "gitops", "ingest", "kernel", "logging", "mail", "metrics", "node",
	"operator", "python", "runtime", "storage", "sync", "tools", "web", "sdk",
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

func getJSON(url string, into any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", url, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(into)
}

func officialImages() ([]repository, error) {
	var repos []repository
	url := "https://hub.docker.com/v2/repositories/library/?page_size=100"

	for url != "" {
		var page struct {
			Next    string `json:"next"`
			Results []struct {
				Name string `json:"name"`
			} `json:"results"`
		}
		if err := getJSON(url, &page); err != nil {
			return repos, err
		}
		for _, r := range page.Results {
			repos = append(repos, repository{Registry: "docker.io", Name: "library/" + r.Name})
		}
		url = page.Next
	}
	return repos, nil
}

func searchImages(term string, pages int) ([]repository, error) {
	var repos []repository
	for page := 1; page <= pages; page++ {
		var result struct {
			Results []struct {
				Name string `json:"repo_name"`
			} `json:"results"`
		}
		url := fmt.Sprintf("https://hub.docker.com/v2/search/repositories/?query=%s&page_size=100&page=%d", term, page)
		if err := getJSON(url, &result); err != nil {
			return repos, err
		}
		if len(result.Results) == 0 {
			break
		}
		for _, r := range result.Results {
			name := r.Name
			if !strings.Contains(name, "/") {
				name = "library/" + name
			}
			repos = append(repos, repository{Registry: "docker.io", Name: name})
		}
	}
	return repos, nil
}

func sampleRepositories(want int) ([]repository, error) {
	seen := map[string]bool{}
	var pool []repository

	add := func(repos []repository) {
		for _, r := range repos {
			if seen[r.String()] {
				continue
			}
			seen[r.String()] = true
			pool = append(pool, r)
		}
	}

	official, err := officialImages()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  official images: %v\n", err)
	}
	add(official)
	fmt.Fprintf(os.Stderr, "official images: %d\n", len(official))

	for _, term := range searchTerms {
		found, err := searchImages(term, 2)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  search %q: %v\n", term, err)
			continue
		}
		add(found)
	}
	fmt.Fprintf(os.Stderr, "pool after search: %d repositories\n", len(pool))

	// Every official image stays in; the rest is a random draw, not a curated pick.
	chosen := append([]repository(nil), official...)
	rest := pool[len(official):]
	rand.New(rand.NewSource(time.Now().UnixNano())).Shuffle(len(rest), func(i, j int) {
		rest[i], rest[j] = rest[j], rest[i]
	})
	for _, r := range rest {
		if len(chosen) >= want {
			break
		}
		chosen = append(chosen, r)
	}

	sort.Slice(chosen, func(i, j int) bool { return chosen[i].String() < chosen[j].String() })
	return chosen, nil
}

func manifestPath(corpus string) string { return filepath.Join(corpus, "audit-manifest.tsv") }

func writeManifest(corpus string, repos []repository) error {
	if err := os.MkdirAll(corpus, 0o755); err != nil {
		return err
	}
	f, err := os.Create(manifestPath(corpus))
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintln(f, "registry\trepository")
	for _, r := range repos {
		fmt.Fprintf(f, "%s\t%s\n", r.Registry, r.Name)
	}
	fmt.Printf("wrote %d repositories to %s\n", len(repos), manifestPath(corpus))
	return nil
}

func readManifest(corpus string) ([]repository, error) {
	f, err := os.Open(manifestPath(corpus))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var repos []repository
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		parts := strings.Split(strings.TrimSpace(sc.Text()), "\t")
		if len(parts) != 2 || parts[0] == "registry" {
			continue
		}
		repos = append(repos, repository{Registry: parts[0], Name: parts[1]})
	}
	return repos, sc.Err()
}
