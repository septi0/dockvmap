package taganalyzer

import (
	"regexp"
	"strings"
)

var pretokenizeRewrites = []func(string) (string, bool){
	rewriteReleaseTimestamp,
}

func pretokenize(tag string) string {
	for _, rewrite := range pretokenizeRewrites {
		if rewritten, ok := rewrite(tag); ok {
			return rewritten
		}
	}

	return tag
}

var releaseTimestampRE = regexp.MustCompile(`^([A-Za-z]+)\.(\d{4})-(\d{2})-(\d{2})T(\d{2})-(\d{2})-(\d{2})Z(\.[A-Za-z0-9]+)?`)

func rewriteReleaseTimestamp(tag string) (string, bool) {
	m := releaseTimestampRE.FindStringSubmatch(tag)
	if m == nil {
		return tag, false
	}

	rewritten := m[1] + "-" + m[2] + m[3] + m[4] + m[5] + m[6] + m[7]
	if m[8] != "" {
		rewritten += "-" + m[8][1:]
	}

	return rewritten + tag[len(m[0]):], true
}

func Tokenize(tag string) []Token {
	if tag == "" {
		return nil
	}

	tag = pretokenize(tag)

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
