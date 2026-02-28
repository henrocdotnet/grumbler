package output

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/henrocdotnet/grumbler/internal/model"
)

func TestMarkdown_FromFinalJSON(t *testing.T) {
	// Locate repo root by walking up from the test's working dir.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := wd
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatal("could not find repo root")
		}
		root = parent
	}

	src := filepath.Join(root, "reports", "2026-02-25T18-25-33", "final.json")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Skipf("skipping: fixture not present: %v", err)
	}

	var result model.ReviewResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshalling: %v", err)
	}
	t.Logf("loaded %d suggestions across %d files", len(result.Suggestions), result.FilesCount)

	// Write markdown alongside final.json.
	rw := &ReportWriter{dir: filepath.Dir(src)}

	if err := rw.Markdown(result); err != nil {
		t.Fatalf("Markdown: %v", err)
	}

	mdPath := filepath.Join(filepath.Dir(src), "report.md")
	t.Logf("wrote %s", mdPath)
	md, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("reading report.md: %v", err)
	}

	content := string(md)
	t.Logf("report.md: %d bytes", len(md))

	// Structural assertions.
	assertions := []struct {
		label string
		check func() bool
	}{
		{"has title", func() bool { return strings.Contains(content, "# Grumbler Code Review Report") }},
		{"has summary table", func() bool { return strings.Contains(content, "| Files reviewed |") }},
		{"has severity counts", func() bool { return strings.Contains(content, "**CRITICAL**") }},
		{"has file headings", func() bool { return strings.Contains(content, "## internal/") }},
		{"has code blocks", func() bool { return strings.Contains(content, "```go") }},
		{"has passes", func() bool { return strings.Contains(content, "inspect") }},
		{"provider populated", func() bool { return strings.Contains(content, result.Provider) }},
		{"suggestion count matches", func() bool {
			// Each suggestion produces a <details> collapsed section.
			return strings.Count(content, "<details>") == len(result.Suggestions)
		}},
	}

	for _, a := range assertions {
		if !a.check() {
			t.Errorf("assertion failed: %s", a.label)
		}
	}

	// Print first 80 lines for visual inspection.
	lines := strings.Split(content, "\n")
	if len(lines) > 80 {
		lines = lines[:80]
	}
	t.Logf("--- report.md (first 80 lines) ---\n%s", strings.Join(lines, "\n"))
}
