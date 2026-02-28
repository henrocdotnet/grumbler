package rules

import (
	"strings"
	"testing"
)

func TestValidateRules_ValidPrefixes(t *testing.T) {
	rules := []Rule{
		{ID: "SEC-001"},
		{ID: "ERR-001"},
		{ID: "PERF-001"},
		{ID: "CONC-001"},
		{ID: "LOG-001"},
		{ID: "TEST-001"},
		{ID: "API-001"},
		{ID: "RES-001"},
		{ID: "PROJ-001"},
	}
	if err := ValidateRules(rules); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateRules_InvalidPrefix(t *testing.T) {
	rules := []Rule{
		{ID: "SEC-001"},
		{ID: "CUSTOM-001"},
	}
	err := ValidateRules(rules)
	if err == nil {
		t.Fatal("expected error for invalid prefix")
	}
	if !strings.Contains(err.Error(), "CUSTOM-001") {
		t.Errorf("error should mention the bad ID, got: %v", err)
	}
}

func TestValidateRules_EmptyID(t *testing.T) {
	rules := []Rule{{ID: ""}}
	err := ValidateRules(rules)
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestValidateRules_Empty(t *testing.T) {
	if err := ValidateRules(nil); err != nil {
		t.Fatalf("nil slice should pass, got: %v", err)
	}
	if err := ValidateRules([]Rule{}); err != nil {
		t.Fatalf("empty slice should pass, got: %v", err)
	}
}

func TestValidateRules_DefaultsPass(t *testing.T) {
	if err := ValidateRules(DefaultRules()); err != nil {
		t.Fatalf("default rules should pass validation: %v", err)
	}
}
