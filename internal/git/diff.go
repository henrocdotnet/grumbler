package git

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	glog "github.com/henrocdotnet/grumbler/internal/log"
	"github.com/henrocdotnet/grumbler/internal/model"
)

// DiffMode controls which changes GetDiff captures.
type DiffMode int

const (
	DiffStaged DiffMode = iota // git diff --cached (default)
	DiffAll                    // git diff HEAD (staged + unstaged)
	DiffBranch                 // git diff base...HEAD (branch commits)
)

// DefaultContextLines is the diff context width used when none is configured.
// It is wider than git's own default of 3: a wider window is the cheap way to
// give the model surrounding-code context without embedding whole files.
const DefaultContextLines = 15

// GetDiff returns a unified diff for the given mode, with contextLines lines of
// context around each hunk (<= 0 selects DefaultContextLines).
func GetDiff(dir, base string, mode DiffMode, contextLines int) (string, error) {
	if contextLines <= 0 {
		contextLines = DefaultContextLines
	}
	glog.L().Debug("GetDiff", "dir", dir, "base", base, "mode", mode, "contextLines", contextLines)

	unified := fmt.Sprintf("--unified=%d", contextLines)

	var args []string
	switch mode {
	case DiffAll:
		args = []string{"diff", "HEAD", unified}
	case DiffBranch:
		base = resolveRemoteRef(dir, base)
		args = []string{"diff", base + "...HEAD", unified}
	default: // DiffStaged
		args = []string{"diff", "--cached", unified}
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil && mode == DiffBranch {
		glog.L().Debug("GetDiff three-dot failed, trying two-dot")
		args[1] = base + "..HEAD"
		cmd = exec.Command("git", args...)
		cmd.Dir = dir
		out, err = cmd.Output()
	}
	if err != nil {
		return "", fmt.Errorf("git diff %s: %w%s", strings.Join(args, " "), err, gitStderr(err))
	}
	glog.L().Debug("GetDiff done", "outputLen", len(out))
	return string(out), nil
}

// ParseDiff splits a unified diff into per-file FileChange structs.
func ParseDiff(raw string) []model.FileChange {
	var files []model.FileChange
	chunks := splitOnPrefix(raw, "diff --git ")
	for _, chunk := range chunks {
		fc := parseOneFile(chunk)
		if fc.Path != "" {
			files = append(files, fc)
		}
	}
	return files
}

func parseOneFile(chunk string) model.FileChange {
	lines := strings.Split(chunk, "\n")
	fc := model.FileChange{}

	for _, line := range lines {
		applyHeaderLine(&fc, line)
	}

	if fc.Status == "" {
		fc.Status = model.FileModified
	}

	fc.Diff = chunk
	fc.Language = model.LanguageFromPath(fc.Path)
	fc.AddedLines, fc.DeletedLines = countChangedLines(lines)

	return fc
}

// applyHeaderLine updates path and status from one diff header line.
func applyHeaderLine(fc *model.FileChange, line string) {
	switch {
	case strings.HasPrefix(line, "diff --git "):
		parts := strings.Fields(line)
		if len(parts) >= 4 {
			fc.Path = strings.TrimPrefix(parts[3], "b/")
		}
	case strings.HasPrefix(line, "new file"):
		fc.Status = model.FileAdded
	case strings.HasPrefix(line, "deleted file"):
		fc.Status = model.FileDeleted
	case strings.HasPrefix(line, "rename from "):
		fc.OldPath = strings.TrimPrefix(line, "rename from ")
		fc.Status = model.FileRenamed
	case strings.HasPrefix(line, "--- a/") && fc.Status == "":
		fc.Status = model.FileModified
	}
}

// countChangedLines counts added and deleted lines, excluding file headers.
func countChangedLines(lines []string) (added, deleted int) {
	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			added++
		}
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			deleted++
		}
	}
	return added, deleted
}

// resolveRemoteRef returns "origin/<ref>" if that remote ref exists,
// otherwise falls back to the local ref.
func resolveRemoteRef(dir, ref string) string {
	// Revision expressions are relative to the exact ref the caller supplied.
	// Prefixing HEAD^ with origin/ silently selects the remote HEAD's parent.
	if ref == "HEAD" || strings.ContainsAny(ref, "^~:@{}") {
		return ref
	}
	// Already qualified (e.g. "origin/main" or "refs/heads/main").
	if isQualifiedRef(dir, ref) {
		return ref
	}
	remote := "origin/" + ref
	cmd := exec.Command("git", "rev-parse", "--verify", "--quiet", remote)
	cmd.Dir = dir
	if err := cmd.Run(); err == nil {
		glog.L().Debug("resolveRemoteRef: using remote ref", "ref", remote)
		return remote
	}
	glog.L().Debug("resolveRemoteRef: falling back to local ref", "ref", ref)
	return ref
}

func isQualifiedRef(dir, ref string) bool {
	if strings.HasPrefix(ref, "refs/") {
		return true
	}
	cmd := exec.Command("git", "remote")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	for _, remote := range strings.Fields(string(out)) {
		if strings.HasPrefix(ref, remote+"/") {
			return true
		}
	}
	return false
}

func gitStderr(err error) string {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || len(exitErr.Stderr) == 0 {
		return ""
	}
	return ": " + strings.TrimSpace(string(exitErr.Stderr))
}

func splitOnPrefix(s, prefix string) []string {
	var result []string
	remaining := s
	for {
		idx := strings.Index(remaining, prefix)
		if idx == -1 {
			break
		}
		// Find the next occurrence after this one
		next := strings.Index(remaining[idx+len(prefix):], prefix)
		if next == -1 {
			result = append(result, remaining[idx:])
			break
		}
		result = append(result, remaining[idx:idx+len(prefix)+next])
		remaining = remaining[idx+len(prefix)+next:]
	}
	return result
}

// MergeBase returns the merge-base commit between two refs.
func MergeBase(dir, ref1, ref2 string) (string, error) {
	cmd := exec.Command("git", "merge-base", ref1, ref2)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git merge-base: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
