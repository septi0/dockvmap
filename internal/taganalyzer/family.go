package taganalyzer

import (
	"fmt"
	"sort"
	"strings"
)

func analyzeFamilies(tags []TagAnalysis) []Family {
	bloodGroups := map[string][]TagAnalysis{}
	for _, tag := range tags {
		key := familyKey(tag.Segments)
		bloodGroups[key] = append(bloodGroups[key], tag)
	}

	families := make([]Family, 0, len(bloodGroups))
	singletons := make([]TagAnalysis, 0)
	nextID := 1

	keys := make([]string, 0, len(bloodGroups))
	for key := range bloodGroups {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		group := bloodGroups[key]
		if len(group) == 1 {
			singletons = append(singletons, group[0])
			continue
		}

		family := Family{
			ID:       nextID,
			Key:      key,
			TagCount: len(group),
			Kind:     FamilyBlood,
		}
		nextID++
		for _, tag := range group {
			family.Tags = append(family.Tags, tag.Tag)
		}
		families = append(families, family)
	}

	ancestorIndexes := make([]int, len(singletons))
	for i := range ancestorIndexes {
		ancestorIndexes[i] = i
	}
	tagIndex := make(map[string]*TagAnalysis, len(tags))
	for i := range tags {
		tagIndex[tags[i].Tag] = &tags[i]
	}

	ancestorFamilies, remaining, nextID := attachAncestorFamilies(singletons, ancestorIndexes, nil, tagIndex, nextID)
	families = append(families, ancestorFamilies...)

	stepSingletons := make([]TagAnalysis, 0, len(remaining))
	for _, index := range remaining {
		stepSingletons = append(stepSingletons, singletons[index])
	}
	stepFamilies, _ := buildStepFamilies(stepSingletons, nextID)
	families = append(families, stepFamilies...)

	return families
}

type stepCluster struct {
	tags    []TagAnalysis
	indexes []int // Positions in the singleton input; avoids tag-string identity.
	level   int
}

type stepPrefixKeys struct {
	ids   [][]int
	texts []string // Indexed by the corresponding interned prefix ID.
}

func buildStepFamilies(singletons []TagAnalysis, nextID int) ([]Family, []TagAnalysis) {
	if len(singletons) == 0 {
		return nil, nil
	}
	if len(singletons) == 1 {
		tag := singletons[0]
		return []Family{{
			ID:        nextID,
			Key:       "singleton:" + familyKey(tag.Segments),
			TagCount:  1,
			Tags:      []string{tag.Tag},
			Kind:      FamilyStep,
			StepLevel: 0,
		}}, nil
	}

	prefixKeys := precomputeStepPrefixKeys(singletons)
	remaining := make([]int, len(singletons))
	for i := range remaining {
		remaining[i] = i
	}

	maxSegments := 0
	for _, tag := range singletons {
		if len(tag.Segments) > maxSegments {
			maxSegments = len(tag.Segments)
		}
	}

	families := make([]Family, 0)

	for prefixLen := maxSegments; prefixLen >= 1; prefixLen-- {
		groups := make(map[int][]int)
		for _, index := range remaining {
			if len(singletons[index].Segments) < prefixLen {
				continue
			}
			groups[prefixKeys.ids[index][prefixLen]] = append(groups[prefixKeys.ids[index][prefixLen]], index)
		}

		keys := make([]int, 0, len(groups))
		for key := range groups {
			keys = append(keys, key)
		}
		sort.Ints(keys)

		consumed := make(map[int]bool)
		for _, key := range keys {
			indexes := groups[key]
			if len(indexes) < 2 {
				continue
			}
			sort.Ints(indexes)

			family := Family{
				ID:        nextID,
				Key:       "step:" + prefixKeys.texts[key],
				TagCount:  len(indexes),
				Kind:      FamilyStep,
				StepLevel: prefixLen,
			}
			nextID++
			for _, index := range indexes {
				family.Tags = append(family.Tags, singletons[index].Tag)
				consumed[index] = true
			}
			families = append(families, family)
		}

		if len(consumed) == 0 {
			continue
		}
		newRemaining := make([]int, 0, len(remaining)-len(consumed))
		for _, index := range remaining {
			if !consumed[index] {
				newRemaining = append(newRemaining, index)
			}
		}
		remaining = newRemaining
		if len(remaining) < 2 {
			break
		}
	}

	if len(remaining) >= 2 {
		lengthGroups := make(map[int][]int)
		for _, index := range remaining {
			lengthGroups[len(singletons[index].Segments)] = append(lengthGroups[len(singletons[index].Segments)], index)
		}

		lengths := make([]int, 0, len(lengthGroups))
		for length := range lengthGroups {
			lengths = append(lengths, length)
		}
		sort.Ints(lengths)

		for _, length := range lengths {
			indexes := lengthGroups[length]
			if len(indexes) < 2 {
				continue
			}
			sort.Ints(indexes)

			family := Family{
				ID:        nextID,
				Key:       fmt.Sprintf("step:len=%d", length),
				TagCount:  len(indexes),
				Kind:      FamilyStep,
				StepLevel: 0,
			}
			nextID++
			for _, index := range indexes {
				family.Tags = append(family.Tags, singletons[index].Tag)
			}
			families = append(families, family)
		}

		assigned := make(map[int]bool)
		for _, length := range lengths {
			indexes := lengthGroups[length]
			if len(indexes) >= 2 {
				for _, index := range indexes {
					assigned[index] = true
				}
			}
		}
		newRemaining := make([]int, 0, len(remaining))
		for _, index := range remaining {
			if !assigned[index] {
				newRemaining = append(newRemaining, index)
			}
		}
		remaining = newRemaining
	}

	sort.Ints(remaining)
	for _, index := range remaining {
		tag := singletons[index]
		families = append(families, Family{
			ID:        nextID,
			Key:       "step:singleton",
			TagCount:  1,
			Tags:      []string{tag.Tag},
			Kind:      FamilyStep,
			StepLevel: 0,
		})
		nextID++
	}

	return families, nil
}

