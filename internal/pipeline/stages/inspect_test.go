package stages

import (
	"strings"
	"testing"

	"github.com/henrocdotnet/grumbler/internal/model"
)

func TestEstimateTokens_NonZero(t *testing.T) {
	f := model.FileChange{
		Path:  "main.go",
		Patch: "@@ -1,3 +1,5 @@\n func main() {\n+\tfmt.Println(\"hello\")\n }",
	}
	n := estimateTokens(f, true, 0)
	if n <= 0 {
		t.Errorf("estimateTokens = %d, want > 0", n)
	}
}

func TestEstimateTokens_MinFloor(t *testing.T) {
	f := model.FileChange{}
	n := estimateTokens(f, true, 0)
	if n < 100 {
		t.Errorf("estimateTokens = %d, want >= 100 (min floor)", n)
	}
}

func TestEstimateTokens_FastVsFull(t *testing.T) {
	f := model.FileChange{
		Path:    "main.go",
		Patch:   "@@ -1,3 +1,5 @@\n func main() {}",
		Content: "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n",
	}
	fast := estimateTokens(f, true, 0)
	full := estimateTokens(f, false, 0)
	if full < fast {
		t.Errorf("full (%d) < fast (%d): full mode should count more tokens", full, fast)
	}
}

func TestContentForPrompt_FastIsEmpty(t *testing.T) {
	f := model.FileChange{Content: "package main\n"}
	if got := contentForPrompt(f, true, 0); got != "" {
		t.Errorf("contentForPrompt(fast) = %q, want empty", got)
	}
}

func TestContentForPrompt_UnlimitedReturnsFull(t *testing.T) {
	f := model.FileChange{Content: strings.Repeat("x := 1\n", 500)}
	if got := contentForPrompt(f, false, 0); got != f.Content {
		t.Errorf("contentForPrompt with maxTokens=0 should return full content")
	}
}

func TestContentForPrompt_CapTruncatesWithMarker(t *testing.T) {
	f := model.FileChange{Content: "HEADMARK\n" + strings.Repeat("filler := 1\n", 4000) + "TAILMARK\n"}

	got := contentForPrompt(f, false, 200)

	if len(got) >= len(f.Content) {
		t.Fatalf("capped content (%d bytes) should be shorter than full (%d bytes)", len(got), len(f.Content))
	}
	if !strings.Contains(got, "truncated") {
		t.Errorf("capped content should carry the elision marker, got prefix %q", got[:min(80, len(got))])
	}
	if !strings.Contains(got, "HEADMARK") {
		t.Errorf("capped content should retain the head of the file")
	}
	if !strings.Contains(got, "TAILMARK") {
		t.Errorf("capped content should retain the tail of the file")
	}
}

// estimateTokens must measure exactly what reviewBatch sends, otherwise batch
// sizing drifts from reality.
func TestEstimateTokens_MatchesCappedContent(t *testing.T) {
	f := model.FileChange{
		Path:    "big.go",
		Patch:   "@@ -1,1 +1,1 @@\n-a\n+b",
		Content: strings.Repeat("filler := 1\n", 4000),
	}
	capped := estimateTokens(f, false, 200)
	uncapped := estimateTokens(f, false, 0)
	if capped >= uncapped {
		t.Errorf("capped estimate (%d) should be below uncapped (%d)", capped, uncapped)
	}
}

func TestBuildBatches_RespectsBatchBudget(t *testing.T) {
	files := make([]model.FileChange, 10)
	for i := range files {
		files[i] = model.FileChange{
			Path:  "f.go",
			Patch: strings.Repeat("+ line of code here\n", 200),
		}
	}

	wide := buildBatches(files, true, 0, 100000)
	narrow := buildBatches(files, true, 0, 1000)

	if len(narrow) <= len(wide) {
		t.Errorf("a smaller batch budget should produce more batches: narrow=%d wide=%d", len(narrow), len(wide))
	}
	total := 0
	for _, b := range narrow {
		total += len(b)
	}
	if total != len(files) {
		t.Errorf("batching lost files: got %d, want %d", total, len(files))
	}
}

func TestEffectiveConcurrency(t *testing.T) {
	// SetLimit(0) blocks errgroup forever, so 0 and negatives must be clamped.
	for _, in := range []int{0, -1, -99} {
		if got := effectiveConcurrency(in); got != defaultConcurrency {
			t.Errorf("effectiveConcurrency(%d) = %d, want %d", in, got, defaultConcurrency)
		}
	}
	if got := effectiveConcurrency(9); got != 9 {
		t.Errorf("effectiveConcurrency(9) = %d, want 9", got)
	}
}
