package taganalyzer

import (
	"sort"
	"strings"
)

type sortableTag struct {
	raw      string
	analysis *TagAnalysis
}

func orderFamilies(tags []TagAnalysis, families []Family) []OrderedFamily {
	ordered := make([]OrderedFamily, 0, len(families))
	tagIndex := make(map[string]*TagAnalysis, len(tags))
	for i := range tags {
		tagIndex[tags[i].Tag] = &tags[i]
	}

	relevance := make(map[int64]familyRelevance, len(families))
	for _, family := range families {
		relevance[family.ID] = relevanceOf(family, tagIndex)
	}

	sort.SliceStable(families, func(i, j int) bool {
		return compareFamilies(families[i], families[j], relevance) < 0
	})

	for _, family := range families {
		sortable := make([]sortableTag, 0, len(family.Tags))
		for _, tag := range family.Tags {
			sortable = append(sortable, sortableTag{
				raw:      tag,
				analysis: tagIndex[tag],
			})
		}

		sort.SliceStable(sortable, func(i, j int) bool {
			left, right := sortable[i], sortable[j]

			if left.analysis == nil || right.analysis == nil {
				return left.raw > right.raw
			}

			result := compareSegments(left.analysis.Segments, right.analysis.Segments)
			if result != 0 {
				return result > 0
			}

			return left.raw > right.raw
		})

		tags := make([]string, len(sortable))
		for i, item := range sortable {
			tags[i] = item.raw
		}

		ordered = append(ordered, OrderedFamily{
			Family:      family,
			OrderedTags: tags,
		})
	}

	return ordered
}

// familyRelevance ranks how likely a family is to be the one someone opening this
// repository for the first time is looking for. Everything is derived from the
// family's root: its most general member, the tag with the fewest segments.
type familyRelevance struct {
	singleton    bool
	rootHasHash  bool
	rootOrder    int
	rootDepth    int
	releaseShape int
	tagCount     int
}

const (
	releaseShapeMultiPlain = iota
	releaseShapeMultiNamed
	releaseShapeSoloPlain
	releaseShapeOther
)

func relevanceOf(family Family, tagIndex map[string]*TagAnalysis) familyRelevance {
	relevance := familyRelevance{
		singleton:    family.TagCount == 1,
		tagCount:     family.TagCount,
		rootOrder:    orderTypeRank(OrderUnknown),
		releaseShape: releaseShapeOther,
		rootHasHash:  true,
	}

	rootDepth := 0
	for _, tag := range family.Tags {
		analysis := tagIndex[tag]
		if analysis == nil || len(analysis.Segments) == 0 {
			continue
		}
		if rootDepth == 0 || len(analysis.Segments) < rootDepth {
			rootDepth = len(analysis.Segments)
		}
	}
	if rootDepth == 0 {
		relevance.rootHasHash = false
		return relevance
	}

	// Several members can be equally general; judge the family by the best of them.
	relevance.rootDepth = rootDepth
	for _, tag := range family.Tags {
		analysis := tagIndex[tag]
		if analysis == nil || len(analysis.Segments) != rootDepth {
			continue
		}
		segments := analysis.Segments
		if rank := orderTypeRank(segments[0].OrderType); rank < relevance.rootOrder {
			relevance.rootOrder = rank
		}
		if shape := releaseShapeRank(segments[0]); shape < relevance.releaseShape {
			relevance.releaseShape = shape
		}
		if relevance.rootHasHash && !containsHashLiteral(segments) {
			relevance.rootHasHash = false
		}
	}

	return relevance
}

func containsHashLiteral(segments []SegmentAnalysis) bool {
	for _, segment := range segments {
		if !segment.IsVariable && looksLikeHash(segment.Raw) {
			return true
		}
	}
	return false
}

func releaseShapeRank(segment SegmentAnalysis) int {
	if segment.OrderType != OrderVersion {
		return releaseShapeOther
	}

	plainPrefix := segment.Prefix == "" || strings.EqualFold(segment.Prefix, "v")
	if len(segment.Numbers) >= 2 || segment.Prerelease != nil {
		if plainPrefix {
			return releaseShapeMultiPlain
		}
		return releaseShapeMultiNamed
	}
	if plainPrefix {
		return releaseShapeSoloPlain
	}

	return releaseShapeOther
}

// relevanceCriteria decides which family someone opening a repository sees first.
// Each scores lower-is-better, and the order is load-bearing: plainness must come
// before release shape, or a bare JDK major like "26" loses to "8u492-b09-jdk".
// Reordering or adding a criterion changes what every repository shows, so re-run
// the audit over sampledata/ before and after.
var relevanceCriteria = []struct {
	name  string
	score func(familyRelevance) int
}{
	{"multi-tag family before a lone tag", func(r familyRelevance) int { return boolScore(r.singleton) }},
	{"hash-free root before a commit build", func(r familyRelevance) int { return boolScore(r.rootHasHash) }},
	{"version-led root, then date, then name", func(r familyRelevance) int { return r.rootOrder }},
	{"shallowest root first", func(r familyRelevance) int { return r.rootDepth }},
	{"plainest version shape first", func(r familyRelevance) int { return r.releaseShape }},
	{"more tags first", func(r familyRelevance) int { return -r.tagCount }},
}

