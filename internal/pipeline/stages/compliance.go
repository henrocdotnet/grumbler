package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/henrocdotnet/grumbler/internal/llm"
	glog "github.com/henrocdotnet/grumbler/internal/log"
	"github.com/henrocdotnet/grumbler/internal/model"
	"github.com/henrocdotnet/grumbler/internal/pipeline"
	"github.com/henrocdotnet/grumbler/internal/prompt"
	"github.com/henrocdotnet/grumbler/internal/rules"
)

// Compliance discovers located violations of the configured review rules.
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

	violated, err := parseReviewTeamResponse(resp)
	if err != nil {
		rc.AddError(err)
		return nil
	}
	glog.L().Debug("compliance violations", "count", len(violated))

	addRuleViolations(rc, violated)
	return nil
}

// addRuleViolations preserves independently discovered, located violations.
// A rule matching a file is not evidence that every finding in it violates it.
func addRuleViolations(rc *pipeline.ReviewContext, violations []reviewTeamViolation) {
	for index, v := range violations {
		var rule *rules.Rule
		for _, r := range rules.MatchRules(rc.Rules, v.FilePath) {
			if r.ID == v.ID {
				copy := r
				rule = &copy
				break
			}
		}
		var file *model.FileChange
		for i := range rc.Files {
			if rc.Files[i].Path == v.FilePath {
				file = &rc.Files[i]
				break
			}
		}
		if rule == nil || file == nil || v.Title == "" || v.Reason == "" || v.Snippet == "" || !intersectsAddedLine(file.Patch, v.StartLine, v.EndLine) {
			rc.AddError(fmt.Errorf("compliance: invalid or unlocated violation %s at %s:%d", v.ID, v.FilePath, v.StartLine))
			continue
		}
		severity := v.Severity
		if severity == "" {
			severity = rule.Severity
		}
		category := v.Category
		if category == "" {
			category = "bug"
		}
		if category == "maintainability" {
			severity = "low"
		}
		matched := false
		for i := range rc.Suggestions {
			s := &rc.Suggestions[i]
			if s.FilePath == v.FilePath && s.StartLine <= v.EndLine && v.StartLine <= s.EndLine &&
				(s.Title == v.Title || strings.TrimSpace(s.Snippet) == strings.TrimSpace(v.Snippet)) {
				if !containsRuleID(s.RuleIDs, v.ID) {
					s.RuleIDs = append(s.RuleIDs, v.ID)
				}
				matched = true
				break
			}
		}
		if !matched {
			rc.AddSuggestions([]model.CodeSuggestion{{
				ID:       fmt.Sprintf("rule:%s:%s:%d:%d", v.ID, v.FilePath, v.StartLine, index),
				FilePath: v.FilePath, Language: file.Language,
				Severity: model.ParseSeverity(severity), SeverityStr: severity,
				Category: category, Title: v.Title, Description: v.Reason,
				Snippet: v.Snippet, Proposal: v.Proposal, Synopsis: v.Synopsis,
				StartLine: v.StartLine, EndLine: v.EndLine, RuleIDs: []string{v.ID},
			}})
		}
	}
}

func containsRuleID(ids []string, id string) bool {
	for _, existing := range ids {
		if existing == id {
			return true
		}
	}
	return false
}

var addedPatchLine = regexp.MustCompile(`(?m)^L([0-9]+) +\+`)

func intersectsAddedLine(patch string, start, end int) bool {
	if start <= 0 || end < start {
		return false
	}
	for _, match := range addedPatchLine.FindAllStringSubmatch(patch, -1) {
		line, _ := strconv.Atoi(match[1])
		if start <= line && line <= end {
			return true
		}
	}
	return false
}

type reviewTeamViolation struct {
	ID     string `json:"ruleId"`
	Reason string `json:"reason"`
	llmSuggestion
}

type reviewTeamResponse struct {
	Rules []reviewTeamViolation `json:"rules"`
}

func parseReviewTeamResponse(raw string) ([]reviewTeamViolation, error) {
	extracted := llm.ExtractJSON(raw)

	var resp reviewTeamResponse
	if err := json.Unmarshal([]byte(extracted), &resp); err != nil || resp.Rules == nil {
		// Fallback: bare array of violations.
		var arr []reviewTeamViolation
		if err2 := json.Unmarshal([]byte(extracted), &arr); err2 != nil || arr == nil {
			preview := raw
			if len(preview) > 200 {
				preview = preview[:200]
			}
			return nil, fmt.Errorf("compliance: expected {\"rules\":[...]} or bare array; response preview: %s", preview)
		}
		glog.L().Warn("compliance: LLM returned bare array instead of {\"rules\":[...]}")
		return arr, nil
	}
	return resp.Rules, nil
}
