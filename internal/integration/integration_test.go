//go:build integration

package integration_test

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"path/filepath"

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

func TestMain(m *testing.M) {
	glog.InitTest(os.Stderr)
	os.Exit(m.Run())
}

// TestIntegration_BasicLLMRoundTrip validates basic LLM connectivity using the configured provider.
func TestIntegration_BasicLLMRoundTrip(t *testing.T) {
	projDir := testProjectDir(t)

	cfg, err := config.Load(projDir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	provider, err := llm.NewProvider(cfg.LLM)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	t.Logf("using provider: %s", provider.Name())

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resp, err := provider.Complete(ctx, []llm.Message{
		llm.UserMsg("What color is the sky? Reply with a single word."),
	}, llm.CompletionOpts{Temperature: 0, MaxTokens: 32})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	t.Logf("response: %q", resp)
	if !strings.Contains(strings.ToLower(resp), "blue") {
		t.Errorf("expected 'blue' in response, got: %q", resp)
	}
}

// TestIntegration_ReviewTestProject runs the full pipeline on the synthetic test project,
// mirroring CLI behaviour: logging, report writing, and token accounting.
func TestIntegration_ReviewTestProject(t *testing.T) {
	projDir := testProjectDir(t)
	result := runReview(t, projDir)

	t.Logf("passes run: %v", result.PassesRun)
	t.Logf("suggestions: %d", len(result.Suggestions))
	t.Logf("prompt tokens: %d  completion tokens: %d", result.TotalPromptTokens, result.TotalCompletionTokens)

	for i, s := range result.Suggestions {
		t.Logf("  [%d] %s: %s (%s)", i, s.SeverityStr, s.Title, s.FilePath)
		if s.FilePath == "" {
			t.Errorf("suggestion %d missing filePath", i)
		}
	}
	if len(result.Suggestions) == 0 {
		t.Error("expected at least one suggestion for the buggy service file")
	}
	if result.TotalPromptTokens == 0 {
		t.Error("expected non-zero prompt token count")
	}

	// --- Assertion 1: all 15 compliance rules fired ---
	allRules := map[string]bool{
		"SEC-001": false, "SEC-002": false, "SEC-003": false, "SEC-004": false,
		"ERR-001": false, "ERR-002": false, "ERR-003": false,
		"PERF-001": false, "PERF-002": false,
		"CONC-001": false,
		"LOG-001":  false, "LOG-002": false,
		"TEST-001": false,
		"API-001":  false,
		"RES-001":  false,
	}
	for _, s := range result.Suggestions {
		for _, rid := range s.RuleIDs {
			allRules[rid] = true
		}
	}
	for rid, seen := range allRules {
		if !seen {
			t.Errorf("compliance rule %s never triggered", rid)
		}
	}

	// --- Assertion 2: expected buggy files have suggestions ---
	buggyFiles := map[string]bool{
		"api.go": false, "api_types.go": false, "auth.go": false,
		"cache.go": false, "decode.go": false, "deploy.go": false,
		"handlers/http.go": false, "internal/config/loader.go": false,
		"logger.go": false, "service.go": false,
		"store/queries.go": false,
	}
	for _, s := range result.Suggestions {
		if _, ok := buggyFiles[s.FilePath]; ok {
			buggyFiles[s.FilePath] = true
		}
	}
	for f, seen := range buggyFiles {
		if !seen {
			t.Errorf("buggy file %s has no suggestions", f)
		}
	}
	// Soft-check files — log but don't fail. service_test.go issues are
	// typically attributed to the source file rather than the test itself.
	softFiles := map[string]bool{"pkg/utils/strings.go": true, "pkg/utils/math.go": true, "service_test.go": true}
	for _, s := range result.Suggestions {
		if softFiles[s.FilePath] {
			t.Logf("note: soft-check file %s received suggestion: %s", s.FilePath, s.Title)
		}
	}

	// --- Assertion 3: pipeline stages ran in expected order ---
	wantStages := []string{"prepare", "inspect", "compliance", "audit", "crossfile", "aggregate"}
	if !reflect.DeepEqual(result.PassesRun, wantStages) {
		t.Errorf("stages: got %v, want %v", result.PassesRun, wantStages)
	}

	// --- Assertion 4: files count ---
	if result.FilesCount != 14 {
		t.Errorf("files reviewed: got %d, want 14", result.FilesCount)
	}
}

// runReview mirrors the full CLI review flow: config loading, provider setup,
// pipeline execution, report writing, and token accounting.
// pipelineCtx returns a context whose deadline is driven by the
// INTEGRATION_TIMEOUT env var (e.g. "10m", "5m30s"), with 10 seconds
// reserved for report writing. Defaults to 10 minutes if unset or unparseable.
func pipelineCtx(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	d := 10 * time.Minute
	if v := os.Getenv("INTEGRATION_TIMEOUT"); v != "" {
		if parsed, err := time.ParseDuration(v); err == nil {
			d = parsed
		}
	}
	return context.WithTimeout(context.Background(), d-10*time.Second)
}

func runReview(t *testing.T, projDir string) model.ReviewResult {
	t.Helper()

	if err := glog.InitTee(filepath.Join(projDir, config.Dir), os.Stderr); err != nil {
		t.Logf("warning: init logging: %v", err)
	}

	cfg, err := config.Load(projDir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	if err := prompt.LoadOverrides(projDir); err != nil {
		t.Logf("warning: loading prompt overrides: %v", err)
	}

	provider, err := llm.NewProvider(cfg.LLM)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	lp := llm.NewLoggingProvider(provider)
	t.Logf("using provider: %s", provider.Name())

	rulesList, err := rules.LoadRules(projDir)
	if err != nil {
		t.Logf("warning: loading rules: %v", err)
	}

	rc := &pipeline.ReviewContext{
		Config:   cfg,
		Provider: lp,
		RepoDir:  projDir,
		Rules:    rulesList,
		DiffMode: int(git.DiffBranch),
	}

	p := pipeline.New(
		stages.Prepare{},
		stages.Inspect{},
		stages.Compliance{},
		stages.Audit{},
		stages.CrossFile{},
		stages.Aggregate{},
	)

	rw, err := output.NewReportWriter(projDir)
	if err != nil {
		t.Logf("warning: report writer: %v", err)
	}

	p.OnStep(func(name string, dur time.Duration) {
		t.Logf("  ✓ %s (%s)", name, dur.Round(time.Millisecond))
		if rw != nil {
			rw.StageSnapshot(name, rc.Result())
		}
	})

	ctx, cancel := pipelineCtx(t)
	defer cancel()

	if err := p.Run(ctx, rc); err != nil {
		if ctx.Err() != nil {
			t.Errorf("pipeline timed out — writing partial results")
		} else {
			t.Fatalf("pipeline: %v", err)
		}
	}

	result := rc.Result()

	exchanges := lp.Exchanges()
	for _, ex := range exchanges {
		result.TotalPromptTokens += ex.PromptTokens
		result.TotalCompletionTokens += ex.CompletionTokens
	}

	if rw != nil {
		if err := rw.Final(result); err != nil {
			t.Logf("warning: final report: %v", err)
		}
		if err := rw.Markdown(result); err != nil {
			t.Logf("warning: markdown report: %v", err)
		}
		if err := rw.Conversations(exchanges); err != nil {
			t.Logf("warning: conversations: %v", err)
		}
		t.Logf("report saved to %s", rw.Dir())
	}

	return result
}
