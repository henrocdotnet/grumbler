package stages

import (
	"context"
	"strings"
	"testing"

	"github.com/henrocdotnet/grumbler/internal/config"
	"github.com/henrocdotnet/grumbler/internal/llm"
	"github.com/henrocdotnet/grumbler/internal/model"
	"github.com/henrocdotnet/grumbler/internal/pipeline"
)

type auditTestProvider struct {
	complete func([]llm.Message) string
}

func (p auditTestProvider) Name() string { return "audit-test" }
func (p auditTestProvider) Complete(_ context.Context, msgs []llm.Message, _ llm.CompletionOpts) (string, error) {
	return p.complete(msgs), nil
}

func TestAuditIncludesCrossFileEvidenceAndCounterEvidence(t *testing.T) {
	rc := &pipeline.ReviewContext{
		Config: config.Defaults(),
		Files: []model.FileChange{
			{Path: "summary.test.ts", Patch: "+expect(heading).toMatch(/.+[(].+[)]/)", Content: "expect(heading).toMatch(/.+[(].+[)]/)"},
			{Path: "heading.ts", Patch: "+export const heading = (name, acronym) => acronym ? name + acronym : name;"},
			{Path: "separate/_tests_/focus.test.ts", Patch: "+expect(document.activeElement).toBe(searchBox);"},
		},
		Suggestions: []model.CodeSuggestion{{ID: "xf:summary:1", FilePath: "summary.test.ts", Description: "heading.ts returns a name without an acronym", Proposal: "old"}},
	}
	rc.Provider = auditTestProvider{complete: func(msgs []llm.Message) string {
		user := msgs[1].Content
		for _, evidence := range []string{"heading.ts", "acronym ? name", "separate/_tests_/focus.test.ts", "toBe(searchBox)"} {
			if !strings.Contains(user, evidence) {
				t.Errorf("audit lost supporting evidence %q", evidence)
			}
		}
		return `{"reviews":[{"id":"xf:summary:1","verdict":"revise","proposal":"","severity":"low","category":"maintainability","rationale":"The formatter allows a name-only heading; a safe exact replacement needs the expected system data."}]}`
	}}
	if err := (Audit{}).Execute(context.Background(), rc); err != nil {
		t.Fatal(err)
	}
	if err := (Aggregate{}).Execute(context.Background(), rc); err != nil {
		t.Fatal(err)
	}
	if len(rc.Suggestions) != 1 {
		t.Fatalf("supported finding was lost: %+v", rc.Suggestions)
	}
	s := rc.Suggestions[0]
	if s.AuditResult != "update" || s.Proposal != "" || s.Severity != model.SeverityLow || s.Category != "maintainability" || s.AuditRationale == "" {
		t.Fatalf("finding and repair decisions were not kept separately: %+v", s)
	}
}

func TestAuditContextBudgetAndPrioritizedEvidence(t *testing.T) {
	rc := &pipeline.ReviewContext{Config: config.Defaults()}
	rc.Config.Passes.MaxContextTokens = 1800
	primary := model.FileChange{Path: "test.ts", Patch: "+assert(result)", Content: "assert(result)"}
	rc.Files = []model.FileChange{
		primary,
		{Path: "noise.ts", Patch: strings.Repeat("+const noise = 1;\n", 4000)},
		{Path: "contract.ts", Patch: "+export const contract = true;"},
	}
	msgs, err := auditMessages(rc, primary, []model.CodeSuggestion{{ID: "1", Description: "contract.ts supplies the contract"}})
	if err != nil {
		t.Fatal(err)
	}
	combined := msgs[0].Content + msgs[1].Content
	if countTokens(combined) > 1800 {
		t.Fatal("audit prompt exceeded its complete token budget")
	}
	if !strings.Contains(msgs[1].Content, "export const contract") {
		t.Fatal("cited evidence lost to an oversized unrelated file")
	}
	if !strings.Contains(msgs[1].Content, "Missing evidence is not counter-evidence") {
		t.Fatal("omissions were not disclosed")
	}
	if rc.Files[1].Path != "noise.ts" {
		t.Fatal("audit reordered shared review files")
	}
}

