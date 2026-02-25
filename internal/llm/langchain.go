package llm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/henrocdotnet/grumbler/internal/config"
	glog "github.com/henrocdotnet/grumbler/internal/log"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/anthropic"
	"github.com/tmc/langchaingo/llms/googleai"
	"github.com/tmc/langchaingo/llms/openai"
)

// LangChainProvider wraps langchaingo for API-key-based multi-provider support.
type LangChainProvider struct {
	llm      llms.Model
	provider string
	model    string
}

// NewLangChainProvider creates a provider backed by langchaingo.
func NewLangChainProvider(cfg config.LLMConfig) (*LangChainProvider, error) {
	var (
		m   llms.Model
		err error
	)

	switch strings.ToLower(cfg.Provider) {
	case "anthropic":
		opts := []anthropic.Option{anthropic.WithModel(cfg.Model)}
		if cfg.APIKey != "" {
			opts = append(opts, anthropic.WithToken(cfg.APIKey))
		}
		m, err = anthropic.New(opts...)
	case "openai":
		opts := []openai.Option{openai.WithModel(cfg.Model)}
		if cfg.APIKey != "" {
			opts = append(opts, openai.WithToken(cfg.APIKey))
		}
		if cfg.BaseURL != "" {
			opts = append(opts, openai.WithBaseURL(cfg.BaseURL))
		}
		m, err = openai.New(opts...)
	case "google":
		opts := []googleai.Option{googleai.WithDefaultModel(cfg.Model)}
		if cfg.APIKey != "" {
			opts = append(opts, googleai.WithAPIKey(cfg.APIKey))
		}
		m, err = googleai.New(context.Background(), opts...)
	default:
		// For other providers, try openai-compatible with base URL
		opts := []openai.Option{openai.WithModel(cfg.Model)}
		if cfg.APIKey != "" {
			opts = append(opts, openai.WithToken(cfg.APIKey))
		}
		if cfg.BaseURL != "" {
			opts = append(opts, openai.WithBaseURL(cfg.BaseURL))
		}
		m, err = openai.New(opts...)
	}

	if err != nil {
		return nil, fmt.Errorf("init %s provider: %w", cfg.Provider, err)
	}

	return &LangChainProvider{llm: m, provider: cfg.Provider, model: cfg.Model}, nil
}

func (p *LangChainProvider) Complete(ctx context.Context, msgs []Message, opts CompletionOpts) (string, error) {
	lcMsgs := make([]llms.MessageContent, 0, len(msgs))
	for _, m := range msgs {
		var role llms.ChatMessageType
		switch m.Role {
		case "system":
			role = llms.ChatMessageTypeSystem
		case "user":
			role = llms.ChatMessageTypeHuman
		case "assistant":
			role = llms.ChatMessageTypeAI
		default:
			role = llms.ChatMessageTypeHuman
		}
		lcMsgs = append(lcMsgs, llms.MessageContent{
			Role:  role,
			Parts: []llms.ContentPart{llms.TextContent{Text: m.Content}},
		})
	}

	callOpts := []llms.CallOption{
		llms.WithTemperature(opts.Temperature),
	}
	if opts.MaxTokens > 0 {
		callOpts = append(callOpts, llms.WithMaxTokens(opts.MaxTokens))
	}
	if opts.JSONMode {
		callOpts = append(callOpts, llms.WithJSONMode())
	}

	glog.L().Debug("langchain call", "provider", p.provider, "model", p.model, "msgCount", len(lcMsgs), "jsonMode", opts.JSONMode)
	start := time.Now()

	resp, err := p.llm.GenerateContent(ctx, lcMsgs, callOpts...)
	if err != nil {
		glog.L().Error("langchain error", "provider", p.provider, "err", err)
		return "", fmt.Errorf("langchain %s: %w", p.provider, err)
	}

	glog.L().Debug("langchain done", "elapsed", time.Since(start), "choices", len(resp.Choices))
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("langchain %s: no choices returned", p.provider)
	}

	result := resp.Choices[0].Content
	glog.L().Debug("langchain result", "resultLen", len(result))
	return result, nil
}

func (p *LangChainProvider) Name() string {
	if p.model != "" {
		return fmt.Sprintf("%s/%s", p.provider, p.model)
	}
	return p.provider
}
