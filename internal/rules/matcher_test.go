package rules

import "testing"

func TestMatchRules(t *testing.T) {
	rules := []Rule{
		{ID: "PROJ-001", Name: "TS rule", FileGlob: "src/**/*.ts"},
		{ID: "PROJ-002", Name: "Global rule", FileGlob: ""},
		{ID: "PROJ-003", Name: "Go rule", FileGlob: "**/*.go"},
	}

	// TypeScript file should match r1 and r2
	matched := MatchRules(rules, "src/utils/helper.ts")
	if len(matched) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matched))
	}

	// Go file should match r2 and r3
	matched = MatchRules(rules, "internal/main.go")
	if len(matched) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matched))
	}

	// Python file should match only r2 (global)
	matched = MatchRules(rules, "script.py")
	if len(matched) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matched))
	}
}

func TestShouldIgnore(t *testing.T) {
	patterns := []string{"yarn.lock", "**/*.min.js", "vendor/**"}

	if !ShouldIgnore("yarn.lock", patterns) {
		t.Error("yarn.lock should be ignored")
	}
	if !ShouldIgnore("dist/bundle.min.js", patterns) {
		t.Error("*.min.js should be ignored")
	}
	if !ShouldIgnore("vendor/lib/foo.go", patterns) {
		t.Error("vendor/** should be ignored")
	}
	if ShouldIgnore("src/main.go", patterns) {
		t.Error("src/main.go should not be ignored")
	}
}
