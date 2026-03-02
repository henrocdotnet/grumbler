package stages

import (
	"testing"
)

// --- Wrapped object (happy path) ---

func TestParseReviewTeamResponse_WrappedObject(t *testing.T) {
	input := `{"rules":[{"ruleId":"SEC-001","reason":"SQL injection found"},{"ruleId":"ERR-002","reason":"Error ignored"}]}`
	got := parseReviewTeamResponse(input)
	if len(got) != 2 {
		t.Fatalf("got %d violations, want 2", len(got))
	}
	if got[0].ID != "SEC-001" {
		t.Errorf("[0].ID: got %q, want %q", got[0].ID, "SEC-001")
	}
	if got[1].Reason != "Error ignored" {
		t.Errorf("[1].Reason: got %q", got[1].Reason)
	}
}

// --- Bare array fallback ---

func TestParseReviewTeamResponse_BareArray(t *testing.T) {
	input := `[{"ruleId":"PERF-001","reason":"N+1 query"}]`
	got := parseReviewTeamResponse(input)
	if len(got) != 1 {
		t.Fatalf("got %d, want 1", len(got))
	}
	if got[0].ID != "PERF-001" {
		t.Errorf("ID: got %q", got[0].ID)
	}
}

// --- Code-fenced JSON ---

func TestParseReviewTeamResponse_CodeFenced(t *testing.T) {
	input := "Here are the violations:\n```json\n{\"rules\":[{\"ruleId\":\"SEC-002\",\"reason\":\"Hardcoded secret\"}]}\n```\n"
	got := parseReviewTeamResponse(input)
	if len(got) != 1 {
		t.Fatalf("got %d, want 1", len(got))
	}
	if got[0].ID != "SEC-002" {
		t.Errorf("ID: got %q", got[0].ID)
	}
}

// --- Prose / garbage returns nil ---

func TestParseReviewTeamResponse_Prose(t *testing.T) {
	input := "I analyzed the code and found no rule violations. Everything looks clean."
	got := parseReviewTeamResponse(input)
	if got != nil {
		t.Errorf("expected nil for prose, got %d violations", len(got))
	}
}

// --- Empty rules array ---

func TestParseReviewTeamResponse_EmptyRules(t *testing.T) {
	input := `{"rules":[]}`
	got := parseReviewTeamResponse(input)
	if len(got) != 0 {
		t.Errorf("got %d, want 0", len(got))
	}
}

func TestParseReviewTeamResponse_EmptyBareArray(t *testing.T) {
	input := `[]`
	got := parseReviewTeamResponse(input)
	if len(got) != 0 {
		t.Errorf("got %d, want 0", len(got))
	}
}

// --- Wrong key yields nil (no "rules" key) ---

func TestParseReviewTeamResponse_WrongKey(t *testing.T) {
	input := `{"violations":[{"ruleId":"SEC-001","reason":"r"}]}`
	got := parseReviewTeamResponse(input)
	// Go unmarshal succeeds with nil slice for missing "rules" key.
	if len(got) != 0 {
		t.Errorf("got %d, want 0", len(got))
	}
}

// --- Malformed JSON returns nil ---

func TestParseReviewTeamResponse_MalformedJSON(t *testing.T) {
	input := `{"rules":[{"ruleId":"SEC-001"`
	got := parseReviewTeamResponse(input)
	if got != nil {
		t.Errorf("expected nil for malformed JSON, got %d", len(got))
	}
}

// --- Extra fields are ignored ---

func TestParseReviewTeamResponse_ExtraFields(t *testing.T) {
	input := `{"rules":[{"ruleId":"LOG-001","reason":"Missing context","severity":"high","confidence":0.9}]}`
	got := parseReviewTeamResponse(input)
	if len(got) != 1 {
		t.Fatalf("got %d, want 1", len(got))
	}
	if got[0].ID != "LOG-001" {
		t.Errorf("ID: got %q", got[0].ID)
	}
}

// --- Multiple violations ---

func TestParseReviewTeamResponse_Multiple(t *testing.T) {
	input := `{"rules":[{"ruleId":"SEC-001","reason":"r1"},{"ruleId":"ERR-001","reason":"r2"},{"ruleId":"PERF-001","reason":"r3"}]}`
	got := parseReviewTeamResponse(input)
	if len(got) != 3 {
		t.Fatalf("got %d, want 3", len(got))
	}
	wantIDs := []string{"SEC-001", "ERR-001", "PERF-001"}
	for i, want := range wantIDs {
		if got[i].ID != want {
			t.Errorf("[%d].ID: got %q, want %q", i, got[i].ID, want)
		}
	}
}
