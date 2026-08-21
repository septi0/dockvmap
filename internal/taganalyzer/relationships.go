package taganalyzer

import (
	"sort"
	"strings"
)

func analyzeRelationships(tags []TagAnalysis) []TokenRelationship {
	type stats struct {
		values          map[string]int
		separators      map[string]map[string]int
		presentTagCount int
	}

	positions := map[int]*stats{}

	for _, tag := range tags {
		for position, token := range tag.Tokens {
			s := positions[position]
			if s == nil {
				s = &stats{values: map[string]int{}, separators: map[string]map[string]int{}}
				positions[position] = s
			}
			s.values[token.Token.Value]++
			if s.separators[token.Token.Value] == nil {
				s.separators[token.Token.Value] = map[string]int{}
			}
			s.separators[token.Token.Value][token.Token.Separator]++
			s.presentTagCount++
		}
	}

	var result []TokenRelationship

	for position, stats := range positions {
		for value, occurrences := range stats.values {
			result = append(result, TokenRelationship{
				Position:       position,
				Value:          value,
				Occurrences:    occurrences,
				UniqueValues:   len(stats.values),
				RepeatedValue:  occurrences > 1,
				StablePosition: stats.presentTagCount == len(tags),
				Separator:      mostCommonSeparator(stats.separators[value]),
			})
		}
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Position != result[j].Position {
			return result[i].Position < result[j].Position
		}
		if result[i].Occurrences != result[j].Occurrences {
			return result[i].Occurrences > result[j].Occurrences
		}
		return result[i].Value < result[j].Value
	})

	return result
}

func mostCommonSeparator(counts map[string]int) string {
	best := ""
	bestCount := -1

	for separator, count := range counts {
		if count > bestCount || (count == bestCount && separator < best) {
			best = separator
			bestCount = count
		}
	}

	return best
}

func analyzePartPatterns(tags []TagAnalysis) []PartPattern {
	type patternKey struct {
		position int
		pattern  string
	}
	type stats struct {
		count    int
		examples []string // Sorted, unique, and limited to the eight smallest values.
	}

	patterns := map[patternKey]*stats{}

	for _, tag := range tags {
		for position, token := range tag.Tokens {
			kinds := make([]string, 0, len(token.Parts))
			for _, part := range token.Parts {
				kinds = append(kinds, part.Kind)
			}

			key := patternKey{position: position, pattern: strings.Join(kinds, ".")}
			s := patterns[key]
			if s == nil {
				s = &stats{}
				patterns[key] = s
			}

			s.count++
			s.examples = insertExample(s.examples, token.Token.Value)
		}
	}

	var result []PartPattern

	for key, stats := range patterns {
		result = append(result, PartPattern{
			TokenPosition: key.position,
			Pattern:       key.pattern,
			Occurrences:   stats.count,
			Examples:      append([]string(nil), stats.examples...),
		})
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].TokenPosition != result[j].TokenPosition {
			return result[i].TokenPosition < result[j].TokenPosition
		}
		return result[i].Pattern < result[j].Pattern
	})

	return result
}

func insertExample(examples []string, value string) []string {
	index := sort.SearchStrings(examples, value)
	if index < len(examples) && examples[index] == value {
		return examples
	}
	if len(examples) == 8 && index == len(examples) {
		return examples
	}
	examples = append(examples, "")
	copy(examples[index+1:], examples[index:])
	examples[index] = value
	if len(examples) > 8 {
		examples = examples[:8]
	}
	return examples
}
