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
	Mode                                Mode `json:",omitempty"`
}

type Progress struct {
	ID, Topic                          string
	Mode                               Mode `json:",omitempty"`
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
	proof       *conceptProof
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
	mu     sync.Mutex
	engine runEngine
	runs   map[string]*graphRun
}

type runEngine struct {
	generator Generator
	batch     BatchGenerator
	planner   ContextBatchPlanner
}

func newRunEngine(g Generator) runEngine {
	batch, _ := g.(BatchGenerator)
	planner, _ := g.(ContextBatchPlanner)
	return runEngine{generator: g, batch: batch, planner: planner}
}

func NewRunStore(g Generator) *RunStore {
	return &RunStore{engine: newRunEngine(g), runs: make(map[string]*graphRun)}
}

func (s *RunStore) Start(ctx context.Context, target Target, req Request, snapshotKey string, partial bool) (RunResult, error) {
	if s == nil {
		if len(req.Sources) == 0 {
			return RunResult{Status: "no_sources"}, nil
		}
		return RunResult{Status: "unavailable"}, nil
	}
	run, err := s.engine.prepare(ctx, target, req, snapshotKey, partial)
	if err != nil {
		return RunResult{}, err
	}
	result := run.result()
	if run.status == "ready" && !s.save(run) {
		return RunResult{Status: "unavailable"}, nil
	}
	return result, nil
}

func (e runEngine) prepare(ctx context.Context, target Target, req Request, snapshotKey string, partial bool) (*graphRun, error) {
	run, err := e.newRun(target, req, snapshotKey, partial)
	if err != nil || run.status != "planning" {
		return run, err
	}
	plan, err := e.plan(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	run.applyPlan(plan)
	return run, nil
}

func (e runEngine) newRun(target Target, req Request, snapshotKey string, partial bool) (*graphRun, error) {
	if len(req.Sources) == 0 {
		return &graphRun{status: "no_sources"}, nil
	}
	if e.generator == nil {
		return &graphRun{status: "unavailable"}, nil
	}
	if len(req.Sources) > MaxRunSources || snapshotKey == "" {
		return nil, errors.New("invalid process graph run request")
	}
	run := &graphRun{
		target: target, snapshotKey: snapshotKey, gate: make(chan struct{}, 1), original: req,
		progress: Progress{ID: rand.Text(), Topic: req.Anchor.Name, Mode: req.Mode, Revision: 1},
		status:   "planning", draft: Draft{Partial: partial},
	}
	if req.Mode == RequirementMode && len(req.Sources) == 1 {
		run.status = "insufficient"
		run.clearWork()
	}
	return run, nil
}

func (r *graphRun) applyPlan(plan BatchPlan) {
	if r.original.Mode != RequirementMode {
		r.proof = buildConceptProof(r.original)
	}
	r.inspected = make(map[string]bool)
	r.status = "ready"
	r.progress.Total = len(plan.Batches)
	r.draft.Partial = r.draft.Partial || plan.Partial
	for _, batch := range plan.Batches {
		r.pending = append(r.pending, pendingBatch{request: batch})
	}
	if len(r.pending) == 0 {
		r.finish()
	}
}

func (e runEngine) plan(ctx context.Context, req Request) (BatchPlan, error) {
	if err := ctx.Err(); err != nil {
		return BatchPlan{}, err
	}
	if e.planner != nil {
		return e.planner.PlanProcessGraphWithContext(ctx, req)
	}
	if e.batch != nil {
		return e.batch.PlanProcessGraph(req)
	}
	var plan BatchPlan
	for offset := 0; offset < len(req.Sources); {
		last := min(offset+MaxSources, len(req.Sources))
		batch := req
		if len(batch.OriginalSources) == 0 {
			batch.OriginalSources = req.Sources
		}
		batch.Sources = req.Sources[offset:last]
		plan.Batches = append(plan.Batches, batch)
		offset = last
		if req.Mode == RequirementMode && last < len(req.Sources) {
			plan.Partial = true
			offset--
		}
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

	if err := s.engine.advance(ctx, run, check); err != nil {
		return RunResult{}, err
	}
	return run.result(), nil
}

func (e runEngine) advance(ctx context.Context, run *graphRun, check func(context.Context, Target) (string, error)) error {
	current := run.pending[0]
	generate := e.generator.GenerateProcessGraph
	if e.batch != nil {
		generate = e.batch.GenerateProcessGraphBatch
	}
	suggestion, err := generate(ctx, current.request)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if snapshotErr := run.check(ctx, check); snapshotErr != nil {
		return snapshotErr
	}
	if run.status == "stale" {
		return nil
	}
	if errors.Is(err, ErrBatchTooLarge) && e.batch != nil && current.depth < maxSplitDepth {
		left, right, ok := e.batch.SplitProcessGraphBatch(current.request)
		if ok {
			run.pending = append([]pendingBatch{
				{request: left, depth: current.depth + 1},
				{request: right, depth: current.depth + 1},
			}, run.pending[1:]...)
			run.progress.Total++
			if run.original.Mode == RequirementMode {
				run.draft.Partial = true
			}
			run.progress.Revision++
			return nil
		}
	}
	run.settle(current.request, suggestion, err)
	return nil
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
		draft, err := groundDraft(suggestion, batch, r.proof)
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
	if r.original.Mode != RequirementMode && augmentLiteralDraft(&r.draft, r.original, r.proof) {
		for _, source := range r.original.Sources {
			r.inspected[source.ID] = true
		}
		r.draft.SourceCount = len(r.inspected)
	}
	if r.original.Mode != RequirementMode {
		r.proof.mergeActorCards(&r.draft)
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
	r.proof = nil
	r.pending = nil
	r.inspected = nil
}

func (r *graphRun) result() RunResult {
	result := RunResult{Status: r.status}
	if r.progress.ID != "" {
		progress := r.progress
		result.Run = &progress
	}
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
	run.lastUsed = now
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
