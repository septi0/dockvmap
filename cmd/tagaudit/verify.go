package main

import (
	"fmt"
	"math/rand"
	"strings"

	"github.com/septi0/dockvmap/internal/taganalyzer"
)

// every counter reported here must stay at zero
func reportInvariants(repos []analysedRepo) {
	var (
		tags, families, multi, grouped   int
		lost, duplicated, collidingIDs   int
		literalViolations, inversions    int
		orderable, nonDeterministic      int
		missedGrouping, correctSingleton int
	)

	for _, repo := range repos {
		tags += len(repo.tags)
		families += len(repo.analysis.Ordered)

		placed := map[string]int{}
		ids := map[int64]int{}
		for _, family := range repo.analysis.Ordered {
			ids[family.ID]++
			for _, tag := range family.OrderedTags {
				placed[tag]++
			}
		}
		for _, count := range ids {
			if count > 1 {
				collidingIDs++
			}
		}
		for _, tag := range repo.tags {
			if placed[tag] == 0 {
				lost++
			}
		}
		for _, count := range placed {
			if count > 1 {
				duplicated++
			}
		}

		if !sameAnalysis(repo) {
			nonDeterministic++
			fmt.Printf("  NON-DETERMINISTIC %s\n", repo.name)
		}

		cardinality := cardinalityOf(repo)
		for _, family := range repo.analysis.Ordered {
			if family.TagCount == 1 {
				tag := family.OrderedTags[0]
				blocking, ok := blockingSegment(repo, tag, cardinality)
				if ok && looksHashShaped(blocking) {
					missedGrouping++
				} else {
					correctSingleton++
				}
				continue
			}

			multi++
			grouped += family.TagCount

			if crossesLiteralAxis(family, repo) {
				literalViolations++
				fmt.Printf("  LITERAL-AXIS %s %s\n", repo.name, family.Key)
			}
			if versionLedThroughout(family, repo) {
				orderable++
				if inverted(family, repo) {
					inversions++
					fmt.Printf("  INVERSION %s %s\n", repo.name, family.Key)
				}
			}
		}
	}

	fmt.Println("=== invariants ===")
	fmt.Printf("%d repositories, %d tags, %d families (%d with 2+ tags)\n\n", len(repos), tags, families, multi)
	fmt.Printf("  tags lost                      %d\n", lost)
	fmt.Printf("  tags in more than one family   %d\n", duplicated)
	fmt.Printf("  colliding family IDs           %d\n", collidingIDs)
	fmt.Printf("  literal-axis violations        %d / %d families\n", literalViolations, multi)
	fmt.Printf("  ordering inversions            %d / %d families\n", inversions, orderable)
	fmt.Printf("  non-deterministic repositories %d\n\n", nonDeterministic)
	fmt.Printf("  grouped into a 2+ family       %d (%.2f%%)\n", grouped, percent(grouped, tags))
	fmt.Printf("  correct singletons             %d\n", correctSingleton)
	fmt.Printf("  should have grouped            %d\n", missedGrouping)
	fmt.Printf("  tag placement accuracy         %.2f%%\n\n", percent(tags-missedGrouping, tags))
}

func percent(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return 100 * float64(a) / float64(b)
}

// shuffled re-run: input order must not change the result
func sameAnalysis(repo analysedRepo) bool {
	shuffled := append([]string(nil), repo.tags...)
	rand.New(rand.NewSource(1)).Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})
	return fingerprint(repo.analysis) == fingerprint(taganalyzer.Analyze(shuffled))
}

func fingerprint(analysis taganalyzer.Analysis) string {
	var b strings.Builder
	for _, family := range analysis.Ordered {
		b.WriteString(family.Key)
		b.WriteByte('\n')
		b.WriteString(strings.Join(family.OrderedTags, ","))
		b.WriteByte('\n')
	}
	return b.String()
}

func crossesLiteralAxis(family taganalyzer.OrderedFamily, repo analysedRepo) bool {
	base := namedSegments(repo.segments[family.OrderedTags[0]])
	for _, tag := range family.OrderedTags[1:] {
		other := namedSegments(repo.segments[tag])
		if !subsequence(base, other) && !subsequence(other, base) {
			return true
		}
	}
	return false
}

func namedSegments(segments []taganalyzer.SegmentAnalysis) []string {
	var out []string
	for _, segment := range segments {
		if !segment.IsVariable && !looksHashShaped(segment.Raw) {
			out = append(out, segment.Raw)
		}
	}
	return out
}

func subsequence(a, b []string) bool {
	i := 0
	for _, value := range b {
		if i < len(a) && a[i] == value {
			i++
		}
	}
	return i == len(a)
}

func versionLedThroughout(family taganalyzer.OrderedFamily, repo analysedRepo) bool {
	if !family.HasOrder {
		return false
	}
	for _, tag := range family.OrderedTags {
		segments := repo.segments[tag]
		if len(segments) == 0 || segments[0].OrderType != taganalyzer.OrderVersion || len(segments[0].Numbers) == 0 {
			return false
		}
	}
	return true
}

func inverted(family taganalyzer.OrderedFamily, repo analysedRepo) bool {
	for i := 0; i+1 < len(family.OrderedTags); i++ {
		a := repo.segments[family.OrderedTags[i]][0].Numbers
		b := repo.segments[family.OrderedTags[i+1]][0].Numbers
		if compareNumbers(a, b) < 0 {
			return true
		}
	}
	return false
}

func compareNumbers(a, b []int64) int {
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		var x, y int64
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
	}
	return 0
}
