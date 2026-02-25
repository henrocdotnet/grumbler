package pipeline

import "context"

// Stage is a single step in the review pipeline.
type Stage interface {
	Name() string
	Execute(ctx context.Context, rc *ReviewContext) error
}
