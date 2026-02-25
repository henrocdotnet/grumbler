package git

import (
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

// GetDiff returns a unified diff for the given mode.
func GetDiff(dir, base string, mode DiffMode) (string, error) {
	glog.L().Debug("GetDiff", "dir", dir, "base", base, "mode", mode)

	var args []string
	switch mode {
	case DiffAll:
		args = []string{"diff", "HEAD", "--unified=3"}
	case DiffBranch:
		args = []string{"diff", base + "...HEAD", "--unified=3"}
	default: // DiffStaged
		args = []string{"diff", "--cached", "--unified=3"}
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
		return "", fmt.Errorf("git diff: %w", err)
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
		case strings.HasPrefix(line, "--- a/"):
			if fc.Status == "" {
				fc.Status = model.FileModified
			}
		}
	}

	if fc.Status == "" {
		fc.Status = model.FileModified
	}

	fc.Diff = chunk
	fc.Language = model.LanguageFromPath(fc.Path)

	// Count added/deleted lines
	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			fc.AddedLines++
		}
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			fc.DeletedLines++
		}
	}

	return fc
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