func TestAuditIncompleteResponsesRemainUnresolved(t *testing.T) {
	for _, response := range []string{
		`{"reviews":[]}`,
		`{"data":[]}`,
		`{"reviews":[{"id":"1","verdict":"accept","rationale":"typo"}]}`,
		`{"reviews":[{"id":"other","verdict":"reject","rationale":"wrong candidate"}]}`,
	} {
		t.Run(response, func(t *testing.T) {
			calls := 0
			rc := &pipeline.ReviewContext{
				Config:      config.Defaults(),
				Provider:    auditTestProvider{complete: func([]llm.Message) string { calls++; return response }},
				Files:       []model.FileChange{{Path: "file.ts", Patch: "+changed()"}},
				Suggestions: []model.CodeSuggestion{{ID: "1", FilePath: "file.ts", Severity: model.SeverityHigh, Proposal: "original"}},
			}
			if err := (Audit{}).Execute(context.Background(), rc); err != nil {
				t.Fatal(err)
			}
			if calls != 2 {
				t.Errorf("got %d attempts, want 2", calls)
			}
			if err := (Aggregate{}).Execute(context.Background(), rc); err != nil {
				t.Fatal(err)
			}
			if len(rc.Suggestions) != 1 || rc.Suggestions[0].AuditResult != "unresolved" || rc.Suggestions[0].AuditRationale == "" || len(rc.Errors) != 1 {
				t.Fatalf("incomplete audit was treated as verified: %+v, errors: %v", rc.Suggestions, rc.Errors)
			}
		})
	}
}

func TestAuditRequiredContextTooLargePreservesFinding(t *testing.T) {
	rc := &pipeline.ReviewContext{
		Config:      config.Defaults(),
		Provider:    auditTestProvider{complete: func([]llm.Message) string { t.Fatal("oversized prompt sent to provider"); return "" }},
		Files:       []model.FileChange{{Path: "file.ts", Patch: strings.Repeat("+changed();\n", 1000)}},
		Suggestions: []model.CodeSuggestion{{ID: "1", FilePath: "file.ts", Severity: model.SeverityHigh}},
	}
	rc.Config.Passes.MaxContextTokens = 100
	if err := (Audit{}).Execute(context.Background(), rc); err != nil {
		t.Fatal(err)
	}
	if rc.Suggestions[0].AuditResult != "unresolved" {
		t.Fatalf("finding lost: %+v", rc.Suggestions[0])
	}
}

func TestValidateAuditVerdictsRequiresCompleteUniqueDecisions(t *testing.T) {
	candidates := []model.CodeSuggestion{{ID: "1"}}
	for _, verdicts := range [][]auditVerdict{
		{{ID: "1", Verdict: "keep"}},
		{{ID: "1", Verdict: "revise", Rationale: "proposal needs correction"}},
		{{ID: "1", Verdict: "keep", Rationale: "valid"}, {ID: "1", Verdict: "reject", Rationale: "duplicate"}},
	} {
		if err := validateAuditVerdicts(verdicts, candidates); err == nil {
			t.Fatalf("accepted invalid verdicts: %+v", verdicts)
		}
	}
}

func TestAuditDisprovenClaimIsDiscarded(t *testing.T) {
	s := model.CodeSuggestion{ID: "1", FilePath: "test.ts", Severity: model.SeverityHigh}
	applyAuditVerdict(&s, &auditVerdict{Verdict: "reject", Rationale: "The separate focus test asserts the behavior."})
	rc := &pipeline.ReviewContext{Config: config.Defaults(), Suggestions: []model.CodeSuggestion{s}}
	if err := (Aggregate{}).Execute(context.Background(), rc); err != nil {
		t.Fatal(err)
	}
	if len(rc.Suggestions) != 0 {
		t.Fatal("disproven finding survived aggregate")
	}
}

func TestAuditRepairsProposalWithoutDiscardingFinding(t *testing.T) {
	verdicts, err := parseAuditResponse(`{"reviews":[{"id":"credential","verdict":"revise","proposal":"if (location.origin === appOrigin) restore(saved);","description":"Saved credentials are restored without an origin check.","severity":"high","rationale":"The missing origin boundary is visible; remove the unrelated baseURL guard from the proposal."}]}`)
	if err != nil {
		t.Fatal(err)
	}
	s := model.CodeSuggestion{ID: "credential", FilePath: "session.ts", Description: "original claim", Proposal: "if (!baseURL) throw Error(); restore(saved);", Severity: model.SeverityHigh}
	if err := validateAuditVerdicts(verdicts, []model.CodeSuggestion{s}); err != nil {
		t.Fatal(err)
	}
	applyAuditVerdict(&s, &verdicts[0])
	if s.AuditResult != "update" || strings.Contains(s.Proposal, "!baseURL") || !strings.Contains(s.Proposal, "location.origin") || s.AuditRationale == "" {
		t.Fatalf("supported claim discarded or unsafe fix retained: %+v", s)
	}
}

