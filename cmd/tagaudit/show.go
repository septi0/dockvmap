package main

import (
	"fmt"
	"strings"
)

func reportRepository(repos []analysedRepo, name string) {
	for _, repo := range repos {
		if repo.name != name {
			continue
		}

		fmt.Printf("%s: %d tags, %d families\n\n", repo.name, len(repo.tags), len(repo.analysis.Ordered))
		for i, family := range repo.analysis.Ordered {
			if i >= 20 {
				fmt.Printf("... %d more families\n", len(repo.analysis.Ordered)-20)
				break
			}
			fmt.Printf("[%-8s] n=%-5d order=%-5v %s\n", family.Kind, family.TagCount, family.HasOrder, family.Key)
			for j, tag := range family.OrderedTags {
				if j >= 4 {
					fmt.Printf("        ... %d more\n", len(family.OrderedTags)-4)
					break
				}
				fmt.Printf("        %s\n", tag)
			}
		}
		return
	}

	var names []string
	for _, repo := range repos {
		names = append(names, repo.name)
	}
	fmt.Printf("no repository %q in the corpus; it holds %d (e.g. %s)\n", name, len(repos), strings.Join(names[:min(3, len(names))], ", "))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
