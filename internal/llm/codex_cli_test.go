package llm

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/henrocdotnet/grumbler/internal/config"
)

func TestCodexCLIArgsWithoutModel(t *testing.T) {
	got := codexCLIArgs("")
	want := []string{
		"--ask-for-approval", "never",
		"exec",
		"--ignore-user-config",
		"--sandbox", "workspace-write",
		"--disable", "shell_snapshot",
		"--disable", "workspace_dependencies",
		"-c", "include_permissions_instructions=false",
		"-c", "include_environment_context=false",
		"-c", "skills.bundled.enabled=false",
		"--color", "never",
		"-",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestCodexCLIArgsWithModel(t *testing.T) {
	got := codexCLIArgs("gpt-5.5")
	want := []string{
		"--ask-for-approval", "never",
		"exec",
		"--ignore-user-config",
		"--sandbox", "workspace-write",
		"--disable", "shell_snapshot",
		"--disable", "workspace_dependencies",
		"-c", "include_permissions_instructions=false",
		"-c", "include_environment_context=false",
		"-c", "skills.bundled.enabled=false",
		"--color", "never",
		"--model", "gpt-5.5",
		"-",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestBuildCodexPrompt(t *testing.T) {
	got := buildCodexPrompt([]Message{
		SystemMsg("Use JSON only."),
		UserMsg("Review this diff."),
		{Role: "assistant", Content: "Prior context."},
		{Role: "user", Content: "  Follow up.  "},
		{Role: "user", Content: "   "},
	})

	for _, want := range []string{
		"[System Instructions]\nUse JSON only.",
		"[User]\nReview this diff.",
		"[Assistant Context]\nPrior context.",
		"[User]\nFollow up.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}

	if strings.Contains(got, "   ") {
		t.Fatalf("prompt contains untrimmed whitespace:\n%s", got)
	}
}

func TestCodexCLIEnvUsesManagedAuthSymlink(t *testing.T) {
	sourceHome, managedHome := setupManagedCodexHomeTest(t)
	if err := config.InitGlobalCodexAuth(filepath.Dir(managedHome)); err != nil {
		t.Fatal(err)
	}

	got, err := codexCLIEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !containsEnv(got, "CODEX_HOME="+managedHome) {
		t.Fatalf("env missing managed CODEX_HOME: %#v", got)
	}

	linkPath := filepath.Join(managedHome, "auth.json")
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("auth link: %v", err)
	}
	if target != filepath.Join(sourceHome, "auth.json") {
		t.Fatalf("auth link target: got %q, want %q", target, filepath.Join(sourceHome, "auth.json"))
	}
}

func TestCodexCLIEnvRejectsMissingManagedAuthSymlink(t *testing.T) {
	_, managedHome := setupManagedCodexHomeTest(t)

	_, err := codexCLIEnv()
	if err == nil {
		t.Fatal("expected missing auth config error")
	}
	if !strings.Contains(err.Error(), "rerun `grumbler init --global`") {
		t.Fatalf("error: got %q, want init instruction", err)
	}
	if _, statErr := os.Lstat(filepath.Join(managedHome, "auth.json")); !os.IsNotExist(statErr) {
		t.Fatalf("managed auth link should not be created during validation: %v", statErr)
	}
}

func TestCodexCLIEnvRejectsRegularManagedAuthFile(t *testing.T) {
	_, managedHome := setupManagedCodexHomeTest(t)
	if err := os.MkdirAll(managedHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(managedHome, "auth.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := codexCLIEnv()
	if err == nil {
		t.Fatal("expected invalid auth config error")
	}
	if !strings.Contains(err.Error(), "not a symlink") {
		t.Fatalf("error: got %q, want not a symlink", err)
	}
}

func TestCodexCLIEnvRejectsWrongManagedAuthSymlink(t *testing.T) {
	_, managedHome := setupManagedCodexHomeTest(t)
	if err := os.MkdirAll(managedHome, 0o700); err != nil {
		t.Fatal(err)
	}
	wrongTarget := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(wrongTarget, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(wrongTarget, filepath.Join(managedHome, "auth.json")); err != nil {
		t.Fatal(err)
	}

	_, err := codexCLIEnv()
	if err == nil {
		t.Fatal("expected invalid auth config error")
	}
	if !strings.Contains(err.Error(), "points to") {
		t.Fatalf("error: got %q, want points to", err)
	}
}

func TestCodexCLIEnvUsesExplicitGrumblerCodexHome(t *testing.T) {
	t.Setenv("CODEX_HOME", "/tmp/current-codex-home")
	t.Setenv("GRUMBLER_CODEX_HOME", "/tmp/grumbler-codex-home")

	got, err := codexCLIEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !containsEnv(got, "CODEX_HOME=/tmp/grumbler-codex-home") {
		t.Fatalf("env missing overridden CODEX_HOME: %#v", got)
	}
	if containsEnv(got, "CODEX_HOME=/tmp/current-codex-home") {
		t.Fatalf("env kept stale CODEX_HOME: %#v", got)
	}
}

func containsEnv(env []string, want string) bool {
	for _, got := range env {
		if got == want {
			return true
		}
	}
	return false
}

func TestNewProviderCodexCLI(t *testing.T) {
	_, managedHome := setupManagedCodexHomeTest(t)
	if err := config.InitGlobalCodexAuth(filepath.Dir(managedHome)); err != nil {
		t.Fatal(err)
	}

	p, err := NewProvider(config.LLMConfig{Provider: "codex-cli", Model: "gpt-5.5"})
	if err != nil {
		t.Fatal(err)
	}

	codex, ok := p.(*CodexCLI)
	if !ok {
		t.Fatalf("provider type: got %T, want *CodexCLI", p)
	}
	if codex.model != "gpt-5.5" {
		t.Fatalf("model: got %q, want gpt-5.5", codex.model)
	}
	if p.Name() != "codex-cli" {
		t.Fatalf("name: got %q, want codex-cli", p.Name())
	}
}

func setupManagedCodexHomeTest(t *testing.T) (sourceHome string, managedHome string) {
	t.Helper()

	root := t.TempDir()
	sourceHome = filepath.Join(root, "source-codex")
	if err := os.MkdirAll(sourceHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceHome, "auth.json"), []byte(`{"auth_mode":"chatgpt"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	xdgHome := filepath.Join(root, "xdg")
	t.Setenv("CODEX_HOME", sourceHome)
	t.Setenv("XDG_CONFIG_HOME", xdgHome)
	t.Setenv("GRUMBLER_CODEX_HOME", "")

	return sourceHome, filepath.Join(xdgHome, "grumbler", ".codex")
}
