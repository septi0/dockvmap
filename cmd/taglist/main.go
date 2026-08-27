package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/septi0/dockvmap/internal/oci"
	"github.com/septi0/dockvmap/internal/taganalyzer"
	"github.com/septi0/dockvmap/internal/tagfilter"
)

func main() {
	registry := flag.String("registry", "", "registry host, e.g. docker.io or ghcr.io")
	repository := flag.String("repository", "", "repository path, e.g. library/nginx or org/repo")
	flag.Parse()

	if *registry == "" || *repository == "" {
		fmt.Fprintln(os.Stderr, "usage: tagdebug -registry <host> -repository <path>")
		os.Exit(2)
	}

	client := oci.NewClient(nil, nil, nil)

	tags, err := client.ListTags(context.Background(), *registry, *repository)
	if err != nil {
		fmt.Fprintf(os.Stderr, "listing tags: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("fetched %d tags from %s/%s\n\n", len(tags), *registry, *repository)

	filter, err := tagfilter.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "loading tag filters: %v\n", err)
		os.Exit(1)
	}
	tags = filter.Apply(tags)

	fmt.Printf("%d tags after filtering\n\n", len(tags))

	analysis := taganalyzer.Analyze(tags)

	printFamilies(analysis)
}

func printFamilies(analysis taganalyzer.Analysis) {
	fmt.Println("=== families (ordered, newest first) ===")
	for _, family := range analysis.Ordered {
		fmt.Printf("\nfamily #%d [%s] key=%q tags=%d\n", family.ID, family.Kind, family.Key, family.TagCount)
		for i, tag := range family.OrderedTags {
			fmt.Printf("  %2d. %s\n", i+1, tag)
		}
	}
	fmt.Println()
}
