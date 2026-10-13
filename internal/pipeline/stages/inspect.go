package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"unicode/utf8"

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

// defaultMaxTokensPerBatch is the fallback token budget for one inspect batch
// when review.maxBatchTokens is unset.
//
// Deliberately small. Detection quality collapses as the reviewed diff grows —
// measured on a 143-file branch, 6 batches of ~50k tokens found 9 issues while
// 40 batches of ~10k found 30, including two critical command injections the
// large-batch run missed entirely. Bigger batches amortize the per-spawn cost
// but dilute the review; concurrency is the right lever for wall clock.
const defaultMaxTokensPerBatch = 10000

func reviewDiffBased(ctx context.Context, rc *pipeline.ReviewContext) error {
	glog.L().Debug("review_diff entry", "fileCount", len(rc.Files))

	maxBatch := rc.Config.Review.MaxBatchTokens
	if maxBatch <= 0 {
		maxBatch = defaultMaxTokensPerBatch
	}
	maxFileContent := rc.Config.Review.MaxFileContentTokens

	batches := buildBatches(rc.Files, rc.Fast, maxFileContent, maxBatch)
	glog.L().Debug("review_diff batches", "count", len(batches),
		"maxBatchTokens", maxBatch, "maxFileContentTokens", maxFileContent)

	// Aggregate rules across all files for the system prompt.
	allRules := aggregateRules(rc)

	systemPrompt, err := prompt.Render("review_system", prompt.ReviewData{
		Rules: formatRulesForPrompt(allRules),
	})
	if err != nil {
		return fmt.Errorf("rendering system prompt: %w", err)
	}

	concurrency := effectiveConcurrency(rc.Config.Review.Concurrency)
	glog.L().Debug("review_diff concurrency", "limit", concurrency)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	// Indexed slots rather than a mutex-guarded append: rc.AddSuggestions and
	// rc.AddError are not thread-safe, and aggregate's dedup is first-wins with
	// an unstable sort, so a nondeterministic append order can change the
	// reported set once filter.maxSuggestions truncates. Draining in batch
	// order after Wait keeps results deterministic for free.
	results := make([][]model.CodeSuggestion, len(batches))
	errs := make([]error, len(batches))

	for i, batch := range batches {
		i, batch := i, batch
		g.Go(func() error {
			glog.L().Debug("review_diff batch start", "batch", i, "files", len(batch))
			suggestions, err := reviewBatch(gctx, rc, systemPrompt, batch)
			if err != nil {
				errs[i] = fmt.Errorf("diff batch %d: %w", i, err)
				return nil // non-fatal: one bad batch must not kill the stage
			}
			glog.L().Debug("review_diff batch done", "batch", i, "suggestions", len(suggestions))
			results[i] = suggestions
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	for i := range batches {
		if errs[i] != nil {
			rc.AddError(errs[i])
			continue
		}
		rc.AddSuggestions(results[i])
	}

	glog.L().Debug("review_diff exit", "totalSuggestions", len(rc.Suggestions))
	return nil
}

// buildBatches partitions files into token-bounded batches.
func buildBatches(files []model.FileChange, fast bool, maxFileContent, maxBatchTokens int) [][]model.FileChange {
	var batches [][]model.FileChange
	var cur []model.FileChange
	var curTokens int

	for _, f := range files {
		est := estimateTokens(f, fast, maxFileContent)
		if len(cur) > 0 && curTokens+est > maxBatchTokens {
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

// contentForPrompt returns the file content to embed alongside the diff.
//
// Empty when fast (diff-only). Otherwise the full content, capped at maxTokens
// with a head+tail window so imports and trailing declarations both survive.
// An explicit elision marker tells the model it is not seeing the whole file.
func contentForPrompt(f model.FileChange, fast bool, maxTokens int) string {
	if fast || f.Content == "" {
		return ""
	}
	if maxTokens <= 0 {
		return f.Content
	}

	enc, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		// Fall back to the char/4 heuristic used elsewhere in this file.
		limit := maxTokens * 4
		if len(f.Content) <= limit {
			return f.Content
		}
		head := limit * 6 / 10
		tail := limit - head
		return truncAt(f.Content, head, true) + elisionMarker + truncAt(f.Content, tail, false)
	}

	toks := enc.Encode(f.Content, nil, nil)
	if len(toks) <= maxTokens {
		return f.Content
	}
	head := maxTokens * 6 / 10
	tail := maxTokens - head
	return enc.Decode(toks[:head]) + elisionMarker + enc.Decode(toks[len(toks)-tail:])
}

const elisionMarker = "\n\n... [file content truncated: middle section elided] ...\n\n"

// truncAt returns the first (head) or last (!head) n bytes of s, backing off to
// a rune boundary so the byte-slicing fallback can't emit invalid UTF-8.
func truncAt(s string, n int, head bool) string {
	if n <= 0 || n >= len(s) {
		if head {
			return s
		}
		return s
	}
	if head {
		for n > 0 && !utf8.RuneStart(s[n]) {
			n--
		}
		return s[:n]
	}
	i := len(s) - n
	for i < len(s) && !utf8.RuneStart(s[i]) {
		i++
	}
	return s[i:]
}

// estimateTokens returns the token count for a file's contribution using tiktoken,
// falling back to a char/4 heuristic if the encoder is unavailable.
//
// It must measure exactly what reviewBatch sends, so both route the embedded
// content through contentForPrompt.
func estimateTokens(f model.FileChange, fast bool, maxFileContent int) int {
	content := contentForPrompt(f, fast, maxFileContent)

	enc, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		t := (len(f.Patch) + len(content)) / 4
		if t < 100 {
			return 100
		}
		return t
	}
	n := len(enc.Encode(f.Patch, nil, nil))
	if content != "" {
		n += len(enc.Encode(content, nil, nil))
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
			Content:  contentForPrompt(f, rc.Fast, rc.Config.Review.MaxFileContentTokens),
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
	concurrency := effectiveConcurrency(rc.Config.Review.Concurrency)
	glog.L().Debug("review_files entry", "fileCount", len(rc.Files), "concurrency", concurrency)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

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
			rc.AddSuggestions(suggestions)
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
	suggestions, err := unmarshalReviewResponse(raw)
	if err != nil {
		return nil, err
	}

	var result []model.CodeSuggestion
	for i, s := range suggestions {
		fp := s.FilePath
		if fp == "" {
			fp = defaultFilePath
		}
		lang := defaultLanguage
		if l, ok := pathSet[fp]; ok {
			lang = l
		}
		result = append(result, model.CodeSuggestion{
			ID:          fmt.Sprintf("%s:%d:%d", fp, s.StartLine, i),
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
		})
	}

	return result, nil
}

// unmarshalReviewResponse accepts {"suggestions":[...]} or a bare array.
func unmarshalReviewResponse(raw string) ([]llmSuggestion, error) {
	extracted := llm.ExtractJSON(raw)

	var resp llmReviewResponse
	err := json.Unmarshal([]byte(extracted), &resp)
	if err == nil {
		return resp.Suggestions, nil
	}

	var arr []llmSuggestion
	if err2 := json.Unmarshal([]byte(extracted), &arr); err2 != nil {
		preview := raw
		if len(preview) > 200 {
			preview = preview[:200]
		}
		return nil, fmt.Errorf("inspect: LLM response is not valid JSON (expected {\"suggestions\":[...]} or bare array)\n  parse error: %w\n  response preview: %s", err, preview)
	}
	glog.L().Warn("inspect: LLM returned bare array instead of {\"suggestions\":[...]}")
	return arr, nil
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
