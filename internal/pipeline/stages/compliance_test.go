package stages

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/henrocdotnet/grumbler/internal/config"
	gitutil "github.com/henrocdotnet/grumbler/internal/git"
	"github.com/henrocdotnet/grumbler/internal/llm"
	"github.com/henrocdotnet/grumbler/internal/model"
	"github.com/henrocdotnet/grumbler/internal/pipeline"
	"github.com/henrocdotnet/grumbler/internal/rules"
)

func TestParseReviewTeamResponse(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		count     int
		bad       bool
	}{
		{"wrapped", `{"rules":[{"ruleId":"SEC-001","reason":"secret"}]}`, 1, false},
		{"bare", `[{"ruleId":"SEC-001","reason":"secret"}]`, 1, false},
		{"empty", `{"rules":[]}`, 0, false},
		{"empty bare", `[]`, 0, false},
		{"null", `null`, 0, true},
		{"wrong key", `{"violations":[]}`, 0, true},
		{"null rules", `{"rules":null}`, 0, true},
		{"prose", "No violations found.", 0, true},
		{"malformed", `{"rules":[`, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseReviewTeamResponse(tc.raw)
			if (err != nil) != tc.bad {
				t.Fatalf("err=%v, want bad=%v", err, tc.bad)
			}
			if len(got) != tc.count {
				t.Fatalf("got %d violations, want %d", len(got), tc.count)
			}
		})
	}
}

func ruleContext() *pipeline.ReviewContext {
	return &pipeline.ReviewContext{
		Config: config.Defaults(),
		Rules:  []rules.Rule{{ID: "SEC-005", FileGlob: "support/*.ts", Severity: "low"}},
		Files: []model.FileChange{
			{Path: "support/backend.ts", Language: "typescript", Patch: gitutil.AnnotatePatch("@@ -1,1 +1,1 @@\n+return backend === prod;\n")},
			{Path: "support/other.ts", Language: "typescript", Patch: gitutil.AnnotatePatch("@@ -1,1 +1,1 @@\n+return backend === prod;\n")},
			{Path: "excluded.ts", Patch: gitutil.AnnotatePatch("@@ -1,1 +1,1 @@\n+return backend === prod;\n")},
		},
	}
}

func boundaryViolation(path string) reviewTeamViolation {
	return reviewTeamViolation{ID: "SEC-005", Reason: "The guard recognizes the canonical origin; system IDs constrain writes.", llmSuggestion: llmSuggestion{
		FilePath: path, Title: "Canonical origin boundary", Snippet: "return backend === prod;", StartLine: 1, EndLine: 1, Category: "maintainability", Severity: "low",
	}}
}

func TestComplianceCreatesFindingsWhenInspectFoundNothing(t *testing.T) {
	rc := ruleContext()
	rc.Provider = auditTestProvider{complete: func([]llm.Message) string {
		data, err := json.Marshal(reviewTeamResponse{Rules: []reviewTeamViolation{boundaryViolation("support/backend.ts"), boundaryViolation("support/other.ts")}})
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}}
	if err := (Compliance{}).Execute(context.Background(), rc); err != nil {
		t.Fatal(err)
	}
	if len(rc.Errors) != 0 || len(rc.Suggestions) != 2 {
		t.Fatalf("independent violations were lost: %+v; errors: %v", rc.Suggestions, rc.Errors)
	}
	if rc.Suggestions[0].ID == rc.Suggestions[1].ID {
		t.Fatal("different locations collided")
	}
	for _, s := range rc.Suggestions {
		if len(s.RuleIDs) != 1 || s.RuleIDs[0] != "SEC-005" {
			t.Errorf("missing rule provenance: %+v", s)
		}
	}
}

func TestComplianceTagsOnlyMatchingIssue(t *testing.T) {
	rc := ruleContext()
	rc.Suggestions = []model.CodeSuggestion{
		{ID: "same", FilePath: "support/backend.ts", Title: "Canonical origin boundary", Snippet: "return backend === prod;", StartLine: 1, EndLine: 1},
		{ID: "different issue", FilePath: "support/backend.ts", Title: "Different issue", Snippet: "other()", StartLine: 1, EndLine: 1},
		{ID: "other file", FilePath: "support/other.ts", Title: "Other", StartLine: 1, EndLine: 1},
	}
	v := boundaryViolation("support/backend.ts")
	addRuleViolations(rc, []reviewTeamViolation{v, v})
	if len(rc.Suggestions) != 3 {
		t.Fatalf("matching finding duplicated: %+v", rc.Suggestions)
	}
	if len(rc.Suggestions[0].RuleIDs) != 1 {
		t.Fatal("rule ID missing or duplicated")
	}
	if len(rc.Suggestions[1].RuleIDs) != 0 || len(rc.Suggestions[2].RuleIDs) != 0 {
		t.Fatal("unrelated findings inherited rule tags")
	}
}

func TestComplianceRejectsInvalidLocationsAndRuleScopes(t *testing.T) {
	for _, tc := range []string{"unknown file", "wrong glob", "unknown rule", "context line", "invalid range", "missing title", "legacy unlocated"} {
		t.Run(tc, func(t *testing.T) {
			rc := ruleContext()
			v := boundaryViolation("support/backend.ts")
			switch tc {
			case "unknown file":
				v.FilePath = "support/missing.ts"
			case "wrong glob":
				v.FilePath = "excluded.ts"
			case "unknown rule":
				v.ID = "SEC-999"
			case "context line":
				v.StartLine = 2
				v.EndLine = 2
			case "invalid range":
				v.EndLine = 0
			case "missing title":
				v.Title = ""
			case "legacy unlocated":
				v = reviewTeamViolation{ID: "SEC-005", Reason: "no location"}
			}
			addRuleViolations(rc, []reviewTeamViolation{v})
			if len(rc.Errors) != 1 || len(rc.Suggestions) != 0 {
				t.Fatalf("invalid violation accepted: %+v, errors: %v", rc.Suggestions, rc.Errors)
			}
		})
	}
}

func TestComplianceMalformedResponseRecordsError(t *testing.T) {
	rc := ruleContext()
	rc.Provider = auditTestProvider{complete: func([]llm.Message) string { return `{"wrongKey":[]}` }}
	if err := (Compliance{}).Execute(context.Background(), rc); err != nil {
		t.Fatal(err)
	}
	if len(rc.Errors) != 1 || !strings.Contains(rc.Errors[0].Error(), "compliance:") {
		t.Fatalf("missing parse warning: %v", rc.Errors)
	}
}
