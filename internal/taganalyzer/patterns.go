package taganalyzer

import (
	"regexp"
	"strings"
	"time"
)

var (
	versionRE = regexp.MustCompile(
		`(?i)^[A-Za-z]*(?:0|[1-9][0-9]*)(?:\.(?:0|[1-9][0-9]*))*(?:(?:alpha|beta|rc|pre|preview|dev|snapshot)[._]?[0-9]*(?:[._](?:alpha|beta|rc|pre|preview|dev|snapshot)[._]?[0-9]*)*)?(?:\+[0-9A-Za-z.-]+)?$`,
	)

	embeddedVersionRE = regexp.MustCompile(
		`(?i)^[A-Za-z]+[0-9]+(?:\.[0-9]+)*(?:(?:alpha|beta|rc|pre|preview|dev|snapshot)[._]?[0-9]*(?:[._](?:alpha|beta|rc|pre|preview|dev|snapshot)[._]?[0-9]*)*)?(?:\+[0-9A-Za-z.-]+)?$`,
	)

	semverRE = regexp.MustCompile(
		`^(?:v)?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:\.(0|[1-9][0-9]*))?(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`,
	)

	timeRE = regexp.MustCompile(`^(?:[01][0-9]|2[0-3])[0-5][0-9][0-5][0-9]$`)
)

var dateLayouts = []string{
	"20060102",
	"2006-01-02",
	"2006.01.02",
}

var dateTimeLayouts = []string{
	"20060102150405",
	"2006-01-02T15:04:05",
	"2006-01-02T15:04:05Z07:00",
}

func classifyToken(s string) []TokenType {
	var matches []TokenType

	if matchesDateTime(s) {
		matches = append(matches, TokenDateTime)
	}
	if matchesDate(s) {
		matches = append(matches, TokenDate)
	}
	if len(s) == 6 && timeRE.MatchString(s) {
		matches = append(matches, TokenTime)
	}

	if hasASCIIDigit(s) && (versionRE.MatchString(s) || (startsWithASCIIAlpha(s) && embeddedVersionRE.MatchString(s))) {
		matches = append(matches, TokenVersion)
	}
	if strings.ContainsRune(s, '.') && semverRE.MatchString(s) {
		matches = append(matches, TokenSemVer)
	}
	if strings.ContainsRune(s, '.') && isCanonicalDottedNumber(s) {
		matches = append(matches, TokenNumber)
	}
	if isCanonicalInteger(s) {
		matches = append(matches, TokenInteger)
	}
	if isHex(s) {
		matches = append(matches, TokenHex)
	}

	if len(matches) == 0 {
		matches = append(matches, TokenString)
	}
	return matches
}

func matchesDate(s string) bool {
	var layouts []string
	switch len(s) {
	case 8:
		if !allDigits(s) {
			return false
		}
		layouts = dateLayouts[:1]
	case 10:
		if !((s[4] == '-' || s[4] == '.') && s[7] == s[4] && allDigits(s[:4]) && allDigits(s[5:7]) && allDigits(s[8:])) {
			return false
		}
		layouts = dateLayouts[1:]
	default:
		return false
	}
	for _, layout := range layouts {
		if _, err := time.Parse(layout, s); err == nil {
			return true
		}
	}
	return false
}

func matchesDateTime(s string) bool {
	if !looksLikeDateTime(s) {
		return false
	}
	layouts := dateTimeLayouts[:1]
	if len(s) >= 19 {
		layouts = dateTimeLayouts
	}
	for _, layout := range layouts {
		if _, err := time.Parse(layout, s); err == nil {
			return true
		}
	}
	return false
}

func looksLikeDateTime(s string) bool {
	if len(s) >= 14 && allDigits(s[:14]) {
		return true
	}
	return len(s) >= 19 && (s[4] == '-') && s[7] == '-' && s[10] == 'T' && s[13] == ':' && s[16] == ':' &&
		allDigits(s[:4]) && allDigits(s[5:7]) && allDigits(s[8:10]) && allDigits(s[11:13]) && allDigits(s[14:16]) && allDigits(s[17:19])
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isASCIIDigit(s[i]) {
			return false
		}
	}
	return true
}

func hasASCIIDigit(s string) bool {
	for i := 0; i < len(s); i++ {
		if isASCIIDigit(s[i]) {
			return true
		}
	}
	return false
}

func isCanonicalDottedNumber(s string) bool {
	start := 0
	dots := 0
	for i := 0; i <= len(s); i++ {
		if i != len(s) && s[i] != '.' {
			continue
		}
		if !isCanonicalInteger(s[start:i]) {
			return false
		}
		if i < len(s) {
			dots++
			start = i + 1
		}
	}
	return dots > 0
}

func isHex(s string) bool {
	if len(s) < 7 || len(s) > 40 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !isASCIIDigit(c) && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

func startsWithASCIIAlpha(s string) bool {
	return len(s) > 0 && isASCIIAlpha(s[0])
}
