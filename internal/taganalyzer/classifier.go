package taganalyzer

import "unicode"

func ClassifyToken(token Token) TokenClassification {
	return classifyTokenWithParts(token, true)
}

func classifyTokenWithParts(token Token, includeParts bool) TokenClassification {
	result := TokenClassification{
		Token:   token,
		Matches: classifyToken(token.Value),
	}
	if includeParts {
		result.Parts = splitParts(token.Value)
	}
	return result
}

func splitParts(s string) []TokenPart {
	if s == "" {
		return nil
	}

	var out []TokenPart
	start := 0
	kind := charKind(rune(s[0]))

	for i, r := range s {
		if i == 0 {
			continue
		}

		k := charKind(r)
		if k != kind {
			out = append(out, TokenPart{
				Value: s[start:i],
				Start: start,
				End:   i,
				Kind:  kind,
			})
			start = i
			kind = k
		}
	}

	out = append(out, TokenPart{
		Value: s[start:],
		Start: start,
		End:   len(s),
		Kind:  kind,
	})

	return out
}

func charKind(r rune) string {
	switch {
	case unicode.IsDigit(r):
		return "digit"
	case unicode.IsLetter(r):
		return "letter"
	case r == '.':
		return "dot"
	default:
		return "other"
	}
}
