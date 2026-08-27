package taganalyzer

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
)

func analyzeFamilies(tags []TagAnalysis) []Family {
	hashLengths := collectHashLengths(tags)
	normalizeHashSegments(tags, hashLengths)
	knownMajors := collectKnownMajors(tags, hashLengths)

	identities := make(map[string][]string, len(tags))
	tagSegments := make(map[string][]SegmentAnalysis, len(tags))
	for _, tag := range tags {
		identities[tag.Tag] = segmentIdentities(tag.Segments, knownMajors, hashLengths)
		tagSegments[tag.Tag] = tag.Segments
	}

	families := buildBloodFamilies(tags, identities)
	families = attachAncestors(families, identities, tagSegments, hashLengths)

	for i := range families {
		families[i].ID = familyID(families[i].Key)
		families[i].HasOrder = familyHasOrder(families[i], tagSegments, hashLengths)
	}

	return families
}

func buildBloodFamilies(tags []TagAnalysis, identities map[string][]string) []Family {
	groups := map[string][]string{}
	for _, tag := range tags {
		groups[strings.Join(identities[tag.Tag], "|")] = append(groups[strings.Join(identities[tag.Tag], "|")], tag.Tag)
	}

	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	families := make([]Family, 0, len(groups))
	for _, key := range keys {
		// registries return tags in any order; downstream tie-breaks are "first wins"
		group := groups[key]
		sort.Strings(group)
		family := Family{Key: key, Kind: FamilyBlood, Tags: group, TagCount: len(group)}
		if len(group) == 1 {
			family.Key, family.Kind = "singleton:"+key, FamilyStep
		}
		families = append(families, family)
	}

	return families
}

func familyHasOrder(family Family, tagSegments map[string][]SegmentAnalysis, hashLengths map[hashSlot]bool) bool {
	if len(family.Tags) < 2 {
		return true
	}

	first := tagSegments[family.Tags[0]]
	for _, tag := range family.Tags[1:] {
		if len(tagSegments[tag]) != len(first) {
			return true
		}
	}

	varying := false
	for i := range first {
		differs := false
		for _, tag := range family.Tags[1:] {
			if tagSegments[tag][i].Raw != first[i].Raw {
				differs = true
				break
			}
		}
		if !differs {
			continue
		}
		varying = true
		if _, isHash := hashSegmentIdentity(first[i], i, hashLengths); !isHash {
			return true
		}
	}

	return !varying
}

type majorSlot struct {
	Context string
	Prefix  string
	Major   int64
}

func collectKnownMajors(tags []TagAnalysis, hashLengths map[hashSlot]bool) map[majorSlot]bool {
	known := make(map[majorSlot]bool)
	for _, tag := range tags {
		context := ""
		for i, segment := range tag.Segments {
			if segment.IsVariable && segment.OrderType == OrderVersion && len(segment.Numbers) > 0 &&
				(len(segment.Numbers) >= 2 || segment.Prerelease != nil) {
				known[majorSlot{Context: context, Prefix: segment.Prefix, Major: segment.Numbers[0]}] = true
			}
			context = appendSegmentContext(context, segment, i, hashLengths)
		}
	}
	return known
}

func appendSegmentContext(context string, segment SegmentAnalysis, index int, hashLengths map[hashSlot]bool) string {
	part := string(segment.OrderType)
	if id, ok := hashSegmentIdentity(segment, index, hashLengths); ok {
		part = id
	} else if !segment.IsVariable {
		part = "S:" + segment.Raw
	}
	if context == "" {
		return part
	}
	return context + "|" + part
}

const hashMinDistinctValues = 5

type hashSlot struct {
	Index  int
	Length int
}

type hashSlotStats struct {
	distinct map[string]bool
	hasHex   bool
}

