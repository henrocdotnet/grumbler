package output

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/henrocdotnet/grumbler/internal/model"
)

// testFixture returns a synthetic ReviewResult with mixed severities and fields.
func testFixture() model.ReviewResult {
	return model.ReviewResult{
		Suggestions: []model.CodeSuggestion{
			{
				ID: "s1", FilePath: "internal/api.go", Language: "go",
				Severity: model.SeverityCritical, SeverityStr: "critical",
				Category: "SEC-001", Title: "SQL injection",
				Description: "User input interpolated into query.",
				Snippet:     "db.Query(\"SELECT * FROM users WHERE id=\" + id)",
				Proposal:    "db.Query(\"SELECT * FROM users WHERE id=$1\", id)",
				StartLine:   42, EndLine: 42,
				RuleIDs: []string{"SEC-001"},
			},
			{
				ID: "s2", FilePath: "internal/api.go", Language: "go",
				Severity: model.SeverityHigh, SeverityStr: "high",
				Category: "ERR-001", Title: "Ignored error",
				Description: "Return value of Close not checked.",
				Snippet:     "f.Close()", Proposal: "if err := f.Close(); err != nil { return err }",
				StartLine: 88, EndLine: 88,
				AuditResult: "keep",
			},
			{
				ID: "s3", FilePath: "pkg/utils/math.go", Language: "go",
				Severity: model.SeverityMedium, SeverityStr: "medium",
				Category: "PERF-001", Title: "Redundant allocation",
				Description: "Preallocate slice with known capacity.",
				StartLine:   10, EndLine: 15,
			},
			{
				ID: "s4", FilePath: "service.go", Language: "go",
				Severity: model.SeverityLow, SeverityStr: "low",
				Category: "LOG-001", Title: "Missing context",
				Description: "Log call lacks request ID.",
				StartLine:   5, EndLine: 5,
				AuditResult: "discard",
			},
		},
		FilesCount:            3,
		PassesRun:             []string{"prepare", "inspect", "compliance", "audit", "crossfile", "aggregate"},
		Provider:              "test-fixture",
		Model:                 "test-model",
		TotalPromptTokens:     1000,
		TotalCompletionTokens: 500,
	}
}

// newTempReportWriter creates a ReportWriter in a temp directory.
func newTempReportWriter(t *testing.T) *ReportWriter {
	t.Helper()
	dir := t.TempDir()
	return &ReportWriter{dir: dir}
}

