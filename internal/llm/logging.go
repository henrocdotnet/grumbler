package llm

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/pkoukk/tiktoken-go"
)

type contextKey string

// StageKey is the context key used to propagate the current pipeline stage name.
const StageKey contextKey = "stage"

// Exchange records a single LLM call: prompts sent, response received, timing.
type Exchange struct {
	Stage            string        `json:"stage"`
	Messages         []Message     `json:"messages"`
	Response         string        `json:"response"`
	Duration         time.Duration `json:"duration_ms"`
	Error            string        `json:"error,omitempty"`
	PromptTokens        int           `json:"promptTokens"`
	CompletionTokens    int           `json:"completionTokens"`
	CacheCreationTokens int           `json:"cacheCreationTokens,omitempty"`
	CacheReadTokens     int           `json:"cacheReadTokens,omitempty"`
}

// countTokens returns the total token count for a set of messages using cl100k_base.
func countTokens(msgs []Message) int {
	enc, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		return 0
	}
	n := 0
	for _, m := range msgs {
		n += len(enc.Encode(m.Role+m.Content, nil, nil))
	}
	return n
}

// LoggingProvider wraps any Provider, intercepting Complete() to record exchanges.
type LoggingProvider struct {
	inner     Provider
	mu        sync.Mutex
	exchanges []Exchange
}

// NewLoggingProvider decorates inner with conversation logging.
func NewLoggingProvider(inner Provider) *LoggingProvider {
	return &LoggingProvider{inner: inner}
}

// Complete delegates to the inner provider and records the exchange.
func (lp *LoggingProvider) Complete(ctx context.Context, msgs []Message, opts CompletionOpts) (string, error) {
	stage, _ := ctx.Value(StageKey).(string)
	start := time.Now()
	resp, err := lp.inner.Complete(ctx, msgs, opts)

	// Prefer real usage from the provider; fall back to tiktoken estimate.
	var promptToks, completionToks, cacheCreation, cacheRead int
	if tp, ok := lp.inner.(TokenUsageProvider); ok {
		u := tp.LastTokenUsage()
		promptToks = u.PromptTokens
		completionToks = u.CompletionTokens
		cacheCreation = u.CacheCreationTokens
		cacheRead = u.CacheReadTokens
	}
	if promptToks == 0 && cacheCreation == 0 && cacheRead == 0 {
		promptToks = countTokens(msgs)
	}
	if completionToks == 0 {
		completionToks = countTokens([]Message{{Role: "assistant", Content: resp}})
	}

	ex := Exchange{
		Stage:               stage,
		Messages:            msgs,
		Response:            resp,
		Duration:            time.Since(start),
		PromptTokens:        promptToks,
		CompletionTokens:    completionToks,
		CacheCreationTokens: cacheCreation,
		CacheReadTokens:     cacheRead,
	}
	if err != nil {
		ex.Error = err.Error()
	}
	slog.Debug("llm_tokens", "stage", stage, "prompt", promptToks, "completion", completionToks)
	lp.mu.Lock()
	lp.exchanges = append(lp.exchanges, ex)
	lp.mu.Unlock()
	return resp, err
}

// Name delegates to the inner provider.
func (lp *LoggingProvider) Name() string { return lp.inner.Name() }

// Exchanges returns all recorded exchanges.
func (lp *LoggingProvider) Exchanges() []Exchange {
	lp.mu.Lock()
	defer lp.mu.Unlock()
	out := make([]Exchange, len(lp.exchanges))
	copy(out, lp.exchanges)
	return out
}
