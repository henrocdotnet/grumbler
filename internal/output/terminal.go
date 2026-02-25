package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/henrocdotnet/grumbler/internal/model"
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	fileStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	lineStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	codeStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(lipgloss.Color("235")).Padding(0, 1)
	divider    = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(strings.Repeat("─", 60))
)

func severityStyle(s model.Severity) lipgloss.Style {
	colors := map[model.Severity]string{
		model.SeverityCritical: "196", // red
		model.SeverityHigh:     "208", // orange
		model.SeverityMedium:   "220", // yellow
		model.SeverityLow:      "75",  // blue
		model.SeverityInfo:     "242", // gray
	}
	c, ok := colors[s]
	if !ok {
		c = "242"
	}
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(c))
}

func categoryStyle(cat string) lipgloss.Style {
	colors := map[string]string{
		"bug":         "196",
		"performance": "208",
		"security":    "161",
	}
	c, ok := colors[strings.ToLower(cat)]
	if !ok {
		c = "75"
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(c))
}

// WriteTerminal renders suggestions as pretty terminal output.
func WriteTerminal(w io.Writer, result model.ReviewResult) {
	if len(result.Suggestions) == 0 {
		fmt.Fprintln(w, titleStyle.Render("No issues found. Code looks good!"))
		fmt.Fprintf(w, "\n%s files reviewed | provider: %s | passes: %s\n",
			lineStyle.Render(fmt.Sprintf("%d", result.FilesCount)),
			lineStyle.Render(result.Provider),
			lineStyle.Render(strings.Join(result.PassesRun, " → ")),
		)
		return
	}

	header := fmt.Sprintf("Found %d issue(s)", len(result.Suggestions))
	fmt.Fprintln(w, titleStyle.Render(header))
	fmt.Fprintln(w)

	for i, s := range result.Suggestions {
		sevTag := severityStyle(s.Severity).Render(strings.ToUpper(s.SeverityStr))
		catTag := categoryStyle(s.Category).Render(s.Category)

		fmt.Fprintf(w, "%s  %s %s\n", sevTag, catTag, titleStyle.Render(s.Title))
		fmt.Fprintf(w, "   %s L%d–%d\n",
			fileStyle.Render(s.FilePath),
			s.StartLine, s.EndLine,
		)
		fmt.Fprintf(w, "   %s\n", s.Description)

		if s.ExistingCode != "" {
			fmt.Fprintf(w, "\n   %s\n%s\n", lineStyle.Render("existing:"), indentCode(s.ExistingCode))
		}
		if s.ImprovedCode != "" {
			fmt.Fprintf(w, "\n   %s\n%s\n", lineStyle.Render("suggested:"), indentCode(s.ImprovedCode))
		}

		if i < len(result.Suggestions)-1 {
			fmt.Fprintf(w, "\n%s\n\n", divider)
		}
	}

	fmt.Fprintf(w, "\n%s\n", divider)
	fmt.Fprintf(w, "%s files reviewed | %s issue(s) | provider: %s | passes: %s\n",
		lineStyle.Render(fmt.Sprintf("%d", result.FilesCount)),
		lineStyle.Render(fmt.Sprintf("%d", len(result.Suggestions))),
		lineStyle.Render(result.Provider),
		lineStyle.Render(strings.Join(result.PassesRun, " → ")),
	)
}

func indentCode(code string) string {
	lines := strings.Split(code, "\n")
	for i, l := range lines {
		lines[i] = "     " + codeStyle.Render(l)
	}
	return strings.Join(lines, "\n")
}
