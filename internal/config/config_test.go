package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()
	if cfg.LLM.Provider != "claude-cli" {
		t.Errorf("default provider: got %q, want claude-cli", cfg.LLM.Provider)
	}
	if cfg.Review.BaseBranch != "main" {
		t.Errorf("default base branch: got %q, want main", cfg.Review.BaseBranch)
	}
	if cfg.Review.Concurrency != 5 {
		t.Errorf("default concurrency: got %d, want 5", cfg.Review.Concurrency)
	}
}

func TestLoadMissing(t *testing.T) {
	cfg, err := Load("/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	// Should return defaults
	if cfg.LLM.Provider != "claude-cli" {
		t.Errorf("expected defaults when no config file")
	}
}

func TestLoadWithEnvExpansion(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, Dir)
	os.MkdirAll(cfgDir, 0755)

	os.Setenv("TEST_GRUMBLER_KEY", "sk-test-123")
	defer os.Unsetenv("TEST_GRUMBLER_KEY")

	content := `llm:
  provider: openai
  apiKey: ${TEST_GRUMBLER_KEY}
`
	os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(content), 0644)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.APIKey != "sk-test-123" {
		t.Errorf("env expansion failed: got %q", cfg.LLM.APIKey)
	}
	if cfg.LLM.Provider != "openai" {
		t.Errorf("provider: got %q, want openai", cfg.LLM.Provider)
	}
}

func TestGlobalLocalCascade(t *testing.T) {
	// Set up a fake global dir
	globalDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", globalDir)

	// Global sets provider + apiKey
	gDir := filepath.Join(globalDir, "grumbler")
	os.MkdirAll(gDir, 0755)
	os.WriteFile(filepath.Join(gDir, "config.yaml"), []byte(`llm:
  provider: openai
  apiKey: global-key
  model: gpt-4o
`), 0644)

	// Project-local overrides model only
	projDir := t.TempDir()
	lDir := filepath.Join(projDir, Dir)
	os.MkdirAll(lDir, 0755)
	os.WriteFile(filepath.Join(lDir, "config.yaml"), []byte(`llm:
  model: gpt-4o-mini
review:
  baseBranch: develop
`), 0644)

	cfg, err := Load(projDir)
	if err != nil {
		t.Fatal(err)
	}

	// Provider and apiKey from global
	if cfg.LLM.Provider != "openai" {
		t.Errorf("provider: got %q, want openai (from global)", cfg.LLM.Provider)
	}
	if cfg.LLM.APIKey != "global-key" {
		t.Errorf("apiKey: got %q, want global-key", cfg.LLM.APIKey)
	}
	// Model overridden by project-local
	if cfg.LLM.Model != "gpt-4o-mini" {
		t.Errorf("model: got %q, want gpt-4o-mini (from local)", cfg.LLM.Model)
	}
	// BaseBranch from project-local
	if cfg.Review.BaseBranch != "develop" {
		t.Errorf("baseBranch: got %q, want develop", cfg.Review.BaseBranch)
	}
	// Concurrency from defaults (neither layer set it)
	if cfg.Review.Concurrency != 5 {
		t.Errorf("concurrency: got %d, want 5 (from defaults)", cfg.Review.Concurrency)
	}
}

func TestOverridesApply(t *testing.T) {
	cfg := Defaults()
	o := &Overrides{
		Provider:   "google",
		BaseBranch: "develop",
		Format:     "json",
	}
	o.Apply(cfg)

	if cfg.LLM.Provider != "google" {
		t.Errorf("provider override: got %q", cfg.LLM.Provider)
	}
	if cfg.Review.BaseBranch != "develop" {
		t.Errorf("base branch override: got %q", cfg.Review.BaseBranch)
	}
	if cfg.Output.Format != "json" {
		t.Errorf("format override: got %q", cfg.Output.Format)
	}
}
