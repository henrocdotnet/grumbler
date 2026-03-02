package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/henrocdotnet/grumbler/internal/llm"
	glog "github.com/henrocdotnet/grumbler/internal/log"
	"github.com/henrocdotnet/grumbler/internal/model"
	"github.com/henrocdotnet/grumbler/internal/pipeline"
)

// CrossFile analyzes cross-file dependencies and interactions.
type CrossFile struct{}

func (CrossFile) Name() string { return "crossfile" }

func (CrossFile) Execute(ctx context.Context, rc *pipeline.ReviewContext) error {
	glog.L().Debug("crossfile entry", "enabled", rc.Config.Passes.CrossFile, "fileCount", len(rc.Files))
	if !rc.Config.Passes.CrossFile || len(rc.Files) < 2 {
		glog.L().Debug("crossfile skipped", "reason", "disabled or <2 files")
		return nil
	}

	// Build a combined view of all changes
	var allDiffs strings.Builder
	for _, f := range rc.Files {
		fmt.Fprintf(&allDiffs, "=== %s (%s) ===\n%s\n\n", f.Path, f.Language, f.Patch)
	}

	systemPrompt := `You are a cross-file analysis expert. Analyze the following multi-file diff for issues that span across files:

1. **Contract Violations**: Function signatures changed in one file but callers in other files not updated
2. **Import/Export Mismatches**: Exported names changed but imports not updated
3. **Shared State Issues**: Multiple files modifying shared state inconsistently
4. **Type Mismatches**: Types defined in one file used incorrectly in another
5. **Configuration Drift**: Config changes that affect multiple consumers

Only report issues you can PROVE from the visible diffs. No speculation.

Return JSON:
` + "```json\n" + `{
    "suggestions": [
        {
            "title": "Short issue title",
            "description": "Full description with cross-file evidence",
            "snippet": "Code from source file",
            "proposal": "Fixed code",
            "synopsis": "Brief summary",
            "filePath": "affected/file/path",
            "startLine": 1,
            "endLine": 10,
            "category": "bug|performance|security",
            "severity": "low|medium|high|critical"
        }
    ]
}` + "\n```"

	userPrompt := fmt.Sprintf("## Cross-File Analysis\n\nAnalyze these %d changed files for cross-file issues:\n\n%s",
		len(rc.Files), allDiffs.String())

	msgs := []llm.Message{
		llm.SystemMsg(systemPrompt),
		llm.UserMsg(userPrompt),
	}

	resp, err := llm.Retry(ctx, 2, rc.Provider, msgs, llm.DefaultOpts())
	if err != nil {
		rc.AddError(fmt.Errorf("crossfile: %w", err))
		return nil
	}
	glog.L().Debug("crossfile response", "respLen", len(resp))

	suggestions, err := parseCrossFileResponse(resp)
	if err != nil {
		rc.AddError(fmt.Errorf("crossfile parse: %w", err))
		return nil
	}

	glog.L().Debug("crossfile done", "suggestions", len(suggestions))
	rc.AddSuggestions(suggestions)
	return nil
}

type crossFileResponse struct {
	Suggestions []struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Snippet     string `json:"snippet"`
		Proposal    string `json:"proposal"`
		Synopsis    string `json:"synopsis"`
		FilePath    string `json:"filePath"`
		StartLine   int    `json:"startLine"`
		EndLine     int    `json:"endLine"`
		Category    string `json:"category"`
		Severity    string `json:"severity"`
	} `json:"suggestions"`
}

func parseCrossFileResponse(raw string) ([]model.CodeSuggestion, error) {
	extracted := llm.ExtractJSON(raw)

	var resp crossFileResponse
	if err := json.Unmarshal([]byte(extracted), &resp); err != nil {
		// Fallback: bare array of suggestions.
		if err2 := json.Unmarshal([]byte(extracted), &resp.Suggestions); err2 != nil {
			preview := raw
			if len(preview) > 200 {
				preview = preview[:200]
			}
			return nil, fmt.Errorf("crossfile: LLM response is not valid JSON (expected {\"suggestions\":[...]} or bare array)\n  parse error: %w\n  response preview: %s", err, preview)
		}
		glog.L().Warn("crossfile: LLM returned bare array instead of {\"suggestions\":[...]}")
	}

	var result []model.CodeSuggestion
	for _, s := range resp.Suggestions {
		result = append(result, model.CodeSuggestion{
			FilePath:    s.FilePath,
			Language:    model.LanguageFromPath(s.FilePath),
			Severity:    model.ParseSeverity(s.Severity),
			SeverityStr: s.Severity,
			Category:    s.Category,
			Title:       s.Title,
			Description: s.Description,
			Snippet:     s.Snippet,
			Proposal:    s.Proposal,
			Synopsis:    s.Synopsis,
			StartLine:   s.StartLine,
			EndLine:     s.EndLine,
		})
	}
	return result, nil
}
