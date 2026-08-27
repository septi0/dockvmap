package taganalyzer

func Analyze(tags []string) Analysis {
	result := Analysis{
		Tags: make([]TagAnalysis, 0, len(tags)),
	}
	classificationCache := make(map[string][]tokenType)

	for _, tag := range tags {
		rawTokens := tokenize(tag)
		classified := make([]tokenClassification, 0, len(rawTokens))

		for _, token := range rawTokens {
			matches, ok := classificationCache[token.Value]
			if !ok {
				matches = classifyToken(token.Value)
				classificationCache[token.Value] = matches
			}
			classified = append(classified, tokenClassification{Token: token, Matches: matches})
		}

		result.Tags = append(result.Tags, TagAnalysis{
			Tag:      tag,
			Segments: normalizeSegments(classified),
		})
	}

	result.Ordered = orderFamilies(result.Tags, analyzeFamilies(result.Tags))

	return result
}

func IsPrerelease(tag TagAnalysis) bool {
	for _, segment := range tag.Segments {
		if segment.Prerelease != nil {
			return true
		}
	}
	return false
}
