package rules

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
