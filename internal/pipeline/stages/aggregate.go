package stages

import (
	"context"
	"sort"

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
		if s.VetVerdict == "discard" {
			continue
		}
		kept = append(kept, s)
	}
	glog.L().Debug("aggregate after discard", "count", len(kept))

	// Deduplicate by file+startLine+title
	seen := map[string]bool{}
	var deduped []model.CodeSuggestion
	for _, s := range kept {
		key := s.FilePath + ":" + s.Title + ":" + string(rune(s.StartLine))
		if seen[key] {
			continue
		}
		seen[key] = true
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
	sort.Slice(filtered, func(i, j int) bool {
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
