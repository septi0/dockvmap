package taganalyzer

func AnalyzeWithOptions(tags []string, options AnalysisOptions) Analysis {
	result := Analysis{
		Tags: make([]TagAnalysis, 0, len(tags)),
	}
	classificationCache := make(map[string][]TokenType)

	for _, tag := range tags {
		rawTokens := Tokenize(tag)
		classified := make([]TokenClassification, 0, len(rawTokens))

		for _, token := range rawTokens {
			matches, ok := classificationCache[token.Value]
			if !ok {
				matches = classifyToken(token.Value)
				classificationCache[token.Value] = matches
			}
			if options.IncludeTokens {
				matches = append([]TokenType(nil), matches...)
			}
			classified = append(classified, TokenClassification{Token: token, Matches: matches})
		}

		result.Tags = append(result.Tags, TagAnalysis{
			Tag:      tag,
			Tokens:   classified,
			Segments: NormalizeSegments(classified),
		})
	}

	result.Families = analyzeFamilies(result.Tags)
	OrderFamilies(&result)

	if !options.IncludeTokens {
		for i := range result.Tags {
			result.Tags[i].Tokens = nil
		}
	}

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
