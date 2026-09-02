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

	"github.com/henrocdotnet/grumbler/internal/config"
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
	env, err := codexCLIEnv()
	if err != nil {
		return "", err
	}

	glog.L().Debug("codex-cli call", "model", c.model, "stdinLen", len(prompt))
	start := time.Now()

	cmd := exec.CommandContext(ctx, "codex", args...)
	cmd.Env = env
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

func (c *CodexCLI) ValidateConfig() error {
	_, err := codexCLIHome()
	return err
}

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

func codexCLIEnv() ([]string, error) {
	codexHome, err := codexCLIHome()
	if err != nil {
		return nil, err
	}

	env := os.Environ()
	env = setEnv(env, "CODEX_HOME", codexHome)
	return env, nil
}

func codexCLIHome() (string, error) {
	if codexHome := strings.TrimSpace(os.Getenv("GRUMBLER_CODEX_HOME")); codexHome != "" {
		return codexHome, nil
	}

	sourceHome, err := sourceCodexHome()
	if err != nil {
		return "", err
	}
	return prepareManagedCodexHome(sourceHome)
}

func sourceCodexHome() (string, error) {
	if codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME")); codexHome != "" {
		return filepath.Abs(codexHome)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating Codex home: %w", err)
	}
	return filepath.Join(home, ".codex"), nil
}

func prepareManagedCodexHome(sourceHome string) (string, error) {
	managedHome := filepath.Join(config.GlobalDir(), ".codex")
	if managedHome == ".codex" {
		return "", fmt.Errorf("locating Grumbler Codex home: user config directory is unavailable")
	}

	sourceAuth := filepath.Join(sourceHome, "auth.json")
	sourceAuth, err := filepath.Abs(sourceAuth)
	if err != nil {
		return "", fmt.Errorf("resolving Codex auth path: %w", err)
	}
	if err := validateSourceAuth(sourceAuth); err != nil {
		return "", err
	}

	managedAuth := filepath.Join(managedHome, "auth.json")
	if err := ensureAuthSymlink(managedAuth, sourceAuth); err != nil {
		return "", err
	}
	return managedHome, nil
}

func validateSourceAuth(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("codex-cli auth unavailable: %s does not exist; run `codex login` first", path)
		}
		return fmt.Errorf("checking Codex auth %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("codex-cli auth invalid: %s is not a regular file", path)
	}
	if info.Size() == 0 {
		return fmt.Errorf("codex-cli auth invalid: %s is empty; run `codex login` again", path)
	}
	return nil
}

func ensureAuthSymlink(linkPath, targetPath string) error {
	info, err := os.Lstat(linkPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("codex-cli auth config missing: %s was not found; rerun `grumbler init --global` to create the Codex auth symlink", linkPath)
		}
		return fmt.Errorf("checking Grumbler Codex auth %s: %w", linkPath, err)
	}

	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("codex-cli auth config invalid: %s exists but is not a symlink; rerun `grumbler init --global` or set GRUMBLER_CODEX_HOME explicitly", linkPath)
	}

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
	if filepath.Clean(target) != filepath.Clean(targetPath) {
		return fmt.Errorf("codex-cli auth config invalid: %s points to %s, want %s; rerun `grumbler init --global` or set GRUMBLER_CODEX_HOME explicitly", linkPath, target, targetPath)
	}
	return nil
}

func setEnv(env []string, key, value string) []string {
	prefix := key + "="
	next := prefix + value
	for i, item := range env {
		if strings.HasPrefix(item, prefix) {
			env[i] = next
			return env
		}
	}
	return append(env, next)
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
