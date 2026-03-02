package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/henrocdotnet/grumbler/internal/llm"
	glog "github.com/henrocdotnet/grumbler/internal/log"
	"github.com/henrocdotnet/grumbler/internal/model"
	"github.com/henrocdotnet/grumbler/internal/pipeline"
	"github.com/henrocdotnet/grumbler/internal/prompt"
	"golang.org/x/sync/errgroup"
)

// Audit runs the Bouncer/Syntax/Logic/Style/Arbiter audit panel.
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
	g.SetLimit(rc.Config.Review.Concurrency)

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
	systemPrompt, err := prompt.Render("safeguard_system", prompt.SafeguardData{})
	if err != nil {
		return nil, err
	}

	sugJSON, err := json.Marshal(suggestions)
	if err != nil {
		return nil, err
	}

	userPrompt, err := prompt.Render("safeguard_user", prompt.SafeguardData{
		FileContent:     file.Content,
		Diff:            file.Patch,
		SuggestionsJSON: string(sugJSON),
	})
	if err != nil {
		return nil, err
	}

	msgs := []llm.Message{
		llm.SystemMsg(systemPrompt),
		llm.UserMsg(userPrompt),
	}

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := llm.Retry(ctx, 2, rc.Provider, msgs, llm.DefaultOpts())
		if err != nil {
			lastErr = err
			continue
		}
		glog.L().Debug("audit response", "respLen", len(resp), "attempt", attempt)
		verdicts, err := parseAuditResponse(resp)
		if err != nil {
			lastErr = err
			glog.L().Warn("audit parse retry", "path", file.Path, "attempt", attempt, "err", err)
			continue
		}
		return verdicts, nil
	}
	return nil, lastErr
}

type auditVerdict struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Snippet     string `json:"snippet"`
	Proposal    string `json:"proposal"`
	Synopsis    string `json:"synopsis"`
	StartLine   int    `json:"startLine"`
	EndLine     int    `json:"endLine"`
	Category    string `json:"category"`
	Severity    string `json:"severity"`
	Verdict     string `json:"verdict"` // keep, revise, reject
	Rationale   string `json:"rationale"`
}

type auditResponse struct {
	Reviews []auditVerdict `json:"reviews"`
}

func parseAuditResponse(raw string) ([]auditVerdict, error) {
	extracted := llm.ExtractJSON(raw)

	// Try object shape first: {"reviews": [...]}
	var resp auditResponse
	if err := json.Unmarshal([]byte(extracted), &resp); err == nil {
		return resp.Reviews, nil
	}

	// Fallback: bare array [{...}, ...]
	var verdicts []auditVerdict
	if err := json.Unmarshal([]byte(extracted), &verdicts); err != nil {
		return nil, fmt.Errorf("parsing audit response: %w", err)
	}

	glog.L().Warn("audit response was bare array, expected {reviews:[...]}")
	return verdicts, nil
}

func applyAuditVerdicts(rc *pipeline.ReviewContext, filePath string, verdicts []auditVerdict) {
	verdictMap := make(map[string]*auditVerdict, len(verdicts))
	for i := range verdicts {
		// Match by index position since we may not have stable IDs
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

		switch v.Verdict {
		case "reject":
			s.AuditResult = "discard"
		case "revise":
			s.AuditResult = "update"
			if v.Proposal != "" {
				s.Proposal = v.Proposal
			}
			if v.Description != "" {
				s.Description = v.Description
			}
			if v.Synopsis != "" {
				s.Synopsis = v.Synopsis
			}
		default:
			s.AuditResult = "fix"
		}
	}
}

func groupSuggestionsByFile(suggestions []model.CodeSuggestion) map[string][]model.CodeSuggestion {
	m := make(map[string][]model.CodeSuggestion)
	for _, s := range suggestions {
		m[s.FilePath] = append(m[s.FilePath], s)
	}
	return m
}
