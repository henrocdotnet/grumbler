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

// ClassifyRules runs the 3-expert panel (Alice/Bob/Charles) to identify rule violations.
type ClassifyRules struct{}

func (ClassifyRules) Name() string { return "classify_rules" }

func (ClassifyRules) Execute(ctx context.Context, rc *pipeline.ReviewContext) error {
	glog.L().Debug("classify_rules entry", "enabled", rc.Config.Passes.ExpertPanel, "rulesCount", len(rc.Rules))
	if !rc.Config.Passes.ExpertPanel || len(rc.Rules) == 0 {
		glog.L().Debug("classify_rules skipped")
		return nil
	}

	systemPrompt, err := prompt.Render("expert_panel_system", nil)
	if err != nil {
		return fmt.Errorf("rendering expert panel system prompt: %w", err)
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

	userPrompt, err := prompt.Render("expert_panel_user", prompt.ExpertPanelData{
		Diff:      combinedDiff,
		RulesJSON: string(rulesJSON),
	})
	if err != nil {
		return fmt.Errorf("rendering expert panel user prompt: %w", err)
	}

	msgs := []llm.Message{
		llm.SystemMsg(systemPrompt),
		llm.UserMsg(userPrompt),
	}

	resp, err := llm.Retry(ctx, 2, rc.Provider, msgs, llm.DefaultOpts())
	if err != nil {
		rc.AddError(fmt.Errorf("expert panel: %w", err))
		return nil
	}
	glog.L().Debug("classify_rules response", "respLen", len(resp))

	violated := parseExpertPanelResponse(resp)
	glog.L().Debug("classify_rules violations", "count", len(violated))

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

type expertPanelViolation struct {
	ID     string `json:"uuid"`
	Reason string `json:"reason"`
}

type expertPanelResponse struct {
	Rules []expertPanelViolation `json:"rules"`
}

func parseExpertPanelResponse(raw string) []expertPanelViolation {
	extracted := llm.ExtractJSON(raw)
	var resp expertPanelResponse
	if err := json.Unmarshal([]byte(extracted), &resp); err != nil {
		return nil
	}
	return resp.Rules
}
