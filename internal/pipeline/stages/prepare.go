package stages

import (
	"context"
	"fmt"

	"github.com/henrocdotnet/grumbler/internal/git"
	glog "github.com/henrocdotnet/grumbler/internal/log"
	"github.com/henrocdotnet/grumbler/internal/model"
	"github.com/henrocdotnet/grumbler/internal/pipeline"
	"github.com/henrocdotnet/grumbler/internal/rules"
)

// Prepare validates inputs, computes diff, filters files, and enriches with content.
type Prepare struct{}

func (Prepare) Name() string { return "prepare" }

func (Prepare) Execute(ctx context.Context, rc *pipeline.ReviewContext) error {
	glog.L().Debug("prepare entry", "diffMode", rc.DiffMode, "preloadedFiles", len(rc.Files))

	if len(rc.Files) > 0 {
		return filterAndEnrich(rc)
	}

	diff, err := git.GetDiff(rc.RepoDir, rc.Config.Review.BaseBranch, git.DiffMode(rc.DiffMode))
	if err != nil {
		return fmt.Errorf("getting diff: %w", err)
	}
	glog.L().Debug("diff obtained", "len", len(diff))

	modeNames := []string{"staged", "all", "branch"}
	modeName := "staged"
	if rc.DiffMode < len(modeNames) {
		modeName = modeNames[rc.DiffMode]
	}
	if diff == "" {
		return fmt.Errorf("no changes found (mode: %s, base: %s)", modeName, rc.Config.Review.BaseBranch)
	}

	rc.Files = git.ParseDiff(diff)
	glog.L().Debug("diff parsed", "fileCount", len(rc.Files))
	if len(rc.Files) == 0 {
		return fmt.Errorf("diff parsed but no files extracted")
	}

	return filterAndEnrich(rc)
}

func filterAndEnrich(rc *pipeline.ReviewContext) error {
	before := len(rc.Files)
	var kept []model.FileChange
	for _, f := range rc.Files {
		if rules.ShouldIgnore(f.Path, rc.Config.Review.IgnorePaths) {
			continue
		}
		if f.Status == model.FileDeleted {
			continue
		}
		kept = append(kept, f)
	}
	rc.Files = kept
	glog.L().Debug("filter", "before", before, "after", len(rc.Files))

	if len(rc.Files) == 0 {
		return fmt.Errorf("all changed files are ignored or deleted")
	}

	for i := range rc.Files {
		rc.Files[i].Patch = git.AnnotatePatch(rc.Files[i].Diff)

		if !rc.Fast {
			content, err := git.ReadFile(rc.RepoDir, rc.Files[i].Path)
			if err != nil {
				rc.AddError(fmt.Errorf("reading %s: %w", rc.Files[i].Path, err))
			} else {
				rc.Files[i].Content = content
				glog.L().Debug("enriched", "path", rc.Files[i].Path, "contentLen", len(content))
			}
		}
	}

	return nil
}
