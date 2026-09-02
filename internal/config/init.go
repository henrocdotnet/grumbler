package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// InitProject scaffolds the .grumbler directory structure under base:
// config.yaml, rules.yaml, prompts/, and .gitignore.
// Existing files are not overwritten.
func InitProject(base string) error {
	for _, d := range []string{base, filepath.Join(base, "prompts")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", d, err)
		}
	}
	writes := []struct{ path, content string }{
		{filepath.Join(base, "config.yaml"), DefaultConfigYAML},
		{filepath.Join(base, "rules.yaml"), DefaultRulesYAML},
		{filepath.Join(base, ".gitignore"), "# Ignore API keys in config\n# config.yaml\n\n# Review reports\nreports/\n"},
	}
	for _, w := range writes {
		if err := writeIfAbsent(w.path, w.content); err != nil {
			return err
		}
	}
	return nil
}

// InitGlobal scaffolds the global config directory under base (config.yaml + prompts/ only).
func InitGlobal(base string) error {
	for _, d := range []string{base, filepath.Join(base, "prompts")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", d, err)
		}
	}
	if err := writeIfAbsent(filepath.Join(base, "config.yaml"), DefaultGlobalConfigYAML); err != nil {
		return err
	}
	return InitGlobalCodexAuth(base)
}

// InitGlobalCodexAuth prepares the isolated Codex home used by the codex-cli provider.
func InitGlobalCodexAuth(base string) error {
	sourceAuth, err := sourceCodexAuthPath()
	if err != nil {
		return err
	}
	if err := validateCodexAuthSource(sourceAuth); err != nil {
		return err
	}

	codexDir := filepath.Join(base, ".codex")
	if err := os.MkdirAll(codexDir, 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", codexDir, err)
	}
	return createOrRepairAuthSymlink(filepath.Join(codexDir, "auth.json"), sourceAuth)
}

func sourceCodexAuthPath() (string, error) {
	if codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME")); codexHome != "" {
		return filepath.Abs(filepath.Join(codexHome, "auth.json"))
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating Codex home: %w", err)
	}
	return filepath.Join(home, ".codex", "auth.json"), nil
}

func validateCodexAuthSource(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("codex auth unavailable: %s does not exist; run `codex login` and rerun `grumbler init --global`", path)
		}
		return fmt.Errorf("checking Codex auth %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("codex auth invalid: %s is not a regular file", path)
	}
	if info.Size() == 0 {
		return fmt.Errorf("codex auth invalid: %s is empty; run `codex login` and rerun `grumbler init --global`", path)
	}
	return nil
}

func createOrRepairAuthSymlink(linkPath, targetPath string) error {
	targetPath, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("resolving Codex auth path: %w", err)
	}

	info, err := os.Lstat(linkPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("checking Grumbler Codex auth %s: %w", linkPath, err)
		}
		return os.Symlink(targetPath, linkPath)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(linkPath)
		if err != nil {
			return fmt.Errorf("reading Grumbler Codex auth symlink %s: %w", linkPath, err)
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(linkPath), target)
		}
		target, err = filepath.Abs(target)
		if err != nil {
			return fmt.Errorf("resolving Grumbler Codex auth symlink %s: %w", linkPath, err)
		}
		if filepath.Clean(target) == filepath.Clean(targetPath) {
			return nil
		}
		if err := os.Remove(linkPath); err != nil {
			return fmt.Errorf("removing stale Grumbler Codex auth symlink %s: %w", linkPath, err)
		}
		return os.Symlink(targetPath, linkPath)
	}

	backupPath := linkPath + ".bak-" + time.Now().Format("20060102150405")
	if err := os.Rename(linkPath, backupPath); err != nil {
		return fmt.Errorf("backing up stale Grumbler Codex auth %s: %w", linkPath, err)
	}
	if err := os.Symlink(targetPath, linkPath); err != nil {
		return fmt.Errorf("creating Grumbler Codex auth symlink %s -> %s: %w", linkPath, targetPath, err)
	}
	return nil
}

// writeIfAbsent writes content to path only if the file does not already exist.
func writeIfAbsent(path, content string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
