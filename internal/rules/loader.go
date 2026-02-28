package rules

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/henrocdotnet/grumbler/internal/config"
	"gopkg.in/yaml.v3"
)

// LoadRules reads rules from the config directory.
// Falls back to DefaultRules() when no user rules are found.
func LoadRules(dir string) ([]Rule, error) {
	path := filepath.Join(dir, config.Dir, "rules.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Info("no rules file found, using defaults", "count", len(DefaultRules()))
			return DefaultRules(), nil
		}
		return nil, fmt.Errorf("reading rules: %w", err)
	}

	var rf RulesFile
	if err := yaml.Unmarshal(data, &rf); err != nil {
		return nil, fmt.Errorf("parsing rules: %w", err)
	}
	if len(rf.Rules) == 0 {
		slog.Info("rules file empty, using defaults", "count", len(DefaultRules()))
		return DefaultRules(), nil
	}
	if err := ValidateRules(rf.Rules); err != nil {
		return nil, fmt.Errorf("validating rules: %w", err)
	}
	slog.Info("loaded user rules", "count", len(rf.Rules))
	return rf.Rules, nil
}

// LoadRulesFromFile reads rules from a specific YAML file.
func LoadRulesFromFile(path string) ([]Rule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var rf RulesFile
	if err := yaml.Unmarshal(data, &rf); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if err := ValidateRules(rf.Rules); err != nil {
		return nil, fmt.Errorf("validating rules in %s: %w", path, err)
	}
	return rf.Rules, nil
}

// SaveRules writes rules to the config directory.
func SaveRules(dir string, rules []Rule) error {
	path := filepath.Join(dir, config.Dir, "rules.yaml")
	rf := RulesFile{Rules: rules}
	data, err := yaml.Marshal(rf)
	if err != nil {
		return fmt.Errorf("marshaling rules: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}
