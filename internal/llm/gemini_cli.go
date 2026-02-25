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
	model string
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

	glog.L().Debug("gemini-cli call", "model", g.model, "stdinLen", len(combined))
	start := time.Now()

	cmd := exec.CommandContext(ctx, "gemini", args...)
	cmd.Stdin = strings.NewReader(combined)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		glog.L().Error("gemini-cli error", "err", err, "stderr", stderr.String())
		return "", fmt.Errorf("gemini cli: %w: %s", err, stderr.String())
	}

	result := strings.TrimSpace(stdout.String())
	glog.L().Debug("gemini-cli done", "elapsed", time.Since(start), "stdoutLen", stdout.Len(), "stderrLen", stderr.Len(), "resultLen", len(result))
	return result, nil
}

func (g *GeminiCLI) Name() string { return "gemini-cli" }
