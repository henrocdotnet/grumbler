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

func TestCodexCLIEnvUsesEmptyCodexHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	want := "CODEX_HOME=" + filepath.Join(home, ".codex-empty")
	got := codexCLIEnv()
	if len(got) == 0 || got[len(got)-1] != want {
		t.Fatalf("last env:\n got: %#v\nwant: %q", got, want)
	}
}

func TestNewProviderCodexCLI(t *testing.T) {
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
