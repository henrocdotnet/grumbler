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

// GeminiCLI shells out to `gemini` for LLM calls without API keys.
type GeminiCLI struct {
	model   string
	timeout time.Duration
}

func NewGeminiCLI() *GeminiCLI { return &GeminiCLI{} }

func NewGeminiCLIWithModel(model string) *GeminiCLI { return &GeminiCLI{model: model} }

func (g *GeminiCLI) Complete(ctx context.Context, msgs []Message, opts CompletionOpts) (string, error) {
	var parts []string
	for _, m := range msgs {
		switch m.Role {
		case "system":
			parts = append(parts, "[System Instructions]\n"+m.Content)
		case "user":
			parts = append(parts, m.Content)
		}
	}
	combined := strings.Join(parts, "\n\n")

	args := []string{"-p", ""}
	if g.model != "" {
		args = append(args, "-m", g.model)
	}

	timeout := callTimeout(opts, g.timeout)
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	glog.L().Debug("gemini-cli call", "model", g.model, "stdinLen", len(combined), "timeout", timeout)
	start := time.Now()

	cmd := exec.CommandContext(ctx, "gemini", args...)
	cmd.Stdin = strings.NewReader(combined)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.WaitDelay = 10 * time.Second

	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			glog.L().Error("gemini-cli timeout", "elapsed", time.Since(start), "err", ctxErr)
			return "", fmt.Errorf("gemini cli: %w after %s", ctxErr, time.Since(start).Round(time.Second))
		}
		glog.L().Error("gemini-cli error", "err", err, "stderr", stderr.String())
		return "", fmt.Errorf("gemini cli: %w: %s", err, stderr.String())
	}

	result := strings.TrimSpace(stdout.String())
	glog.L().Debug("gemini-cli done", "elapsed", time.Since(start), "stdoutLen", stdout.Len(), "stderrLen", stderr.Len(), "resultLen", len(result))
	return result, nil
}

func (g *GeminiCLI) Name() string { return "gemini-cli" }