func attachAncestorFamilies(singletons []TagAnalysis, remaining []int, families []Family, tagIndex map[string]*TagAnalysis, nextID int) ([]Family, []int, int) {
	if len(remaining) == 0 {
		return families, remaining, nextID
	}

	remainingSet := make(map[int]bool, len(remaining))
	for _, index := range remaining {
		remainingSet[index] = true
	}

	rootIndexes := append([]int(nil), remaining...)
	sort.Slice(rootIndexes, func(i, j int) bool {
		li := len(singletons[rootIndexes[i]].Segments)
		lj := len(singletons[rootIndexes[j]].Segments)
		if li != lj {
			return li < lj
		}
		return singletons[rootIndexes[i]].Tag < singletons[rootIndexes[j]].Tag
	})

	consumedFamilies := make(map[int]bool)
	consumedRoots := make(map[int]bool)
	ancestorFamilies := make([]Family, 0)

	for _, rootIndex := range rootIndexes {
		if !remainingSet[rootIndex] || consumedRoots[rootIndex] {
			continue
		}
		root := singletons[rootIndex]

		familyIndexes := make([]int, 0)
		for fi := range families {
			if families[fi].Kind == FamilyBlood || consumedFamilies[fi] || len(families[fi].Tags) == 0 {
				continue
			}
			matched := false
			for _, descendantTag := range families[fi].Tags {
				descendant := tagIndex[descendantTag]
				if descendant != nil && isStrictSegmentPrefix(root.Segments, descendant.Segments) {
					matched = true
					break
				}
			}
			if matched {
				familyIndexes = append(familyIndexes, fi)
			}
		}
		sort.Ints(familyIndexes)

		descendantRoots := make([]int, 0)
		for _, candidate := range rootIndexes {
			if candidate == rootIndex || !remainingSet[candidate] || consumedRoots[candidate] {
				continue
			}
			if isStrictSegmentPrefix(root.Segments, singletons[candidate].Segments) {
				descendantRoots = append(descendantRoots, candidate)
			}
		}
		sort.Slice(descendantRoots, func(i, j int) bool {
			return singletons[descendantRoots[i]].Tag < singletons[descendantRoots[j]].Tag
		})

		if len(familyIndexes) == 0 && len(descendantRoots) == 0 {
			continue
		}

		family := Family{
			ID:        nextID,
			Key:       "ancestor:" + segmentPrefixKey(root.Segments),
			Kind:      FamilyAncestor,
			StepLevel: len(root.Segments),
			Tags:      []string{root.Tag},
			TagCount:  1,
		}
		nextID++

		for _, fi := range familyIndexes {
			family.Tags = append(family.Tags, families[fi].Tags...)
			family.TagCount += len(families[fi].Tags)
			consumedFamilies[fi] = true
		}
		for _, descendantIndex := range descendantRoots {
			family.Tags = append(family.Tags, singletons[descendantIndex].Tag)
			family.TagCount++
			consumedRoots[descendantIndex] = true
		}

		ancestorFamilies = append(ancestorFamilies, family)
		consumedRoots[rootIndex] = true
	}

	if len(ancestorFamilies) == 0 {
		return families, remaining, nextID
	}

	newFamilies := make([]Family, 0, len(families)-len(consumedFamilies)+len(ancestorFamilies))
	for fi, family := range families {
		if !consumedFamilies[fi] {
			newFamilies = append(newFamilies, family)
		}
	}
	newFamilies = append(newFamilies, ancestorFamilies...)

	newRemaining := make([]int, 0, len(remaining))
	for _, index := range remaining {
		if !consumedRoots[index] {
			newRemaining = append(newRemaining, index)
		}
	}

	return newFamilies, newRemaining, nextID
}

