package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/henrocdotnet/grumbler/internal/config"
	"github.com/henrocdotnet/grumbler/internal/git"
	"github.com/henrocdotnet/grumbler/internal/llm"
	glog "github.com/henrocdotnet/grumbler/internal/log"
	"github.com/henrocdotnet/grumbler/internal/model"
	"github.com/henrocdotnet/grumbler/internal/output"
	"github.com/henrocdotnet/grumbler/internal/pipeline"
	"github.com/henrocdotnet/grumbler/internal/pipeline/stages"
	"github.com/henrocdotnet/grumbler/internal/prompt"
	"github.com/henrocdotnet/grumbler/internal/rules"
)

// Flag names referenced in more than one place.
const (
	flagIncludeFileContent = "include-file-content"
	flagMaxFileTokens      = "max-file-tokens"
)

func init() {
	reviewCmd := &cobra.Command{
		Use:   "review",
		Short: "Review code changes against a base branch",
		Long:  "Analyzes git diff using multi-pass LLM review with review team and audit validation.",
		RunE:  runReview,
	}

	reviewCmd.Flags().String("base", "", "Base branch for branch diff mode")
	reviewCmd.Flags().Bool("all", false, "Include all uncommitted changes (staged + unstaged)")
	reviewCmd.Flags().Bool("fast", false, "Diff-only mode (skip full file content)")
	reviewCmd.Flags().Bool(flagIncludeFileContent, false, "Embed each changed file's full content alongside its diff (slower, deeper)")
	reviewCmd.Flags().String("reasoning", "", "Reasoning effort for claude-cli or codex-cli: low|medium|high|xhigh|max (Codex also supports minimal|ultra)")
	reviewCmd.Flags().Int("max-batch-tokens", 0, "Token budget per inspect batch (0 = config)")
	reviewCmd.Flags().Int(flagMaxFileTokens, -1, "Cap per-file embedded content in tokens (0 = unlimited, -1 = config)")
	reviewCmd.Flags().Duration("timeout", 0, "Overall review timeout (0 = config)")
	reviewCmd.Flags().Int("concurrency", 0, "Max concurrent inspect batches and audit calls")
	reviewCmd.Flags().String("min-severity", "", "Minimum severity filter")
	reviewCmd.Flags().Bool("no-rules", false, "Skip compliance pass (ignore all rules)")
	reviewCmd.Flags().Bool("publish", false, "Use repo-relative links in markdown (for PR comments)")

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
	if err := glog.Init(filepath.Join(repoRoot, config.Dir)); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: init logging: %v\n", err)
	}

	cfg, err := loadConfig(cmd, repoRoot)
	if err != nil {
		return err
	}

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

	rulesList := loadRules(cmd, repoRoot)

	fast := resolveFast(cmd, cfg)
	glog.L().Debug("content mode", "fast", fast, "includeFileContent", !fast)

	diffMode := resolveDiffMode(cmd)

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
		stages.Inspect{},
		stages.Compliance{},
		stages.CrossFile{},
		stages.Audit{},
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

	if err := runPipeline(p, rc, cfg.Review.TimeoutDuration()); err != nil {
		return err
	}

	glog.L().Info("pipeline done", "suggestions", len(rc.Suggestions), "elapsed", time.Since(start))

	// Output
	result := rc.Result()

	// Persist final report
	exchanges := lp.Exchanges()
	addTokenTotals(&result, exchanges)
	if rw != nil {
		writeReports(rw, result, exchanges, cfg.Output.Publish)
	}

	return writeOutput(cfg.Output.Format, result, rc.Errors, start)
}

// writeOutput renders result to stdout in format. Terminal output also
// reports elapsed time and non-fatal errors on stderr.
func writeOutput(format string, result model.ReviewResult, errs []error, start time.Time) error {
	switch format {
	case "json":
		return output.WriteJSON(os.Stdout, result)
	case "yaml":
		return output.WriteYAML(os.Stdout, result)
	case "sarif":
		return output.WriteSARIF(os.Stdout, result)
	default:
		output.WriteTerminal(os.Stdout, result)
	}

	if format == "terminal" {
		fmt.Fprintf(os.Stderr, "\nCompleted in %s\n", time.Since(start).Round(time.Millisecond))
		printWarnings(errs)
	}
	return nil
}

