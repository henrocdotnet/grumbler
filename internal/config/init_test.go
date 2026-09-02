package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitGlobalCodexAuthCreatesSymlink(t *testing.T) {
	sourceHome := t.TempDir()
	sourceAuth := filepath.Join(sourceHome, "auth.json")
	if err := os.WriteFile(sourceAuth, []byte(`{"auth_mode":"chatgpt"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", sourceHome)

	base := t.TempDir()
	if err := InitGlobalCodexAuth(base); err != nil {
		t.Fatal(err)
	}

	linkPath := filepath.Join(base, ".codex", "auth.json")
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("auth link: %v", err)
	}
	if target != sourceAuth {
		t.Fatalf("auth link target: got %q, want %q", target, sourceAuth)
	}
}

func TestInitGlobalCodexAuthRepairsRegularAuthFile(t *testing.T) {
	sourceHome := t.TempDir()
	sourceAuth := filepath.Join(sourceHome, "auth.json")
	if err := os.WriteFile(sourceAuth, []byte(`{"auth_mode":"chatgpt"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", sourceHome)

	base := t.TempDir()
	codexDir := filepath.Join(base, ".codex")
	if err := os.MkdirAll(codexDir, 0o700); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(codexDir, "auth.json")
	if err := os.WriteFile(linkPath, []byte(`{"stale":true}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := InitGlobalCodexAuth(base); err != nil {
		t.Fatal(err)
	}

	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("auth link: %v", err)
	}
	if target != sourceAuth {
		t.Fatalf("auth link target: got %q, want %q", target, sourceAuth)
	}

	matches, err := filepath.Glob(filepath.Join(codexDir, "auth.json.bak-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("backup count: got %d, want 1", len(matches))
	}
}
