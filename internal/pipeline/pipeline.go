package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/henrocdotnet/grumbler/internal/llm"
	glog "github.com/henrocdotnet/grumbler/internal/log"
)

// Pipeline executes stages sequentially.
type Pipeline struct {
	stages []Stage
	onStep func(name string, dur time.Duration) // progress callback
}

// New creates a pipeline with the given stages.
func New(stages ...Stage) *Pipeline {
	return &Pipeline{stages: stages}
}

// Stages returns the stage list (for logging/introspection).
func (p *Pipeline) Stages() []Stage { return p.stages }

// OnStep sets a callback invoked after each stage completes.
func (p *Pipeline) OnStep(fn func(name string, dur time.Duration)) {
	p.onStep = fn
}

// Run executes all stages in order. Stops on first error.
func (p *Pipeline) Run(ctx context.Context, rc *ReviewContext) error {
	for _, s := range p.stages {
		glog.L().Info("stage entry", "stage", s.Name())
		stageCtx := context.WithValue(ctx, llm.StageKey, s.Name())
		start := time.Now()
		if err := s.Execute(stageCtx, rc); err != nil {
			glog.L().Error("stage failed", "stage", s.Name(), "err", err)
			return fmt.Errorf("stage %s: %w", s.Name(), err)
		}
		dur := time.Since(start)
		glog.L().Info("stage done", "stage", s.Name(), "duration", dur, "suggestions", len(rc.Suggestions))
		rc.PassesRun = append(rc.PassesRun, s.Name())
		if p.onStep != nil {
			p.onStep(s.Name(), dur)
		}
	}
	glog.L().Info("pipeline complete", "passesRun", rc.PassesRun, "totalSuggestions", len(rc.Suggestions))
	return nil
}
