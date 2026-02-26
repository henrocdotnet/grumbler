package llm

import (
	"context"
	"testing"
)

// mockProvider implements Provider for testing.
type mockProvider struct{ resp string }

func (m *mockProvider) Complete(_ context.Context, _ []Message, _ CompletionOpts) (string, error) {
	return m.resp, nil
}
func (m *mockProvider) Name() string { return "mock" }

func TestLoggingProvider_TokenCounts(t *testing.T) {
	inner := &mockProvider{resp: "this is a test response with several tokens"}
	lp := NewLoggingProvider(inner)

	msgs := []Message{
		SystemMsg("You are a helpful assistant."),
		UserMsg("Review this code for issues."),
	}
	ctx := context.WithValue(context.Background(), StageKey, "inspect")
	if _, err := lp.Complete(ctx, msgs, DefaultOpts()); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	exs := lp.Exchanges()
	if len(exs) != 1 {
		t.Fatalf("expected 1 exchange, got %d", len(exs))
	}
	ex := exs[0]
	if ex.PromptTokens <= 0 {
		t.Errorf("PromptTokens = %d, want > 0", ex.PromptTokens)
	}
	if ex.CompletionTokens <= 0 {
		t.Errorf("CompletionTokens = %d, want > 0", ex.CompletionTokens)
	}
}

func TestCountTokens_NonZero(t *testing.T) {
	msgs := []Message{UserMsg("hello world, this is a short message")}
	n := countTokens(msgs)
	if n <= 0 {
		t.Errorf("countTokens = %d, want > 0", n)
	}
}

func TestCountTokens_Empty(t *testing.T) {
	if n := countTokens(nil); n != 0 {
		t.Errorf("countTokens(nil) = %d, want 0", n)
	}
	if n := countTokens([]Message{}); n != 0 {
		t.Errorf("countTokens([]) = %d, want 0", n)
	}
}
