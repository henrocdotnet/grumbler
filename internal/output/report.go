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
	dir string // e.g. <repoRoot>/reports/<timestamp>/
	seq int    // monotonic stage counter
}

// NewReportWriter creates a timestamped report directory under <repoRoot>/reports/.
func NewReportWriter(repoRoot string) (*ReportWriter, error) {
	ts := time.Now().Format("2006-01-02T15-04-05")
	dir := filepath.Join(repoRoot, "reports", ts)
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
	path := filepath.Join(rw.dir, "final.json")
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

// Markdown writes a human-readable markdown report alongside final.json.
func (rw *ReportWriter) Markdown(result model.ReviewResult) error {
	path := filepath.Join(rw.dir, "report.md")

	var b strings.Builder
	b.WriteString("# Code Review Report\n\n")
	b.WriteString(fmt.Sprintf("| Metric | Value |\n|--------|-------|\n"))
	b.WriteString(fmt.Sprintf("| Files reviewed | %d |\n", result.FilesCount))
	b.WriteString(fmt.Sprintf("| Issues found | %d |\n", len(result.Suggestions)))
	b.WriteString(fmt.Sprintf("| Provider | %s |\n", result.Provider))
	if result.Model != "" {
		b.WriteString(fmt.Sprintf("| Model | %s |\n", result.Model))
	}
	b.WriteString(fmt.Sprintf("| Passes | %s |\n", strings.Join(result.PassesRun, " → ")))
	b.WriteString("\n")

	// Group suggestions by severity for the summary.
	counts := map[string]int{}
	for _, s := range result.Suggestions {
		counts[strings.ToUpper(s.SeverityStr)]++
	}
	for _, sev := range []string{"CRITICAL", "HIGH", "MEDIUM", "LOW"} {
		if n := counts[sev]; n > 0 {
			b.WriteString(fmt.Sprintf("- **%s**: %d\n", sev, n))
		}
	}
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

	for _, fp := range order {
		g := grouped[fp]
		b.WriteString(fmt.Sprintf("## %s\n\n", g.path))
		for _, s := range g.suggestions {
			sev := strings.ToUpper(s.SeverityStr)
			b.WriteString(fmt.Sprintf("### %s %s — %s\n\n", severityBadge(sev), s.Category, s.Title))
			b.WriteString(fmt.Sprintf("**Lines %d–%d**\n\n", s.StartLine, s.EndLine))
			b.WriteString(s.Description + "\n\n")

			if s.ExistingCode != "" {
				lang := s.Language
				if lang == "" {
					lang = langFromPath(fp)
				}
				b.WriteString(fmt.Sprintf("**Current code:**\n```%s\n%s\n```\n\n", lang, s.ExistingCode))
			}
			if s.ImprovedCode != "" {
				lang := s.Language
				if lang == "" {
					lang = langFromPath(fp)
				}
				b.WriteString(fmt.Sprintf("**Suggested fix:**\n```%s\n%s\n```\n\n", lang, s.ImprovedCode))
			}
			if s.SafeguardVerdict != "" {
				b.WriteString(fmt.Sprintf("*Safeguard verdict: %s*\n\n", s.SafeguardVerdict))
			}
			b.WriteString("---\n\n")
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
		b.WriteString(fmt.Sprintf("## %s (call %d) — %.1fs\n\n", stage, callNum[stage], durSec))

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
