package stages

import (
	"fmt"
	"strings"
	"testing"

	"github.com/henrocdotnet/grumbler/internal/model"
)

func bigPatchFiles(n int, linesEach int) []model.FileChange {
	files := make([]model.FileChange, n)
	for i := range files {
		files[i] = model.FileChange{
			Path:  fmt.Sprintf("f%02d.go", i),
			Patch: strings.Repeat(fmt.Sprintf("+ filler line in f%02d\n", i), linesEach),
		}
	}
	return files
}

func render(f model.FileChange) string {
	return fmt.Sprintf("=== %s ===\n%s\n\n", f.Path, f.Patch)
}

func TestBudgetedDiff_UnlimitedIncludesEverything(t *testing.T) {
	files := bigPatchFiles(5, 10)
	out, shown, total := budgetedDiff(files, 0, render)

	if shown != 5 || total != 5 {
		t.Fatalf("shown=%d total=%d, want 5/5", shown, total)
	}
	for _, f := range files {
		if !strings.Contains(out, f.Path) {
			t.Errorf("output missing %s", f.Path)
		}
	}
	if strings.Contains(out, "omitted") {
		t.Error("no truncation note expected when nothing was dropped")
	}
}

// Unbounded, these prompts exceeded the model's context window and the call
// returned nothing useful.
func TestBudgetedDiff_TruncatesToBudget(t *testing.T) {
	files := bigPatchFiles(40, 200)
	out, shown, total := budgetedDiff(files, 500, render)

	if total != 40 {
		t.Fatalf("total = %d, want 40", total)
	}
	if shown >= total {
		t.Fatalf("shown = %d, expected truncation below %d", shown, total)
	}
	if countTokens(out) > 2000 {
		t.Errorf("output is %d tokens, far above the 500-token budget", countTokens(out))
	}
	if !strings.Contains(out, "omitted") {
		t.Error("truncated output must state that files were omitted")
	}
	if !strings.Contains(out, "do not assume they are unchanged") {
		t.Error("truncation note must warn that omitted files were still modified")
	}
}

// A single oversized file must still produce a usable prompt.
func TestBudgetedDiff_AlwaysIncludesFirstFile(t *testing.T) {
	files := bigPatchFiles(3, 5000)
	out, shown, _ := budgetedDiff(files, 10, render)

	if shown != 1 {
		t.Fatalf("shown = %d, want 1", shown)
	}
	if out == "" {
		t.Fatal("output must not be empty even when the first file exceeds the budget")
	}
}

// Files carrying contract-level changes are what cross-file analysis needs, so
// they must survive truncation ahead of pure-body edits.
func TestBudgetedDiff_PrioritizesSignatureChanges(t *testing.T) {
	noise := bigPatchFiles(30, 200)
	signature := model.FileChange{
		Path:  "api.ts",
		Patch: "+export function doThing(a: string): void {\n+  return\n+}\n",
	}
	files := append(noise, signature)

	out, shown, total := budgetedDiff(files, 400, render)

	if shown >= total {
		t.Fatalf("expected truncation, shown=%d total=%d", shown, total)
	}
	if !strings.Contains(out, "api.ts") {
		t.Error("signature-changing file was dropped in favour of body-only edits")
	}
}

func TestSignatureScore(t *testing.T) {
	none := model.FileChange{Patch: "+  x := 1\n+  y := 2\n"}
	if got := signatureScore(none); got != 0 {
		t.Errorf("signatureScore(body-only) = %d, want 0", got)
	}

	some := model.FileChange{Patch: "+export function a() {}\n+type B = string\n+  x := 1\n"}
	if got := signatureScore(some); got < 2 {
		t.Errorf("signatureScore(2 declarations) = %d, want >= 2", got)
	}
}

func TestContextBudget_Fallback(t *testing.T) {
	if got := contextBudget(0); got != defaultMaxContextTokens {
		t.Errorf("contextBudget(0) = %d, want %d", got, defaultMaxContextTokens)
	}
	if got := contextBudget(1234); got != 1234 {
		t.Errorf("contextBudget(1234) = %d, want 1234", got)
	}
}

// Reordering only earns its keep when the budget forces a choice; on the common
// path the caller's file order must survive untouched.
func TestBudgetedDiff_PreservesOrderWhenItFits(t *testing.T) {
	files := []model.FileChange{
		{Path: "z_body.go", Patch: "+  x := 1\n"},
		{Path: "a_api.go", Patch: "+export function f() {}\n"}, // higher signature score
		{Path: "m_body.go", Patch: "+  y := 2\n"},
	}

	out, shown, total := budgetedDiff(files, 100000, render)
	if shown != 3 || total != 3 {
		t.Fatalf("shown=%d total=%d, want 3/3", shown, total)
	}

	want := []string{"z_body.go", "a_api.go", "m_body.go"}
	at := make([]int, len(want))
	for i, p := range want {
		at[i] = strings.Index(out, p)
		if at[i] < 0 {
			t.Fatalf("output missing %s", p)
		}
	}
	for i := 1; i < len(at); i++ {
		if at[i] < at[i-1] {
			t.Errorf("file order changed despite fitting the budget: %s appears before %s", want[i], want[i-1])
		}
	}
}
