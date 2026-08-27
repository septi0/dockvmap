package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/septi0/dockvmap/internal/oci"
	"github.com/septi0/dockvmap/internal/taganalyzer"
	"github.com/septi0/dockvmap/internal/tagfilter"
)

const (
	fetchConcurrency = 6
	fetchTimeout     = 180 * time.Second
)

func corpusPath(corpus string, repo repository) string {
	return filepath.Join(corpus, repo.Registry, repo.Name+".txt")
}

func fetchCorpus(corpus string, repos []repository) error {
	client := oci.NewClient(nil, nil, nil)
	sem := make(chan struct{}, fetchConcurrency)

	var wg sync.WaitGroup
	var mu sync.Mutex
	cached, fetched, failed := 0, 0, 0

	for _, repo := range repos {
		out := corpusPath(corpus, repo)
		if _, err := os.Stat(out); err == nil {
			cached++
			continue
		}

		wg.Add(1)
		go func(repo repository, out string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
			defer cancel()

			tags, err := client.ListTags(ctx, repo.Registry, repo.Name)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed++
				fmt.Fprintf(os.Stderr, "  %s: %v\n", repo, err)
				return
			}
			if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
				failed++
				return
			}
			if err := os.WriteFile(out, []byte(strings.Join(tags, "\n")+"\n"), 0o644); err != nil {
				failed++
				return
			}
			fetched++
		}(repo, out)
	}
	wg.Wait()

	fmt.Printf("cached=%d fetched=%d failed=%d\n", cached, fetched, failed)
	return nil
}

type analysedRepo struct {
	name     string
	tags     []string
	analysis taganalyzer.Analysis
	segments map[string][]taganalyzer.SegmentAnalysis
}

func readTagFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var tags []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if t := strings.TrimSpace(sc.Text()); t != "" {
			tags = append(tags, t)
		}
	}
	return tags, sc.Err()
}

func analyseCorpus(corpus string) ([]analysedRepo, error) {
	filter, err := tagfilter.Load("")
	if err != nil {
		return nil, err
	}

	var files []string
	err = filepath.WalkDir(corpus, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".txt") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)

	var out []analysedRepo
	for _, path := range files {
		tags, err := readTagFile(path)
		if err != nil {
			return nil, err
		}
		tags = filter.Apply(tags)
		if len(tags) == 0 {
			continue
		}

		analysis := taganalyzer.Analyze(tags)
		segments := make(map[string][]taganalyzer.SegmentAnalysis, len(analysis.Tags))
		for i := range analysis.Tags {
			segments[analysis.Tags[i].Tag] = analysis.Tags[i].Segments
		}

		out = append(out, analysedRepo{
			name:     strings.TrimPrefix(strings.TrimSuffix(path, ".txt"), corpus+"/"),
			tags:     tags,
			analysis: analysis,
			segments: segments,
		})
	}
	return out, nil
}
