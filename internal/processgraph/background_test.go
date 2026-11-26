package processgraph

import (
	"context"
	"errors"
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

type backgroundPlanner struct {
	backgroundGenerator
	plan func(context.Context, Request) (BatchPlan, error)
}

func (g backgroundPlanner) PlanProcessGraphWithContext(ctx context.Context, req Request) (BatchPlan, error) {
	return g.plan(ctx, req)
}

func TestBackgroundProcessGraphPublishesPlanningWithoutBlocking(t *testing.T) {
	planning := make(chan struct{})
	planReady := make(chan struct{})
	generating := make(chan struct{})
	generationReady := make(chan struct{})
	var checks atomic.Int32
	check := func(ctx context.Context, target Target) (string, error) {
		if target != sampleTarget {
			t.Errorf("wrong planning scope: %+v", target)
		}
		checks.Add(1)
		return stableGraphSnapshot(ctx, target)
	}
	s := NewBackgroundRuns(backgroundPlanner{
		backgroundGenerator: func(ctx context.Context, req Request) (Suggestion, error) {
			close(generating)
			select {
			case <-generationReady:
				return sampleRelation(req), nil
			case <-ctx.Done():
				return Suggestion{}, ctx.Err()
			}
		},
		plan: func(ctx context.Context, req Request) (BatchPlan, error) {
			if checks.Load() != 1 {
				t.Errorf("snapshot did not precede planning: %d checks", checks.Load())
			}
			close(planning)
			select {
			case <-planReady:
				return BatchPlan{Batches: []Request{req}}, nil
			case <-ctx.Done():
				return BatchPlan{}, ctx.Err()
			}
		},
	})
	defer s.Close()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := make(chan RunResult, 1)
	go func() {
		result, err := s.Start(ctx, sampleTarget, sampleRequest("source"), "original", false, check)
		if err != nil {
			t.Error(err)
		}
		started <- result
	}()
	var start RunResult
	select {
	case start = <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("starting the graph waited for planning")
	}
	if start.Status != "planning" || start.Run == nil || start.Run.ID == "" ||
		start.Run.Topic != "delivery" || start.Run.Total != 0 || start.Draft != nil {
		t.Fatalf("start did not expose a run with an unknown total: %+v", start)
	}
	cancel()
	awaitGraphPhase(t, planning, "planning")

	other := sampleTarget
	other.Keyword = "different"
	duplicate, err := s.Start(t.Context(), other, sampleRequest("other"), "other", false, check)
	if err != nil || duplicate.Run == nil || *duplicate.Run != *start.Run || duplicate.Status != "planning" {
		t.Fatalf("a duplicate start did not return the planning run: %+v, %v", duplicate, err)
	}
	latest, err := s.Latest(t.Context(), sampleTarget.Workspace, sampleTarget.Project, check)
	if err != nil || latest.Run == nil || *latest.Run != *start.Run || latest.Status != "planning" {
		t.Fatalf("latest did not expose the planning run: %+v, %v", latest, err)
	}
	hidden, err := s.Latest(t.Context(), "another-workspace", sampleTarget.Project, check)
	if err != nil || hidden.Status != "" || hidden.Run != nil {
		t.Fatalf("latest exposed a run from another workspace: %+v, %v", hidden, err)
	}

	close(planReady)
	awaitGraphPhase(t, generating, "generation")
	ready, err := s.Latest(t.Context(), sampleTarget.Workspace, sampleTarget.Project, check)
	if err != nil || ready.Status != "ready" || ready.Run == nil || ready.Run.ID != start.Run.ID ||
		ready.Run.Total != 1 || ready.Run.Completed != 0 || ready.Run.Revision <= start.Run.Revision || ready.Draft != nil {
		t.Fatalf("latest did not expose the complete plan before generation: %+v, %v", ready, err)
	}
	close(generationReady)
	done := awaitBackgroundGraph(t, s, check)
	if done.Status != "suggested" || done.Run.ID != start.Run.ID || done.Run.Completed != 1 {
		t.Fatalf("planning did not survive the cancelled request: %+v", done)
	}
}

func TestBackgroundProcessGraphValidatesSourcesAroundPlanning(t *testing.T) {
	for _, tc := range []struct {
		name      string
		failCheck int32
		key       string
		err       error
		status    string
		plans     int32
	}{
		{name: "edited before", failCheck: 1, key: "edited", status: "stale"},
		{name: "edited during", failCheck: 2, key: "edited", status: "stale", plans: 1},
		{name: "denied before", failCheck: 1, err: errors.New("access denied"), status: "unavailable"},
		{name: "denied during", failCheck: 2, err: errors.New("access denied"), status: "unavailable", plans: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var checks, plans, generations atomic.Int32
			check := func(context.Context, Target) (string, error) {
				if checks.Add(1) == tc.failCheck {
					return tc.key, tc.err
				}
				return "original", nil
			}
			s := NewBackgroundRuns(backgroundPlanner{
				backgroundGenerator: func(context.Context, Request) (Suggestion, error) {
					generations.Add(1)
					return Suggestion{}, nil
				},
				plan: func(_ context.Context, req Request) (BatchPlan, error) {
					plans.Add(1)
					return BatchPlan{Batches: []Request{req}}, nil
				},
			})
			defer s.Close()
			start, err := s.Start(t.Context(), sampleTarget, sampleRequest("source"), "original", false, check)
			if err != nil || start.Status != "planning" {
				t.Fatalf("unexpected start: %+v, %v", start, err)
			}
			result := awaitBackgroundGraph(t, s, check)
			if result.Status != tc.status || result.Run == nil || result.Run.Total != 0 || result.Draft != nil ||
				plans.Load() != tc.plans || generations.Load() != 0 {
				t.Fatalf("invalid sources reached planning or generation: %+v, plans=%d, generations=%d",
					result, plans.Load(), generations.Load())
			}
		})
	}
}

func TestBackgroundProcessGraphCloseCancelsPlanning(t *testing.T) {
	planning := make(chan struct{})
	exited := make(chan struct{})
	s := NewBackgroundRuns(backgroundPlanner{
		backgroundGenerator: func(context.Context, Request) (Suggestion, error) {
			return Suggestion{}, errors.New("unexpected generation")
		},
		plan: func(ctx context.Context, _ Request) (BatchPlan, error) {
			close(planning)
			defer close(exited)
			<-ctx.Done()
			return BatchPlan{}, ctx.Err()
		},
	})
	defer s.Close()
	start, err := s.Start(t.Context(), sampleTarget, sampleRequest("source"), "original", false, stableGraphSnapshot)
	if err != nil || start.Status != "planning" {
		t.Fatalf("unexpected start: %+v, %v", start, err)
	}
	awaitGraphPhase(t, planning, "planning")
	closed := make(chan struct{})
	go func() {
		s.Close()
		close(closed)
	}()
	awaitGraphPhase(t, closed, "shutdown")
	select {
	case <-exited:
	default:
		t.Fatal("close returned before planning stopped")
	}
}

func awaitGraphPhase(t *testing.T, phase <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-phase:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s did not start", name)
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
		if result.Status != "ready" && result.Status != "planning" {
			return result
		}
		select {
		case <-tick.C:
		case <-ctx.Done():
			t.Fatal("worker did not finish")
		}
	}
}
