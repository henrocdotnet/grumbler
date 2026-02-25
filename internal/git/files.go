package git

import (
	"fmt"
	"os"
	"path/filepath"
)

// ReadFile reads a file's content from the worktree.
func ReadFile(dir, path string) (string, error) {
	full := filepath.Join(dir, path)
	data, err := os.ReadFile(full)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	return string(data), nil
}

// TopLevel returns the git repository root directory.
func TopLevel(dir string) (string, error) {
	out, err := gitCmd(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return out, nil
}

// CurrentBranch returns the current branch name.
func CurrentBranch(dir string) (string, error) {
	return gitCmd(dir, "rev-parse", "--abbrev-ref", "HEAD")
}
