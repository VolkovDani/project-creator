package processgraph

import (
	"context"
	"crypto/rand"
	"errors"
	"sync"
	"time"
)

const (
	MaxRunSources = 256
	maxStoredRuns = 16
	runIdleLimit  = 15 * time.Minute
	maxSplitDepth = 2
)

type Target struct {
	Workspace, Project, TermID, Keyword string
}

type Progress struct {
	ID                                 string
	Revision, Completed, Total, Failed int
}

type RunResult struct {
	Status string
	Run    *Progress
	Draft  *Draft
}

type pendingBatch struct {
	request Request
	depth   int
}

type graphRun struct {
	target      Target
	snapshotKey string
	gate        chan struct{}
	original    Request
	pending     []pendingBatch
	progress    Progress
	status      string
	draft       Draft
	inspected   map[string]bool

	// Store bookkeeping uses RunStore.mu. The gate serializes the run state.
	lastUsed time.Time
	users    int
}

// RunStore keeps short-lived runs driven by scoped client requests. Each
// Advance invokes at most one generation batch and retains no worker goroutine.
type RunStore struct {
	mu        sync.Mutex
	generator Generator
	batch     BatchGenerator
	runs      map[string]*graphRun
}

func NewRunStore(g Generator) *RunStore {
	batch, _ := g.(BatchGenerator)
	return &RunStore{generator: g, batch: batch, runs: make(map[string]*graphRun)}
}

func (s *RunStore) Start(ctx context.Context, target Target, req Request, snapshotKey string, partial bool) (RunResult, error) {
	if len(req.Sources) == 0 {
		return RunResult{Status: "no_sources"}, nil
	}
	if s == nil || s.generator == nil {
		return RunResult{Status: "unavailable"}, nil
	}
	if len(req.Sources) > MaxRunSources || snapshotKey == "" {
		return RunResult{}, errors.New("invalid process graph run request")
	}
	plan, err := s.plan(req)
	if err != nil {
		return RunResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return RunResult{}, err
	}
	run := &graphRun{
		target: target, snapshotKey: snapshotKey, gate: make(chan struct{}, 1), original: req,
		progress: Progress{ID: rand.Text(), Revision: 1, Total: len(plan.Batches)},
		status:   "ready", draft: Draft{Partial: partial || plan.Partial},
		inspected: make(map[string]bool), lastUsed: time.Now(),
	}
	for _, batch := range plan.Batches {
		run.pending = append(run.pending, pendingBatch{request: batch})
	}
	if len(run.pending) == 0 {
		run.finish()
		return run.result(), nil
	}
	if !s.save(run) {
		return RunResult{Status: "unavailable"}, nil
	}
	return run.result(), nil
}

func (s *RunStore) plan(req Request) (BatchPlan, error) {
	if s.batch != nil {
		return s.batch.PlanProcessGraph(req)
	}
	var plan BatchPlan
	for offset := 0; offset < len(req.Sources); offset += MaxSources {
		last := min(offset+MaxSources, len(req.Sources))
		plan.Batches = append(plan.Batches, Request{Anchor: req.Anchor, Sources: req.Sources[offset:last]})
	}
	return plan, nil
}

func (s *RunStore) Advance(ctx context.Context, workspace, project, id string, revision int,
	check func(context.Context, Target) (string, error),
) (RunResult, error) {
	run := s.find(workspace, project, id)
	if run == nil {
		return RunResult{Status: "not_found"}, nil
	}
	defer s.release(run)
	select {
	case <-ctx.Done():
		return RunResult{}, ctx.Err()
	case run.gate <- struct{}{}:
	}
	defer func() { <-run.gate }()

	if run.status == "stale" {
		return run.result(), nil
	}
	if err := run.check(ctx, check); err != nil {
		return RunResult{}, err
	}
	if run.status != "ready" || revision < run.progress.Revision {
		return run.result(), nil
	}
	if revision != run.progress.Revision {
		return RunResult{Status: "stale"}, nil
	}

	current := run.pending[0]
	generate := s.generator.GenerateProcessGraph
	if s.batch != nil {
		generate = s.batch.GenerateProcessGraphBatch
	}
	suggestion, err := generate(ctx, current.request)
	if ctx.Err() != nil {
		return RunResult{}, ctx.Err()
	}
	if snapshotErr := run.check(ctx, check); snapshotErr != nil {
		return RunResult{}, snapshotErr
	}
	if run.status == "stale" {
		return run.result(), nil
	}
	if errors.Is(err, ErrBatchTooLarge) && s.batch != nil && current.depth < maxSplitDepth {
		left, right, ok := s.batch.SplitProcessGraphBatch(current.request)
		if ok {
			run.pending = append([]pendingBatch{
				{request: left, depth: current.depth + 1},
				{request: right, depth: current.depth + 1},
			}, run.pending[1:]...)
			run.progress.Total++
			run.progress.Revision++
			return run.result(), nil
		}
	}
	run.settle(current.request, suggestion, err)
	return run.result(), nil
}

