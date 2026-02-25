package model

// CodeSuggestion represents a single review suggestion for a code change.
type CodeSuggestion struct {
	ID               string   `json:"id" yaml:"id"`
	FilePath         string   `json:"filePath" yaml:"filePath"`
	Language         string   `json:"language" yaml:"language"`
	Severity         Severity `json:"-" yaml:"-"`
	SeverityStr      string   `json:"severity" yaml:"severity"`
	Category         string   `json:"category" yaml:"category"`
	Title            string   `json:"title" yaml:"title"`
	Description      string   `json:"description" yaml:"description"`
	ExistingCode     string   `json:"existingCode" yaml:"existingCode"`
	ImprovedCode     string   `json:"improvedCode" yaml:"improvedCode"`
	OneSentSummary   string   `json:"oneSentenceSummary" yaml:"oneSentenceSummary"`
	StartLine        int      `json:"startLine" yaml:"startLine"`
	EndLine          int      `json:"endLine" yaml:"endLine"`
	RuleIDs          []string `json:"ruleIds,omitempty" yaml:"ruleIds,omitempty"`
	SafeguardVerdict string   `json:"safeguardVerdict,omitempty" yaml:"safeguardVerdict,omitempty"` // keep|update|discard
}

// ReviewResult holds the complete output of a review run.
type ReviewResult struct {
	Suggestions []CodeSuggestion `json:"suggestions"`
	FilesCount  int              `json:"filesReviewed"`
	PassesRun   []string         `json:"passesRun"`
	Provider    string           `json:"provider"`
	Model       string           `json:"model,omitempty"`
}
