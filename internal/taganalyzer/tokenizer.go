package taganalyzer

import (
	"regexp"
	"strings"
)

var pretokenizeRewrites = []func(string) (string, bool){
	rewriteTimestamp,
}

func pretokenize(tag string) string {
	for _, rewrite := range pretokenizeRewrites {
		if rewritten, ok := rewrite(tag); ok {
			tag = rewritten
		}
	}

	return tag
}

var (
	punctuatedTimestampRE = regexp.MustCompile(`^(?:([A-Za-z]+)[.\-_])?(\d{4})-(\d{2})-(\d{2})T(\d{2})[-:.](\d{2})[-:.](\d{2})Z?([.\-][A-Za-z0-9]+)?`)
	compactTimestampRE    = regexp.MustCompile(`^(\d{8})T(\d{6})Z?`)
)

func rewriteTimestamp(tag string) (string, bool) {
	if m := punctuatedTimestampRE.FindStringSubmatch(tag); m != nil {
		rewritten := m[2] + m[3] + m[4] + m[5] + m[6] + m[7]
		if m[1] != "" {
			rewritten = m[1] + "-" + rewritten
		}
		if m[8] != "" {
			rewritten += "-" + m[8][1:]
		}
		return rewritten + tag[len(m[0]):], true
	}

	if m := compactTimestampRE.FindStringSubmatch(tag); m != nil {
		return m[1] + m[2] + tag[len(m[0]):], true
	}

	return tag, false
}

func tokenize(tag string) []rawToken {
	if tag == "" {
		return nil
	}

	tag = pretokenize(tag)

	var result []rawToken
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
		result = append(result, splitDottedToken(rawToken{Value: tag[start:i], Separator: separator})...)
		separator = ""
	}

	if separator != "" && len(result) == 0 {
		result = append(result, rawToken{Separator: separator})
	}

	return result
}

func splitDottedToken(t rawToken) []rawToken {
	if !strings.Contains(t.Value, ".") || isRecognizedShape(t.Value) {
		return []rawToken{t}
	}

	for i := len(t.Value) - 1; i >= 0; i-- {
		if t.Value[i] != '.' {
			continue
		}
		left, right := t.Value[:i], t.Value[i+1:]
		if left == "" || right == "" {
			continue
		}
		if !isRecognizedShape(left) && !isRecognizedShape(right) {
			continue
		}
		return append(splitDottedToken(rawToken{Value: left, Separator: t.Separator}), rawToken{Value: right, Separator: "."})
	}

	return []rawToken{t}
}

func isRecognizedShape(s string) bool {
	if looksLikeHash(s) {
		return true
	}
	matches := classifyToken(s)
	return len(matches) != 1 || matches[0] != tokenStringType
}
