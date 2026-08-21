package taganalyzer

type TokenType string

const (
	TokenVersion  TokenType = "version"
	TokenSemVer   TokenType = "semver"
	TokenDate     TokenType = "date"
	TokenDateTime TokenType = "datetime"
	TokenTime     TokenType = "time"
	TokenInteger  TokenType = "integer"
	TokenNumber   TokenType = "number"
	TokenHex      TokenType = "hex"
	TokenString   TokenType = "string"
)

type OrderType string

const (
	OrderNumeric      OrderType = "numeric"
	OrderSemVer       OrderType = "semver"
	OrderAlphabetical OrderType = "alphabetical"
	OrderVersion      OrderType = "version"
	OrderDate         OrderType = "date"
	OrderDateTime     OrderType = "datetime"
	OrderTime         OrderType = "time"
	OrderLiteral      OrderType = "literal"
	OrderUnknown      OrderType = "unknown"
)

type Token struct {
	Value             string
	Start             int
	End               int
	Separator         string // Separators immediately preceding this token.
	TrailingSeparator string // Separators following the final token in a tag.
}

type TokenPart struct {
	Value string
	Start int
	End   int
	Kind  string
}

type TokenClassification struct {
	Token   Token
	Matches []TokenType
	Parts   []TokenPart
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
	Prefix        string
	Numbers       []int64
	Suffix        string
	Prerelease    *Prerelease
	BuildMetadata string
}

type SegmentAnalysis struct {
	Raw           string
	OrderType     OrderType
	Prefix        string
	Numbers       []int64
	Suffix        string
	Prerelease    *Prerelease
	BuildMetadata string
	IsVariable    bool
	sortKey       string // Canonical value used internally for temporal ordering.
}

type TagAnalysis struct {
	Tag           string
	Tokens        []TokenClassification
	Segments      []SegmentAnalysis
	FamilyID      int // Effective family: blood family when available, otherwise step family.
	BloodFamilyID int // Exact structural family; zero when this tag is a singleton blood family.
}

type TokenRelationship struct {
	Position       int
	Separator      string
	Value          string
	Occurrences    int
	UniqueValues   int
	RepeatedValue  bool
	StablePosition bool
}

type PartPattern struct {
	TokenPosition int
	Pattern       string
	Occurrences   int
	Examples      []string
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
	Tags          []TagAnalysis
	Relationships []TokenRelationship
	PartPatterns  []PartPattern
	Families      []Family
	Ordered       []OrderedFamily
}

type AnalysisOptions struct {
	IncludeTokens        bool
	IncludeRelationships bool
	IncludePartPatterns  bool
}
