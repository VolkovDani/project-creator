package processgraph

import (
	"context"
	"errors"
)

// BatchPlan preserves the provider's processing order and records any passages
// it could not fit even by themselves.
type BatchPlan struct {
	Batches []Request
	Partial bool
}

// BatchGenerator plans complete passages and makes exactly one provider call
// per batch. SplitProcessGraphBatch may replace a batch rejected for its size.
type BatchGenerator interface {
	Generator
	PlanProcessGraph(Request) (BatchPlan, error)
	GenerateProcessGraphBatch(context.Context, Request) (Suggestion, error)
	SplitProcessGraphBatch(Request) (Request, Request, bool)
}

var ErrBatchTooLarge = errors.New("process graph batch exceeds the provider budget")
