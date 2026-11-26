package processgraph

import "context"

// CachedRun ties a completed graph to the sources that supported it.
type CachedRun struct {
	Target      Target
	SnapshotKey string
	Result      RunResult
}

type CompletedCache interface {
	Reuse(ctx context.Context, target Target, snapshotKey, runID string) (RunResult, error)
	Latest(ctx context.Context, workspace, project string) (CachedRun, error)
	Save(ctx context.Context, run CachedRun) error
}