func isStrictSegmentPrefix(prefix, full []SegmentAnalysis) bool {
	if len(prefix) == 0 || len(prefix) >= len(full) {
		return false
	}
	for i := range prefix {
		if segmentIdentity(prefix[i]) != segmentIdentity(full[i]) {
			return false
		}
	}
	return true
}

func segmentIdentity(segment SegmentAnalysis) string {
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
		return "V:P=" + segment.Prefix + ":S=" + segment.Suffix
	}
}

func segmentPrefixKey(segments []SegmentAnalysis) string {
	parts := make([]string, len(segments))
	for i, segment := range segments {
		parts[i] = segmentIdentity(segment)
	}
	return strings.Join(parts, "|")
}

func precomputeStepPrefixKeys(tags []TagAnalysis) stepPrefixKeys {
	result := stepPrefixKeys{ids: make([][]int, len(tags)), texts: []string{""}}
	interned := make(map[string]int)
	for tagIndex, tag := range tags {
		parts := make([]string, len(tag.Segments))
		keys := make([]int, len(tag.Segments)+1)
		for i, segment := range tag.Segments {
			parts[i] = stepSegmentKey(segment)
			text := fmt.Sprintf("prefix=%d:%s", i+1, strings.Join(parts[:i+1], "|"))
			id, ok := interned[text]
			if !ok {
				id = len(result.texts)
				interned[text] = id
				result.texts = append(result.texts, text)
			}
			keys[i+1] = id
		}
		result.ids[tagIndex] = keys
	}
	return result
}

func stepSegmentKey(segment SegmentAnalysis) string {
	if !segment.IsVariable {
		return "S:" + segment.Raw
	}

	if segment.OrderType == OrderDate {
		return "V:plain"
	}
	if segment.OrderType == OrderDateTime {
		return "DT"
	}
	if segment.OrderType == OrderTime {
		return "T"
	}
	if (segment.OrderType == OrderVersion || segment.OrderType == OrderSemVer) &&
		segment.Prefix == "" && segment.Suffix == "" &&
		segment.Prerelease == nil && segment.BuildMetadata == "" {
		return "V:plain"
	}

	pre := ""
	if segment.Prerelease != nil {
		pre = ":pre"
	}
	return fmt.Sprintf("V:%s:%s:%s%s", segment.OrderType, segment.Prefix, segment.Suffix, pre)
}

func familyKey(segments []SegmentAnalysis) string {
	parts := make([]string, 0, len(segments))

	for _, segment := range segments {
		if segment.IsVariable {
			switch segment.OrderType {
			case OrderDate:
				parts = append(parts, "D")
			case OrderDateTime:
				parts = append(parts, "DT")
			case OrderTime:
				parts = append(parts, "T")
			default:
				parts = append(parts,
					"V:P="+segment.Prefix+":S="+segment.Suffix)
			}
			continue
		}

		parts = append(parts, "S:"+segment.Raw)
	}

	return strings.Join(parts, "|")
}
