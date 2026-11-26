package processgraph

// BackgroundProgress is the compact state of a project's latest retained run.
type BackgroundProgress struct {
	ProjectID string
	Status    string
	Run       Progress
}

// ProgressSnapshots returns the current run progress visible to a workspace.
func (s *BackgroundRuns) ProgressSnapshots(workspace string) []BackgroundProgress {
	s.mu.Lock()
	defer s.mu.Unlock()
	var snapshots []BackgroundProgress
	for scope, entry := range s.latest {
		if scope.workspace == workspace {
			snapshots = append(snapshots, BackgroundProgress{
				ProjectID: scope.project,
				Status:    entry.result.Status,
				Run:       *entry.result.Run,
			})
		}
	}
	return snapshots
}

// WatchProgress signals when a workspace's progress may have changed. Read
// ProgressSnapshots after subscribing and after each signal. Pending signals
// coalesce so a slow reader cannot block a run.
func (s *BackgroundRuns) WatchProgress(workspace string) (<-chan struct{}, func()) {
	changed := make(chan struct{}, 1)
	s.mu.Lock()
	watchers := s.progressWatchers[workspace]
	if watchers == nil {
		watchers = make(map[chan struct{}]struct{})
		s.progressWatchers[workspace] = watchers
	}
	watchers[changed] = struct{}{}
	s.mu.Unlock()
	return changed, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		watchers := s.progressWatchers[workspace]
		delete(watchers, changed)
		if len(watchers) == 0 {
			delete(s.progressWatchers, workspace)
		}
	}
}

// notifyProgress is called with s.mu held.
func (s *BackgroundRuns) notifyProgress(workspace string) {
	for watcher := range s.progressWatchers[workspace] {
		select {
		case watcher <- struct{}{}:
		default:
		}
	}
}
