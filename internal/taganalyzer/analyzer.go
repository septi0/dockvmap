package taganalyzer

func Analyze(tags []string) Analysis {
	return AnalyzeWithOptions(tags, AnalysisOptions{
		IncludeTokens:        true,
		IncludeRelationships: true,
		IncludePartPatterns:  true,
	})
}

func AnalyzeWithOptions(tags []string, options AnalysisOptions) Analysis {
	result := Analysis{
		Tags: make([]TagAnalysis, 0, len(tags)),
	}
	type classificationTemplate struct {
		matches []TokenType
		parts   []TokenPart
	}
	classificationCache := make(map[string]classificationTemplate)

	for _, tag := range tags {
		rawTokens := Tokenize(tag)
		classified := make([]TokenClassification, 0, len(rawTokens))

		for _, token := range rawTokens {
			template, ok := classificationCache[token.Value]
			if !ok {
				template.matches = classifyToken(token.Value)
				if options.IncludePartPatterns {
					template.parts = splitParts(token.Value)
				}
				classificationCache[token.Value] = template
			}
			matches, parts := template.matches, template.parts
			if options.IncludeTokens {
				matches = append([]TokenType(nil), matches...)
				parts = append([]TokenPart(nil), parts...)
			}
			classified = append(classified, TokenClassification{Token: token, Matches: matches, Parts: parts})
		}

		result.Tags = append(result.Tags, TagAnalysis{
			Tag:      tag,
			Tokens:   classified,
			Segments: NormalizeSegments(classified),
		})
	}

	if options.IncludeRelationships {
		result.Relationships = analyzeRelationships(result.Tags)
	}
	if options.IncludePartPatterns {
		result.PartPatterns = analyzePartPatterns(result.Tags)
	}
	result.Families = analyzeFamilies(result.Tags)
	OrderFamilies(&result)

	tagFamilies := make(map[string]Family, len(result.Tags))
	for _, family := range result.Families {
		for _, tag := range family.Tags {
			tagFamilies[tag] = family
		}
	}

	for i := range result.Tags {
		if family, ok := tagFamilies[result.Tags[i].Tag]; ok {
			result.Tags[i].FamilyID = family.ID
			if family.Kind == FamilyBlood {
				result.Tags[i].BloodFamilyID = family.ID
			}
		}
	}

	if !options.IncludeTokens {
		for i := range result.Tags {
			result.Tags[i].Tokens = nil
		}
	}

	return result
}
