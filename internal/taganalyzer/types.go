package taganalyzer

type tokenType string

const (
	tokenVersionType  tokenType = "version"
	tokenDateType     tokenType = "date"
	tokenDateTimeType tokenType = "datetime"
	tokenTimeType     tokenType = "time"
	tokenIntegerType  tokenType = "integer"
	tokenStringType   tokenType = "string"
)

type OrderType string

const (
	OrderAlphabetical OrderType = "alphabetical"
	OrderVersion      OrderType = "version"
	OrderDate         OrderType = "date"
	OrderDateTime     OrderType = "datetime"
	OrderTime         OrderType = "time"
	OrderUnknown      OrderType = "unknown"
)

type rawToken struct {
	Value     string
	Separator string // Separators immediately preceding this token.
}

type tokenClassification struct {
	Token   rawToken
	Matches []tokenType
}

type PrereleaseIdentifier struct {
	Value    string
	Number   *int64
	IsNumber bool
}

type Prerelease struct {
	Identifiers []PrereleaseIdentifier
}

type VersionStructure struct {
	Prefix     string
	Numbers    []int64
	Suffix     string
	Prerelease *Prerelease
}

type SegmentAnalysis struct {
	Raw        string
	OrderType  OrderType
	Prefix     string
	Numbers    []int64
	Suffix     string
	Prerelease *Prerelease
	IsVariable bool
	sortKey    string // Canonical value used internally for temporal ordering.
}

type TagAnalysis struct {
	Tag      string
	Segments []SegmentAnalysis
}

type FamilyKind string

const (
	FamilyBlood    FamilyKind = "blood"
	FamilyStep     FamilyKind = "step"
	FamilyAncestor FamilyKind = "ancestor"
)

type Family struct {
	ID       int64
	Key      string
	TagCount int
	Tags     []string
	Kind     FamilyKind
	HasOrder bool
}

type OrderedFamily struct {
	Family
	OrderedTags []string
}

type Analysis struct {
	Tags    []TagAnalysis
	Ordered []OrderedFamily
}
