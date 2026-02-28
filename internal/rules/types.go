package rules

import (
	"fmt"
	"strings"
)

// ValidPrefixes enumerates the required rule ID prefixes.
var ValidPrefixes = []string{
	"SEC-",  // Security
	"ERR-",  // Error handling
	"PERF-", // Performance
	"CONC-", // Concurrency
	"LOG-",  // Logging / observability
	"TEST-", // Testing
	"API-",  // API design / compatibility
	"RES-",  // Resource management
	"PROJ-", // Project-specific conventions
}

// Rule represents a single code review rule.
type Rule struct {
	ID          string   `yaml:"id" json:"id"`
	Title       string   `yaml:"title" json:"title"`
	Description string   `yaml:"description" json:"description"`
	Severity    string   `yaml:"severity" json:"severity"`
	Path        string   `yaml:"path" json:"path"` // glob pattern
	Tags        []string `yaml:"tags,omitempty" json:"tags,omitempty"`
	Category    string   `yaml:"category,omitempty" json:"category,omitempty"`
	Examples    []string `yaml:"examples,omitempty" json:"examples,omitempty"`
}

// RulesFile is the top-level structure of a rules YAML file.
type RulesFile struct {
	Rules []Rule `yaml:"rules" json:"rules"`
}

// ValidateRules checks that every rule ID starts with a known prefix.
func ValidateRules(rules []Rule) error {
	for _, r := range rules {
		if !hasValidPrefix(r.ID) {
			return fmt.Errorf("rule %q has invalid ID prefix; must start with one of: %s",
				r.ID, strings.Join(ValidPrefixes, ", "))
		}
	}
	return nil
}

func hasValidPrefix(id string) bool {
	for _, p := range ValidPrefixes {
		if strings.HasPrefix(id, p) {
			return true
		}
	}
	return false
}

// FormatForPrompt formats a rule for inclusion in an LLM prompt.
func (r *Rule) FormatForPrompt() string {
	s := "- Rule ID: " + r.ID + "\n"
	s += "  Title: " + r.Title + "\n"
	s += "  Description: " + r.Description + "\n"
	s += "  Severity: " + r.Severity + "\n"
	if r.Path != "" {
		s += "  Applies to: " + r.Path + "\n"
	}
	return s
}