func (r *graphRun) check(ctx context.Context, check func(context.Context, Target) (string, error)) error {
	key, err := check(ctx, r.target)
	if err != nil {
		return err
	}
	if key != r.snapshotKey {
		r.status = "stale"
		r.progress.Revision++
		r.draft = Draft{}
		r.clearWork()
	}
	return nil
}

func (r *graphRun) settle(batch Request, suggestion Suggestion, generationErr error) {
	if generationErr == nil {
		draft, err := groundDraft(suggestion, batch)
		generationErr = err
		if err == nil {
			for _, source := range batch.Sources[:draft.SourceCount] {
				r.inspected[source.ID] = true
			}
			r.draft.SourceCount = len(r.inspected)
			mergeDraft(&r.draft, draft)
		}
	}
	if generationErr != nil {
		r.progress.Failed++
		r.draft.Partial = true
	}
	r.pending[0] = pendingBatch{}
	r.pending = r.pending[1:]
	r.progress.Completed++
	r.progress.Revision++
	if len(r.pending) == 0 {
		r.finish()
	}
}

func (r *graphRun) finish() {
	if len(r.draft.Edges) == 0 {
		literal, err := groundDraft(literalRelations(r.original.Sources), r.original)
		if err == nil && len(literal.Edges) > 0 {
			for _, source := range r.original.Sources[:literal.SourceCount] {
				r.inspected[source.ID] = true
			}
			literal.SourceCount = len(r.inspected)
			r.draft = literal
		}
	}
	switch {
	case len(r.draft.Edges) > 0:
		r.status = "suggested"
	case r.progress.Failed > 0 || r.progress.Total == 0:
		r.status = "unavailable"
	default:
		r.status = "insufficient"
	}
	r.clearWork()
}

func (r *graphRun) clearWork() {
	r.original = Request{}
	r.pending = nil
	r.inspected = nil
}

func (r *graphRun) result() RunResult {
	progress := r.progress
	result := RunResult{Status: r.status, Run: &progress}
	if r.status == "suggested" {
		draft := r.draft
		result.Draft = &draft
	}
	return result
}

func (s *RunStore) save(run *graphRun) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	var oldest *graphRun
	for id, previous := range s.runs {
		if previous.users > 0 {
			continue
		}
		if now.Sub(previous.lastUsed) >= runIdleLimit {
			delete(s.runs, id)
		} else if oldest == nil || previous.lastUsed.Before(oldest.lastUsed) {
			oldest = previous
		}
	}
	if len(s.runs) >= maxStoredRuns {
		if oldest == nil {
			return false
		}
		delete(s.runs, oldest.progress.ID)
	}
	s.runs[run.progress.ID] = run
	return true
}

func (s *RunStore) find(workspace, project, id string) *graphRun {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	run := s.runs[id]
	if run == nil || run.target.Workspace != workspace || run.target.Project != project {
		return nil
	}
	if run.users == 0 && time.Since(run.lastUsed) >= runIdleLimit {
		delete(s.runs, id)
		return nil
	}
	run.users++
	return run
}

func (s *RunStore) release(run *graphRun) {
	s.mu.Lock()
	run.lastUsed = time.Now()
	run.users--
	s.mu.Unlock()
}
