package stages

import (
	"strings"
	"testing"
)

func TestParseAuditResponse_WrappedObject(t *testing.T) {
	input := `{"reviews":[{"id":"1","description":"desc","snippet":"s","proposal":"p","synopsis":"syn","startLine":1,"endLine":2,"category":"bug","severity":"high","verdict":"keep","rationale":"ok"}]}`
	verdicts, err := parseAuditResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(verdicts) != 1 {
		t.Fatalf("got %d verdicts, want 1", len(verdicts))
	}
	if verdicts[0].ID != "1" {
		t.Errorf("got ID=%q, want %q", verdicts[0].ID, "1")
	}
	if verdicts[0].Verdict != "keep" {
		t.Errorf("got Verdict=%q, want %q", verdicts[0].Verdict, "keep")
	}
}

func TestParseAuditResponse_BareArray(t *testing.T) {
	input := `[{"id":"1","description":"desc","snippet":"s","proposal":"p","synopsis":"syn","startLine":1,"endLine":2,"category":"bug","severity":"high","verdict":"keep","rationale":"ok"}]`
	verdicts, err := parseAuditResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(verdicts) != 1 {
		t.Fatalf("got %d verdicts, want 1", len(verdicts))
	}
	if verdicts[0].ID != "1" {
		t.Errorf("got ID=%q, want %q", verdicts[0].ID, "1")
	}
}

func TestParseAuditResponse_ProseText(t *testing.T) {
	input := `Here is my analysis of the code. The function looks correct and handles edge cases well. I would recommend keeping all suggestions as-is.`
	_, err := parseAuditResponse(input)
	if err == nil {
		t.Fatal("expected error for prose input, got nil")
	}
	if !strings.Contains(err.Error(), "audit:") {
		t.Errorf("error should mention 'audit:': %v", err)
	}
	if !strings.Contains(err.Error(), "response preview:") {
		t.Errorf("error should include response preview: %v", err)
	}
}

func TestParseAuditResponse_EmptyWrapped(t *testing.T) {
	input := `{"reviews":[]}`
	verdicts, err := parseAuditResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(verdicts) != 0 {
		t.Errorf("got %d verdicts, want 0", len(verdicts))
	}
}

func TestParseAuditResponse_EmptyBareArray(t *testing.T) {
	input := `[]`
	verdicts, err := parseAuditResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(verdicts) != 0 {
		t.Errorf("got %d verdicts, want 0", len(verdicts))
	}
}

func TestParseAuditResponse_CodeFencedWrapped(t *testing.T) {
	input := "Here is my review:\n```json\n{\"reviews\":[{\"id\":\"1\",\"description\":\"d\",\"snippet\":\"s\",\"proposal\":\"p\",\"synopsis\":\"syn\",\"startLine\":1,\"endLine\":2,\"category\":\"bug\",\"severity\":\"high\",\"verdict\":\"reject\",\"rationale\":\"r\"}]}\n```\n"
	verdicts, err := parseAuditResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(verdicts) != 1 {
		t.Fatalf("got %d verdicts, want 1", len(verdicts))
	}
	if verdicts[0].Verdict != "reject" {
		t.Errorf("got Verdict=%q, want %q", verdicts[0].Verdict, "reject")
	}
}

func TestParseAuditResponse_BareArrayMultiple(t *testing.T) {
	input := `[{"id":"1","description":"d1","snippet":"s","proposal":"p","synopsis":"syn","startLine":1,"endLine":2,"category":"bug","severity":"high","verdict":"keep","rationale":"r"},{"id":"2","description":"d2","snippet":"s","proposal":"p","synopsis":"syn","startLine":3,"endLine":4,"category":"style","severity":"low","verdict":"revise","rationale":"r"},{"id":"3","description":"d3","snippet":"s","proposal":"p","synopsis":"syn","startLine":5,"endLine":6,"category":"logic","severity":"medium","verdict":"reject","rationale":"r"}]`
	verdicts, err := parseAuditResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(verdicts) != 3 {
		t.Fatalf("got %d verdicts, want 3", len(verdicts))
	}
	wantVerdicts := []string{"keep", "revise", "reject"}
	for i, want := range wantVerdicts {
		if verdicts[i].Verdict != want {
			t.Errorf("verdicts[%d].Verdict=%q, want %q", i, verdicts[i].Verdict, want)
		}
	}
}

func TestParseAuditResponse_MalformedJSON(t *testing.T) {
	input := `{"reviews":[{"id":"1"`
	_, err := parseAuditResponse(input)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestParseAuditResponse_MissingReviewsKey(t *testing.T) {
	// Valid JSON object but no "reviews" key — Go unmarshal succeeds with nil slice.
	// This documents that behavior: no error, empty result.
	input := `{"data":[{"id":"1"}]}`
	verdicts, err := parseAuditResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(verdicts) != 0 {
		t.Errorf("got %d verdicts, want 0 (missing reviews key yields nil slice)", len(verdicts))
	}
}
