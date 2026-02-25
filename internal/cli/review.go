package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/henrocdotnet/grumbler/internal/config"
	"github.com/henrocdotnet/grumbler/internal/git"
	"github.com/henrocdotnet/grumbler/internal/llm"
	glog "github.com/henrocdotnet/grumbler/internal/log"
	"github.com/henrocdotnet/grumbler/internal/output"
	"github.com/henrocdotnet/grumbler/internal/pipeline"
	"github.com/henrocdotnet/grumbler/internal/pipeline/stages"
	"github.com/henrocdotnet/grumbler/internal/prompt"
	"github.com/henrocdotnet/grumbler/internal/rules"
)

func init() {
	reviewCmd := &cobra.Command{
		Use:   "review",
		Short: "Review code changes against a base branch",
		Long:  "Analyzes git diff using multi-pass LLM review with expert panel and safeguard validation.",
		RunE:  runReview,
	}

	reviewCmd.Flags().String("base", "", "Base branch for branch diff mode")
	reviewCmd.Flags().Bool("all", false, "Include all uncommitted changes (staged + unstaged)")
	reviewCmd.Flags().Bool("fast", false, "Diff-only mode (skip full file content)")
	reviewCmd.Flags().Int("concurrency", 0, "Max concurrent file reviews")
	reviewCmd.Flags().String("min-severity", "", "Minimum severity filter")

	rootCmd.AddCommand(reviewCmd)
}

func runReview(cmd *cobra.Command, _ []string) error {
	start := time.Now()

	dir, err := resolveDir(cmd)
	if err != nil {
		return err
	}

	// Ensure we're in a git repo
	repoRoot, err := git.TopLevel(dir)
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}

	// Init logging
	if err := glog.Init(repoRoot); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: init logging: %v\n", err)
	}

	// Load config
	cfg, err := config.Load(repoRoot)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	glog.L().Info("config loaded", "provider", cfg.LLM.Provider, "model", cfg.LLM.Model, "format", cfg.Output.Format)

	// Apply flag overrides
	overrides := buildOverrides(cmd)
	overrides.Apply(cfg)
	glog.L().Debug("overrides applied", "provider", cfg.LLM.Provider, "model", cfg.LLM.Model)

	// Load prompt overrides
	if err := prompt.LoadOverrides(repoRoot); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: loading prompt overrides: %v\n", err)
	}

	// Create LLM provider
	provider, err := llm.NewProvider(cfg.LLM)
	if err != nil {
		return fmt.Errorf("creating LLM provider: %w", err)
	}
	lp := llm.NewLoggingProvider(provider)
	glog.L().Info("provider created", "name", provider.Name())

	// Load rules
	rulesList, err := rules.LoadRules(repoRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: loading rules: %v\n", err)
	}

	all, _ := cmd.Flags().GetBool("all")
	fast, _ := cmd.Flags().GetBool("fast")
	baseBranch, _ := cmd.Flags().GetString("base")

	// Determine diff mode: --base → branch, --all → all, default → staged
	diffMode := int(git.DiffStaged)
	if baseBranch != "" {
		diffMode = int(git.DiffBranch)
	} else if all {
		diffMode = int(git.DiffAll)
	}

	// Build review context
	rc := &pipeline.ReviewContext{
		Config:   cfg,
		Provider: lp,
		RepoDir:  repoRoot,
		Rules:    rulesList,
		DiffMode: diffMode,
		Fast:     fast,
	}

	// Build pipeline
	p := pipeline.New(
		stages.Prepare{},
		stages.ReviewFiles{},
		stages.ClassifyRules{},
		stages.Safeguard{},
		stages.CrossFile{},
		stages.Aggregate{},
	)

	// Report writer — incremental snapshots to reports/<timestamp>/
	rw, err := output.NewReportWriter(repoRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: report writer: %v\n", err)
	}

	// Progress callback
	p.OnStep(func(name string, dur time.Duration) {
		if cfg.Output.Format == "terminal" {
			fmt.Fprintf(os.Stderr, "  ✓ %s (%s)\n", name, dur.Round(time.Millisecond))
		}
		if rw != nil {
			rw.StageSnapshot(name, rc.Result())
		}
	})

	if cfg.Output.Format == "terminal" {
		fmt.Fprintf(os.Stderr, "Reviewing with %s...\n", provider.Name())
	}

	glog.L().Info("pipeline start", "stages", len(p.Stages()), "diffMode", diffMode)

	// Execute
	ctx := context.Background()
	if err := p.Run(ctx, rc); err != nil {
		glog.L().Error("pipeline failed", "err", err)
		return err
	}

	glog.L().Info("pipeline done", "suggestions", len(rc.Suggestions), "elapsed", time.Since(start))

	// Output
	result := rc.Result()

	// Persist final report
	if rw != nil {
		if err := rw.Final(result); err != nil {
			glog.L().Error("final report failed", "err", err)
		}
		if err := rw.Markdown(result); err != nil {
			glog.L().Error("markdown report failed", "err", err)
		}
		if err := rw.Conversations(lp.Exchanges()); err != nil {
			glog.L().Error("conversations report failed", "err", err)
		}
		fmt.Fprintf(os.Stderr, "Report saved to %s\n", rw.Dir())
	}

	switch cfg.Output.Format {
	case "json":
		return output.WriteJSON(os.Stdout, result)
	case "sarif":
		return output.WriteSARIF(os.Stdout, result)
	default:
		output.WriteTerminal(os.Stdout, result)
	}

	if cfg.Output.Format == "terminal" {
		fmt.Fprintf(os.Stderr, "\nCompleted in %s\n", time.Since(start).Round(time.Millisecond))
	}

	// Print non-fatal errors
	if len(rc.Errors) > 0 && cfg.Output.Format == "terminal" {
		fmt.Fprintf(os.Stderr, "\nWarnings:\n")
		for _, e := range rc.Errors {
			fmt.Fprintf(os.Stderr, "  - %v\n", e)
		}
	}

	return nil
}

func resolveDir(cmd *cobra.Command) (string, error) {
	cfgDir, _ := cmd.Flags().GetString("config")
	if cfgDir != "" {
		return cfgDir, nil
	}
	return os.Getwd()
}

func buildOverrides(cmd *cobra.Command) *config.Overrides {
	o := &config.Overrides{}
	if v, _ := cmd.Flags().GetString("provider"); v != "" {
		o.Provider = v
	}
	if v, _ := cmd.Flags().GetString("model"); v != "" {
		o.Model = v
	}
	if v, _ := cmd.Flags().GetString("base"); v != "" {
		o.BaseBranch = v
	}
	if v, _ := cmd.Flags().GetString("format"); v != "" {
		o.Format = v
	}
	if v, _ := cmd.Flags().GetInt("concurrency"); v > 0 {
		o.Concurrency = v
	}
	if v, _ := cmd.Flags().GetString("min-severity"); v != "" {
		o.MinSeverity = v
	}
	return o
}
