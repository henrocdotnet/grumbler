package model

import "testing"

func TestParseSeverity(t *testing.T) {
	tests := []struct {
		input string
		want  Severity
	}{
		{"critical", SeverityCritical},
		{"CRITICAL", SeverityCritical},
		{"high", SeverityHigh},
		{"medium", SeverityMedium},
		{"low", SeverityLow},
		{"info", SeverityInfo},
		{"unknown", SeverityInfo},
	}
	for _, tt := range tests {
		got := ParseSeverity(tt.input)
		if got != tt.want {
			t.Errorf("ParseSeverity(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestSeverityAtLeast(t *testing.T) {
	if !SeverityCritical.AtLeast(SeverityHigh) {
		t.Error("critical should be at least high")
	}
	if !SeverityMedium.AtLeast(SeverityMedium) {
		t.Error("medium should be at least medium")
	}
	if SeverityLow.AtLeast(SeverityHigh) {
		t.Error("low should not be at least high")
	}
}

func TestLanguageFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"main.go", "go"},
		{"src/index.ts", "typescript"},
		{"app.py", "python"},
		{"unknown.xyz", "text"},
	}
	for _, tt := range tests {
		got := LanguageFromPath(tt.path)
		if got != tt.want {
			t.Errorf("LanguageFromPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
