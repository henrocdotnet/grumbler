package stages

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/henrocdotnet/grumbler/internal/config"
	"github.com/henrocdotnet/grumbler/internal/llm"
	"github.com/henrocdotnet/grumbler/internal/model"
	"github.com/henrocdotnet/grumbler/internal/pipeline"
)

// concurrentMock records how many calls overlap and returns one suggestion per
// call, tagged with the batch's first file path so ordering is observable.
type concurrentMock struct {
	mu       sync.Mutex
	inFlight int
	maxSeen  int
	calls    int64
	delay    func() time.Duration
}

func (m *concurrentMock) Complete(_ context.Context, msgs []llm.Message, _ llm.CompletionOpts) (string, error) {
	m.mu.Lock()
	m.inFlight++
	if m.inFlight > m.maxSeen {
		m.maxSeen = m.inFlight
	}
	m.mu.Unlock()

	atomic.AddInt64(&m.calls, 1)

	if m.delay != nil {
		time.Sleep(m.delay())
	}

	// Echo back a path present in the user prompt so results are attributable.
	path := "unknown.go"
	for _, msg := range msgs {
		if i := strings.Index(msg.Content, "MARK:"); i >= 0 {
			rest := msg.Content[i+len("MARK:"):]
			if j := strings.IndexAny(rest, " \n"); j > 0 {
				path = rest[:j]
			}
		}
	}

	m.mu.Lock()
	m.inFlight--
	m.mu.Unlock()

	return fmt.Sprintf(`{"suggestions":[{"title":"t","description":"d","filePath":%q,"startLine":1,"endLine":1,"severity":"high","category":"bug"}]}`, path), nil
}

func (m *concurrentMock) Name() string { return "concurrent-mock" }

func newInspectCtx(t *testing.T, p llm.Provider, nFiles, concurrency int) *pipeline.ReviewContext {
	t.Helper()

	cfg := config.Defaults()
	cfg.Review.Concurrency = concurrency
	// One file per batch, so batch count == file count.
	cfg.Review.MaxBatchTokens = 1

	files := make([]model.FileChange, nFiles)
	for i := range files {
		path := fmt.Sprintf("f%02d.go", i)
		files[i] = model.FileChange{
			Path:     path,
			Language: "go",
			Patch:    fmt.Sprintf("MARK:%s\n@@ -1 +1 @@\n+x := %d\n", path, i),
		}
	}

	return &pipeline.ReviewContext{
		Config:   cfg,
		Provider: p,
		Files:    files,
		Fast:     true,
	}
}

// The inspect stage used to run its batches in a plain sequential loop, so
// review.concurrency had no effect on the stage that dominates a run.
func TestInspect_BatchesRunConcurrently(t *testing.T) {
	m := &concurrentMock{delay: func() time.Duration { return 40 * time.Millisecond }}
	rc := newInspectCtx(t, m, 12, 4)

	if err := (Inspect{}).Execute(context.Background(), rc); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if m.calls != 12 {
		t.Errorf("provider calls = %d, want 12 (one per batch)", m.calls)
	}
	if m.maxSeen < 2 {
		t.Errorf("max concurrent calls = %d, want > 1 (batches ran serially)", m.maxSeen)
	}
	if m.maxSeen > 4 {
		t.Errorf("max concurrent calls = %d, exceeds the configured limit of 4", m.maxSeen)
	}
}

// Concurrency must not make the reported suggestion order depend on which
// batch happens to finish first: aggregate dedups first-wins and truncates.
func TestInspect_ResultsAreDeterministic(t *testing.T) {
	run := func() []string {
		m := &concurrentMock{delay: func() time.Duration {
			return time.Duration(rand.Intn(12)) * time.Millisecond
		}}
		rc := newInspectCtx(t, m, 10, 5)
		if err := (Inspect{}).Execute(context.Background(), rc); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		paths := make([]string, len(rc.Suggestions))
		for i, s := range rc.Suggestions {
			paths[i] = s.FilePath
		}
		return paths
	}

	first := run()
	if len(first) != 10 {
		t.Fatalf("got %d suggestions, want 10", len(first))
	}
	for i := 0; i < 4; i++ {
		got := run()
		if strings.Join(got, ",") != strings.Join(first, ",") {
			t.Fatalf("suggestion order is nondeterministic:\n run 1: %v\n run %d: %v", first, i+2, got)
		}
	}
}

// errgroup.SetLimit(0) blocks forever; a YAML `concurrency: 0` must not hang.
func TestInspect_ZeroConcurrencyDoesNotDeadlock(t *testing.T) {
	m := &concurrentMock{}
	rc := newInspectCtx(t, m, 4, 0)

	done := make(chan error, 1)
	go func() { done <- (Inspect{}).Execute(context.Background(), rc) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Inspect deadlocked with concurrency: 0")
	}
}

// vet.go called errgroup.SetLimit(concurrency) with no <= 0 guard, so a
// hand-written `concurrency: 0` in YAML blocked the audit stage forever.
func TestAudit_ZeroConcurrencyDoesNotDeadlock(t *testing.T) {
	m := &concurrentMock{}

	cfg := config.Defaults()
	cfg.Review.Concurrency = 0
	cfg.Passes.Audit = true

	rc := &pipeline.ReviewContext{
		Config:   cfg,
		Provider: m,
		Files: []model.FileChange{
			{Path: "a.go", Language: "go", Patch: "@@ -1 +1 @@\n+x := 1\n", Content: "package a\n"},
			{Path: "b.go", Language: "go", Patch: "@@ -1 +1 @@\n+y := 2\n", Content: "package b\n"},
		},
		Suggestions: []model.CodeSuggestion{
			{ID: "a.go:1:0", FilePath: "a.go", Title: "t1", StartLine: 1},
			{ID: "b.go:1:0", FilePath: "b.go", Title: "t2", StartLine: 1},
		},
	}

	done := make(chan error, 1)
	go func() { done <- (Audit{}).Execute(context.Background(), rc) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Audit deadlocked with concurrency: 0")
	}
}
