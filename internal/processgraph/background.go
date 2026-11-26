package processgraph

import (
	"context"
	"sync"
	"time"
)

type projectScope struct {
	workspace, project string
}

type backgroundRun struct {
	target      Target
	snapshotKey string
	result      RunResult
	finishedAt  time.Time
}

// BackgroundRuns drives a project's latest run independently of HTTP requests.
// It retains completed results in memory within the same bound as active runs.
type BackgroundRuns struct {
	engine runEngine
	ctx    context.Context
	cancel context.CancelFunc

	mu               sync.Mutex
	latest           map[projectScope]*backgroundRun
	progressWatchers map[string]map[chan struct{}]struct{}
	work             sync.WaitGroup
}

func NewBackgroundRuns(g Generator) *BackgroundRuns {
	ctx, cancel := context.WithCancel(context.Background())
	return &BackgroundRuns{
		engine: newRunEngine(g), ctx: ctx, cancel: cancel,
		latest:           make(map[projectScope]*backgroundRun),
		progressWatchers: make(map[string]map[chan struct{}]struct{}),
	}
}

func (s *BackgroundRuns) Close() {
	s.mu.Lock()
	s.cancel()
	s.mu.Unlock()
	s.work.Wait()
}

func (s *BackgroundRuns) Start(ctx context.Context, target Target, req Request, snapshotKey string, partial bool,
	check func(context.Context, Target) (string, error),
) (RunResult, error) {
	scope := projectScope{target.Workspace, target.Project}
	s.mu.Lock()
	current := s.active(scope)
	s.mu.Unlock()
	if current.Status != "" {
		return current, nil
	}

	var run *graphRun
	var err error
	if s.engine.planner == nil {
		run, err = s.engine.prepare(ctx, target, req, snapshotKey, partial)
	} else {
		run, err = s.engine.newRun(target, req, snapshotKey, partial)
	}
	if err != nil {
		return RunResult{}, err
	}
	result := run.result()
	if result.Run == nil {
		return result, nil
	}
	if err := ctx.Err(); err != nil {
		return RunResult{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if current := s.active(scope); current.Status != "" {
		return current, nil
	}
	if s.latest[scope] == nil && len(s.latest) >= maxStoredRuns && !s.evictCompleted() {
		return RunResult{Status: "unavailable"}, nil
	}
	entry := &backgroundRun{target: target, snapshotKey: snapshotKey, result: result}
	s.latest[scope] = entry
	if result.Status == "ready" || result.Status == "planning" {
		s.work.Go(func() { s.execute(run, entry, check) })
	} else {
		entry.finishedAt = time.Now()
	}
	s.notifyProgress(scope.workspace)
	return result, nil
}

// active is called with s.mu held.
func (s *BackgroundRuns) active(scope projectScope) RunResult {
	if s.ctx.Err() != nil {
		return RunResult{Status: "unavailable"}
	}
	if entry := s.latest[scope]; entry != nil && entry.finishedAt.IsZero() {
		return entry.result
	}
	return RunResult{}
}

func (s *BackgroundRuns) Latest(ctx context.Context, workspace, project string,
	check func(context.Context, Target) (string, error),
) (RunResult, error) {
	s.mu.Lock()
	scope := projectScope{workspace, project}
	entry := s.latest[scope]
	if entry == nil {
		s.mu.Unlock()
		return RunResult{}, nil
	}
	result := entry.result
	s.mu.Unlock()

	if result.Draft != nil {
		key, err := check(ctx, entry.target)
		if err != nil {
			return RunResult{}, err
		}
		if key != entry.snapshotKey {
			progress := *result.Run
			progress.Revision++
			result = RunResult{Status: "stale", Run: &progress}

			s.mu.Lock()
			if s.latest[scope] == entry && entry.result.Draft != nil {
				entry.result = result
				s.notifyProgress(workspace)
			}
			s.mu.Unlock()
		}
	}
	return result, nil
}

func (s *BackgroundRuns) execute(run *graphRun, entry *backgroundRun, check func(context.Context, Target) (string, error)) {
	if run.status == "planning" {
		err := s.plan(run, check)
		s.publish(run, entry, err)
	}
	for run.status == "ready" {
		err := run.check(s.ctx, check)
		if err == nil && run.status == "ready" {
			err = s.engine.advance(s.ctx, run, check)
		}
		s.publish(run, entry, err)
	}
}

func (s *BackgroundRuns) plan(run *graphRun, check func(context.Context, Target) (string, error)) error {
	if err := run.check(s.ctx, check); err != nil || run.status == "stale" {
		return err
	}
	plan, err := s.engine.plan(s.ctx, run.original)
	if ctxErr := s.ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if checkErr := run.check(s.ctx, check); checkErr != nil || run.status == "stale" {
		return checkErr
	}
	if err != nil {
		return err
	}
	run.progress.Revision++
	run.applyPlan(plan)
	return nil
}

func (s *BackgroundRuns) publish(run *graphRun, entry *backgroundRun, err error) {
	if err != nil {
		run.status = "unavailable"
		run.progress.Revision++
		run.clearWork()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry.result = run.result()
	if run.status != "ready" {
		entry.finishedAt = time.Now()
	}
	s.notifyProgress(entry.target.Workspace)
}

func (s *BackgroundRuns) evictCompleted() bool {
	var oldestScope projectScope
	var oldest *backgroundRun
	for scope, entry := range s.latest {
		if !entry.finishedAt.IsZero() && (oldest == nil || entry.finishedAt.Before(oldest.finishedAt)) {
			oldestScope = scope
			oldest = entry
		}
	}
	if oldest == nil {
		return false
	}
	delete(s.latest, oldestScope)
	return true
}
