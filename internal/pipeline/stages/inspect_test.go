package stages

import (
	"testing"

	"github.com/henrocdotnet/grumbler/internal/model"
)

func TestEstimateTokens_NonZero(t *testing.T) {
	f := model.FileChange{
		Path:  "main.go",
		Patch: "@@ -1,3 +1,5 @@\n func main() {\n+\tfmt.Println(\"hello\")\n }",
	}
	n := estimateTokens(f, true)
	if n <= 0 {
		t.Errorf("estimateTokens = %d, want > 0", n)
	}
}

func TestEstimateTokens_MinFloor(t *testing.T) {
	f := model.FileChange{}
	n := estimateTokens(f, true)
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
	fast := estimateTokens(f, true)
	full := estimateTokens(f, false)
	if full < fast {
		t.Errorf("full (%d) < fast (%d): full mode should count more tokens", full, fast)
	}
}
