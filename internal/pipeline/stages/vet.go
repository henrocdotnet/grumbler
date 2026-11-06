package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/henrocdotnet/grumbler/internal/llm"
	glog "github.com/henrocdotnet/grumbler/internal/log"
	"github.com/henrocdotnet/grumbler/internal/model"
	"github.com/henrocdotnet/grumbler/internal/pipeline"
	"github.com/henrocdotnet/grumbler/internal/prompt"
	"golang.org/x/sync/errgroup"
)

// Audit verifies findings using supporting code and checks proposals separately.
type Audit struct{}

func (Audit) Name() string { return "audit" }

func (Audit) Execute(ctx context.Context, rc *pipeline.ReviewContext) error {
	glog.L().Debug("audit entry", "enabled", rc.Config.Passes.Audit, "suggestions", len(rc.Suggestions))
	if !rc.Config.Passes.Audit || len(rc.Suggestions) == 0 {
		glog.L().Debug("audit skipped")
		return nil
	}

	// Group suggestions by file
	byFile := groupSuggestionsByFile(rc.Suggestions)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(effectiveConcurrency(rc.Config.Review.Concurrency))

	var mu sync.Mutex

	for filePath, suggestions := range byFile {
		filePath := filePath
		suggestions := suggestions

		// Find matching file
		var file model.FileChange
		for _, f := range rc.Files {
			if f.Path == filePath {
				file = f
				break
			}
		}

		g.Go(func() error {
			verdicts, err := auditFile(gctx, rc, file, suggestions)
			if err != nil {
				glog.L().Error("audit file failed", "path", filePath, "err", err)
				rc.AddError(fmt.Errorf("audit %s: %w", filePath, err))
				mu.Lock()
				for i := range rc.Suggestions {
					if rc.Suggestions[i].FilePath == filePath {
						rc.Suggestions[i].AuditResult = "unresolved"
						rc.Suggestions[i].AuditRationale = err.Error()
					}
				}
				mu.Unlock()
				return nil
			}
			glog.L().Debug("audit file done", "path", filePath, "verdicts", len(verdicts))
			mu.Lock()
			applyAuditVerdicts(rc, filePath, verdicts)
			mu.Unlock()
			return nil
		})
	}

	return g.Wait()
}

func auditFile(ctx context.Context, rc *pipeline.ReviewContext, file model.FileChange, suggestions []model.CodeSuggestion) ([]auditVerdict, error) {
	msgs, err := auditMessages(rc, file, suggestions)
	if err != nil {
		return nil, err
	}

	// One retry loop, not two. This previously wrapped llm.Retry(ctx, 2, ...)
	// in its own 2-iteration loop, costing up to 4 serial provider calls for a
	// single file. A single loop that retries on transport *or* parse failure
	// caps a file at auditAttempts calls.
	const auditAttempts = 2

	var lastErr error
	for attempt := 0; attempt < auditAttempts; attempt++ {
		resp, err := llm.Retry(ctx, 1, rc.Provider, msgs, llm.DefaultOpts())
		if err != nil {
			lastErr = err
			glog.L().Warn("audit call retry", "path", file.Path, "attempt", attempt, "err", err)
			continue
		}
		glog.L().Debug("audit response", "respLen", len(resp), "attempt", attempt)
		verdicts, err := parseAuditResponse(resp)
		if err == nil {
			err = validateAuditVerdicts(verdicts, suggestions)
		}
		if err == nil {
			for _, v := range verdicts {
				if v.Verdict == "revise" && v.StartLine > 0 && !intersectsAddedLine(file.Patch, v.StartLine, v.EndLine) {
					err = fmt.Errorf("audit: revised location for %q does not intersect changed lines", v.ID)
					break
				}
			}
		}
		if err != nil {
			lastErr = err
			glog.L().Warn("audit parse retry", "path", file.Path, "attempt", attempt, "err", err)
			continue
		}
		return verdicts, nil
	}
	return nil, lastErr
}

