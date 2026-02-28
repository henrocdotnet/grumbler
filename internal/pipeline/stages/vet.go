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

// Vet runs the Worf/Data/Geordi/Troi/Picard vetting panel.
type Vet struct{}

func (Vet) Name() string { return "vet" }

func (Vet) Execute(ctx context.Context, rc *pipeline.ReviewContext) error {
	glog.L().Debug("vet entry", "enabled", rc.Config.Passes.Vet, "suggestions", len(rc.Suggestions))
	if !rc.Config.Passes.Vet || len(rc.Suggestions) == 0 {
		glog.L().Debug("vet skipped")
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
			verdicts, err := vetFile(gctx, rc, file, suggestions)
			if err != nil {
				glog.L().Error("vet file failed", "path", filePath, "err", err)
				rc.AddError(fmt.Errorf("vet %s: %w", filePath, err))
				return nil
			}
			glog.L().Debug("vet file done", "path", filePath, "verdicts", len(verdicts))
			mu.Lock()
			applyVetVerdicts(rc, filePath, verdicts)
			mu.Unlock()
			return nil
		})
	}

	return g.Wait()
}

func vetFile(ctx context.Context, rc *pipeline.ReviewContext, file model.FileChange, suggestions []model.CodeSuggestion) ([]vetVerdict, error) {
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
		glog.L().Debug("vet response", "respLen", len(resp), "attempt", attempt)
		verdicts, err := parseVetResponse(resp)
		if err != nil {
			lastErr = err
			glog.L().Warn("vet parse retry", "path", file.Path, "attempt", attempt, "err", err)
			continue
		}
		return verdicts, nil
	}
	return nil, lastErr
}

type vetVerdict struct {
	ID                string `json:"id"`
	SuggestionContent string `json:"suggestionContent"`
	Snippet           string `json:"snippet"`
	Proposal          string `json:"proposal"`
	Synopsis          string `json:"synopsis"`
	StartLine         int    `json:"relevantLinesStart"`
	EndLine           int    `json:"relevantLinesEnd"`
	Label             string `json:"label"`
	Severity          string `json:"severity"`
	Action            string `json:"action"` // no_changes, update, discard
	Reason            string `json:"reason"`
}

type vetResponse struct {
	Suggestions []vetVerdict `json:"codeSuggestions"`
}

func parseVetResponse(raw string) ([]vetVerdict, error) {
	extracted := llm.ExtractJSON(raw)
	var resp vetResponse
	if err := json.Unmarshal([]byte(extracted), &resp); err != nil {
		return nil, fmt.Errorf("parsing vet response: %w", err)
	}
	return resp.Suggestions, nil
}

func applyVetVerdicts(rc *pipeline.ReviewContext, filePath string, verdicts []vetVerdict) {
	verdictMap := make(map[string]*vetVerdict, len(verdicts))
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

		switch v.Action {
		case "discard":
			s.VetVerdict = "discard"
		case "update":
			s.VetVerdict = "update"
			if v.Proposal != "" {
				s.Proposal = v.Proposal
			}
			if v.SuggestionContent != "" {
				s.Description = v.SuggestionContent
			}
			if v.Synopsis != "" {
				s.Synopsis = v.Synopsis
			}
		default:
			s.VetVerdict = "keep"
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
