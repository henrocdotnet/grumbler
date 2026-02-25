package model

import "strings"

// Severity represents the severity level of a code suggestion.
type Severity int

const (
	SeverityCritical Severity = iota
	SeverityHigh
	SeverityMedium
	SeverityLow
	SeverityInfo
)

var severityNames = [...]string{"critical", "high", "medium", "low", "info"}

func (s Severity) String() string {
	if int(s) < len(severityNames) {
		return severityNames[s]
	}
	return "unknown"
}

// ParseSeverity parses a severity string (case-insensitive).
func ParseSeverity(s string) Severity {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical":
		return SeverityCritical
	case "high":
		return SeverityHigh
	case "medium":
		return SeverityMedium
	case "low":
		return SeverityLow
	case "info":
		return SeverityInfo
	default:
		return SeverityInfo
	}
}

// AtLeast returns true if this severity is >= the given minimum.
func (s Severity) AtLeast(min Severity) bool {
	return s <= min // lower ordinal = higher severity
}