func TestAuditPreservesLaterCallerPatchBeforeEarlyFullSource(t *testing.T) {
	primary := model.FileChange{Path: "primary.ts", Patch: "+assert()"}
	rc := &pipeline.ReviewContext{Config: config.Defaults(), Files: []model.FileChange{primary}}
	base, err := auditMessages(rc, primary, nil)
	if err != nil {
		t.Fatal(err)
	}
	rc.Config.Passes.MaxContextTokens = countTokens(base[0].Content+base[1].Content) + 1000
	rc.Files = append(rc.Files,
		model.FileChange{Path: "early.ts", Patch: "+export const value = 1;", Content: strings.Repeat("noise ", 700)},
		model.FileChange{Path: "later-caller.ts", Patch: strings.Repeat("+call();\n", 100)},
	)
	msgs, err := auditMessages(rc, primary, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msgs[1].Content, "+call();") {
		t.Fatal("early full source crowded out later caller evidence")
	}
	if countTokens(msgs[0].Content+msgs[1].Content) > rc.Config.Passes.MaxContextTokens {
		t.Fatal("context exceeded budget")
	}
}

func TestAuditRevisionKeepsLocationAndSummaryConsistent(t *testing.T) {
	proposal := "explicitCapabilityCheck()"
	s := model.CodeSuggestion{ID: "same", FilePath: "helper.ts", Title: "Runtime failure", Snippet: "call()", StartLine: 49, EndLine: 49}
	v := auditVerdict{ID: "same", Verdict: "revise", Title: "Capability permits disabled mode", Description: "The caller blocks this mode, but the capability predicate returns true.", Snippet: "const canWrite = enabled;", StartLine: 17, EndLine: 17, Category: "maintainability", Severity: "low", Proposal: &proposal, Rationale: "Project selection prevents execution; clarify the local invariant."}
	if err := validateAuditVerdicts([]auditVerdict{v}, []model.CodeSuggestion{s}); err != nil {
		t.Fatal(err)
	}
	applyAuditVerdict(&s, &v)
	if s.Title != v.Title || s.StartLine != 17 || s.EndLine != 17 || s.Snippet != v.Snippet || s.Category != "maintainability" || s.Severity != model.SeverityLow {
		t.Fatalf("revised finding retained stale metadata: %+v", s)
	}
}

func TestAggregateMergesOverlappingAdvisoriesAndPreservesRuleIDs(t *testing.T) {
	first := model.CodeSuggestion{ID: "inspect", FilePath: "select.tsx", Category: "maintainability", Title: "Selector contract", Snippet: "root.querySelector('button[aria-haspopup=listbox]')", StartLine: 134, EndLine: 134, Severity: model.SeverityLow, AuditResult: "update"}
	second := first
	second.ID = "rule"
	second.Title = "Dependency DOM contract"
	second.Snippet = "const target = " + first.Snippet + ";"
	second.StartLine = 131
	second.RuleIDs = []string{"API-002"}
	rc := &pipeline.ReviewContext{Config: config.Defaults(), Suggestions: []model.CodeSuggestion{first, second}}
	if err := (Aggregate{}).Execute(context.Background(), rc); err != nil {
		t.Fatal(err)
	}
	if len(rc.Suggestions) != 1 || len(rc.Suggestions[0].RuleIDs) != 1 || rc.Suggestions[0].RuleIDs[0] != "API-002" {
		t.Fatalf("duplicate advisories or lost provenance: %+v", rc.Suggestions)
	}
	for _, tc := range []string{"other file", "other category", "other lines", "other snippet", "unresolved"} {
		t.Run(tc, func(t *testing.T) {
			other := second
			switch tc {
			case "other file":
				other.FilePath = "other.tsx"
			case "other category":
				other.Category = "security"
			case "other lines":
				other.StartLine = 200
				other.EndLine = 201
			case "other snippet":
				other.Snippet = "aDifferentControl.focusWithoutAnySelector()"
			case "unresolved":
				other.AuditResult = "unresolved"
			}
			if sameAdvisoryEvidence(first, other) {
				t.Fatal("distinct or unresolved finding would be lost")
			}
		})
	}
}

func TestUnresolvedAuditCanRemoveAnUnsupportedProposal(t *testing.T) {
	empty := ""
	s := model.CodeSuggestion{ID: "1", Proposal: "inventedSchemaFix()"}
	applyAuditVerdict(&s, &auditVerdict{ID: "1", Verdict: "unresolved", Proposal: &empty, Rationale: "The successful response contract is not supplied."})
	if s.AuditResult != "unresolved" || s.Proposal != "" {
		t.Fatalf("unsupported proposal remains actionable: %+v", s)
	}
}

func TestLegacyAuditReclassificationUsesCorrectedSynopsis(t *testing.T) {
	proposal := "requireTarget()"
	s := model.CodeSuggestion{ID: "1", Title: "Runtime failure", Category: "bug", StartLine: 49, EndLine: 49}
	applyAuditVerdict(&s, &auditVerdict{ID: "1", Verdict: "revise", Category: "maintainability", Severity: "low", StartLine: 17, EndLine: 17, Synopsis: "Capability omits live target", Proposal: &proposal})
	if s.Title != "Capability omits live target" || s.StartLine != 17 {
		t.Fatalf("reclassified finding retains a stale heading or location: %+v", s)
	}
}
