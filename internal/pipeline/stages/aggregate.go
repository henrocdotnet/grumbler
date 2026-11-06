package stages

import (
	"context"
	"sort"
	"strconv"
	"strings"

	glog "github.com/henrocdotnet/grumbler/internal/log"
	"github.com/henrocdotnet/grumbler/internal/model"
	"github.com/henrocdotnet/grumbler/internal/pipeline"
)

// Aggregate merges, deduplicates, filters, and sorts suggestions.
type Aggregate struct{}

func (Aggregate) Name() string { return "aggregate" }

func (Aggregate) Execute(_ context.Context, rc *pipeline.ReviewContext) error {
	glog.L().Debug("aggregate entry", "suggestionsIn", len(rc.Suggestions))

	// Remove discarded suggestions (from safeguard)
	var kept []model.CodeSuggestion
	for _, s := range rc.Suggestions {
		if s.AuditResult == "discard" {
			continue
		}
		kept = append(kept, s)
	}
	glog.L().Debug("aggregate after discard", "count", len(kept))

	// Deduplicate by file+startLine+title
	seen := map[string]bool{}
	var deduped []model.CodeSuggestion
	for _, s := range kept {
		key := s.FilePath + ":" + s.Title + ":" + strconv.Itoa(s.StartLine)
		if seen[key] {
			continue
		}
		seen[key] = true
		duplicate := false
		for i := range deduped {
			if sameAdvisoryEvidence(deduped[i], s) {
				for _, id := range s.RuleIDs {
					if !containsRuleID(deduped[i].RuleIDs, id) {
						deduped[i].RuleIDs = append(deduped[i].RuleIDs, id)
					}
				}
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		deduped = append(deduped, s)
	}
	glog.L().Debug("aggregate after dedup", "count", len(deduped))

	// Apply severity filter
	minSev := model.ParseSeverity(rc.Config.Filter.MinSeverity)
	var filtered []model.CodeSuggestion
	for _, s := range deduped {
		if s.Severity.AtLeast(minSev) {
			filtered = append(filtered, s)
		}
	}
	glog.L().Debug("aggregate after severity filter", "count", len(filtered))

	// Sort: severity desc, then file path
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Severity != filtered[j].Severity {
			return filtered[i].Severity < filtered[j].Severity // lower ordinal = higher severity
		}
		if filtered[i].FilePath != filtered[j].FilePath {
			return filtered[i].FilePath < filtered[j].FilePath
		}
		return filtered[i].StartLine < filtered[j].StartLine
	})

	// Apply max suggestions cap
	if rc.Config.Filter.MaxSuggestions > 0 && len(filtered) > rc.Config.Filter.MaxSuggestions {
		filtered = filtered[:rc.Config.Filter.MaxSuggestions]
	}

	glog.L().Debug("aggregate exit", "suggestionsOut", len(filtered))
	rc.Suggestions = filtered
	return nil
}

// Independent passes can describe the same advisory using a whole expression
// versus its nested selector. Limit containment merging to located, supported
// maintainability notes; distinct defect categories and unresolved claims must
// remain independently visible.
func sameAdvisoryEvidence(a, b model.CodeSuggestion) bool {
	if a.Category != "maintainability" || b.Category != a.Category || a.FilePath != b.FilePath ||
		a.StartLine <= 0 || b.StartLine <= 0 || a.EndLine < b.StartLine || b.EndLine < a.StartLine ||
		a.AuditResult == "unresolved" || b.AuditResult == "unresolved" {
		return false
	}
	left := strings.Join(strings.Fields(a.Snippet), " ")
	right := strings.Join(strings.Fields(b.Snippet), " ")
	if len(left) < 24 || len(right) < 24 {
		return false
	}
	return strings.Contains(left, right) || strings.Contains(right, left)
}
