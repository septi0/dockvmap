package taganalyzer

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

var prereleaseRE = regexp.MustCompile(`(?i)^(` + prereleaseAlternation + `)(?:[._]?([0-9]+))?$`)

var gluedPrereleaseRE = regexp.MustCompile(`(?i)^(a|b)([0-9]+)?$`)

var patchSuffixRE = regexp.MustCompile(`(?i)^p([0-9]+)$`)

var updateSuffixRE = regexp.MustCompile(`(?i)^u([0-9]+)$`)

var revisionSuffixRE = regexp.MustCompile(`(?i)^r([0-9]+)$`)

func normalizeSegments(tokens []tokenClassification) []SegmentAnalysis {
	segments := make([]SegmentAnalysis, 0, len(tokens))

	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		raw := token.Token.Value

		if hasMatch(token.Matches, tokenDateTimeType) {
			segments = append(segments, SegmentAnalysis{Raw: raw, OrderType: OrderDateTime, IsVariable: true, sortKey: temporalSortKey(raw, dateTimeLayout)})
			continue
		}
		if hasMatch(token.Matches, tokenDateType) {
			numbers, sortKey := parseDate(raw)
			segments = append(segments, SegmentAnalysis{Raw: raw, OrderType: OrderDate, IsVariable: true, Numbers: numbers, sortKey: sortKey})
			continue
		}

		if hasMatch(token.Matches, tokenTimeType) && i > 0 && len(segments) > 0 &&
			(segments[len(segments)-1].OrderType == OrderDate || segments[len(segments)-1].OrderType == OrderDateTime) &&
			token.Token.Separator == "_" {
			segments = append(segments, SegmentAnalysis{Raw: raw, OrderType: OrderTime, IsVariable: true})
			continue
		}

		if i > 0 && len(segments) > 0 && segments[len(segments)-1].OrderType == OrderVersion {
			if parsed, ok := parsePrereleaseToken(raw); ok {
				prerelease := appendPrerelease(segments[len(segments)-1:], parsed)

				if last := len(prerelease.Identifiers) - 1; last >= 0 && prerelease.Identifiers[last].Number == nil &&
					i+1 < len(tokens) && hasMatch(tokens[i+1].Matches, tokenIntegerType) {
					if n, err := strconv.ParseInt(tokens[i+1].Token.Value, 10, 64); err == nil {
						prerelease.Identifiers[last].Number = &n
						i++
					}
				}
				continue
			}

			// Alpine/apk package revision: 1.2.3-r1 is a rebuild of 1.2.3, so it folds
			// into the version rather than becoming a segment that blocks grouping.
			last := &segments[len(segments)-1]
			if revision, ok := parseRevisionSuffix(raw); ok && last.Prerelease == nil {
				last.Numbers = append(last.Numbers, revision)
				continue
			}
		}

		if hasMatch(token.Matches, tokenVersionType) {
			if version, ok := parseVersionStructure(raw); ok {
				segments = append(segments, SegmentAnalysis{
					Raw: raw, OrderType: OrderVersion, Prefix: version.Prefix,
					Numbers: version.Numbers, Suffix: normalizeVersionSuffix(version),
					Prerelease: version.Prerelease,
					IsVariable: true,
				})
				continue
			}
		}

		segments = append(segments, SegmentAnalysis{Raw: raw, OrderType: OrderAlphabetical, IsVariable: false})
	}

	return segments
}

func temporalSortKey(value, layout string) string {
	parsed, err := time.Parse(layout, value)
	if err != nil {
		return ""
	}
	return parsed.UTC().Format("20060102150405.000000000Z")
}

func parseDate(value string) (numbers []int64, sortKey string) {
	base := value
	var respin int64
	if m := dateRespinRE.FindStringSubmatch(value); m != nil {
		base = m[1]
		respin, _ = strconv.ParseInt(m[2], 10, 64)
	}

	parsed, err := time.Parse(dateLayout, base)
	if err != nil {
		return nil, ""
	}
	y, mo, d := parsed.Date()
	sortTime := parsed.Add(time.Duration(respin) * time.Second)
	return []int64{int64(y), int64(mo), int64(d)}, sortTime.UTC().Format("20060102150405.000000000Z")
}