// loadConfig loads the config cascade, applies flag overrides, and
// re-validates: Load validated the file layers, flags can still be invalid.
func loadConfig(cmd *cobra.Command, repoRoot string) (*config.Config, error) {
	cfg, err := config.Load(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	glog.L().Info("config loaded", "provider", cfg.LLM.Provider, "model", cfg.LLM.Model, "format", cfg.Output.Format)

	buildOverrides(cmd).Apply(cfg)
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	glog.L().Debug("overrides applied", "provider", cfg.LLM.Provider, "model", cfg.LLM.Model)
	return cfg, nil
}

// loadRules loads project rules; --no-rules discards them. Load failures
// are warnings, not fatal.
func loadRules(cmd *cobra.Command, repoRoot string) []rules.Rule {
	rulesList, err := rules.LoadRules(repoRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: loading rules: %v\n", err)
	}
	if noRules, _ := cmd.Flags().GetBool("no-rules"); noRules {
		return nil
	}
	return rulesList
}

// runPipeline executes p, bounded by timeout when positive.
func runPipeline(p *pipeline.Pipeline, rc *pipeline.ReviewContext, timeout time.Duration) error {
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
		glog.L().Debug("run timeout armed", "timeout", timeout)
	}
	if err := p.Run(ctx, rc); err != nil {
		glog.L().Error("pipeline failed", "err", err)
		return err
	}
	return nil
}

// resolveFast decides diff-only mode. Full file content is opt-in
// (review.includeFileContent); --fast forces it off, --include-file-content on.
func resolveFast(cmd *cobra.Command, cfg *config.Config) bool {
	fast := !cfg.Review.IncludeFileContent
	if cmd.Flags().Changed(flagIncludeFileContent) {
		v, _ := cmd.Flags().GetBool(flagIncludeFileContent)
		fast = !v
	}
	if v, _ := cmd.Flags().GetBool("fast"); v {
		fast = true
	}
	return fast
}

// resolveDiffMode maps flags to a diff mode: --base → branch, --all → all,
// default → staged.
func resolveDiffMode(cmd *cobra.Command) int {
	if baseBranch, _ := cmd.Flags().GetString("base"); baseBranch != "" {
		return int(git.DiffBranch)
	}
	if all, _ := cmd.Flags().GetBool("all"); all {
		return int(git.DiffAll)
	}
	return int(git.DiffStaged)
}

func addTokenTotals(result *model.ReviewResult, exchanges []llm.Exchange) {
	for _, ex := range exchanges {
		result.TotalPromptTokens += ex.PromptTokens
		result.TotalCompletionTokens += ex.CompletionTokens
		result.TotalCacheCreationTokens += ex.CacheCreationTokens
		result.TotalCacheReadTokens += ex.CacheReadTokens
	}
}

// writeReports persists every report format. Failures are logged, not fatal.
func writeReports(rw *output.ReportWriter, result model.ReviewResult, exchanges []llm.Exchange, publish bool) {
	writes := []struct {
		name string
		fn   func() error
	}{
		{"final", func() error { return rw.Final(result) }},
		{"sarif", func() error { return rw.SARIF(result) }},
		{"yaml", func() error { return rw.YAML(result) }},
		{"markdown", func() error { return rw.Markdown(result, publish) }},
		{"conversations", func() error { return rw.Conversations(exchanges) }},
	}
	for _, w := range writes {
		if err := w.fn(); err != nil {
			glog.L().Error(w.name+" report failed", "err", err)
		}
	}
	fmt.Fprintf(os.Stderr, "Report saved to %s\n", rw.Dir())
}

// printWarnings lists non-fatal pipeline errors.
func printWarnings(errs []error) {
	if len(errs) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "\nWarnings:\n")
	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "  - %v\n", e)
	}
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
	if v, _ := cmd.Flags().GetString("reasoning"); v != "" {
		o.ReasoningEffort = v
	}
	if v, _ := cmd.Flags().GetInt("max-batch-tokens"); v > 0 {
		o.MaxBatchTokens = v
	}
	if cmd.Flags().Changed(flagMaxFileTokens) {
		v, _ := cmd.Flags().GetInt(flagMaxFileTokens)
		if v >= 0 {
			o.MaxFileContentTokens = &v
		}
	}
	if cmd.Flags().Changed("timeout") {
		v, _ := cmd.Flags().GetDuration("timeout")
		o.Timeout = v.String()
	}
	if v, _ := cmd.Flags().GetInt("concurrency"); v > 0 {
		o.Concurrency = v
	}
	if v, _ := cmd.Flags().GetString("min-severity"); v != "" {
		o.MinSeverity = v
	}
	if cmd.Flags().Changed("publish") {
		v, _ := cmd.Flags().GetBool("publish")
		o.Publish = &v
	}
	return o
}
