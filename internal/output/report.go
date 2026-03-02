package output

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/henrocdotnet/grumbler/internal/llm"
	glog "github.com/henrocdotnet/grumbler/internal/log"
	"github.com/henrocdotnet/grumbler/internal/model"
)

// ReportWriter persists incremental snapshots to a reports/ directory.
type ReportWriter struct {
	dir string // e.g. <repoRoot>/.grumbler/reports/<timestamp>/
	seq int    // monotonic stage counter
}

// NewReportWriter creates a timestamped report directory under <repoRoot>/.grumbler/reports/.
func NewReportWriter(repoRoot string) (*ReportWriter, error) {
	ts := time.Now().Format("2006-01-02T15-04-05")
	dir := filepath.Join(repoRoot, ".grumbler", "reports", ts)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating report dir: %w", err)
	}
	glog.L().Info("report dir created", "path", dir)
	return &ReportWriter{dir: dir}, nil
}

// Dir returns the report directory path.
func (rw *ReportWriter) Dir() string { return rw.dir }

// StageSnapshot writes the current ReviewContext state after a stage completes.
func (rw *ReportWriter) StageSnapshot(stageName string, rc model.ReviewResult) {
	rw.seq++
	name := fmt.Sprintf("%02d-%s.json", rw.seq, stageName)
	path := filepath.Join(rw.dir, name)

	data, err := json.MarshalIndent(rc, "", "  ")
	if err != nil {
		glog.L().Error("report snapshot marshal", "stage", stageName, "err", err)
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		glog.L().Error("report snapshot write", "path", path, "err", err)
		return
	}
	glog.L().Debug("report snapshot saved", "path", name, "suggestions", len(rc.Suggestions))
}

// Final writes the completed review result.
func (rw *ReportWriter) Final(result model.ReviewResult) error {
	path := filepath.Join(rw.dir, "report.grumbler.json")
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal final report: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write final report: %w", err)
	}
	glog.L().Info("final report saved", "path", path, "suggestions", len(result.Suggestions))
	return nil
}

// SARIF writes a SARIF 2.1.0 report to the report directory.
func (rw *ReportWriter) SARIF(result model.ReviewResult) error {
	path := filepath.Join(rw.dir, "report.sarif.json")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create sarif report: %w", err)
	}
	defer f.Close()
	if err := WriteSARIF(f, result); err != nil {
		return fmt.Errorf("write sarif report: %w", err)
	}
	glog.L().Info("sarif report saved", "path", path)
	return nil
}

// Markdown writes a human-readable markdown report alongside the JSON reports.
func (rw *ReportWriter) Markdown(result model.ReviewResult) error {
	path := filepath.Join(rw.dir, "report.md")

	var b strings.Builder
	b.WriteString("# Grumbler Code Review Report\n\n")
	b.WriteString(fmt.Sprintf("| Metric | Value |\n|--------|-------|\n"))
	// Severity counts for the table.
	counts := map[string]int{}
	for _, s := range result.Suggestions {
		counts[strings.ToUpper(s.SeverityStr)]++
	}

	b.WriteString(fmt.Sprintf("| **Files reviewed** | %d |\n", result.FilesCount))
	b.WriteString(fmt.Sprintf("| **Issues found** | %d |\n", len(result.Suggestions)))
	for _, sev := range []string{"CRITICAL", "HIGH", "MEDIUM", "LOW"} {
		if n := counts[sev]; n > 0 {
			b.WriteString(fmt.Sprintf("| \u00a0\u00a0\u00a0%s | %d |\n", severityBadge(sev), n))
		}
	}
	b.WriteString(fmt.Sprintf("| **Provider** | %s |\n", result.Provider))
	if result.Model != "" {
		b.WriteString(fmt.Sprintf("| **Model** | %s |\n", result.Model))
	}
	if result.TotalPromptTokens > 0 {
		b.WriteString(fmt.Sprintf("| **Prompt tokens** | %d |\n", result.TotalPromptTokens))
	}
	if result.TotalCompletionTokens > 0 {
		b.WriteString(fmt.Sprintf("| **Completion tokens** | %d |\n", result.TotalCompletionTokens))
	}
	b.WriteString(fmt.Sprintf("| **Passes** | %s |\n", strings.Join(result.PassesRun, " → ")))
	b.WriteString("\n---\n\n")

	// Group by file.
	type fileGroup struct {
		path        string
		suggestions []model.CodeSuggestion
	}
	order := []string{}
	grouped := map[string]*fileGroup{}
	for _, s := range result.Suggestions {
		fp := s.FilePath
		if fp == "" {
			fp = "(unknown)"
		}
		g, ok := grouped[fp]
		if !ok {
			g = &fileGroup{path: fp}
			grouped[fp] = g
			order = append(order, fp)
		}
		g.suggestions = append(g.suggestions, s)
	}

	for i, fp := range order {
		if i > 0 {
			b.WriteString("<p align=\"center\">· · ·</p>\n\n")
		}
		g := grouped[fp]
		b.WriteString(fmt.Sprintf("## %s\n\n", g.path))
		for _, s := range g.suggestions {
			sev := strings.ToUpper(s.SeverityStr)
			lang := s.Language
			if lang == "" {
				lang = langFromPath(fp)
			}

			b.WriteString("<details>\n")
			b.WriteString(fmt.Sprintf("<summary>%s (%s) — %s</summary>\n\n",
				severityBadge(sev), s.Category, suggestionTLDR(s)))

			b.WriteString(fmt.Sprintf("**Lines %d–%d**\n\n", s.StartLine, s.EndLine))
			b.WriteString(s.Description + "\n\n")

			if s.Snippet != "" {
				b.WriteString(fmt.Sprintf("**Current code:**\n```%s\n%s\n```\n\n", lang, s.Snippet))
			}
			if s.Proposal != "" {
				b.WriteString(fmt.Sprintf("**Suggested fix:**\n```%s\n%s\n```\n\n", lang, s.Proposal))
			}
			if s.AuditResult != "" {
				b.WriteString(fmt.Sprintf("*Audit result: %s*\n\n", s.AuditResult))
			}

			b.WriteString("</details>\n\n")
		}
	}

	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("write markdown report: %w", err)
	}
	glog.L().Info("markdown report saved", "path", path)
	return nil
}

