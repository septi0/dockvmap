package taganalyzer

import "strings"

func Tokenize(tag string) []Token {
	if tag == "" {
		return nil
	}

	var result []Token
	separator := ""
	for i := 0; i < len(tag); {
		if tag[i] == '-' || tag[i] == '_' {
			start := i
			for i < len(tag) && (tag[i] == '-' || tag[i] == '_') {
				i++
			}
			separator += tag[start:i]
			continue
		}

		start := i
		for i < len(tag) && tag[i] != '-' && tag[i] != '_' {
			i++
		}
		result = append(result, Token{Value: tag[start:i], Start: start, End: i, Separator: separator})
		separator = ""
	}

	if separator != "" {
		if len(result) == 0 {
			result = append(result, Token{Start: len(tag), End: len(tag), Separator: separator})
		} else {
			result[len(result)-1].TrailingSeparator = separator
		}
	}

	return result
}

func Reconstruct(tokens []Token) string {
	if len(tokens) == 0 {
		return ""
	}

	var b strings.Builder

	for i, token := range tokens {
		b.WriteString(token.Separator)
		b.WriteString(token.Value)
		if i == len(tokens)-1 {
			b.WriteString(token.TrailingSeparator)
		}
	}

	return b.String()
}