func compareFamilies(left, right Family, relevance map[int64]familyRelevance) int {
	l, r := relevance[left.ID], relevance[right.ID]

	for _, criterion := range relevanceCriteria {
		if result := compareInt(criterion.score(l), criterion.score(r)); result != 0 {
			return result
		}
	}

	return compareString(left.Key, right.Key)
}

func boolScore(v bool) int {
	if v {
		return 1
	}
	return 0
}

func compareInt(left, right int) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func orderTypeRank(orderType OrderType) int {
	switch orderType {
	case OrderVersion:
		return 0
	case OrderDate:
		return 1
	case OrderDateTime:
		return 2
	case OrderTime:
		return 3
	case OrderAlphabetical:
		return 4
	default:
		return 5
	}
}

func compareSegments(left, right []SegmentAnalysis) int {
	n := len(left)
	if len(right) < n {
		n = len(right)
	}

	for i := 0; i < n; i++ {
		if result := compareSegment(left[i], right[i]); result != 0 {
			return result
		}
	}

	switch {
	case len(left) < len(right):
		return -1
	case len(left) > len(right):
		return 1
	default:
		return 0
	}
}

func compareSegment(left, right SegmentAnalysis) int {
	if left.OrderType != right.OrderType {
		if result, ok := compareTemporalMismatch(left, right); ok {
			return result
		}
		return compareOrderType(left.OrderType, right.OrderType)
	}

	switch left.OrderType {
	case OrderVersion:
		if result := compareString(left.Prefix, right.Prefix); result != 0 {
			return result
		}
		if result := compareVersionNumbers(left.Numbers, right.Numbers); result != 0 {
			return result
		}
		if result := comparePrerelease(left.Prerelease, right.Prerelease); result != 0 {
			return result
		}
		return compareString(left.Suffix, right.Suffix)

	case OrderAlphabetical:
		return compareString(left.Raw, right.Raw)

	case OrderDate, OrderDateTime:
		if left.sortKey != "" && right.sortKey != "" {
			return compareString(left.sortKey, right.sortKey)
		}
		return compareString(left.Raw, right.Raw)

	case OrderTime:
		return compareString(left.Raw, right.Raw)

	default:
		return compareString(left.Raw, right.Raw)
	}
}

func compareOrderType(left, right OrderType) int {
	l, r := orderTypeRank(left), orderTypeRank(right)
	switch {
	case l < r:
		return 1
	case l > r:
		return -1
	default:
		return 0
	}
}

func compareTemporalMismatch(left, right SegmentAnalysis) (int, bool) {
	leftNumbers, leftOK := temporalNumbers(left)
	rightNumbers, rightOK := temporalNumbers(right)

	if !leftOK || !rightOK {
		return 0, false
	}

	return compareVersionNumbers(leftNumbers, rightNumbers), true
}

func temporalNumbers(segment SegmentAnalysis) ([]int64, bool) {
	if segment.OrderType == OrderDate && len(segment.Numbers) > 0 {
		return segment.Numbers, true
	}
	if isPlainVersionSegment(segment) {
		return segment.Numbers, true
	}
	return nil, false
}

func compareVersionNumbers(left, right []int64) int {
	n := len(left)
	if len(right) > n {
		n = len(right)
	}

	for i := 0; i < n; i++ {
		var l, r int64
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		switch {
		case l < r:
			return -1
		case l > r:
			return 1
		}
	}

	return 0
}

func compareString(left, right string) int {
	return strings.Compare(left, right)
}

func comparePrerelease(left, right *Prerelease) int {
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return 1
	}
	if right == nil {
		return -1
	}

	li := left.Identifiers
	ri := right.Identifiers

	n := len(li)
	if len(ri) < n {
		n = len(ri)
	}
	for i := 0; i < n; i++ {
		l, r := li[i], ri[i]
		if l.Number != nil && r.Number != nil && l.IsNumber && r.IsNumber {
			if *l.Number < *r.Number {
				return -1
			}
			if *l.Number > *r.Number {
				return 1
			}
			continue
		}
		if l.Number != nil && r.Number == nil && l.IsNumber {
			return -1
		}
		if l.Number == nil && r.Number != nil && r.IsNumber {
			return 1
		}

		lr, rr := prereleaseRank(l.Value), prereleaseRank(r.Value)
		if lr < rr {
			return -1
		}
		if lr > rr {
			return 1
		}
		if result := compareString(l.Value, r.Value); result != 0 {
			return result
		}
		if l.Number != nil && r.Number != nil {
			if *l.Number < *r.Number {
				return -1
			}
			if *l.Number > *r.Number {
				return 1
			}
		}
	}

	if len(li) < len(ri) {
		return -1
	}
	if len(li) > len(ri) {
		return 1
	}
	return 0
}

var unrankedPrerelease = len(prereleaseKeywords)

var prereleaseRanks = buildPrereleaseRanks()

func buildPrereleaseRanks() map[string]int {
	ranks := make(map[string]int, len(prereleaseKeywords)+len(gluedPrereleaseAliases))
	for rank, keyword := range prereleaseKeywords {
		ranks[keyword] = rank
	}
	for alias, keyword := range gluedPrereleaseAliases {
		ranks[alias] = ranks[keyword]
	}
	return ranks
}

func prereleaseRank(value string) int {
	if rank, ok := prereleaseRanks[strings.ToLower(value)]; ok {
		return rank
	}
	return unrankedPrerelease
}
