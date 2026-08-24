package taganalyzer

type TokenType string

const (
	TokenVersion  TokenType = "version"
	TokenDate     TokenType = "date"
	TokenDateTime TokenType = "datetime"
	TokenTime     TokenType = "time"
	TokenInteger  TokenType = "integer"
	TokenString   TokenType = "string"
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

type Token struct {
	Value     string
	Separator string // Separators immediately preceding this token.
}

type TokenClassification struct {
	Token   Token
	Matches []TokenType
}

type PrereleaseIdentifier struct {
	Value    string
	Number   *int64
	IsNumber bool
}

type Prerelease struct {
	Type        string
	Number      *int64
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
	Tokens   []TokenClassification
	Segments []SegmentAnalysis
}

type FamilyKind string

const (
	FamilyBlood    FamilyKind = "blood"
	FamilyStep     FamilyKind = "step"
	FamilyAncestor FamilyKind = "ancestor"
)

type Family struct {
	ID        int
	Key       string
	TagCount  int
	Tags      []string
	Kind      FamilyKind
	StepLevel int
}

type OrderedFamily struct {
	Family
	OrderedTags []string
}

type Analysis struct {
	Tags     []TagAnalysis
	Families []Family
	Ordered  []OrderedFamily
}

type AnalysisOptions struct {
	IncludeTokens bool
}
