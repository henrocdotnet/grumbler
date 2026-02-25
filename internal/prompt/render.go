package prompt

import (
	"bytes"
	"fmt"
	"text/template"
	"time"
)

// ReviewData holds data for review_system.tmpl.
type ReviewData struct {
	Language string
	Rules    string // pre-formatted rules text
	Date     string
}

// ReviewFileData holds data for review_user.tmpl (per-file mode).
type ReviewFileData struct {
	FilePath    string
	Language    string
	Diff        string
	FileContent string
	Fast        bool
}

// ReviewDiffData holds data for review_user_diff.tmpl (combined diff mode).
type ReviewDiffData struct {
	Files []ReviewDiffFile
	Rules string
	Fast  bool
	Date  string
}

// ReviewDiffFile is a single file entry within a combined diff review.
type ReviewDiffFile struct {
	Path     string
	Language string
	Diff     string
	Content  string
}

// ExpertPanelData holds data for expert_panel_user.tmpl.
type ExpertPanelData struct {
	Diff      string
	RulesJSON string
}

// SafeguardData holds data for safeguard templates.
type SafeguardData struct {
	FileContent     string
	Diff            string
	SuggestionsJSON string
	Date            string
}

// GuardianData holds data for guardian templates.
type GuardianData struct {
	SuggestionsJSON string
	RulesJSON       string
}

var parsedTemplates *template.Template

func init() {
	var err error
	parsedTemplates, err = template.ParseFS(templatesFS, "templates/*.tmpl")
	if err != nil {
		panic(fmt.Sprintf("parsing embedded templates: %v", err))
	}
}

// Render renders a named template with the given data.
func Render(name string, data any) (string, error) {
	// Inject date if the data has a Date field
	injectDate(data)

	tmpl := getTemplate(name)
	if tmpl == nil {
		// Check for override
		tmpl = parsedTemplates.Lookup(name + ".tmpl")
	}
	if tmpl == nil {
		return "", fmt.Errorf("template %q not found", name)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template %q: %w", name, err)
	}
	return buf.String(), nil
}

func getTemplate(name string) *template.Template {
	// First check overrides, then embedded
	if overrideTmpls != nil {
		if t := overrideTmpls.Lookup(name + ".tmpl"); t != nil {
			return t
		}
	}
	return parsedTemplates.Lookup(name + ".tmpl")
}

func injectDate(data any) {
	today := time.Now().Format("02/01/2006")
	switch d := data.(type) {
	case ReviewData:
		if d.Date == "" {
			d.Date = today
		}
	case *ReviewData:
		if d.Date == "" {
			d.Date = today
		}
	case SafeguardData:
		if d.Date == "" {
			d.Date = today
		}
	case *SafeguardData:
		if d.Date == "" {
			d.Date = today
		}
	case ReviewDiffData:
		if d.Date == "" {
			d.Date = today
		}
	case *ReviewDiffData:
		if d.Date == "" {
			d.Date = today
		}
	}
}
