package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/henrocdotnet/grumbler/internal/config"
)

// Message represents a chat message.
type Message struct {
	Role    string // "system", "user", "assistant"
	Content string
}

// CompletionOpts controls LLM generation parameters.
type CompletionOpts struct {
	Temperature float64
	MaxTokens   int
	JSONMode    bool
}

// Provider is the interface all LLM backends implement.
type Provider interface {
	Complete(ctx context.Context, msgs []Message, opts CompletionOpts) (string, error)
	Name() string
}

// TokenUsage holds token counts from a single call, including cache breakdown.
type TokenUsage struct {
	PromptTokens        int
	CompletionTokens    int
	CacheCreationTokens int
	CacheReadTokens     int
}

// TokenUsageProvider is optionally implemented by providers that report real token usage.
type TokenUsageProvider interface {
	LastTokenUsage() TokenUsage
}

// NewProvider constructs the appropriate provider from config.
func NewProvider(cfg config.LLMConfig) (Provider, error) {
	switch strings.ToLower(cfg.Provider) {
	case "claude-cli":
		return NewClaudeCLIWithModel(cfg.Model), nil
	case "gemini-cli":
		return NewGeminiCLIWithModel(cfg.Model), nil
	case "codex-cli":
		return NewCodexCLIWithModel(cfg.Model), nil
	default:
		return NewLangChainProvider(cfg)
	}
}

// SystemMsg is a convenience constructor.
func SystemMsg(content string) Message {
	return Message{Role: "system", Content: content}
}

// UserMsg is a convenience constructor.
func UserMsg(content string) Message {
	return Message{Role: "user", Content: content}
}

// DefaultOpts returns standard completion options.
func DefaultOpts() CompletionOpts {
	return CompletionOpts{
		Temperature: 0.2,
		MaxTokens:   16384,
		JSONMode:    true,
	}
}

// MustJSON wraps a completion call and returns an error if the result is empty.
func MustJSON(ctx context.Context, p Provider, msgs []Message, opts CompletionOpts) (string, error) {
	resp, err := p.Complete(ctx, msgs, opts)
	if err != nil {
		return "", err
	}
	resp = ExtractJSON(resp)
	if resp == "" {
		return "", fmt.Errorf("provider %s returned empty response", p.Name())
	}
	return resp, nil
}
