package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/henrocdotnet/grumbler/internal/config"
	"github.com/henrocdotnet/grumbler/internal/llm"
	"github.com/henrocdotnet/grumbler/internal/model"
)

// mockProvider returns canned responses for testing.
type mockProvider struct {
	responses []string
	callIdx   int
}

func (m *mockProvider) Complete(_ context.Context, msgs []llm.Message, _ llm.CompletionOpts) (string, error) {
	if m.callIdx >= len(m.responses) {
		return "{\"suggestions\":[]}", nil
	}
	resp := m.responses[m.callIdx]
	m.callIdx++
	return resp, nil
}

func (m *mockProvider) Name() string { return "mock" }

// testStage is a simple stage for testing.
type testStage struct {
	name string
	fn   func(context.Context, *ReviewContext) error
}

func (s testStage) Name() string                                         { return s.name }
func (s testStage) Execute(ctx context.Context, rc *ReviewContext) error { return s.fn(ctx, rc) }

func TestPipelineRun(t *testing.T) {
	var order []string

	stages := []Stage{
		testStage{"a", func(_ context.Context, _ *ReviewContext) error {
			order = append(order, "a")
			return nil
		}},
		testStage{"b", func(_ context.Context, _ *ReviewContext) error {
			order = append(order, "b")
			return nil
		}},
	}

	p := New(stages...)
	rc := &ReviewContext{Config: config.Defaults(), Provider: &mockProvider{}}
	if err := p.Run(context.Background(), rc); err != nil {
		t.Fatal(err)
	}

	if len(order) != 2 || order[0] != "a" || order[1] != "b" {
		t.Errorf("unexpected execution order: %v", order)
	}
	if len(rc.PassesRun) != 2 {
		t.Errorf("expected 2 passes run, got %d", len(rc.PassesRun))
	}
}

func TestPipelineStopsOnError(t *testing.T) {
	stages := []Stage{
		testStage{"fail", func(_ context.Context, _ *ReviewContext) error {
			return fmt.Errorf("boom")
		}},
		testStage{"never", func(_ context.Context, _ *ReviewContext) error {
			t.Fatal("should not run")
			return nil
		}},
	}

	p := New(stages...)
	rc := &ReviewContext{Config: config.Defaults(), Provider: &mockProvider{}}
	err := p.Run(context.Background(), rc)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("expected error containing 'boom', got: %v", err)
	}
}

func TestReviewContextResult(t *testing.T) {
	rc := &ReviewContext{
		Config:   config.Defaults(),
		Provider: &mockProvider{},
		Files: []model.FileChange{
			{Path: "a.go"},
			{Path: "b.go"},
		},
		PassesRun: []string{"prepare", "review"},
	}

	rc.AddSuggestions([]model.CodeSuggestion{
		{Title: "test", FilePath: "a.go"},
	})

	result := rc.Result()
	if result.FilesCount != 2 {
		t.Errorf("files count: got %d, want 2", result.FilesCount)
	}
	if len(result.Suggestions) != 1 {
		t.Errorf("suggestions: got %d, want 1", len(result.Suggestions))
	}

	// Verify JSON serialization works
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("empty JSON output")
	}
}
