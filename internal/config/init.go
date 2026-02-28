package config

import (
	"fmt"
	"os"
	"path/filepath"
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
	return writeIfAbsent(filepath.Join(base, "config.yaml"), DefaultGlobalConfigYAML)
}

// writeIfAbsent writes content to path only if the file does not already exist.
func writeIfAbsent(path, content string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
