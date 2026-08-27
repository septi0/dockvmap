package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/septi0/dockvmap/internal/taganalyzer"
)

func rootOf(family taganalyzer.OrderedFamily, repo analysedRepo) []taganalyzer.SegmentAnalysis {
	best := family.OrderedTags[0]
	for _, tag := range family.OrderedTags {
		if len(repo.segments[tag]) < len(repo.segments[best]) {
			best = tag
		}
	}
	return repo.segments[best]
}

func rootCarriesHash(family taganalyzer.OrderedFamily, repo analysedRepo) bool {
	for _, segment := range rootOf(family, repo) {
		if !segment.IsVariable && looksHashShaped(segment.Raw) {
			return true
		}
	}
	return false
}

func looksHashShaped(value string) bool {
	value = strings.TrimPrefix(value, "g")
	if len(value) < 7 || len(value) > 40 {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func isVersionLed(family taganalyzer.OrderedFamily, repo analysedRepo) bool {
	root := rootOf(family, repo)
	return len(root) > 0 && root[0].OrderType == taganalyzer.OrderVersion
}

func reportRanking(repos []analysedRepo) {
	type flag struct {
		repo   string
		reason string
		chosen string
		better string
	}
	var flags []flag

	for _, repo := range repos {
		if len(repo.analysis.Ordered) == 0 {
			continue
		}
		first := repo.analysis.Ordered[0]

		largestClean := taganalyzer.OrderedFamily{}
		for _, family := range repo.analysis.Ordered {
			if family.TagCount < 2 || !isVersionLed(family, repo) || rootCarriesHash(family, repo) {
				continue
			}
			if family.TagCount > largestClean.TagCount {
				largestClean = family
			}
		}

		describe := func(f taganalyzer.OrderedFamily) string {
			if f.TagCount == 0 {
				return "-"
			}
			return fmt.Sprintf("%s (n=%d)", f.OrderedTags[0], f.TagCount)
		}

		switch {
		case rootCarriesHash(first, repo) && largestClean.TagCount > 0:
			flags = append(flags, flag{repo.name, "commit-built family chosen over a release line", describe(first), describe(largestClean)})
		case !isVersionLed(first, repo) && largestClean.TagCount > 0:
			flags = append(flags, flag{repo.name, "name-led family chosen over a version line", describe(first), describe(largestClean)})
		case largestClean.TagCount > 0 && first.TagCount*20 < largestClean.TagCount:
			flags = append(flags, flag{repo.name, "chosen family is tiny next to the main version line", describe(first), describe(largestClean)})
		}
	}

	sort.Slice(flags, func(i, j int) bool {
		if flags[i].reason != flags[j].reason {
			return flags[i].reason < flags[j].reason
		}
		return flags[i].repo < flags[j].repo
	})

	fmt.Println("=== repositories whose first family looks wrong ===")
	fmt.Printf("%d of %d repositories flagged\n\n", len(flags), len(repos))
	for _, f := range flags {
		fmt.Printf("  %-46s %s\n      chose %-34s expected around %s\n", f.repo, f.reason, f.chosen, f.better)
	}
	if len(flags) == 0 {
		fmt.Println("  none")
	}
	fmt.Println()
}
