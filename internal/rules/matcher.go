package rules

import (
	"github.com/bmatcuk/doublestar/v4"
)

// MatchRules returns the subset of rules whose path glob matches the given file path.
func MatchRules(rules []Rule, filePath string) []Rule {
	if len(rules) == 0 {
		return nil
	}

	var matched []Rule
	for _, r := range rules {
		if r.Path == "" {
			// No path restriction — applies to all files
			matched = append(matched, r)
			continue
		}
		ok, err := doublestar.Match(r.Path, filePath)
		if err == nil && ok {
			matched = append(matched, r)
		}
	}
	return matched
}

// ShouldIgnore checks if a file path matches any of the ignore patterns.
func ShouldIgnore(path string, patterns []string) bool {
	for _, p := range patterns {
		ok, err := doublestar.Match(p, path)
		if err == nil && ok {
			return true
		}
	}
	return false
}
