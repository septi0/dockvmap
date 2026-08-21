package taganalyzer

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

var prereleaseRE = regexp.MustCompile(`(?i)^(alpha|beta|rc|pre|preview|dev|snapshot)(?:[._]?([0-9]+))?$`)

func NormalizeSegments(tokens []TokenClassification) []SegmentAnalysis {
	segments := make([]SegmentAnalysis, 0, len(tokens))

	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		raw := token.Token.Value

		if hasMatch(token.Matches, TokenDateTime) {
			segments = append(segments, SegmentAnalysis{Raw: raw, OrderType: OrderDateTime, IsVariable: true, sortKey: temporalSortKey(raw, dateTimeLayouts)})
			continue
		}
		if hasMatch(token.Matches, TokenDate) {
			segments = append(segments, SegmentAnalysis{Raw: raw, OrderType: OrderDate, IsVariable: true, sortKey: temporalSortKey(raw, dateLayouts)})
			continue
		}

		if hasMatch(token.Matches, TokenTime) && i > 0 && len(segments) > 0 &&
			(segments[len(segments)-1].OrderType == OrderDate || segments[len(segments)-1].OrderType == OrderDateTime) &&
			token.Token.Separator == "_" {
			segments = append(segments, SegmentAnalysis{Raw: raw, OrderType: OrderTime, IsVariable: true})
			continue
		}

		if i > 0 && len(segments) > 0 && segments[len(segments)-1].OrderType == OrderVersion {
			if prerelease, build, ok := parsePrereleaseToken(raw); ok {
				appendPrerelease(segments[len(segments)-1:], prerelease)
				if build != "" {
					segments[len(segments)-1].BuildMetadata = build
				}

				if prerelease.Number == nil && i+1 < len(tokens) && hasMatch(tokens[i+1].Matches, TokenInteger) {
					if n, err := strconv.ParseInt(tokens[i+1].Token.Value, 10, 64); err == nil {
						prerelease.Number = &n
						if len(prerelease.Identifiers) > 0 {
							prerelease.Identifiers[len(prerelease.Identifiers)-1].Number = &n
							prerelease.Identifiers[len(prerelease.Identifiers)-1].IsNumber = false
						}
						i++
					}
				}
				continue
			}
		}

		if hasMatch(token.Matches, TokenVersion) {
			if version, ok := parseVersionStructure(raw); ok {
				segments = append(segments, SegmentAnalysis{
					Raw: raw, OrderType: OrderVersion, Prefix: version.Prefix,
					Numbers: version.Numbers, Suffix: normalizeVersionSuffix(version),
					Prerelease: version.Prerelease, BuildMetadata: version.BuildMetadata,
					IsVariable: true,
				})
				continue
			}
		}

		if hasMatch(token.Matches, TokenInteger) {
			if isCanonicalInteger(raw) {
				if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
					segments = append(segments, SegmentAnalysis{Raw: raw, OrderType: OrderVersion, Numbers: []int64{n}, IsVariable: true})
					continue
				}
			}
		}

		if hasMatch(token.Matches, TokenNumber) {
			if nums, ok := parseDottedNumbers(raw); ok {
				segments = append(segments, SegmentAnalysis{Raw: raw, OrderType: OrderVersion, Numbers: nums, IsVariable: true})
				continue
			}
		}

		segments = append(segments, SegmentAnalysis{Raw: raw, OrderType: OrderAlphabetical, IsVariable: false})
	}

	return segments
}

func temporalSortKey(value string, layouts []string) string {
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC().Format("20060102150405.000000000Z")
		}
	}
	return ""
}

func parseVersionStructure(value string) (VersionStructure, bool) {
	if value == "" {
		return VersionStructure{}, false
	}

	base := value
	build := ""
	if idx := strings.IndexByte(base, '+'); idx >= 0 {
		build = base[idx+1:]
		if build == "" || strings.ContainsAny(build, " +") {
			return VersionStructure{}, false
		}
		base = base[:idx]
	}

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
		if prefix == "" {
			if !isCanonicalInteger(component) {
				return VersionStructure{}, false
			}
		} else if !isDigits(component) {
			return VersionStructure{}, false
		}
		n, err := strconv.ParseInt(component, 10, 64)
		if err != nil {
			return VersionStructure{}, false
		}
		numbers = append(numbers, n)

		if i >= len(base) || base[i] != '.' {
			break
		}
		i++
	}

	suffix := base[i:]
	var prerelease *Prerelease
	if suffix != "" {
		prerelease = parsePrereleaseSequence(suffix)
		if prerelease == nil && prefix == "" {
			return VersionStructure{}, false
		}
	}

	return VersionStructure{Prefix: prefix, Numbers: numbers, Suffix: suffix, Prerelease: prerelease, BuildMetadata: build}, true
}

func parsePrereleaseToken(value string) (*Prerelease, string, bool) {
	base := value
	build := ""
	if idx := strings.IndexByte(base, '+'); idx >= 0 {
		build = base[idx+1:]
		if build == "" {
			return nil, "", false
		}
		base = base[:idx]
	}
	p := parsePrereleaseSequence(base)
	return p, build, p != nil
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
	for _, part := range parts {
		if part == "" {
			return nil
		}
		if m := prereleaseRE.FindStringSubmatch(part); m != nil {
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
				if last.Number == nil && !last.IsNumber && prereleaseRank(last.Value) < 80 {
					last.Number = &n
					continue
				}
			}
			result.Identifiers = append(result.Identifiers, PrereleaseIdentifier{Value: part, Number: &n, IsNumber: true})
			continue
		}
		return nil
	}

	if len(result.Identifiers) == 0 {
		return nil
	}
	result.Type = result.Identifiers[0].Value
	if len(result.Identifiers) == 1 && result.Identifiers[0].Number != nil && !result.Identifiers[0].IsNumber {
		result.Number = result.Identifiers[0].Number
	}
	return result
}

func appendPrerelease(dst []SegmentAnalysis, p *Prerelease) {
	if len(dst) == 0 || p == nil {
		return
	}
	current := dst[0].Prerelease
	if current == nil {
		dst[0].Prerelease = p
		return
	}
	current.Identifiers = append(current.Identifiers, p.Identifiers...)
	if len(current.Identifiers) > 0 {
		current.Type = current.Identifiers[0].Value
	}
}

func parseDottedNumbers(value string) ([]int64, bool) {
	parts := strings.Split(value, ".")
	if len(parts) < 2 {
		return nil, false
	}
	result := make([]int64, len(parts))
	for i, part := range parts {
		if !isCanonicalInteger(part) {
			return nil, false
		}
		n, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return nil, false
		}
		result[i] = n
	}
	return result, true
}

func normalizeVersionSuffix(version VersionStructure) string {
	if version.Prerelease != nil || version.BuildMetadata != "" {
		return ""
	}
	return version.Suffix
}

func hasMatch(matches []TokenType, wanted TokenType) bool {
	for _, match := range matches {
		if match == wanted {
			return true
		}
	}
	return false
}

func isCanonicalInteger(s string) bool { return s == "0" || (len(s) > 0 && s[0] != '0' && isDigits(s)) }
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
