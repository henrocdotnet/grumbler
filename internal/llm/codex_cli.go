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
	model   string
	effort  string // model_reasoning_effort; "" = provider default
	sandbox string // sandbox policy; "" = defaultCodexSandbox
	timeout time.Duration
}

// defaultCodexSandbox is read-only: the prompt already embeds every diff and
// file the model needs, so a writable workspace only buys agentic shell turns
// that cost wall clock. See openspec add-codex-cli-support design Decision 4.
const defaultCodexSandbox = "read-only"

func NewCodexCLI() *CodexCLI { return &CodexCLI{} }

func NewCodexCLIWithModel(model string) *CodexCLI { return &CodexCLI{model: model} }

// NewCodexCLIWithOptions builds a provider with model, reasoning effort and
// sandbox policy. Empty strings select the defaults.
func NewCodexCLIWithOptions(model, effort, sandbox string) *CodexCLI {
	return &CodexCLI{model: model, effort: effort, sandbox: sandbox}
}

func (c *CodexCLI) Complete(ctx context.Context, msgs []Message, opts CompletionOpts) (string, error) {
	prompt := buildCodexPrompt(msgs)
	env, err := codexCLIEnv()
	if err != nil {
		return "", err
	}

	// Ask codex to write just the agent's final message to a file, so the
	// result doesn't depend on scraping the full stdout transcript.
	lastMsg, err := os.CreateTemp("", "grumbler-codex-*.txt")
	if err != nil {
		return "", fmt.Errorf("codex cli: creating output file: %w", err)
	}
	lastMsgPath := lastMsg.Name()
	_ = lastMsg.Close()
	defer func() { _ = os.Remove(lastMsgPath) }()

	args := codexCLIArgs(c.model, c.effort, c.sandbox, lastMsgPath)

	// Nothing else in this codebase bounds a provider call; without a deadline
	// a wedged subprocess hangs the whole run forever.
	timeout := callTimeout(opts, c.timeout)
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	glog.L().Debug("codex-cli call", "model", c.model, "effort", c.effort,
		"sandbox", c.sandbox, "stdinLen", len(prompt), "timeout", timeout)
	start := time.Now()

	cmd := exec.CommandContext(ctx, "codex", args...)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(prompt)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// With a bytes.Buffer, exec uses an os.Pipe plus a copier goroutine, so a
	// killed child that left a grandchild holding the pipe still hangs Run().
	cmd.WaitDelay = 10 * time.Second

	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			glog.L().Error("codex-cli timeout", "elapsed", time.Since(start), "err", ctxErr)
			return "", fmt.Errorf("codex cli: %w after %s", ctxErr, time.Since(start).Round(time.Second))
		}
		glog.L().Error("codex-cli error", "err", err, "stderr", stderr.String())
		return "", fmt.Errorf("codex cli: %w: %s", err, stderr.String())
	}

	result := strings.TrimSpace(stdout.String())
	if data, rerr := os.ReadFile(lastMsgPath); rerr == nil {
		if last := strings.TrimSpace(string(data)); last != "" {
			result = last
		}
	}

	glog.L().Debug("codex-cli done", "elapsed", time.Since(start), "stdoutLen", stdout.Len(), "stderrLen", stderr.Len(), "resultLen", len(result))
	return result, nil
}

func (c *CodexCLI) Name() string { return "codex-cli" }

func (c *CodexCLI) ValidateConfig() error {
	_, err := codexCLIHome()
	return err
}

// codexCLIArgs builds the `codex` argument vector. Note the ordering: only
// --ask-for-approval is a global flag; everything else is scoped to the `exec`
// subcommand and must follow it.
func codexCLIArgs(model, effort, sandbox, lastMessagePath string) []string {
	if sandbox == "" {
		sandbox = defaultCodexSandbox
	}
	args := []string{
		"--ask-for-approval", "never",
		"exec",
		"--ignore-user-config",
		"--sandbox", sandbox,
		// Every call is a fresh, self-contained completion; persisting session
		// rollouts only grows the managed CODEX_HOME and slows each spawn.
		"--ephemeral",
		"--skip-git-repo-check",
		"--disable", "shell_snapshot",
		"--disable", "workspace_dependencies",
		"-c", "include_permissions_instructions=false",
		"-c", "include_environment_context=false",
		"-c", "skills.bundled.enabled=false",
		"--color", "never",
	}
	if lastMessagePath != "" {
		args = append(args, "--output-last-message", lastMessagePath)
	}
	if effort != "" {
		// --ignore-user-config discards the user's config.toml, so effort has
		// to be passed explicitly or the model uses its built-in default.
		args = append(args, "-c", "model_reasoning_effort="+effort)
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

// codexHomeDirName is the Codex home directory name under a parent dir.
const codexHomeDirName = ".codex"

func sourceCodexHome() (string, error) {
	if codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME")); codexHome != "" {
		return filepath.Abs(codexHome)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating Codex home: %w", err)
	}
	return filepath.Join(home, codexHomeDirName), nil
}

func prepareManagedCodexHome(sourceHome string) (string, error) {
	managedHome := filepath.Join(config.GlobalDir(), codexHomeDirName)
	if managedHome == codexHomeDirName {
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
