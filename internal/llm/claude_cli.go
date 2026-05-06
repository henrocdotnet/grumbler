package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	glog "github.com/henrocdotnet/grumbler/internal/log"
)

// ClaudeCLI shells out to `claude -p` for LLM calls without API keys.
type ClaudeCLI struct {
	model string // optional; empty = CLI default

	// last call usage (populated from JSON output)
	lastUsage TokenUsage
}

// cliEvent represents a single event in the Claude CLI JSON output array.
type cliEvent struct {
	Type  string          `json:"type"`
	Usage json.RawMessage `json:"usage,omitempty"`

	// result event fields
	Result string `json:"result,omitempty"`
}

// cliUsage captures token counts from the result event.
type cliUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
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

	args = append(args, "--output-format", "json")

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

	result, usage, err := parseCliJSON(stdout.Bytes())
	if err != nil {
		return "", fmt.Errorf("claude cli parse: %w", err)
	}

	c.lastUsage = TokenUsage{
		PromptTokens:        usage.InputTokens,
		CompletionTokens:    usage.OutputTokens,
		CacheCreationTokens: usage.CacheCreationInputTokens,
		CacheReadTokens:     usage.CacheReadInputTokens,
	}
	glog.L().Debug("claude-cli result", "resultLen", len(result),
		"inputTokens", usage.InputTokens, "outputTokens", usage.OutputTokens,
		"cacheCreation", usage.CacheCreationInputTokens, "cacheRead", usage.CacheReadInputTokens)
	return result, nil
}

// LastTokenUsage returns token counts from the most recent call.
func (c *ClaudeCLI) LastTokenUsage() TokenUsage { return c.lastUsage }

func (c *ClaudeCLI) Name() string { return "claude-cli" }

// parseCliJSON extracts the result text and usage from Claude CLI JSON output.
// Handles both JSON array and streaming JSONL formats.
func parseCliJSON(data []byte) (string, cliUsage, error) {
	trimmed := bytes.TrimSpace(data)

	var events []cliEvent

	// Try JSON array first.
	if len(trimmed) > 0 && trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &events); err != nil {
			return "", cliUsage{}, fmt.Errorf("unmarshal array: %w", err)
		}
	} else {
		// JSONL: one JSON object per line.
		dec := json.NewDecoder(bytes.NewReader(trimmed))
		for dec.More() {
			var ev cliEvent
			if err := dec.Decode(&ev); err != nil {
				return "", cliUsage{}, fmt.Errorf("decode jsonl event: %w", err)
			}
			events = append(events, ev)
		}
	}

	for _, ev := range events {
		if ev.Type != "result" {
			continue
		}
		var u cliUsage
		if len(ev.Usage) > 0 {
			_ = json.Unmarshal(ev.Usage, &u)
		}
		return strings.TrimSpace(ev.Result), u, nil
	}

	return "", cliUsage{}, fmt.Errorf("no result event in claude cli output")
}
