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

// Safeguard runs the 5-expert validation panel (Edward/Alice/Bob/Charles/Diana).
type Safeguard struct{}

func (Safeguard) Name() string { return "safeguard" }

func (Safeguard) Execute(ctx context.Context, rc *pipeline.ReviewContext) error {
	glog.L().Debug("safeguard entry", "enabled", rc.Config.Passes.Safeguard, "suggestions", len(rc.Suggestions))
	if !rc.Config.Passes.Safeguard || len(rc.Suggestions) == 0 {
		glog.L().Debug("safeguard skipped")
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
			verdicts, err := safeguardFile(gctx, rc, file, suggestions)
			if err != nil {
				glog.L().Error("safeguard file failed", "path", filePath, "err", err)
				rc.AddError(fmt.Errorf("safeguard %s: %w", filePath, err))
				return nil
			}
			glog.L().Debug("safeguard file done", "path", filePath, "verdicts", len(verdicts))
			mu.Lock()
			applySafeguardVerdicts(rc, verdicts)
			mu.Unlock()
			return nil
		})
	}

	return g.Wait()
}

func safeguardFile(ctx context.Context, rc *pipeline.ReviewContext, file model.FileChange, suggestions []model.CodeSuggestion) ([]safeguardVerdict, error) {
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

	resp, err := llm.Retry(ctx, 2, rc.Provider, msgs, llm.DefaultOpts())
	if err != nil {
		return nil, err
	}
	glog.L().Debug("safeguard response", "respLen", len(resp))

	return parseSafeguardResponse(resp)
}

type safeguardVerdict struct {
	ID                string `json:"id"`
	SuggestionContent string `json:"suggestionContent"`
	ExistingCode      string `json:"existingCode"`
	ImprovedCode      string `json:"improvedCode"`
	OneSentSummary    string `json:"oneSentenceSummary"`
	StartLine         int    `json:"relevantLinesStart"`
	EndLine           int    `json:"relevantLinesEnd"`
	Label             string `json:"label"`
	Severity          string `json:"severity"`
	Action            string `json:"action"` // no_changes, update, discard
	Reason            string `json:"reason"`
}

type safeguardResponse struct {
	Suggestions []safeguardVerdict `json:"codeSuggestions"`
}

func parseSafeguardResponse(raw string) ([]safeguardVerdict, error) {
	extracted := llm.ExtractJSON(raw)
	var resp safeguardResponse
	if err := json.Unmarshal([]byte(extracted), &resp); err != nil {
		return nil, fmt.Errorf("parsing safeguard response: %w", err)
	}
	return resp.Suggestions, nil
}

func applySafeguardVerdicts(rc *pipeline.ReviewContext, verdicts []safeguardVerdict) {
	verdictMap := make(map[string]*safeguardVerdict, len(verdicts))
	for i := range verdicts {
		// Match by index position since we may not have stable IDs
		verdictMap[verdicts[i].ID] = &verdicts[i]
	}

	for i := range rc.Suggestions {
		s := &rc.Suggestions[i]
		// Try to match by ID or by title
		v, ok := verdictMap[s.ID]
		if !ok {
			continue
		}

		switch v.Action {
		case "discard":
			s.SafeguardVerdict = "discard"
		case "update":
			s.SafeguardVerdict = "update"
			if v.ImprovedCode != "" {
				s.ImprovedCode = v.ImprovedCode
			}
			if v.SuggestionContent != "" {
				s.Description = v.SuggestionContent
			}
			if v.OneSentSummary != "" {
				s.OneSentSummary = v.OneSentSummary
			}
		default:
			s.SafeguardVerdict = "keep"
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
