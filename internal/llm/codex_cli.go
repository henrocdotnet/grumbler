package llm

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	glog "github.com/henrocdotnet/grumbler/internal/log"
)

// CodexCLI shells out to `codex exec` for LLM calls without API keys.
type CodexCLI struct {
	model string
}

func NewCodexCLI() *CodexCLI { return &CodexCLI{} }

func NewCodexCLIWithModel(model string) *CodexCLI { return &CodexCLI{model: model} }

func (c *CodexCLI) Complete(ctx context.Context, msgs []Message, opts CompletionOpts) (string, error) {
	prompt := buildCodexPrompt(msgs)
	args := codexCLIArgs(c.model)

	glog.L().Debug("codex-cli call", "model", c.model, "stdinLen", len(prompt))
	start := time.Now()

	cmd := exec.CommandContext(ctx, "codex", args...)
	cmd.Env = codexCLIEnv()
	cmd.Stdin = strings.NewReader(prompt)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		glog.L().Error("codex-cli error", "err", err, "stderr", stderr.String())
		return "", fmt.Errorf("codex cli: %w: %s", err, stderr.String())
	}

	result := strings.TrimSpace(stdout.String())
	glog.L().Debug("codex-cli done", "elapsed", time.Since(start), "stdoutLen", stdout.Len(), "stderrLen", stderr.Len(), "resultLen", len(result))
	return result, nil
}

func (c *CodexCLI) Name() string { return "codex-cli" }

func codexCLIArgs(model string) []string {
	args := []string{
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
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	return append(args, "-")
}

func codexCLIEnv() []string {
	env := os.Environ()
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		env = append(env, "CODEX_HOME="+filepath.Join(home, ".codex-empty"))
	}
	return env
}

func buildCodexPrompt(msgs []Message) string {
	var parts []string
	for _, m := range msgs {
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		switch m.Role {
		case "system":
			parts = append(parts, "[System Instructions]\n"+content)
		case "assistant":
			parts = append(parts, "[Assistant Context]\n"+content)
		default:
			parts = append(parts, "[User]\n"+content)
		}
	}
	return strings.Join(parts, "\n\n")
}
