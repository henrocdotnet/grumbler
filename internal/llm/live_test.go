//go:build live

package llm

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/henrocdotnet/grumbler/internal/config"
)

// TestLiveClaudeCLI sends a basic prompt to claude -p and validates the response.
// Skipped if claude binary is not installed.
func TestLiveClaudeCLI(t *testing.T) {
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude CLI not installed")
	}

	provider := NewClaudeCLI()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	msgs := []Message{
		UserMsg("What color is the sky? Reply with a single word."),
	}

	resp, err := provider.Complete(ctx, msgs, CompletionOpts{
		Temperature: 0,
		MaxTokens:   32,
		JSONMode:    false,
	})
	if err != nil {
		t.Fatalf("claude-cli error: %v", err)
	}

	resp = strings.ToLower(strings.TrimSpace(resp))
	t.Logf("response: %q", resp)

	if !strings.Contains(resp, "blue") {
		t.Errorf("expected response containing 'blue', got: %q", resp)
	}
}

// TestLiveGeminiCLI sends a basic prompt to gemini -p and validates the response.
// Skipped if gemini binary is not installed.
func TestLiveGeminiCLI(t *testing.T) {
	if _, err := exec.LookPath("gemini"); err != nil {
		t.Skip("gemini CLI not installed")
	}

	provider := NewGeminiCLI()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	msgs := []Message{
		UserMsg("What color is the sky? Reply with a single word."),
	}

	resp, err := provider.Complete(ctx, msgs, CompletionOpts{
		Temperature: 0,
		MaxTokens:   32,
		JSONMode:    false,
	})
	if err != nil {
		t.Fatalf("gemini-cli error: %v", err)
	}

	resp = strings.ToLower(strings.TrimSpace(resp))
	t.Logf("response: %q", resp)

	if !strings.Contains(resp, "blue") {
		t.Errorf("expected response containing 'blue', got: %q", resp)
	}
}

// TestLiveAnthropicAPI sends a basic prompt via the Anthropic API (langchaingo).
// Skipped if ANTHROPIC_API_KEY is not set.
func TestLiveAnthropicAPI(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}

	provider, err := NewLangChainProvider(config.LLMConfig{
		Provider: "anthropic",
		Model:    "claude-sonnet-4-6",
		APIKey:   apiKey,
	})
	if err != nil {
		t.Fatalf("creating anthropic provider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	msgs := []Message{
		UserMsg("What color is the sky? Reply with a single word."),
	}

	resp, err := provider.Complete(ctx, msgs, CompletionOpts{
		Temperature: 0,
		MaxTokens:   32,
		JSONMode:    false,
	})
	if err != nil {
		t.Fatalf("anthropic API error: %v", err)
	}

	resp = strings.ToLower(strings.TrimSpace(resp))
	t.Logf("response: %q", resp)

	if !strings.Contains(resp, "blue") {
		t.Errorf("expected response containing 'blue', got: %q", resp)
	}
}

// TestLiveGoogleAPI sends a basic prompt via the Google AI API (langchaingo).
// Skipped if GEMINI_API_KEY is not set.
func TestLiveGoogleAPI(t *testing.T) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("GEMINI_API_KEY not set")
	}

	model := os.Getenv("GEMINI_MODEL")
	if model == "" {
		model = "gemini-3-flash-preview"
	}

	provider, err := NewLangChainProvider(config.LLMConfig{
		Provider: "google",
		Model:    model,
		APIKey:   apiKey,
	})
	if err != nil {
		t.Fatalf("creating google provider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	msgs := []Message{
		UserMsg("What color is the sky? Reply with a single word."),
	}

	resp, err := provider.Complete(ctx, msgs, CompletionOpts{
		Temperature: 0,
		MaxTokens:   32,
		JSONMode:    false,
	})
	if err != nil {
		t.Fatalf("google API error: %v", err)
	}

	resp = strings.ToLower(strings.TrimSpace(resp))
	t.Logf("response: %q", resp)

	if !strings.Contains(resp, "blue") {
		t.Errorf("expected response containing 'blue', got: %q", resp)
	}
}
