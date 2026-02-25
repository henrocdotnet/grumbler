package llm

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	glog "github.com/henrocdotnet/grumbler/internal/log"
)

// ClaudeCLI shells out to `claude -p` for LLM calls without API keys.
type ClaudeCLI struct {
	model string // optional; empty = CLI default
}

func NewClaudeCLI() *ClaudeCLI { return &ClaudeCLI{} }

func NewClaudeCLIWithModel(model string) *ClaudeCLI { return &ClaudeCLI{model: model} }

func (c *ClaudeCLI) Complete(ctx context.Context, msgs []Message, opts CompletionOpts) (string, error) {
	var systemPrompt, userPrompt string
	for _, m := range msgs {
		switch m.Role {
		case "system":
			systemPrompt = m.Content
		case "user":
			userPrompt += m.Content + "\n"
		}
	}

	args := []string{"-p"}

	if c.model != "" {
		args = append(args, "--model", c.model)
	}

	if systemPrompt != "" {
		args = append(args, "--system-prompt", systemPrompt)
	}

	// Always use text output — JSON wrapping is pointless for CLI;
	// the system prompt already instructs the model to return JSON.
	args = append(args, "--output-format", "text")

	glog.L().Debug("claude-cli call", "stdinLen", len(userPrompt))
	start := time.Now()

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Stdin = strings.NewReader(userPrompt)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		glog.L().Error("claude-cli error", "err", err, "stderr", stderr.String())
		return "", fmt.Errorf("claude cli: %w: %s", err, stderr.String())
	}

	glog.L().Debug("claude-cli done", "elapsed", time.Since(start), "stdoutLen", stdout.Len(), "stderrLen", stderr.Len())

	result := strings.TrimSpace(stdout.String())
	glog.L().Debug("claude-cli result", "resultLen", len(result))
	return result, nil
}

func (c *ClaudeCLI) Name() string { return "claude-cli" }
