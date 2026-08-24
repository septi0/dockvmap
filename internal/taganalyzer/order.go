package taganalyzer

import (
	"sort"
	"strings"
)

type sortableTag struct {
	raw      string
	analysis *TagAnalysis
}

func OrderFamilies(analysis *Analysis) {
	if analysis == nil {
		return
	}

	ordered := make([]OrderedFamily, 0, len(analysis.Families))
	tagIndex := make(map[string]*TagAnalysis, len(analysis.Tags))
	for i := range analysis.Tags {
		tagIndex[analysis.Tags[i].Tag] = &analysis.Tags[i]
	}

	families := append([]Family(nil), analysis.Families...)

	orderTypes := make(map[int]OrderType, len(families))
	for _, family := range families {
		orderTypes[family.ID] = familyOrderType(family, tagIndex)
	}

	sort.SliceStable(families, func(i, j int) bool {
		return compareFamilies(families[i], families[j], orderTypes) < 0
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

	analysis.Ordered = ordered
}

func compareFamilies(left, right Family, orderTypes map[int]OrderType) int {
	if left.Kind != right.Kind {
		return compareFamilyKind(left.Kind, right.Kind)
	}

	leftOrderType := orderTypes[left.ID]
	rightOrderType := orderTypes[right.ID]
	if leftOrderType != rightOrderType {
		return compareFamilyOrderType(leftOrderType, rightOrderType)
	}

	return compareString(left.Key, right.Key)
}

func compareFamilyKind(left, right FamilyKind) int {
	rank := func(kind FamilyKind) int {
		switch kind {
		case FamilyBlood:
			return 0
		case FamilyAncestor:
			return 1
		case FamilyStep:
			return 2
		default:
			return 3
		}
	}

	l, r := rank(left), rank(right)
	switch {
	case l < r:
		return -1
	case l > r:
		return 1
	default:
		return 0
	}
}

func familyOrderType(family Family, tagIndex map[string]*TagAnalysis) OrderType {
	best := OrderUnknown
	bestRank := orderTypeRank(OrderUnknown)

	for _, tag := range family.Tags {
		analysis := tagIndex[tag]
		if analysis == nil || len(analysis.Segments) == 0 {
			continue
		}

		orderType := analysis.Segments[0].OrderType
		if rank := orderTypeRank(orderType); rank < bestRank {
			best, bestRank = orderType, rank
			if rank == 0 {
				break
			}
		}
	}

	return best
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

func compareFamilyOrderType(left, right OrderType) int {
	l, r := orderTypeRank(left), orderTypeRank(right)
	switch {
	case l < r:
		return -1
	case l > r:
		return 1
	default:
		return 0
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
	return strings.Compare(string(left), string(right))
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
	if len(li) == 0 {
		li = []PrereleaseIdentifier{{Value: left.Type, Number: left.Number}}
	}
	if len(ri) == 0 {
		ri = []PrereleaseIdentifier{{Value: right.Type, Number: right.Number}}
	}

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

func prereleaseRank(value string) int {
	switch strings.ToLower(value) {
	case "dev":
		return 10
	case "snapshot":
		return 20
	case "alpha", "a":
		return 30
	case "beta", "b":
		return 40
	case "pre":
		return 50
	case "preview":
		return 60
	case "rc":
		return 70
	default:
		return 80
	}
}
