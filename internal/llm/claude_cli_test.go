package llm

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/henrocdotnet/grumbler/internal/config"
)

// Exercise the factory-to-subprocess path without a live model call: omitting
// effort must preserve CLI defaults, and configured effort must reach argv.
func TestClaudeCLIForwardsConfiguredEffort(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake CLI requires a POSIX shell")
	}
	dir := t.TempDir()
	script := `#!/bin/sh
printf '%s\n' "$@" > "$GRUMBLER_TEST_CLAUDE_ARGS"
cat > "$GRUMBLER_TEST_CLAUDE_INPUT"
printf '%s\n' '{"type":"result","result":"{\"suggestions\":[]}","usage":{"input_tokens":12,"output_tokens":4}}'
`
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	argsPath, inputPath := filepath.Join(dir, "args"), filepath.Join(dir, "input")
	t.Setenv("GRUMBLER_TEST_CLAUDE_ARGS", argsPath)
	t.Setenv("GRUMBLER_TEST_CLAUDE_INPUT", inputPath)
	for _, effort := range []string{"", "low", "medium", "high", "xhigh", "max"} {
		name := effort
		if name == "" {
			name = "provider default"
		}
		t.Run(name, func(t *testing.T) {
			provider, err := NewProvider(config.LLMConfig{
				Provider: "claude-cli", Model: "test-model", ReasoningEffort: effort, CallTimeout: "5s",
			})
			if err != nil {
				t.Fatal(err)
			}
			response, err := provider.Complete(context.Background(), []Message{SystemMsg("Use JSON only."), UserMsg("Review code.")}, DefaultOpts())
			if err != nil {
				t.Fatal(err)
			}
			if response != `{"suggestions":[]}` {
				t.Fatalf("unexpected response: %q", response)
			}
			data, err := os.ReadFile(argsPath)
			if err != nil {
				t.Fatal(err)
			}
			got := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
			want := []string{"-p", "--model", "test-model"}
			if effort != "" {
				want = append(want, "--effort", effort)
			}
			want = append(want, "--system-prompt", "Use JSON only.", "--output-format", "json")
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("args:\n got: %q\nwant: %q", got, want)
			}
			input, err := os.ReadFile(inputPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(input) != "Review code.\n" {
				t.Fatalf("unexpected stdin: %q", input)
			}
		})
	}
}