// auditMessages keeps cross-file evidence and counter-evidence available to
// audit, even when discovery placed the files in separate inspect batches.
// The budget covers both rendered prompts, including candidates and markers.
func auditMessages(rc *pipeline.ReviewContext, file model.FileChange, suggestions []model.CodeSuggestion) ([]llm.Message, error) {
	systemPrompt, err := prompt.Render("safeguard_system", &prompt.SafeguardData{})
	if err != nil {
		return nil, err
	}
	sugJSON, err := json.Marshal(suggestions)
	if err != nil {
		return nil, err
	}
	data := prompt.SafeguardData{
		FileContent:     contentForPrompt(file, rc.Fast, rc.Config.Review.MaxFileContentTokens),
		Diff:            file.Patch,
		SuggestionsJSON: string(sugJSON),
	}
	budget := contextBudget(rc.Config.Passes.MaxContextTokens)
	const omitted = "\n[Context limited: some changed-file content was omitted. Missing evidence is not counter-evidence; use unresolved when needed.]\n"
	render := func() (string, error) { return prompt.Render("safeguard_user", &data) }
	userPrompt, err := render()
	if err != nil {
		return nil, err
	}
	// Leave room for the omission marker even if no supporting file fits.
	remaining := budget - countTokens(systemPrompt+userPrompt+omitted) - 32
	if remaining < 0 {
		return nil, fmt.Errorf("audit context for %s exceeds %d tokens", file.Path, budget)
	}
	files := append([]model.FileChange(nil), rc.Files...)
	// Prioritize explicitly cited evidence; stable ordering makes omissions
	// reproducible. All remaining changed files can supply guards or tests.
	sort.SliceStable(files, func(i, j int) bool {
		return strings.Contains(string(sugJSON), files[i].Path) && !strings.Contains(string(sugJSON), files[j].Path)
	})
	limited := data.FileContent != file.Content && file.Content != ""
	// Preserve changed-line coverage before spending the remaining budget on
	// supplementary source. Otherwise early large files crowd out later callers
	// and tests, recreating the context loss audit is meant to avoid.
	for _, other := range files {
		if other.Path == file.Path {
			continue
		}
		chunk := fmt.Sprintf("=== %s ===\nPatch:\n%s\n\n", other.Path, other.Patch)
		cost := countTokens(chunk)
		if cost > remaining {
			limited = true
			continue
		}
		data.SupportingContext += chunk
		remaining -= cost
	}
	for _, other := range files {
		if other.Path == file.Path || other.Content == "" {
			continue
		}
		content := contentForPrompt(other, rc.Fast, rc.Config.Review.MaxFileContentTokens)
		if content == "" {
			limited = true
			continue
		}
		chunk := fmt.Sprintf("=== %s (source) ===\n%s\n\n", other.Path, content)
		cost := countTokens(chunk)
		if cost > remaining {
			limited = true
			continue
		}
		data.SupportingContext += chunk
		remaining -= cost
		limited = limited || content != other.Content
	}
	if limited {
		data.SupportingContext += omitted
	}
	userPrompt, err = render()
	if err != nil {
		return nil, err
	}
	if countTokens(systemPrompt+userPrompt) > budget {
		return nil, fmt.Errorf("audit context for %s exceeds %d tokens", file.Path, budget)
	}
	return []llm.Message{llm.SystemMsg(systemPrompt), llm.UserMsg(userPrompt)}, nil
}

type auditVerdict struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Snippet     string  `json:"snippet"`
	Proposal    *string `json:"proposal"`
	Synopsis    string  `json:"synopsis"`
	StartLine   int     `json:"startLine"`
	EndLine     int     `json:"endLine"`
	Category    string  `json:"category"`
	Severity    string  `json:"severity"`
	Verdict     string  `json:"verdict"` // keep, revise, reject, unresolved
	Rationale   string  `json:"rationale"`
}

type auditResponse struct {
	Reviews []auditVerdict `json:"reviews"`
}

func parseAuditResponse(raw string) ([]auditVerdict, error) {
	extracted := llm.ExtractJSON(raw)

	// Try object shape first: {"reviews": [...]}
	var resp auditResponse
	if err := json.Unmarshal([]byte(extracted), &resp); err == nil && resp.Reviews != nil {
		return resp.Reviews, nil
	}

	// Fallback: bare array [{...}, ...]
	var verdicts []auditVerdict
	if err := json.Unmarshal([]byte(extracted), &verdicts); err != nil || verdicts == nil {
		preview := raw
		if len(preview) > 200 {
			preview = preview[:200]
		}
		return nil, fmt.Errorf("audit: expected {\"reviews\":[...]} or bare array\n  response preview: %s", preview)
	}

	glog.L().Warn("audit: LLM returned bare array instead of {\"reviews\":[...]}")
	return verdicts, nil
}

