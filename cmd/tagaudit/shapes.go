package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/septi0/dockvmap/internal/taganalyzer"
)

// character-class fingerprint of a string; hex runs collapse first so shas bucket together
func shapeOf(value string) string {
	if value == "" {
		return "(empty)"
	}

	var b strings.Builder
	for i := 0; i < len(value); {
		if run := hexRunLength(value[i:]); run >= minHexRun {
			b.WriteString("x")
			b.WriteString(bucket(run))
			i += run
			continue
		}

		class := classOf(value[i])
		run := 0
		for i < len(value) && classOf(value[i]) == class && hexRunLength(value[i:]) < minHexRun {
			run++
			i++
		}
		if run == 0 {
			continue
		}
		b.WriteString(class)
		if class == "d" || class == "a" || class == "A" {
			b.WriteString(bucket(run))
		}
	}
	return b.String()
}

// minHexRun matches the shortest abbreviated git sha in common use.
const minHexRun = 7

func hexRunLength(value string) int {
	n := 0
	for n < len(value) {
		c := value[n]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			break
		}
		n++
	}
	return n
}

func classOf(c byte) string {
	switch {
	case c >= '0' && c <= '9':
		return "d"
	case c >= 'a' && c <= 'z':
		return "a"
	case c >= 'A' && c <= 'Z':
		return "A"
	default:
		return string(c)
	}
}

func bucket(n int) string {
	switch {
	case n == 1:
		return "{1}"
	case n <= 3:
		return "{2-3}"
	case n <= 6:
		return "{4-6}"
	default:
		return "{7+}"
	}
}

func blockingSegment(repo analysedRepo, tag string, cardinality map[string]map[string]bool) (string, bool) {
	segments := repo.segments[tag]
	best, bestCount := "", 0

	for i, segment := range segments {
		if segment.IsVariable {
			continue
		}
		if n := len(cardinality[holeKey(segments, i)]); n > bestCount {
			bestCount, best = n, segment.Raw
		}
	}
	return best, bestCount >= 2
}

// key for a tag with segment[hole] blanked, so tags differing only there collide
func holeKey(segments []taganalyzer.SegmentAnalysis, hole int) string {
	parts := make([]string, 0, len(segments))
	for i, segment := range segments {
		switch {
		case i == hole:
			parts = append(parts, "?")
		case segment.IsVariable:
			parts = append(parts, "*")
		default:
			parts = append(parts, segment.Raw)
		}
	}
	return strings.Join(parts, "|")
}

func cardinalityOf(repo analysedRepo) map[string]map[string]bool {
	cardinality := map[string]map[string]bool{}
	for _, tag := range repo.analysis.Tags {
		for i, segment := range tag.Segments {
			if segment.IsVariable {
				continue
			}
			key := holeKey(tag.Segments, i)
			if cardinality[key] == nil {
				cardinality[key] = map[string]bool{}
			}
			cardinality[key][segment.Raw] = true
		}
	}
	return cardinality
}

type shapeStat struct {
	shape    string
	tags     int
	repos    map[string]bool
	values   map[string]bool
	examples []string
}

func reportShapes(repos []analysedRepo, minRepos int) {
	stats := map[string]*shapeStat{}
	singletons := 0

	for _, repo := range repos {
		cardinality := cardinalityOf(repo)
		for _, family := range repo.analysis.Ordered {
			if family.TagCount != 1 {
				continue
			}
			singletons++

			tag := family.OrderedTags[0]
			blocking, ok := blockingSegment(repo, tag, cardinality)
			if !ok {
				continue
			}

			shape := shapeOf(blocking)
			stat := stats[shape]
			if stat == nil {
				stat = &shapeStat{shape: shape, repos: map[string]bool{}, values: map[string]bool{}}
				stats[shape] = stat
			}
			stat.tags++
			stat.repos[repo.name] = true
			stat.values[blocking] = true
			if len(stat.examples) < 3 {
				stat.examples = append(stat.examples, tag)
			}
		}
	}

	ranked := make([]*shapeStat, 0, len(stats))
	for _, stat := range stats {
		if len(stat.repos) >= minRepos {
			ranked = append(ranked, stat)
		}
	}
	// rank by volume x distinct-rate: a real convention has ~one distinct value per tag
	sort.Slice(ranked, func(i, j int) bool {
		si := float64(ranked[i].tags) * distinctRate(ranked[i])
		sj := float64(ranked[j].tags) * distinctRate(ranked[j])
		return si > sj
	})

	fmt.Println("=== segment shapes blocking tags from grouping ===")
	fmt.Printf("%d singleton tags examined; shapes seen in >=%d repositories:\n\n", singletons, minRepos)
	fmt.Printf("%-22s %8s %7s %9s  %s\n", "shape", "tags", "repos", "distinct", "examples")

	for i, stat := range ranked {
		if i >= 25 {
			fmt.Printf("... %d more shapes\n", len(ranked)-25)
			break
		}
		fmt.Printf("%-22s %8d %7d %8.0f%%  %s\n",
			stat.shape, stat.tags, len(stat.repos), 100*distinctRate(stat), strings.Join(stat.examples, ", "))
	}

	fmt.Println("\nA high distinct rate means nearly every tag has its own value there, which is")
	fmt.Println("what a machine-generated identifier looks like. Those are the candidates.")
	fmt.Println()
}

func distinctRate(stat *shapeStat) float64 {
	if stat.tags == 0 {
		return 0
	}
	return float64(len(stat.values)) / float64(stat.tags)
}
