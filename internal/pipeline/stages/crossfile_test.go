package stages

import (
	"strings"
	"testing"
)

// --- Wrapped object (happy path) ---

func TestParseCrossFileResponse_WrappedObject(t *testing.T) {
	input := `{"suggestions":[{"title":"Contract mismatch","description":"Caller not updated","snippet":"s","proposal":"p","synopsis":"syn","filePath":"api.go","startLine":10,"endLine":15,"category":"bug","severity":"high"}]}`
	got, err := parseCrossFileResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d, want 1", len(got))
	}
	if got[0].Title != "Contract mismatch" {
		t.Errorf("title: got %q", got[0].Title)
	}
	if got[0].FilePath != "api.go" {
		t.Errorf("filePath: got %q", got[0].FilePath)
	}
	if got[0].SeverityStr != "high" {
		t.Errorf("severity: got %q", got[0].SeverityStr)
	}
}

// --- Bare array fallback ---

func TestParseCrossFileResponse_BareArray(t *testing.T) {
	input := `[{"title":"Type mismatch","description":"d","snippet":"s","proposal":"p","synopsis":"syn","filePath":"types.go","startLine":1,"endLine":2,"category":"bug","severity":"critical"}]`
	got, err := parseCrossFileResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d, want 1", len(got))
	}
	if got[0].FilePath != "types.go" {
		t.Errorf("filePath: got %q", got[0].FilePath)
	}
}

// --- Code-fenced JSON ---

func TestParseCrossFileResponse_CodeFenced(t *testing.T) {
	input := "Here are the cross-file issues:\n```json\n{\"suggestions\":[{\"title\":\"Import drift\",\"description\":\"d\",\"snippet\":\"s\",\"proposal\":\"p\",\"synopsis\":\"syn\",\"filePath\":\"main.go\",\"startLine\":1,\"endLine\":1,\"category\":\"bug\",\"severity\":\"medium\"}]}\n```\n"
	got, err := parseCrossFileResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d, want 1", len(got))
	}
}

// --- Prose / garbage ---

func TestParseCrossFileResponse_Prose(t *testing.T) {
	input := "After careful analysis, I found no cross-file issues in the codebase."
	_, err := parseCrossFileResponse(input)
	if err == nil {
		t.Fatal("expected error for prose input")
	}
	if !strings.Contains(err.Error(), "crossfile:") {
		t.Errorf("error should mention 'crossfile:': %v", err)
	}
	if !strings.Contains(err.Error(), "response preview:") {
		t.Errorf("error should include response preview: %v", err)
	}
}

// --- Empty suggestions ---

func TestParseCrossFileResponse_EmptyWrapped(t *testing.T) {
	input := `{"suggestions":[]}`
	got, err := parseCrossFileResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d, want 0", len(got))
	}
}

func TestParseCrossFileResponse_EmptyBareArray(t *testing.T) {
	input := `[]`
	got, err := parseCrossFileResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d, want 0", len(got))
	}
}

// --- Malformed JSON ---

func TestParseCrossFileResponse_MalformedJSON(t *testing.T) {
	input := `{"suggestions":[{"title":"t"`
	_, err := parseCrossFileResponse(input)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

// --- Wrong key yields empty ---

func TestParseCrossFileResponse_WrongKey(t *testing.T) {
	input := `{"results":[{"title":"t"}]}`
	got, err := parseCrossFileResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d, want 0", len(got))
	}
}

// --- Extra fields are ignored ---

func TestParseCrossFileResponse_ExtraFields(t *testing.T) {
	input := `{"suggestions":[{"title":"t","description":"d","snippet":"s","proposal":"p","synopsis":"syn","filePath":"a.go","startLine":1,"endLine":1,"category":"C","severity":"low","confidence":0.8,"reasoning":"extra"}]}`
	got, err := parseCrossFileResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d, want 1", len(got))
	}
}

// --- Language inferred from filePath ---

func TestParseCrossFileResponse_LanguageFromPath(t *testing.T) {
	input := `{"suggestions":[{"title":"t","description":"d","snippet":"s","proposal":"p","synopsis":"syn","filePath":"app.ts","startLine":1,"endLine":1,"category":"C","severity":"low"}]}`
	got, err := parseCrossFileResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].Language != "typescript" {
		t.Errorf("language: got %q, want %q", got[0].Language, "typescript")
	}
}

// --- Multiple suggestions ---

func TestParseCrossFileResponse_Multiple(t *testing.T) {
	input := `{"suggestions":[
		{"title":"t1","description":"d","snippet":"s","proposal":"p","synopsis":"syn","filePath":"a.go","startLine":1,"endLine":1,"category":"C","severity":"critical"},
		{"title":"t2","description":"d","snippet":"s","proposal":"p","synopsis":"syn","filePath":"b.go","startLine":5,"endLine":10,"category":"C","severity":"low"}
	]}`
	got, err := parseCrossFileResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	if got[0].Title != "t1" || got[1].Title != "t2" {
		t.Errorf("titles: got %q, %q", got[0].Title, got[1].Title)
	}
}

// --- Long prose error preview is truncated ---

func TestParseCrossFileResponse_LongProsePreview(t *testing.T) {
	input := strings.Repeat("The code looks fine to me overall. ", 50)
	_, err := parseCrossFileResponse(input)
	if err == nil {
		t.Fatal("expected error")
	}
	errStr := err.Error()
	if idx := strings.Index(errStr, "response preview:"); idx != -1 {
		preview := errStr[idx:]
		if len(preview) > 300 {
			t.Errorf("preview too long (%d chars), should be truncated", len(preview))
		}
	}
}