func collectHashLengths(tags []TagAnalysis) map[hashSlot]bool {
	stats := map[hashSlot]*hashSlotStats{}
	for _, tag := range tags {
		for i, segment := range tag.Segments {
			if !isHashCandidate(segment) {
				continue
			}
			slot := hashSlot{Index: i, Length: len(segment.Raw)}
			slotStats := stats[slot]
			if slotStats == nil {
				slotStats = &hashSlotStats{distinct: map[string]bool{}}
				stats[slot] = slotStats
			}
			slotStats.distinct[segment.Raw] = true
			if hasHexLetter(segment.Raw) {
				slotStats.hasHex = true
			}
		}
	}

	dynamic := make(map[hashSlot]bool, len(stats))
	for slot, slotStats := range stats {
		if slotStats.hasHex && len(slotStats.distinct) >= hashMinDistinctValues {
			dynamic[slot] = true
		}
	}
	return dynamic
}

func normalizeHashSegments(tags []TagAnalysis, hashLengths map[hashSlot]bool) {
	for _, tag := range tags {
		for i := range tag.Segments {
			if _, ok := hashSegmentIdentity(tag.Segments[i], i, hashLengths); !ok {
				continue
			}
			tag.Segments[i] = SegmentAnalysis{Raw: tag.Segments[i].Raw, OrderType: OrderAlphabetical}
		}
	}
}

func isHashCandidate(segment SegmentAnalysis) bool {
	switch segment.OrderType {
	case OrderDate, OrderDateTime, OrderTime:
		return false
	}
	return looksLikeHash(segment.Raw)
}

func hashSegmentIdentity(segment SegmentAnalysis, index int, hashLengths map[hashSlot]bool) (string, bool) {
	if !isHashCandidate(segment) {
		return "", false
	}
	slot := hashSlot{Index: index, Length: len(segment.Raw)}
	if !hashLengths[slot] {
		return "", false
	}
	return fmt.Sprintf("HASH:len=%d", slot.Length), true
}

type familyRep struct {
	identity []string
	segments []SegmentAnalysis
	hasHash  bool
}

func collectFamilyReps(families []Family, identities map[string][]string, tagSegments map[string][]SegmentAnalysis) [][]familyRep {
	reps := make([][]familyRep, len(families))
	for fi, family := range families {
		seen := make(map[string]bool, 1)
		for _, tag := range family.Tags {
			identity := identities[tag]
			key := strings.Join(identity, "|")
			if seen[key] {
				continue
			}
			seen[key] = true
			reps[fi] = append(reps[fi], familyRep{
				identity: identity,
				segments: tagSegments[tag],
				hasHash:  containsHashLiteral(tagSegments[tag]),
			})
		}
	}
	return reps
}

func attachAncestors(families []Family, identities map[string][]string, tagSegments map[string][]SegmentAnalysis, hashLengths map[hashSlot]bool) []Family {
	for {
		merged, changed := mergeAncestorPass(families, identities, tagSegments, hashLengths)
		if !changed {
			return families
		}
		families = merged
	}
}

func mergeAncestorPass(families []Family, identities map[string][]string, tagSegments map[string][]SegmentAnalysis, hashLengths map[hashSlot]bool) ([]Family, bool) {
	reps := collectFamilyReps(families, identities, tagSegments)

	// Longest pattern first, so a more specific root claims a target before a shorter one can.
	order := make([]int, len(families))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool {
		ra, rb := rootRep(reps[order[a]]), rootRep(reps[order[b]])
		if len(ra.identity) != len(rb.identity) {
			return len(ra.identity) > len(rb.identity)
		}
		return families[order[a]].Key < families[order[b]].Key
	})

	consumed := make(map[int]bool)
	result := make([]Family, 0, len(families))

	for _, rootIndex := range order {
		if consumed[rootIndex] || len(reps[rootIndex]) == 0 {
			continue
		}
		root := rootRep(reps[rootIndex])

		target, matches := -1, 0
		for candidate := range families {
			if candidate == rootIndex || consumed[candidate] {
				continue
			}
			for _, rep := range reps[candidate] {
				// a hash-free root must not join a hash-varying family (release line vs commit builds)
				if !root.hasHash && rep.hasHash {
					continue
				}
				if isSkeletonExtension(root.identity, rep.identity, root.segments, rep.segments, hashLengths) {
					target, matches = candidate, matches+1
					break
				}
			}
			if matches > 1 {
				break
			}
		}

		if matches != 1 {
			continue
		}

		family := Family{
			Key:  "ancestor:" + strings.Join(root.identity, "|"),
			Kind: FamilyAncestor,
			Tags: append(append([]string(nil), families[rootIndex].Tags...), families[target].Tags...),
		}
		family.TagCount = len(family.Tags)

		consumed[rootIndex] = true
		consumed[target] = true
		result = append(result, family)
	}

	if len(result) == 0 {
		return families, false
	}

	for i, family := range families {
		if !consumed[i] {
			result = append(result, family)
		}
	}

	return result, true
}

