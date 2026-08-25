package taganalyzer

import (
	"regexp"
	"time"
)

var (
	versionRE = regexp.MustCompile(
		`(?i)^[A-Za-z]*(?:0|[1-9][0-9]*)(?:\.[0-9]+)*(?:(?:alpha|beta|rc|pre|preview|dev|snapshot)[._]?[0-9]*(?:[._](?:alpha|beta|rc|pre|preview|dev|snapshot)[._]?[0-9]*)*)?$`,
	)

	embeddedVersionRE = regexp.MustCompile(
		`(?i)^[A-Za-z]+[0-9]+(?:\.[0-9]+)*(?:(?:alpha|beta|rc|pre|preview|dev|snapshot)[._]?[0-9]*(?:[._](?:alpha|beta|rc|pre|preview|dev|snapshot)[._]?[0-9]*)*)?$`,
	)

	timeRE = regexp.MustCompile(`^(?:[01][0-9]|2[0-3])[0-5][0-9][0-5][0-9]$`)

	hashRE = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
)

const dateLayout = "20060102"
const dateTimeLayout = "20060102150405"

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
	if isCanonicalInteger(s) {
		matches = append(matches, TokenInteger)
	}

	if len(matches) == 0 {
		matches = append(matches, TokenString)
	}
	return matches
}

func matchesDate(s string) bool {
	if len(s) != 8 || !allDigits(s) {
		return false
	}
	_, err := time.Parse(dateLayout, s)
	return err == nil
}

func matchesDateTime(s string) bool {
	if len(s) < 14 || !allDigits(s[:14]) {
		return false
	}
	_, err := time.Parse(dateTimeLayout, s)
	return err == nil
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

func startsWithASCIIAlpha(s string) bool {
	return len(s) > 0 && isASCIIAlpha(s[0])
}

// looksLikeHash matches a truncated git hash or similar opaque build ID, not a version.
func looksLikeHash(s string) bool {
	return hashRE.MatchString(s)
}