func parseVersionStructure(value string) (VersionStructure, bool) {
	if value == "" {
		return VersionStructure{}, false
	}

	base := value
	i := 0
	for i < len(base) && isASCIIAlpha(base[i]) {
		i++
	}
	prefix := base[:i]

	var numbers []int64
	for {
		if i >= len(base) || !isASCIIDigit(base[i]) {
			return VersionStructure{}, false
		}
		start := i
		for i < len(base) && isASCIIDigit(base[i]) {
			i++
		}
		component := base[start:i]
		if !isDigits(component) {
			return VersionStructure{}, false
		}
		n, err := strconv.ParseInt(component, 10, 64)
		if err != nil {
			return VersionStructure{}, false
		}
		numbers = append(numbers, n)

		if i >= len(base) || base[i] != '.' || i+1 >= len(base) || !isASCIIDigit(base[i+1]) {
			break
		}
		i++
	}

	suffix := base[i:]
	var prerelease *Prerelease
	if suffix != "" {
		prerelease = parsePrereleaseSequence(suffix)
		if prerelease == nil && prefix == "" {
			prerelease = parseGluedPrereleaseSuffix(suffix)
		}
		if prerelease == nil && prefix == "" {
			if patch, ok := parsePatchSuffix(suffix); ok {
				numbers = append(numbers, patch)
				suffix = ""
			}
		}
		if prerelease == nil && prefix == "" {
			if update, ok := parseUpdateSuffix(suffix); ok {
				numbers = append(numbers, update)
				suffix = ""
			}
		}
		if prerelease == nil && suffix != "" && prefix == "" {
			return VersionStructure{}, false
		}
	}

	return VersionStructure{Prefix: prefix, Numbers: numbers, Suffix: suffix, Prerelease: prerelease}, true
}

func parsePrereleaseToken(value string) (*Prerelease, bool) {
	p := parsePrereleaseSequence(value)
	return p, p != nil
}

func parsePrereleaseSequence(value string) *Prerelease {
	if value == "" {
		return nil
	}

	parts := strings.FieldsFunc(value, func(r rune) bool { return r == '.' || r == '_' || r == '-' })
	if len(parts) == 0 {
		return nil
	}

	result := &Prerelease{}
	sawKeyword := false
	for _, part := range parts {
		if part == "" {
			return nil
		}
		if m := prereleaseRE.FindStringSubmatch(part); m != nil {
			sawKeyword = true
			id := PrereleaseIdentifier{Value: strings.ToLower(m[1])}
			if m[2] != "" {
				n, err := strconv.ParseInt(m[2], 10, 64)
				if err != nil {
					return nil
				}
				id.Number = &n
				id.IsNumber = false
			}
			result.Identifiers = append(result.Identifiers, id)
			continue
		}
		if isDigits(part) {
			n, err := strconv.ParseInt(part, 10, 64)
			if err != nil {
				return nil
			}

			if len(result.Identifiers) > 0 {
				last := &result.Identifiers[len(result.Identifiers)-1]
				if last.Number == nil && !last.IsNumber && prereleaseRank(last.Value) < unrankedPrerelease {
					last.Number = &n
					continue
				}
			}
			result.Identifiers = append(result.Identifiers, PrereleaseIdentifier{Value: part, Number: &n, IsNumber: true})
			continue
		}
		return nil
	}

	if len(result.Identifiers) == 0 || !sawKeyword {
		return nil
	}
	return result
}

func parseGluedPrereleaseSuffix(value string) *Prerelease {
	m := gluedPrereleaseRE.FindStringSubmatch(value)
	if m == nil {
		return nil
	}

	id := PrereleaseIdentifier{Value: strings.ToLower(m[1])}
	if m[2] != "" {
		n, err := strconv.ParseInt(m[2], 10, 64)
		if err != nil {
			return nil
		}
		id.Number = &n
	}

	return &Prerelease{Identifiers: []PrereleaseIdentifier{id}}
}

func parsePatchSuffix(value string) (int64, bool) {
	m := patchSuffixRE.FindStringSubmatch(value)
	if m == nil {
		return 0, false
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func parseRevisionSuffix(value string) (int64, bool) {
	m := revisionSuffixRE.FindStringSubmatch(value)
	if m == nil {
		return 0, false
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func parseUpdateSuffix(value string) (int64, bool) {
	m := updateSuffixRE.FindStringSubmatch(value)
	if m == nil {
		return 0, false
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func appendPrerelease(dst []SegmentAnalysis, p *Prerelease) *Prerelease {
	if len(dst) == 0 || p == nil {
		return p
	}
	current := dst[0].Prerelease
	if current == nil {
		dst[0].Prerelease = p
		return p
	}
	current.Identifiers = append(current.Identifiers, p.Identifiers...)
	return current
}

func normalizeVersionSuffix(version VersionStructure) string {
	if version.Prerelease != nil {
		return ""
	}
	return version.Suffix
}

func hasMatch(matches []tokenType, wanted tokenType) bool {
	for _, match := range matches {
		if match == wanted {
			return true
		}
	}
	return false
}

func isDigits(s string) bool {
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
func isASCIIAlpha(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
func isASCIIDigit(c byte) bool { return c >= '0' && c <= '9' }
