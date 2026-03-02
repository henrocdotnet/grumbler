package stages

import (
	"strings"
	"testing"
)

// --- Wrapped object (happy path) ---

func TestParseReviewResponse_WrappedObject(t *testing.T) {
	input := `{"suggestions":[{"filePath":"api.go","title":"SQL injection","description":"bad","severity":"critical","category":"SEC","snippet":"s","proposal":"p","synopsis":"syn","startLine":1,"endLine":2}]}`
	got, err := parseReviewResponse(input, "default.go", "go", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d suggestions, want 1", len(got))
	}
	if got[0].FilePath != "api.go" {
		t.Errorf("filePath: got %q, want %q", got[0].FilePath, "api.go")
	}
	if got[0].Title != "SQL injection" {
		t.Errorf("title: got %q", got[0].Title)
	}
}

// --- Bare array fallback ---

func TestParseReviewResponse_BareArray(t *testing.T) {
	input := `[{"filePath":"api.go","title":"bug","description":"d","severity":"high","category":"ERR","snippet":"s","proposal":"p","synopsis":"syn","startLine":5,"endLine":6}]`
	got, err := parseReviewResponse(input, "default.go", "go", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d, want 1", len(got))
	}
	if got[0].FilePath != "api.go" {
		t.Errorf("filePath: got %q", got[0].FilePath)
	}
}

// --- Code-fenced JSON ---

func TestParseReviewResponse_CodeFenced(t *testing.T) {
	input := "Sure! Here are the issues:\n```json\n{\"suggestions\":[{\"filePath\":\"x.go\",\"title\":\"t\",\"description\":\"d\",\"severity\":\"low\",\"category\":\"C\",\"snippet\":\"s\",\"proposal\":\"p\",\"synopsis\":\"syn\",\"startLine\":1,\"endLine\":1}]}\n```\n"
	got, err := parseReviewResponse(input, "default.go", "go", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d, want 1", len(got))
	}
}

// --- Prose / garbage ---

func TestParseReviewResponse_Prose(t *testing.T) {
	input := "I reviewed the code and everything looks great. No issues found."
	_, err := parseReviewResponse(input, "default.go", "go", nil)
	if err == nil {
		t.Fatal("expected error for prose input")
	}
	if !strings.Contains(err.Error(), "inspect:") {
		t.Errorf("error should mention 'inspect:': %v", err)
	}
	if !strings.Contains(err.Error(), "response preview:") {
		t.Errorf("error should include response preview: %v", err)
	}
}

// --- Empty suggestions ---

func TestParseReviewResponse_EmptyWrapped(t *testing.T) {
	input := `{"suggestions":[]}`
	got, err := parseReviewResponse(input, "default.go", "go", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d, want 0", len(got))
	}
}

func TestParseReviewResponse_EmptyBareArray(t *testing.T) {
	input := `[]`
	got, err := parseReviewResponse(input, "default.go", "go", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d, want 0", len(got))
	}
}

// --- Missing filePath uses default ---

func TestParseReviewResponse_DefaultFilePath(t *testing.T) {
	input := `{"suggestions":[{"title":"t","description":"d","severity":"medium","category":"C","snippet":"s","proposal":"p","synopsis":"syn","startLine":1,"endLine":1}]}`
	got, err := parseReviewResponse(input, "fallback.go", "go", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].FilePath != "fallback.go" {
		t.Errorf("filePath: got %q, want %q", got[0].FilePath, "fallback.go")
	}
	if got[0].Language != "go" {
		t.Errorf("language: got %q, want %q", got[0].Language, "go")
	}
}

// --- pathSet language lookup ---

func TestParseReviewResponse_PathSetLanguage(t *testing.T) {
	input := `{"suggestions":[{"filePath":"app.ts","title":"t","description":"d","severity":"low","category":"C","snippet":"","proposal":"","synopsis":"","startLine":1,"endLine":1}]}`
	pathSet := map[string]string{"app.ts": "typescript"}
	got, err := parseReviewResponse(input, "", "", pathSet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].Language != "typescript" {
		t.Errorf("language: got %q, want %q", got[0].Language, "typescript")
	}
}

// --- Synthetic ID generation ---

func TestParseReviewResponse_SyntheticID(t *testing.T) {
	input := `{"suggestions":[{"filePath":"main.go","title":"t","description":"d","severity":"low","category":"C","snippet":"","proposal":"","synopsis":"","startLine":42,"endLine":42}]}`
	got, err := parseReviewResponse(input, "", "go", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].ID != "main.go:42:0" {
		t.Errorf("ID: got %q, want %q", got[0].ID, "main.go:42:0")
	}
}

// --- Malformed JSON ---

func TestParseReviewResponse_MalformedJSON(t *testing.T) {
	input := `{"suggestions":[{"title":"t"`
	_, err := parseReviewResponse(input, "x.go", "go", nil)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

// --- Wrong key (no "suggestions") ---

func TestParseReviewResponse_WrongKey(t *testing.T) {
	input := `{"results":[{"title":"t"}]}`
	got, err := parseReviewResponse(input, "x.go", "go", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Go unmarshal succeeds with nil slice — zero suggestions.
	if len(got) != 0 {
		t.Errorf("got %d, want 0 (wrong key should yield empty)", len(got))
	}
}

// --- Extra/unknown fields are ignored ---

func TestParseReviewResponse_ExtraFields(t *testing.T) {
	input := `{"suggestions":[{"filePath":"a.go","title":"t","description":"d","severity":"high","category":"C","snippet":"s","proposal":"p","synopsis":"syn","startLine":1,"endLine":1,"confidence":0.95,"reasoning":"extra field"}]}`
	got, err := parseReviewResponse(input, "", "go", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d, want 1", len(got))
	}
}

// --- Long prose error preview is truncated ---

func TestParseReviewResponse_LongProsePreview(t *testing.T) {
	input := strings.Repeat("This is a very long prose response. ", 50)
	_, err := parseReviewResponse(input, "x.go", "go", nil)
	if err == nil {
		t.Fatal("expected error for prose input")
	}
	// Preview should be capped at 200 chars.
	errStr := err.Error()
	if idx := strings.Index(errStr, "response preview:"); idx != -1 {
		preview := errStr[idx:]
		if len(preview) > 300 { // 200 chars + "response preview: " prefix + some slack
			t.Errorf("preview too long (%d chars), should be truncated", len(preview))
		}
	}
}
