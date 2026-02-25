package pipeline

import (
	"github.com/henrocdotnet/grumbler/internal/config"
	"github.com/henrocdotnet/grumbler/internal/llm"
	"github.com/henrocdotnet/grumbler/internal/model"
	"github.com/henrocdotnet/grumbler/internal/rules"
)

// ReviewContext carries mutable state through the pipeline stages.
type ReviewContext struct {
	// Inputs
	Config   *config.Config
	Provider llm.Provider
	RepoDir  string
	Files    []model.FileChange
	Rules    []rules.Rule

	// Running state
	Suggestions []model.CodeSuggestion
	PassesRun   []string
	Errors      []error

	// Options
	DiffMode int  // 0=staged, 1=all, 2=branch (matches git.DiffMode)
	Fast     bool // diff-only mode, skip full file content
}

// AddSuggestions appends suggestions (thread-safe not needed — stages sync via pipeline).
func (rc *ReviewContext) AddSuggestions(ss []model.CodeSuggestion) {
	rc.Suggestions = append(rc.Suggestions, ss...)
}

// AddError records a non-fatal error.
func (rc *ReviewContext) AddError(err error) {
	rc.Errors = append(rc.Errors, err)
}

// Result produces the final ReviewResult.
func (rc *ReviewContext) Result() model.ReviewResult {
	return model.ReviewResult{
		Suggestions: rc.Suggestions,
		FilesCount:  len(rc.Files),
		PassesRun:   rc.PassesRun,
		Provider:    rc.Provider.Name(),
	}
}