// maxPromptLines caps prompt rendering in the markdown conversation log.
const maxPromptLines = 200

// Conversations writes conversations.json and conversations.md to the report dir.
func (rw *ReportWriter) Conversations(exchanges []llm.Exchange) error {
	// JSON — full fidelity
	jsonPath := filepath.Join(rw.dir, "conversations.json")
	data, err := json.MarshalIndent(exchanges, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal conversations: %w", err)
	}
	if err := os.WriteFile(jsonPath, data, 0o644); err != nil {
		return fmt.Errorf("write conversations.json: %w", err)
	}

	// Markdown — human-scannable
	mdPath := filepath.Join(rw.dir, "conversations.md")
	var b strings.Builder
	b.WriteString("# LLM Conversation Log\n\n")

	callNum := map[string]int{} // per-stage call counter
	for _, ex := range exchanges {
		stage := ex.Stage
		if stage == "" {
			stage = "(unknown)"
		}
		callNum[stage]++
		durSec := ex.Duration.Seconds()
		b.WriteString(fmt.Sprintf("## %s (call %d) — %.1fs  [prompt: %d tokens | completion: %d tokens]\n\n",
			stage, callNum[stage], durSec, ex.PromptTokens, ex.CompletionTokens))

		for _, m := range ex.Messages {
			b.WriteString(fmt.Sprintf("### %s prompt\n\n", strings.Title(m.Role)))
			b.WriteString(truncateLines(m.Content, maxPromptLines))
			b.WriteString("\n\n")
		}

		b.WriteString("### Response\n\n")
		b.WriteString(ex.Response)
		b.WriteString("\n\n")

		if ex.Error != "" {
			b.WriteString(fmt.Sprintf("**Error:** %s\n\n", ex.Error))
		}
		b.WriteString("---\n\n")
	}

	if err := os.WriteFile(mdPath, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("write conversations.md: %w", err)
	}
	glog.L().Info("conversations saved", "exchanges", len(exchanges), "path", rw.dir)
	return nil
}

// truncateLines returns at most n lines, appending an ellipsis if truncated.
func truncateLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "\n") + "\n\n... (truncated)"
}

// suggestionTLDR returns Synopsis if set, otherwise falls back to Title.
func suggestionTLDR(s model.CodeSuggestion) string {
	if s.Synopsis != "" {
		return s.Synopsis
	}
	return s.Title
}

func severityBadge(sev string) string {
	switch sev {
	case "CRITICAL":
		return "🔴 CRITICAL"
	case "HIGH":
		return "🟠 HIGH"
	case "MEDIUM":
		return "🟡 MEDIUM"
	case "LOW":
		return "🔵 LOW"
	default:
		return sev
	}
}

func langFromPath(path string) string {
	ext := filepath.Ext(path)
	switch ext {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".rb":
		return "ruby"
	default:
		return ""
	}
}
