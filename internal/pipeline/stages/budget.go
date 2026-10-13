package stages

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/pkoukk/tiktoken-go"

	"github.com/henrocdotnet/grumbler/internal/model"
)

// defaultMaxContextTokens bounds the whole-diff prompts built by the compliance
// and cross-file passes. Unbounded, these concatenate every changed file and
// overflow the model's context window on a large branch — the call then burns
// full latency and returns nothing useful.
const defaultMaxContextTokens = 80000

// signatureRe matches changed lines that alter a cross-file contract: exported
// or top-level declarations whose callers live in other files. Files carrying
// these are the ones compliance and crossfile actually need to see, so they are
// kept first when the budget forces a truncation.
var signatureRe = regexp.MustCompile(`(?m)^[+-]\s*(?:export\s+|public\s+|pub\s+)?(?:async\s+)?(?:func|function|def|class|interface|type|struct|enum|const|var|let)\b`)

// signatureScore counts contract-level changes in a patch.
func signatureScore(f model.FileChange) int {
	return len(signatureRe.FindAllStringIndex(f.Patch, -1))
}

// countTokens returns a tiktoken count, falling back to the char/4 heuristic
// used elsewhere in this package.
func countTokens(s string) int {
	enc, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		return len(s) / 4
	}
	return len(enc.Encode(s, nil, nil))
}

// budgetedDiff concatenates per-file patches under a token budget.
//
// Files with signature-level changes are emitted first so that a truncated
// prompt keeps the material most relevant to cross-file analysis. When files
// are dropped, the returned text carries an explicit note saying so — the model
// must not infer that the omitted files were unchanged.
//
// maxTokens <= 0 disables the budget. Returns the text, how many files it
// covers, and the total number of candidates.
func budgetedDiff(files []model.FileChange, maxTokens int, render func(model.FileChange) string) (string, int, int) {
	if len(files) == 0 {
		return "", 0, 0
	}

	rendered := make([]string, len(files))
	costs := make([]int, len(files))
	totalCost := 0
	for i, f := range files {
		rendered[i] = render(f)
		if maxTokens > 0 {
			costs[i] = countTokens(rendered[i])
			totalCost += costs[i]
		}
	}

	// Everything fits: keep the caller's original file order untouched.
	if maxTokens <= 0 || totalCost <= maxTokens {
		var b strings.Builder
		for _, chunk := range rendered {
			b.WriteString(chunk)
		}
		return b.String(), len(files), len(files)
	}

	// Over budget: prefer files whose changes alter a cross-file contract.
	ordered := make([]int, len(files))
	for i := range ordered {
		ordered[i] = i
	}
	sort.SliceStable(ordered, func(a, b int) bool {
		return signatureScore(files[ordered[a]]) > signatureScore(files[ordered[b]])
	})

	var b strings.Builder
	used := 0
	included := 0

	for _, idx := range ordered {
		// Always admit the first file, or a single oversized file would yield
		// an empty prompt.
		if included > 0 && used+costs[idx] > maxTokens {
			continue
		}
		used += costs[idx]
		b.WriteString(rendered[idx])
		included++
	}

	if included < len(files) {
		b.WriteString(fmt.Sprintf(
			"\n[NOTE: %d of %d changed files shown; %d omitted to fit the context budget. "+
				"Omitted files were still modified — do not assume they are unchanged.]\n",
			included, len(files), len(files)-included))
	}

	return b.String(), included, len(files)
}

// contextBudget resolves the configured whole-diff token budget.
func contextBudget(n int) int {
	if n <= 0 {
		return defaultMaxContextTokens
	}
	return n
}
