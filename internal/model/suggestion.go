package model

// CodeSuggestion represents a single review suggestion for a code change.
type CodeSuggestion struct {
	ID          string   `json:"id" yaml:"id"`
	FilePath    string   `json:"filePath" yaml:"filePath"`
	Language    string   `json:"language" yaml:"language"`
	Severity    Severity `json:"-" yaml:"-"`
	SeverityStr string   `json:"severity" yaml:"severity"`
	Category    string   `json:"category" yaml:"category"`
	Title       string   `json:"title" yaml:"title"`
	Description string   `json:"description" yaml:"description"`
	Snippet     string   `json:"snippet" yaml:"snippet"`
	Proposal    string   `json:"proposal" yaml:"proposal"`
	Synopsis    string   `json:"synopsis" yaml:"synopsis"`
	StartLine   int      `json:"startLine" yaml:"startLine"`
	EndLine     int      `json:"endLine" yaml:"endLine"`
	RuleIDs     []string `json:"ruleIds,omitempty" yaml:"ruleIds,omitempty"`
	AuditResult string   `json:"auditResult,omitempty" yaml:"auditResult,omitempty"` // fix|update|discard
}

// ReviewResult holds the complete output of a review run.
type ReviewResult struct {
	Suggestions           []CodeSuggestion `json:"suggestions"`
	FilesCount            int              `json:"filesReviewed"`
	PassesRun             []string         `json:"passesRun"`
	Provider              string           `json:"provider"`
	Model                 string           `json:"model,omitempty"`
	TotalPromptTokens     int              `json:"totalPromptTokens"`
	TotalCompletionTokens int              `json:"totalCompletionTokens"`
}
