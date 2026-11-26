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

	mu     sync.Mutex
	latest map[projectScope]*backgroundRun
	work   sync.WaitGroup
}

func NewBackgroundRuns(g Generator) *BackgroundRuns {
	ctx, cancel := context.WithCancel(context.Background())
	return &BackgroundRuns{
		engine: newRunEngine(g), ctx: ctx, cancel: cancel,
		latest: make(map[projectScope]*backgroundRun),
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
		return RunResult{Status: "unavailable"}, nil
	}
	scope := projectScope{target.Workspace, target.Project}
	previous := s.latest[scope]
	if previous != nil && previous.result.Status == "ready" {
		return previous.result, nil
	}
	run, err := s.engine.prepare(ctx, target, req, snapshotKey, partial)
	if err != nil {
		return RunResult{}, err
	}
	result := run.result()
	if result.Run == nil {
		return result, nil
	}
	if previous == nil && len(s.latest) >= maxStoredRuns && !s.evictCompleted() {
		return RunResult{Status: "unavailable"}, nil
	}

	entry := &backgroundRun{target: target, snapshotKey: snapshotKey, result: result}
	s.latest[scope] = entry
	if result.Status == "ready" {
		s.work.Go(func() { s.generate(run, entry, check) })
	} else {
		entry.finishedAt = time.Now()
	}
	return result, nil
}

func (s *BackgroundRuns) Latest(ctx context.Context, workspace, project string,
	check func(context.Context, Target) (string, error),
) (RunResult, error) {
	s.mu.Lock()
	entry := s.latest[projectScope{workspace, project}]
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
			entry.result = result
			s.mu.Unlock()
		}
	}
	return result, nil
}

func (s *BackgroundRuns) generate(run *graphRun, entry *backgroundRun, check func(context.Context, Target) (string, error)) {
	for run.status == "ready" {
		err := run.check(s.ctx, check)
		if err == nil && run.status == "ready" {
			err = s.engine.advance(s.ctx, run, check)
		}
		if err != nil {
			run.status = "unavailable"
			run.progress.Revision++
		}
		s.mu.Lock()
		entry.result = run.result()
		if run.status != "ready" {
			entry.finishedAt = time.Now()
		}
		s.mu.Unlock()
	}
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
