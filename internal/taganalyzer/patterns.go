package taganalyzer

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

// Ordered oldest-to-newest; the rank is the ordering used when comparing prereleases.
var prereleaseKeywords = []string{"dev", "snapshot", "alpha", "beta", "pre", "preview", "rc"}

var gluedPrereleaseAliases = map[string]string{"a": "alpha", "b": "beta"}

var prereleaseAlternation = buildPrereleaseAlternation()

var prereleaseTailPattern = `(?:[._]?(?:` + prereleaseAlternation + `)[._]?[0-9]*(?:[._](?:` + prereleaseAlternation + `)[._]?[0-9]*)*)?`

func buildPrereleaseAlternation() string {
	sorted := append([]string(nil), prereleaseKeywords...)
	sort.Slice(sorted, func(i, j int) bool { return len(sorted[i]) > len(sorted[j]) })
	return strings.Join(sorted, "|")
}

var (
	versionRE = regexp.MustCompile(
		`(?i)^[A-Za-z]*(?:0|[1-9][0-9]*)(?:\.[0-9]+)*` + prereleaseTailPattern + `$`,
	)

	embeddedVersionRE = regexp.MustCompile(
		`(?i)^[A-Za-z]+[0-9]+(?:\.[0-9]+)*` + prereleaseTailPattern + `$`,
	)

	gluedSuffixVersionRE = regexp.MustCompile(
		`(?i)^(?:0|[1-9][0-9]*)(?:\.[0-9]+)*(?:a|b|p|u)[0-9]*$`,
	)

	timeRE = regexp.MustCompile(`^(?:[01][0-9]|2[0-3])[0-5][0-9][0-5][0-9]$`)

	dateRespinRE = regexp.MustCompile(`^([0-9]{8})\.([0-9]+)$`)

	hashRE = regexp.MustCompile(`^g?[0-9a-f]{7,40}$`)
)

const dateLayout = "20060102"
const dateTimeLayout = "20060102150405"

func classifyToken(s string) []tokenType {
	var matches []tokenType

	if matchesDateTime(s) {
		matches = append(matches, tokenDateTimeType)
	}
	if matchesDate(s) {
		matches = append(matches, tokenDateType)
	}
	if len(s) == 6 && timeRE.MatchString(s) {
		matches = append(matches, tokenTimeType)
	}

	digits := allDigits(s)
	if hasASCIIDigit(s) && (digits || versionRE.MatchString(s) || (startsWithASCIIAlpha(s) && embeddedVersionRE.MatchString(s)) || gluedSuffixVersionRE.MatchString(s)) {
		matches = append(matches, tokenVersionType)
	}
	if digits {
		matches = append(matches, tokenIntegerType)
	}

	if len(matches) == 0 {
		matches = append(matches, tokenStringType)
	}
	return matches
}

func matchesDate(s string) bool {
	if len(s) == 8 && allDigits(s) {
		_, err := time.Parse(dateLayout, s)
		return err == nil
	}
	if m := dateRespinRE.FindStringSubmatch(s); m != nil {
		_, err := time.Parse(dateLayout, m[1])
		return err == nil
	}
	return false
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

func looksLikeHash(s string) bool {
	return hashRE.MatchString(s)
}

func hasHexLetter(s string) bool {
	s = strings.TrimPrefix(s, "g")
	for i := 0; i < len(s); i++ {
		if s[i] >= 'a' && s[i] <= 'f' {
			return true
		}
	}
	return false
}
