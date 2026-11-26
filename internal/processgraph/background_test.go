package processgraph

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

type backgroundGenerator func(context.Context, Request) (Suggestion, error)

func (g backgroundGenerator) GenerateProcessGraph(ctx context.Context, req Request) (Suggestion, error) {
	return g(ctx, req)
}

func TestBackgroundProcessGraphSurvivesRequestAndPublishesProgress(t *testing.T) {
	req := sampleRequest()
	for i := range MaxSources + 1 {
		req.Sources = append(req.Sources, sampleRequest(fmt.Sprint(i)).Sources[0])
	}
	started := make(chan struct{}, len(req.Sources))
	step := make(chan struct{})
	s := NewBackgroundRuns(backgroundGenerator(func(ctx context.Context, req Request) (Suggestion, error) {
		started <- struct{}{}
		select {
		case <-step:
			return sampleRelation(req), nil
		case <-ctx.Done():
			return Suggestion{}, ctx.Err()
		}
	}))
	defer s.Close()

	ctx, cancel := context.WithCancel(t.Context())
	result, err := s.Start(ctx, sampleTarget, req, "original", false, stableGraphSnapshot)
	cancel()
	if err != nil || result.Status != "ready" || result.Run == nil || result.Run.Topic != "delivery" || result.Run.Total != 2 {
		t.Fatalf("unexpected start: %+v, %v", result, err)
	}
	nextBatch := func() {
		t.Helper()
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("worker did not start the next batch")
		}
	}
	completeBatch := func() {
		t.Helper()
		select {
		case step <- struct{}{}:
		case <-time.After(2 * time.Second):
			t.Fatal("worker stopped after its HTTP request ended")
		}
	}
	nextBatch()
	other := sampleTarget
	other.Keyword = "different"
	duplicate, err := s.Start(t.Context(), other, sampleRequest("other"), "different", false, stableGraphSnapshot)
	if err != nil || duplicate.Run == nil || duplicate.Run.ID != result.Run.ID {
		t.Fatalf("a second start replaced an active run: %+v, %v", duplicate, err)
	}
	for _, scope := range [][2]string{{"another workspace", sampleTarget.Project}, {sampleTarget.Workspace, "another project"}} {
		wrong, err := s.Latest(t.Context(), scope[0], scope[1], stableGraphSnapshot)
		if err != nil || wrong.Status != "" || wrong.Run != nil {
			t.Fatalf("latest exposed a run from another scope: %+v, %v", wrong, err)
		}
	}

	completeBatch()
	nextBatch()
	readCtx, stopReading := context.WithTimeout(t.Context(), 2*time.Second)
	defer stopReading()
	active, err := s.Latest(readCtx, sampleTarget.Workspace, sampleTarget.Project, stableGraphSnapshot)
	if err != nil || active.Status != "ready" || active.Run == nil || active.Run.ID != result.Run.ID ||
		active.Run.Completed != 1 || active.Run.Total != 2 || active.Draft != nil {
		t.Fatalf("latest did not show settled progress during generation: %+v, %v", active, err)
	}
	completeBatch()
	done := awaitBackgroundGraph(t, s, stableGraphSnapshot)
	if done.Status != "suggested" || done.Run.ID != result.Run.ID || done.Run.Completed != 2 ||
		done.Draft == nil || done.Draft.SourceCount != len(req.Sources) {
		t.Fatalf("unexpected completed graph: %+v, draft=%+v", done, done.Draft)
	}
	stale, err := s.Latest(t.Context(), sampleTarget.Workspace, sampleTarget.Project, func(context.Context, Target) (string, error) {
		return "edited", nil
	})
	if err != nil || stale.Status != "stale" || stale.Draft != nil || stale.Run.ID != result.Run.ID {
		t.Fatalf("latest exposed a result for edited sources: %+v, %v", stale, err)
	}
}

func TestBackgroundProcessGraphCloseCancelsAndJoinsWorker(t *testing.T) {
	started := make(chan struct{})
	exited := make(chan struct{})
	s := NewBackgroundRuns(backgroundGenerator(func(ctx context.Context, _ Request) (Suggestion, error) {
		close(started)
		defer close(exited)
		<-ctx.Done()
		return Suggestion{}, ctx.Err()
	}))
	result, err := s.Start(t.Context(), sampleTarget, sampleRequest("source"), "original", false, stableGraphSnapshot)
	if err != nil || result.Status != "ready" {
		t.Fatalf("unexpected start: %+v, %v", result, err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not start")
	}
	closed := make(chan struct{})
	go func() {
		s.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("close did not stop background generation")
	}
	select {
	case <-exited:
	default:
		t.Fatal("close returned before the worker stopped")
	}
	result, err = s.Start(t.Context(), sampleTarget, sampleRequest("source"), "original", false, stableGraphSnapshot)
	if err != nil || result.Status != "unavailable" || result.Run != nil {
		t.Fatalf("shutdown accepted a new run: %+v, %v", result, err)
	}
}

func TestBackgroundProcessGraphInvalidatesSourcesDuringInference(t *testing.T) {
	var edited atomic.Bool
	check := func(context.Context, Target) (string, error) {
		if edited.Load() {
			return "edited", nil
		}
		return "original", nil
	}
	s := NewBackgroundRuns(backgroundGenerator(func(_ context.Context, req Request) (Suggestion, error) {
		edited.Store(true)
		return sampleRelation(req), nil
	}))
	defer s.Close()
	result, err := s.Start(t.Context(), sampleTarget, sampleRequest("source"), "original", false, check)
	if err != nil || result.Status != "ready" {
		t.Fatalf("unexpected start: %+v, %v", result, err)
	}
	stale := awaitBackgroundGraph(t, s, check)
	if stale.Status != "stale" || stale.Draft != nil || stale.Run == nil || stale.Run.Completed != 0 {
		t.Fatalf("source edit did not stop the run: %+v", stale)
	}
}

func awaitBackgroundGraph(t *testing.T, s *BackgroundRuns, check func(context.Context, Target) (string, error)) RunResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		result, err := s.Latest(ctx, sampleTarget.Workspace, sampleTarget.Project, check)
		if err != nil {
			t.Fatal(err)
		}
		if result.Status != "ready" {
			return result
		}
		select {
		case <-tick.C:
		case <-ctx.Done():
			t.Fatal("worker did not finish")
		}
	}
}