func TestFinal_RoundTrip(t *testing.T) {
	rw := newTempReportWriter(t)
	fixture := testFixture()

	if err := rw.Final(fixture); err != nil {
		t.Fatalf("Final: %v", err)
	}

	path := filepath.Join(rw.dir, "grumbler.report.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var loaded model.ReviewResult
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(loaded.Suggestions) != len(fixture.Suggestions) {
		t.Errorf("suggestions: got %d, want %d", len(loaded.Suggestions), len(fixture.Suggestions))
	}
	if loaded.FilesCount != fixture.FilesCount {
		t.Errorf("filesCount: got %d, want %d", loaded.FilesCount, fixture.FilesCount)
	}
	if loaded.Provider != fixture.Provider {
		t.Errorf("provider: got %q, want %q", loaded.Provider, fixture.Provider)
	}
	if loaded.TotalPromptTokens != fixture.TotalPromptTokens {
		t.Errorf("promptTokens: got %d, want %d", loaded.TotalPromptTokens, fixture.TotalPromptTokens)
	}
	// Verify a field that exercises omitempty — auditResult should survive round-trip.
	if loaded.Suggestions[1].AuditResult != "keep" {
		t.Errorf("auditResult: got %q, want 'keep'", loaded.Suggestions[1].AuditResult)
	}
	// Verify omitempty drops empty auditResult.
	raw := string(data)
	if strings.Contains(raw, `"auditResult":""`) {
		t.Error("empty auditResult should be omitted from JSON")
	}
}

func TestSARIF_File(t *testing.T) {
	rw := newTempReportWriter(t)
	fixture := testFixture()

	if err := rw.SARIF(fixture); err != nil {
		t.Fatalf("SARIF: %v", err)
	}

	path := filepath.Join(rw.dir, "grumbler.report.sarif.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var sarif sarifLog
	if err := json.Unmarshal(data, &sarif); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if sarif.Version != "2.1.0" {
		t.Errorf("version: got %q, want 2.1.0", sarif.Version)
	}
	if len(sarif.Runs) != 1 {
		t.Fatalf("runs: got %d, want 1", len(sarif.Runs))
	}
	if sarif.Runs[0].Tool.Driver.Name != "grumbler" {
		t.Errorf("driver: got %q, want grumbler", sarif.Runs[0].Tool.Driver.Name)
	}
	if len(sarif.Runs[0].Results) != len(fixture.Suggestions) {
		t.Errorf("results: got %d, want %d", len(sarif.Runs[0].Results), len(fixture.Suggestions))
	}
}

func TestWriteSARIF_SeverityMapping(t *testing.T) {
	fixture := testFixture()
	var buf bytes.Buffer

	if err := WriteSARIF(&buf, fixture); err != nil {
		t.Fatalf("WriteSARIF: %v", err)
	}

	var sarif sarifLog
	if err := json.Unmarshal(buf.Bytes(), &sarif); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	want := []string{"error", "error", "warning", "note"} // critical, high, medium, low
	if len(sarif.Runs[0].Results) != len(want) {
		t.Fatalf("results: got %d, want %d", len(sarif.Runs[0].Results), len(want))
	}
	for i, r := range sarif.Runs[0].Results {
		if r.Level != want[i] {
			t.Errorf("result[%d] level: got %q, want %q", i, r.Level, want[i])
		}
	}

	// Verify message format includes title and description.
	msg := sarif.Runs[0].Results[0].Message.Text
	if !strings.Contains(msg, "SQL injection") || !strings.Contains(msg, "User input") {
		t.Errorf("result[0] message missing title/desc: %q", msg)
	}

	// Verify location mapping.
	loc := sarif.Runs[0].Results[0].Locations[0].PhysicalLocation
	if loc.ArtifactLocation.URI != "internal/api.go" {
		t.Errorf("result[0] URI: got %q", loc.ArtifactLocation.URI)
	}
	if loc.Region.StartLine != 42 {
		t.Errorf("result[0] startLine: got %d, want 42", loc.Region.StartLine)
	}
}

func TestMarkdown_Fixture(t *testing.T) {
	rw := newTempReportWriter(t)
	fixture := testFixture()

	if err := rw.Markdown(fixture, false); err != nil {
		t.Fatalf("Markdown: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(rw.dir, "grumbler.report.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	md := string(data)

	checks := []struct {
		label string
		ok    func() bool
	}{
		{"title", func() bool { return strings.Contains(md, "# Grumbler Code Review Report") }},
		{"files reviewed", func() bool { return strings.Contains(md, "| **Files reviewed** | 3 |") }},
		{"issues found", func() bool { return strings.Contains(md, "| **Issues found** | 4 |") }},
		{"critical badge", func() bool { return strings.Contains(md, "CRITICAL") }},
		{"provider", func() bool { return strings.Contains(md, "test-fixture") }},
		{"passes", func() bool { return strings.Contains(md, "prepare") }},
		{"file heading", func() bool { return strings.Contains(md, "## [internal/api.go](../../../internal/api.go)") }},
		{"code block", func() bool { return strings.Contains(md, "```go") }},
		{"outer details wrapper", func() bool { return strings.Contains(md, "<strong>Grumbler Report (") }},
		{"summary severity lines", func() bool { return strings.Contains(md, "🔴 CRITICAL: 1<br>") }},
		{"closing details", func() bool { return strings.HasSuffix(strings.TrimSpace(md), "</details>") }},
		{"details count", func() bool { return strings.Count(md, "<details>") == 5 }},
		{"audit result shown", func() bool { return strings.Contains(md, "Audit result: keep") }},
		{"changes checklist", func() bool { return strings.Contains(md, "- [ ] Fixed") }},
	}
	for _, c := range checks {
		if !c.ok() {
			t.Errorf("assertion failed: %s", c.label)
		}
	}
}
