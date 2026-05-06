package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/henrocdotnet/grumbler/internal/model"
)

// WriteTerminal renders the review result as glamour-styled terminal markdown.
func WriteTerminal(w io.Writer, result model.ReviewResult) {
	md := buildTerminalMD(result)
	out, err := glamour.Render(md, "auto")
	if err != nil {
		fmt.Fprint(w, md)
		return
	}
	fmt.Fprint(w, out)
}

func buildTerminalMD(result model.ReviewResult) string {
	var b strings.Builder

	if len(result.Suggestions) == 0 {
		b.WriteString("# No issues found\n\nCode looks good!\n\n")
	} else {
		b.WriteString(fmt.Sprintf("# Found %d issue(s)\n\n", len(result.Suggestions)))
		for i, s := range result.Suggestions {
			sev := strings.ToUpper(s.SeverityStr)
			b.WriteString(fmt.Sprintf("## %s %s — %s\n\n", severityBadge(sev), s.Category, s.Title))
			b.WriteString(fmt.Sprintf("**%s** L%d–%d\n\n", s.FilePath, s.StartLine, s.EndLine))
			b.WriteString(s.Description + "\n\n")

			if s.Snippet != "" {
				lang := langFromPath(s.FilePath)
				b.WriteString(fmt.Sprintf("**existing:**\n```%s\n%s\n```\n\n", lang, s.Snippet))
			}
			if s.Proposal != "" {
				lang := langFromPath(s.FilePath)
				b.WriteString(fmt.Sprintf("**suggested:**\n```%s\n%s\n```\n\n", lang, s.Proposal))
			}
			if s.AuditResult != "" {
				b.WriteString(fmt.Sprintf("*Audit result: %s*\n\n", s.AuditResult))
			}
			if i < len(result.Suggestions)-1 {
				b.WriteString("---\n\n")
			}
		}
	}

	// Footer
	b.WriteString("\n---\n\n")
	parts := []string{
		fmt.Sprintf("%d files reviewed", result.FilesCount),
		fmt.Sprintf("%d issue(s)", len(result.Suggestions)),
		fmt.Sprintf("provider: `%s`", result.Provider),
		fmt.Sprintf("passes: %s", strings.Join(result.PassesRun, " → ")),
	}
	if result.TotalPromptTokens > 0 || result.TotalCompletionTokens > 0 {
		tok := fmt.Sprintf("tokens: %d→%d", result.TotalPromptTokens, result.TotalCompletionTokens)
		if result.TotalCacheCreationTokens > 0 || result.TotalCacheReadTokens > 0 {
			tok += fmt.Sprintf(" (cache: %d created, %d read)", result.TotalCacheCreationTokens, result.TotalCacheReadTokens)
		}
		parts = append(parts, tok)
	}
	b.WriteString("*" + strings.Join(parts, " | ") + "*\n")

	return b.String()
}
