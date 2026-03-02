package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/henrocdotnet/grumbler/internal/llm"
	glog "github.com/henrocdotnet/grumbler/internal/log"
	"github.com/henrocdotnet/grumbler/internal/model"
	"github.com/henrocdotnet/grumbler/internal/pipeline"
	"github.com/henrocdotnet/grumbler/internal/prompt"
	"github.com/henrocdotnet/grumbler/internal/rules"
	"github.com/pkoukk/tiktoken-go"
	"golang.org/x/sync/errgroup"
)

// Inspect performs the primary defect-detection pass using Execution Trace Analysis.
type Inspect struct{}

func (Inspect) Name() string { return "inspect" }

func (Inspect) Execute(ctx context.Context, rc *pipeline.ReviewContext) error {
	mode := rc.Config.Review.Mode
	if mode == "" {
		mode = "diff"
	}
	if mode == "file" {
		return reviewPerFile(ctx, rc)
	}
	return reviewDiffBased(ctx, rc)
}

// ---------------------------------------------------------------------
// Diff-based review: single (or batched) LLM call for all files
// ---------------------------------------------------------------------

const maxTokensPerBatch = 30000 // rough token budget per batch

func reviewDiffBased(ctx context.Context, rc *pipeline.ReviewContext) error {
	glog.L().Debug("review_diff entry", "fileCount", len(rc.Files))

	batches := buildBatches(rc.Files, rc.Fast)
	glog.L().Debug("review_diff batches", "count", len(batches))

	// Aggregate rules across all files for the system prompt.
	allRules := aggregateRules(rc)

	systemPrompt, err := prompt.Render("review_system", prompt.ReviewData{
		Rules: formatRulesForPrompt(allRules),
	})
	if err != nil {
		return fmt.Errorf("rendering system prompt: %w", err)
	}

	for i, batch := range batches {
		glog.L().Debug("review_diff batch", "batch", i, "files", len(batch))
		suggestions, err := reviewBatch(ctx, rc, systemPrompt, batch)
		if err != nil {
			rc.AddError(fmt.Errorf("diff batch %d: %w", i, err))
			continue
		}
		rc.AddSuggestions(suggestions)
	}

	glog.L().Debug("review_diff exit", "totalSuggestions", len(rc.Suggestions))
	return nil
}

// buildBatches partitions files into token-bounded batches.
func buildBatches(files []model.FileChange, fast bool) [][]model.FileChange {
	var batches [][]model.FileChange
	var cur []model.FileChange
	var curTokens int

	for _, f := range files {
		est := estimateTokens(f, fast)
		if len(cur) > 0 && curTokens+est > maxTokensPerBatch {
			batches = append(batches, cur)
			cur = nil
			curTokens = 0
		}
		cur = append(cur, f)
		curTokens += est
	}
	if len(cur) > 0 {
		batches = append(batches, cur)
	}
	return batches
}

// estimateTokens returns the token count for a file's contribution using tiktoken,
// falling back to a char/4 heuristic if the encoder is unavailable.
func estimateTokens(f model.FileChange, fast bool) int {
	enc, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		n := len(f.Patch)
		if !fast {
			n += len(f.Content)
		}
		t := n / 4
		if t < 100 {
			return 100
		}
		return t
	}
	n := len(enc.Encode(f.Patch, nil, nil))
	if !fast {
		n += len(enc.Encode(f.Content, nil, nil))
	}
	if n < 100 {
		return 100
	}
	return n
}

func reviewBatch(ctx context.Context, rc *pipeline.ReviewContext, systemPrompt string, files []model.FileChange) ([]model.CodeSuggestion, error) {
	diffFiles := make([]prompt.ReviewDiffFile, len(files))
	for i, f := range files {
		diffFiles[i] = prompt.ReviewDiffFile{
			Path:     f.Path,
			Language: f.Language,
			Diff:     f.Patch,
			Content:  f.Content,
		}
	}

	userPrompt, err := prompt.Render("review_user_diff", prompt.ReviewDiffData{
		Files: diffFiles,
		Fast:  rc.Fast,
	})
	if err != nil {
		return nil, fmt.Errorf("rendering user prompt: %w", err)
	}

	msgs := []llm.Message{
		llm.SystemMsg(systemPrompt),
		llm.UserMsg(userPrompt),
	}
	glog.L().Debug("review_diff prompt", "files", len(files), "systemLen", len(systemPrompt), "userLen", len(userPrompt))

	resp, err := llm.Retry(ctx, 2, rc.Provider, msgs, llm.DefaultOpts())
	if err != nil {
		return nil, err
	}

	// Build a set of valid file paths for this batch so we can validate LLM output.
	pathSet := make(map[string]string, len(files)) // path -> language
	for _, f := range files {
		pathSet[f.Path] = f.Language
	}

	return parseReviewResponse(resp, "", "", pathSet)
}

// aggregateRules collects the union of rules matching any file.
func aggregateRules(rc *pipeline.ReviewContext) []rules.Rule {
	seen := map[string]struct{}{}
	var result []rules.Rule
	for _, f := range rc.Files {
		for _, r := range rules.MatchRules(rc.Rules, f.Path) {
			if _, ok := seen[r.ID]; !ok {
				seen[r.ID] = struct{}{}
				result = append(result, r)
			}
		}
	}
	return result
}