func validateAuditVerdicts(verdicts []auditVerdict, suggestions []model.CodeSuggestion) error {
	expected := make(map[string]bool, len(suggestions))
	for _, s := range suggestions {
		expected[s.ID] = false
	}
	for _, v := range verdicts {
		seen, ok := expected[v.ID]
		if !ok || seen {
			return fmt.Errorf("audit: unexpected or duplicate candidate ID %q", v.ID)
		}
		switch v.Verdict {
		case "keep", "reject", "unresolved":
		case "revise":
			if v.Proposal == nil {
				return fmt.Errorf("audit: revised candidate %q needs a proposal (empty clears it)", v.ID)
			}
			if v.Severity != "" && v.Severity != "low" && v.Severity != "medium" && v.Severity != "high" && v.Severity != "critical" {
				return fmt.Errorf("audit: invalid revised severity %q for %q", v.Severity, v.ID)
			}
			if (v.StartLine != 0 || v.EndLine != 0) && (v.StartLine <= 0 || v.EndLine < v.StartLine) {
				return fmt.Errorf("audit: invalid revised location for %q", v.ID)
			}
		default:
			return fmt.Errorf("audit: invalid verdict %q for %q", v.Verdict, v.ID)
		}
		if strings.TrimSpace(v.Rationale) == "" {
			return fmt.Errorf("audit: missing rationale for %q", v.ID)
		}
		expected[v.ID] = true
	}
	for id, seen := range expected {
		if !seen {
			return fmt.Errorf("audit: missing verdict for %q", id)
		}
	}
	return nil
}

func applyAuditVerdicts(rc *pipeline.ReviewContext, filePath string, verdicts []auditVerdict) {
	verdictMap := make(map[string]*auditVerdict, len(verdicts))
	for i := range verdicts {
		// IDs remain stable across discovery, compliance, and cross-file review.
		verdictMap[verdicts[i].ID] = &verdicts[i]
	}

	for i := range rc.Suggestions {
		s := &rc.Suggestions[i]
		if s.FilePath != filePath {
			continue
		}
		v, ok := verdictMap[s.ID]
		if !ok {
			continue
		}

		applyAuditVerdict(s, v)
	}
}

// applyAuditVerdict records one verdict on a suggestion; "revise" also
// replaces revised fields and can explicitly clear an unsafe proposal.
func applyAuditVerdict(s *model.CodeSuggestion, v *auditVerdict) {
	s.AuditRationale = v.Rationale
	switch v.Verdict {
	case "reject":
		s.AuditResult = "discard"
	case "revise":
		s.AuditResult = "update"
		if v.Title != "" {
			s.Title = v.Title
		} else if v.Synopsis != "" && ((v.Category != "" && v.Category != s.Category) || (v.StartLine > 0 && v.StartLine != s.StartLine)) {
			// Older prompt overrides do not emit title. A reclassified or
			// relocated finding still needs a heading describing its new claim.
			s.Title = v.Synopsis
		}
		if v.Proposal != nil {
			s.Proposal = *v.Proposal
		}
		if v.Description != "" {
			s.Description = v.Description
		}
		if v.Snippet != "" {
			s.Snippet = v.Snippet
		}
		if v.StartLine > 0 {
			s.StartLine, s.EndLine = v.StartLine, v.EndLine
		}
		if v.Synopsis != "" {
			s.Synopsis = v.Synopsis
		}
		if v.Category != "" {
			s.Category = v.Category
		}
		if v.Severity != "" {
			s.Severity = model.ParseSeverity(v.Severity)
			s.SeverityStr = s.Severity.String()
		}
	case "keep":
		s.AuditResult = "fix"
	case "unresolved":
		s.AuditResult = "unresolved"
		if v.Proposal != nil && *v.Proposal == "" {
			s.Proposal = ""
		}
	default:
		s.AuditResult = "unresolved"
	}
}

func groupSuggestionsByFile(suggestions []model.CodeSuggestion) map[string][]model.CodeSuggestion {
	m := make(map[string][]model.CodeSuggestion)
	for _, s := range suggestions {
		m[s.FilePath] = append(m[s.FilePath], s)
	}
	return m
}
