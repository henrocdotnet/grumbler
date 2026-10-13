package stages

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/henrocdotnet/grumbler/internal/llm"
	glog "github.com/henrocdotnet/grumbler/internal/log"
	"github.com/henrocdotnet/grumbler/internal/model"
	"github.com/henrocdotnet/grumbler/internal/pipeline"
	"github.com/henrocdotnet/grumbler/internal/prompt"
	"github.com/henrocdotnet/grumbler/internal/rules"
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

	// Build combined diff for all files, bounded so the prompt cannot exceed
	// the model's context window on a large branch.
	budget := contextBudget(rc.Config.Passes.MaxContextTokens)
	combinedDiff, shown, total := budgetedDiff(rc.Files, budget, func(f model.FileChange) string {
		return fmt.Sprintf("--- File: %s ---\n%s\n\n", f.Path, f.Patch)
	})
	if shown < total {
		if rc.Config.Passes.OnOverflow == "skip" {
			glog.L().Warn("compliance skipped: diff exceeds context budget",
				"budget", budget, "files", total)
			return nil
		}
		glog.L().Warn("compliance diff truncated to fit context budget",
			"budget", budget, "shown", shown, "total", total)
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

	tagViolations(rc, violated)
	return nil
}

// tagViolations tags existing suggestions with violated rule IDs.
func tagViolations(rc *pipeline.ReviewContext, violated []reviewTeamViolation) {
	violatedSet := make(map[string]bool, len(violated))
	for _, v := range violated {
		violatedSet[v.ID] = true
	}

	// Scope each violated rule to the files it actually governs. Without the
	// glob check every suggestion is tagged with every violated rule, which
	// makes RuleIDs meaningless on any repo with path-scoped rules.
	for i := range rc.Suggestions {
		for _, r := range rules.MatchRules(rc.Rules, rc.Suggestions[i].FilePath) {
			if violatedSet[r.ID] {
				rc.Suggestions[i].RuleIDs = append(rc.Suggestions[i].RuleIDs, r.ID)
			}
		}
	}
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
		// Fallback: bare array of violations.
		var arr []reviewTeamViolation
		if err2 := json.Unmarshal([]byte(extracted), &arr); err2 != nil {
			preview := raw
			if len(preview) > 200 {
				preview = preview[:200]
			}
			glog.L().Warn("compliance: could not parse LLM response as JSON, treating as zero violations",
				"err", err, "preview", preview)
			return nil
		}
		glog.L().Warn("compliance: LLM returned bare array instead of {\"rules\":[...]}")
		return arr
	}
	return resp.Rules
}