// ---------------------------------------------------------------------
// Per-file review: original concurrent-per-file logic
// ---------------------------------------------------------------------

func reviewPerFile(ctx context.Context, rc *pipeline.ReviewContext) error {
	concurrency := rc.Config.Review.Concurrency
	if concurrency <= 0 {
		concurrency = 5
	}
	glog.L().Debug("review_files entry", "fileCount", len(rc.Files), "concurrency", concurrency)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	var mu sync.Mutex

	for _, f := range rc.Files {
		f := f
		g.Go(func() error {
			glog.L().Debug("review_file start", "path", f.Path)
			suggestions, err := reviewSingleFile(gctx, rc, f)
			if err != nil {
				glog.L().Error("review_file failed", "path", f.Path, "err", err)
				rc.AddError(fmt.Errorf("reviewing %s: %w", f.Path, err))
				return nil
			}
			glog.L().Debug("review_file done", "path", f.Path, "suggestions", len(suggestions))
			mu.Lock()
			rc.AddSuggestions(suggestions)
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}
	glog.L().Debug("review_files exit", "totalSuggestions", len(rc.Suggestions))
	return nil
}

func reviewSingleFile(ctx context.Context, rc *pipeline.ReviewContext, f model.FileChange) ([]model.CodeSuggestion, error) {
	applicableRules := rules.MatchRules(rc.Rules, f.Path)

	systemPrompt, err := prompt.Render("review_system", prompt.ReviewData{
		Language: f.Language,
		Rules:    formatRulesForPrompt(applicableRules),
	})
	if err != nil {
		return nil, fmt.Errorf("rendering system prompt: %w", err)
	}

	userPrompt, err := prompt.Render("review_user", prompt.ReviewFileData{
		FilePath:    f.Path,
		Language:    f.Language,
		Diff:        f.Patch,
		FileContent: f.Content,
		Fast:        rc.Fast,
	})
	if err != nil {
		return nil, fmt.Errorf("rendering user prompt: %w", err)
	}

	msgs := []llm.Message{
		llm.SystemMsg(systemPrompt),
		llm.UserMsg(userPrompt),
	}
	glog.L().Debug("review_file prompt", "path", f.Path, "systemLen", len(systemPrompt), "userLen", len(userPrompt))

	resp, err := llm.Retry(ctx, 2, rc.Provider, msgs, llm.DefaultOpts())
	if err != nil {
		return nil, err
	}
	glog.L().Debug("review_file response", "path", f.Path, "respLen", len(resp))

	return parseReviewResponse(resp, f.Path, f.Language, nil)
}

// ---------------------------------------------------------------------
// Response parsing
// ---------------------------------------------------------------------

type llmSuggestion struct {
	FilePath    string `json:"filePath"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
	Category    string `json:"category"`
	Snippet     string `json:"snippet"`
	Proposal    string `json:"proposal"`
	Synopsis    string `json:"synopsis"`
	StartLine   int    `json:"startLine"`
	EndLine     int    `json:"endLine"`
}

type llmReviewResponse struct {
	Suggestions []llmSuggestion `json:"suggestions"`
}

// parseReviewResponse converts raw LLM JSON into CodeSuggestions.
// In per-file mode, defaultFilePath and defaultLanguage are set; pathSet is nil.
// In diff mode, defaultFilePath is empty; pathSet maps valid paths to languages.
func parseReviewResponse(raw, defaultFilePath, defaultLanguage string, pathSet map[string]string) ([]model.CodeSuggestion, error) {
	extracted := llm.ExtractJSON(raw)

	var resp llmReviewResponse
	if err := json.Unmarshal([]byte(extracted), &resp); err != nil {
		var arr []llmSuggestion
		if err2 := json.Unmarshal([]byte(extracted), &arr); err2 != nil {
			preview := raw
			if len(preview) > 200 {
				preview = preview[:200]
			}
			return nil, fmt.Errorf("inspect: LLM response is not valid JSON (expected {\"suggestions\":[...]} or bare array)\n  parse error: %w\n  response preview: %s", err, preview)
		}
		glog.L().Warn("inspect: LLM returned bare array instead of {\"suggestions\":[...]}")
		resp.Suggestions = arr
	}

	var result []model.CodeSuggestion
	for i, s := range resp.Suggestions {
		fp := s.FilePath
		lang := defaultLanguage
		if fp == "" {
			fp = defaultFilePath
		}
		if pathSet != nil {
			if l, ok := pathSet[fp]; ok {
				lang = l
			}
		}
		cs := model.CodeSuggestion{
			FilePath:    fp,
			Language:    lang,
			Severity:    model.ParseSeverity(s.Severity),
			SeverityStr: s.Severity,
			Category:    s.Category,
			Title:       s.Title,
			Description: s.Description,
			Snippet:     s.Snippet,
			Proposal:    s.Proposal,
			Synopsis:    s.Synopsis,
			StartLine:   s.StartLine,
			EndLine:     s.EndLine,
		}
		if cs.ID == "" {
			cs.ID = fmt.Sprintf("%s:%d:%d", fp, s.StartLine, i)
		}
		result = append(result, cs)
	}

	return result, nil
}

func formatRulesForPrompt(rr []rules.Rule) string {
	if len(rr) == 0 {
		return ""
	}
	var s string
	for _, r := range rr {
		s += r.FormatForPrompt() + "\n"
	}
	return s
}
