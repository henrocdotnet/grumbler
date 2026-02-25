package output

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/henrocdotnet/grumbler/internal/model"
)

// SARIF 2.1.0 types (minimal subset)
type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name    string      `json:"name"`
	Version string      `json:"version"`
	Rules   []sarifRule `json:"rules,omitempty"`
}

type sarifRule struct {
	ID         string             `json:"id"`
	ShortDesc  sarifMultiformat   `json:"shortDescription"`
	FullDesc   sarifMultiformat   `json:"fullDescription"`
	DefaultCfg sarifDefaultConfig `json:"defaultConfiguration"`
}

type sarifMultiformat struct {
	Text string `json:"text"`
}

type sarifDefaultConfig struct {
	Level string `json:"level"`
}

type sarifResult struct {
	RuleID    string           `json:"ruleId"`
	Level     string           `json:"level"`
	Message   sarifMultiformat `json:"message"`
	Locations []sarifLocation  `json:"locations"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysical `json:"physicalLocation"`
}

type sarifPhysical struct {
	ArtifactLocation sarifArtifact `json:"artifactLocation"`
	Region           sarifRegion   `json:"region"`
}

type sarifArtifact struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
	EndLine   int `json:"endLine"`
}

// WriteSARIF renders suggestions as SARIF 2.1.0 JSON.
func WriteSARIF(w io.Writer, result model.ReviewResult) error {
	log := sarifLog{
		Schema:  "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/main/sarif-2.1/schema/sarif-schema-2.1.0.json",
		Version: "2.1.0",
		Runs: []sarifRun{{
			Tool: sarifTool{
				Driver: sarifDriver{
					Name:    "grumbler",
					Version: "0.1.0",
				},
			},
		}},
	}

	for _, s := range result.Suggestions {
		log.Runs[0].Results = append(log.Runs[0].Results, sarifResult{
			RuleID:  s.Category,
			Level:   severityToSARIF(s.Severity),
			Message: sarifMultiformat{Text: fmt.Sprintf("%s: %s", s.Title, s.Description)},
			Locations: []sarifLocation{{
				PhysicalLocation: sarifPhysical{
					ArtifactLocation: sarifArtifact{URI: s.FilePath},
					Region:           sarifRegion{StartLine: s.StartLine, EndLine: s.EndLine},
				},
			}},
		})
	}

	data, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w)
	return err
}

func severityToSARIF(s model.Severity) string {
	switch s {
	case model.SeverityCritical, model.SeverityHigh:
		return "error"
	case model.SeverityMedium:
		return "warning"
	default:
		return "note"
	}
}
