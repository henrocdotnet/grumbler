//go:build integration

package integration_test

import (
	"context"
	"os"
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
}

// runReview mirrors the full CLI review flow: config loading, provider setup,
// pipeline execution, report writing, and token accounting.
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
		stages.Vet{},
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

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if err := p.Run(ctx, rc); err != nil {
		t.Fatalf("pipeline: %v", err)
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
