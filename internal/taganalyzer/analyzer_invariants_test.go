package taganalyzer

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

// fingerprint mirrors cmd/tagaudit: family keys + ordered tags only, since
// Analysis.Tags and Family.Tags follow input order by design.
func fingerprint(a Analysis) string {
	var b strings.Builder
	for _, family := range a.Ordered {
		b.WriteString(family.Key)
		b.WriteByte('\n')
		b.WriteString(strings.Join(family.OrderedTags, ","))
		b.WriteByte('\n')
	}
	return b.String()
}

// sampleTags is a small hand-built set that exercises blood/ancestor/step
// grouping, a named base axis, prereleases, a date family and a commit hash.
var sampleTags = []string{
	"1.2.0", "1.9.0", "1.10.0", "1.11.0",
	"1.11.0-alpine", "1.10.0-alpine",
	"2.0.0-rc1", "2.0.0-rc2",
	"20240101", "20240115", "20240201",
	"jre-17", "jre-21", "jdk-17", "jdk-21",
	"a1b2c3d", "latest",
}

func flattenOrdered(a Analysis) []string {
	out := make([]string, 0)
	for _, family := range a.Ordered {
		out = append(out, family.OrderedTags...)
	}
	return out
}

func multiset(values []string) map[string]int {
	m := make(map[string]int, len(values))
	for _, v := range values {
		m[v]++
	}
	return m
}

func indexOf(values []string, target string) int {
	for i, v := range values {
		if v == target {
			return i
		}
	}
	return -1
}

func TestAnalyze_Deterministic(t *testing.T) {
	want := fingerprint(Analyze(sampleTags))

	rng := rand.New(rand.NewSource(1))

	for iteration := 0; iteration < 25; iteration++ {
		shuffled := append([]string(nil), sampleTags...)
		rng.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})

		if got := fingerprint(Analyze(shuffled)); got != want {
			t.Fatalf("Analyze is not order-independent on iteration %d\n input: %v\n got:\n%s\n want:\n%s",
				iteration, shuffled, got, want)
		}
	}
}

func TestAnalyze_EveryTagInExactlyOneFamily(t *testing.T) {
	a := Analyze(sampleTags)

	got := multiset(flattenOrdered(a))
	want := multiset(sampleTags)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("family membership is not a partition of the input\n got:  %v\n want: %v", got, want)
	}
}

func TestAnalyze_FamilyIDsUnique(t *testing.T) {
	a := Analyze(sampleTags)

	seen := make(map[int64]string, len(a.Ordered))

	for _, family := range a.Ordered {
		if prev, ok := seen[family.ID]; ok {
			t.Fatalf("duplicate family ID %d shared by %q and %q", family.ID, prev, strings.Join(family.OrderedTags, ","))
		}
		seen[family.ID] = strings.Join(family.OrderedTags, ",")
	}
}

func TestAnalyze_NoLiteralSegmentMixing(t *testing.T) {
	a := Analyze([]string{"jre-17", "jre-21", "jre-11", "jdk-17", "jdk-21", "jdk-11"})

	for _, family := range a.Ordered {
		hasJRE, hasJDK := false, false

		for _, tag := range family.OrderedTags {
			if strings.HasPrefix(tag, "jre-") {
				hasJRE = true
			}
			if strings.HasPrefix(tag, "jdk-") {
				hasJDK = true
			}
		}

		if hasJRE && hasJDK {
			t.Fatalf("family %d mixes jre and jdk: %v", family.ID, family.OrderedTags)
		}
	}
}

func TestAnalyze_LeadingVersionNeverInverted(t *testing.T) {
	a := Analyze([]string{"1.2.0", "1.9.0", "1.10.0", "1.11.0"})

	var family *OrderedFamily
	for i := range a.Ordered {
		if indexOf(a.Ordered[i].OrderedTags, "1.10.0") != -1 && indexOf(a.Ordered[i].OrderedTags, "1.9.0") != -1 {
			family = &a.Ordered[i]
			break
		}
	}

	if family == nil {
		t.Fatalf("expected 1.9.0 and 1.10.0 to share a family; got %+v", a.Ordered)
	}

	order := family.OrderedTags

	// newest first: 1.11.0 > 1.10.0 > 1.9.0 > 1.2.0, and 1.10.0 must not sort
	// before 1.9.0 lexically.
	pairs := [][2]string{
		{"1.11.0", "1.10.0"},
		{"1.10.0", "1.9.0"},
		{"1.9.0", "1.2.0"},
	}

	for _, p := range pairs {
		newer, older := indexOf(order, p[0]), indexOf(order, p[1])
		if newer == -1 || older == -1 {
			continue
		}
		if newer > older {
			t.Errorf("%q (index %d) sorted after %q (index %d) in %v", p[0], newer, p[1], older, order)
		}
	}
}
