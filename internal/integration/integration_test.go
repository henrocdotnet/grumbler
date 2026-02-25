package integration_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/henrocdotnet/grumbler/internal/config"
	"github.com/henrocdotnet/grumbler/internal/git"
	"github.com/henrocdotnet/grumbler/internal/llm"
	glog "github.com/henrocdotnet/grumbler/internal/log"
	"github.com/henrocdotnet/grumbler/internal/model"
	"github.com/henrocdotnet/grumbler/internal/pipeline"
	"github.com/henrocdotnet/grumbler/internal/pipeline/stages"
)

func TestMain(m *testing.M) {
	glog.InitTest(os.Stderr)
	os.Exit(m.Run())
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := git.TopLevel(dir)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// loadConfig reads the project config from the repo root.
func loadConfig(t *testing.T) *config.Config {
	t.Helper()
	root := repoRoot(t)
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return cfg
}

// loadProvider reads the project config and constructs the configured provider.
func loadProvider(t *testing.T) llm.Provider {
	t.Helper()
	cfg := loadConfig(t)
	p, err := llm.NewProvider(cfg.LLM)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	t.Logf("using provider: %s", p.Name())
	return p
}

// TestIntegration_BasicLLMRoundTrip validates basic LLM connectivity using the configured provider.
func TestIntegration_BasicLLMRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	provider := loadProvider(t)
	msgs := []llm.Message{
		llm.UserMsg("What color is the sky? Reply with a single word."),
	}
	opts := llm.CompletionOpts{Temperature: 0, MaxTokens: 32, JSONMode: false}

	resp, err := provider.Complete(ctx, msgs, opts)
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	t.Logf("response: %q", resp)
	if !strings.Contains(strings.ToLower(resp), "blue") {
		t.Errorf("expected 'blue' in response, got: %q", resp)
	}
}

// TestIntegration_SingleFileReview runs Prepare+ReviewFiles on a synthetic buggy file.
func TestIntegration_SingleFileReview(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	root := repoRoot(t)
	cfg := loadConfig(t)
	cfg.Passes.ExpertPanel = false
	cfg.Passes.Safeguard = false
	cfg.Passes.CrossFile = false

	provider := loadProvider(t)

	rc := &pipeline.ReviewContext{
		Config:   cfg,
		Provider: provider,
		RepoDir:  root,
		Files:    []model.FileChange{buggyFile()},
	}

	p := pipeline.New(
		stages.Prepare{},
		stages.ReviewFiles{},
	)

	if err := p.Run(ctx, rc); err != nil {
		t.Fatalf("pipeline failed: %v", err)
	}

	t.Logf("suggestions: %d", len(rc.Suggestions))
	for i, s := range rc.Suggestions {
		t.Logf("  [%d] %s: %s (filePath=%s)", i, s.SeverityStr, s.Title, s.FilePath)
		if s.FilePath == "" {
			t.Errorf("suggestion %d missing filePath", i)
		}
	}
	if len(rc.Suggestions) == 0 {
		t.Error("expected at least one suggestion for divide-by-zero bug")
	}
}

// TestIntegration_FullPipelineSingleFile runs all 6 stages on a synthetic file.
func TestIntegration_FullPipelineSingleFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	root := repoRoot(t)
	cfg := loadConfig(t)
	provider := loadProvider(t)

	rc := &pipeline.ReviewContext{
		Config:   cfg,
		Provider: provider,
		RepoDir:  root,
		Files:    []model.FileChange{buggyFile()},
	}

	p := pipeline.New(
		stages.Prepare{},
		stages.ReviewFiles{},
		stages.ClassifyRules{},
		stages.Safeguard{},
		stages.CrossFile{},
		stages.Aggregate{},
	)

	if err := p.Run(ctx, rc); err != nil {
		t.Fatalf("pipeline failed: %v", err)
	}

	t.Logf("passes run: %v", rc.PassesRun)
	t.Logf("final suggestions: %d", len(rc.Suggestions))
	for i, s := range rc.Suggestions {
		t.Logf("  [%d] %s (verdict=%s): %s", i, s.SeverityStr, s.SafeguardVerdict, s.Title)
	}
}

// TestIntegration_BranchDiff runs the full pipeline on the actual branch diff against main.
func TestIntegration_BranchDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()

	root := repoRoot(t)

	diff, err := git.GetDiff(root, "main", git.DiffBranch)
	if err != nil {
		t.Fatalf("GetDiff failed: %v", err)
	}
	if diff == "" {
		t.Skip("no diff between current branch and main")
	}

	files := git.ParseDiff(diff)
	t.Logf("branch diff: %d files, %d bytes", len(files), len(diff))

	cfg := loadConfig(t)
	provider := loadProvider(t)

	rc := &pipeline.ReviewContext{
		Config:   cfg,
		Provider: provider,
		RepoDir:  root,
		DiffMode: int(git.DiffBranch),
	}

	p := pipeline.New(
		stages.Prepare{},
		stages.ReviewFiles{},
		stages.ClassifyRules{},
		stages.Safeguard{},
		stages.CrossFile{},
		stages.Aggregate{},
	)

	if err := p.Run(ctx, rc); err != nil {
		t.Fatalf("pipeline failed: %v", err)
	}

	t.Logf("passes run: %v", rc.PassesRun)
	t.Logf("final suggestions: %d", len(rc.Suggestions))
	for i, s := range rc.Suggestions {
		t.Logf("  [%d] %s: %s (%s)", i, s.SeverityStr, s.Title, s.FilePath)
	}
}

func buggyFile() model.FileChange {
	diff := `diff --git a/buggy.go b/buggy.go
new file mode 100644
--- /dev/null
+++ b/buggy.go
@@ -0,0 +1,10 @@
+package main
+
+func divide(a, b int) int {
+    return a / b
+}
+
+func main() {
+    result := divide(10, 0)
+    println(result)
+}
`
	content := `package main

func divide(a, b int) int {
    return a / b
}

func main() {
    result := divide(10, 0)
    println(result)
}
`
	return model.FileChange{
		Path:     "buggy.go",
		Language: "go",
		Status:   model.FileAdded,
		Diff:     diff,
		Patch:    diff,
		Content:  content,
	}
}