// rootRep is a family's most general member: the one other families extend.
func rootRep(reps []familyRep) familyRep {
	best := reps[0]
	for _, rep := range reps[1:] {
		if len(rep.identity) != len(best.identity) {
			if len(rep.identity) < len(best.identity) {
				best = rep
			}
			continue
		}
		if strings.Join(rep.identity, "|") < strings.Join(best.identity, "|") {
			best = rep
		}
	}
	return best
}

func isSkeletonExtension(skeleton, full []string, skeletonSegments, fullSegments []SegmentAnalysis, hashLengths map[hashSlot]bool) bool {
	if len(skeleton) == 0 || len(skeleton) >= len(full) {
		return false
	}
	si := 0
	skippedAny := false
	for i, id := range full {
		if si < len(skeleton) && id == skeleton[si] {
			si++
			continue
		}
		if !isOmittableSegment(fullSegments[i], i, hashLengths) {
			return false
		}
		if si < len(skeleton) && fullSegments[i].OrderType == OrderVersion && skeletonSegments[si].OrderType == OrderVersion {
			return false
		}
		skippedAny = true
	}
	return si == len(skeleton) && skippedAny
}

func isOmittableSegment(segment SegmentAnalysis, index int, hashLengths map[hashSlot]bool) bool {
	if segment.IsVariable && segment.Prefix == "" && segment.Suffix == "" {
		return true
	}
	_, isHash := hashSegmentIdentity(segment, index, hashLengths)
	return isHash
}

func versionShape(segment SegmentAnalysis, context string, knownMajors map[majorSlot]bool) string {
	if len(segment.Numbers) >= 2 {
		if looksLikeCalVer(segment) {
			return "calver"
		}
		return "multi"
	}
	if len(segment.Numbers) == 1 && knownMajors[majorSlot{Context: context, Prefix: segment.Prefix, Major: segment.Numbers[0]}] {
		return "multi"
	}
	return "solo"
}

func looksLikeCalVer(segment SegmentAnalysis) bool {
	year := segment.Numbers[0]
	return year >= 2000 && year <= 2099
}

func segmentIdentity(segment SegmentAnalysis, index int, context string, knownMajors map[majorSlot]bool, hashLengths map[hashSlot]bool) string {
	if id, ok := hashSegmentIdentity(segment, index, hashLengths); ok {
		return id
	}
	if !segment.IsVariable {
		return "S:" + segment.Raw
	}

	switch segment.OrderType {
	case OrderDate:
		return "D"
	case OrderDateTime:
		return "DT"
	case OrderTime:
		return "T"
	default:
		return "V:P=" + segment.Prefix + ":S=" + segment.Suffix + ":" + versionShape(segment, context, knownMajors)
	}
}

func segmentIdentities(segments []SegmentAnalysis, knownMajors map[majorSlot]bool, hashLengths map[hashSlot]bool) []string {
	ids := make([]string, len(segments))
	context := ""
	for i, segment := range segments {
		ids[i] = segmentIdentity(segment, i, context, knownMajors, hashLengths)
		context = appendSegmentContext(context, segment, i, hashLengths)
	}
	return ids
}

func isPlainVersionSegment(segment SegmentAnalysis) bool {
	return segment.OrderType == OrderVersion &&
		segment.Prefix == "" && segment.Suffix == "" &&
		segment.Prerelease == nil
}

// 52 bits keeps the id exact through a JSON number
func familyID(key string) int64 {
	h := fnv.New64a()
	h.Write([]byte(key))
	return int64(h.Sum64() & 0x000FFFFFFFFFFFFF)
}
