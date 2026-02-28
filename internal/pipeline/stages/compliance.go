package stages

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/henrocdotnet/grumbler/internal/llm"
	glog "github.com/henrocdotnet/grumbler/internal/log"
	"github.com/henrocdotnet/grumbler/internal/pipeline"
	"github.com/henrocdotnet/grumbler/internal/prompt"
)

// Compliance runs the Spock/McCoy/Scotty review team to identify rule violations.
type Compliance struct{}

func (Compliance) Name() string { return "compliance" }

func (Compliance) Execute(ctx context.Context, rc *pipeline.ReviewContext) error {
	glog.L().Debug("compliance entry", "enabled", rc.Config.Passes.ReviewTeam, "rulesCount", len(rc.Rules))
	if !rc.Config.Passes.ReviewTeam || len(rc.Rules) == 0 {
		glog.L().Debug("compliance skipped")
		return nil
	}

	systemPrompt, err := prompt.Render("review_team_system", nil)
	if err != nil {
		return fmt.Errorf("rendering review team system prompt: %w", err)
	}

	// Build combined diff for all files
	var combinedDiff string
	for _, f := range rc.Files {
		combinedDiff += fmt.Sprintf("--- File: %s ---\n%s\n\n", f.Path, f.Patch)
	}

	rulesJSON, err := json.Marshal(rc.Rules)
	if err != nil {
		return fmt.Errorf("marshaling rules: %w", err)
	}

	userPrompt, err := prompt.Render("review_team_user", prompt.ReviewTeamData{
		Diff:      combinedDiff,
		RulesJSON: string(rulesJSON),
	})
	if err != nil {
		return fmt.Errorf("rendering review team user prompt: %w", err)
	}

	msgs := []llm.Message{
		llm.SystemMsg(systemPrompt),
		llm.UserMsg(userPrompt),
	}

	resp, err := llm.Retry(ctx, 2, rc.Provider, msgs, llm.DefaultOpts())
	if err != nil {
		rc.AddError(fmt.Errorf("compliance panel: %w", err))
		return nil
	}
	glog.L().Debug("compliance response", "respLen", len(resp))

	violated := parseReviewTeamResponse(resp)
	glog.L().Debug("compliance violations", "count", len(violated))

	// Tag existing suggestions with violated rule IDs
	violatedSet := make(map[string]bool, len(violated))
	for _, v := range violated {
		violatedSet[v.ID] = true
	}

	for i := range rc.Suggestions {
		for _, r := range rc.Rules {
			if violatedSet[r.ID] {
				rc.Suggestions[i].RuleIDs = append(rc.Suggestions[i].RuleIDs, r.ID)
			}
		}
	}

	return nil
}

type reviewTeamViolation struct {
	ID     string `json:"ruleId"`
	Reason string `json:"reason"`
}

type reviewTeamResponse struct {
	Rules []reviewTeamViolation `json:"rules"`
}

func parseReviewTeamResponse(raw string) []reviewTeamViolation {
	extracted := llm.ExtractJSON(raw)
	var resp reviewTeamResponse
	if err := json.Unmarshal([]byte(extracted), &resp); err != nil {
		return nil
	}
	return resp.Rules
}
