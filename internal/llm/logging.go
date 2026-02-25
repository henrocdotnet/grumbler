package llm

import (
	"context"
	"sync"
	"time"
)

type contextKey string

// StageKey is the context key used to propagate the current pipeline stage name.
const StageKey contextKey = "stage"

// Exchange records a single LLM call: prompts sent, response received, timing.
type Exchange struct {
	Stage    string        `json:"stage"`
	Messages []Message     `json:"messages"`
	Response string        `json:"response"`
	Duration time.Duration `json:"duration_ms"`
	Error    string        `json:"error,omitempty"`
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
	ex := Exchange{
		Stage:    stage,
		Messages: msgs,
		Response: resp,
		Duration: time.Since(start),
	}
	if err != nil {
		ex.Error = err.Error()
	}
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
