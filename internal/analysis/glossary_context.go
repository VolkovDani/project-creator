package analysis

import (
	"context"
	"errors"
)

const glossaryChangedCode = "GLOSSARY_CHANGED"

var errGlossaryChanged = errors.New(glossaryChangedCode)

// The caller holds the same scoped project lock as glossary writes. An empty
// pair ID checks the run before another pair is dispatched.
func checkGlossaryContext(ctx context.Context, q queryer, workspace string, run *Run, pairID string) error {
	var stale bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT FROM analysis_pairs WHERE run_id=$1 AND workspace=$2 AND project_id=$3
		AND glossary_stale AND ($4='' OR id=$4))`, run.ID, workspace, run.ProjectID, pairID).Scan(&stale)
	if err != nil {
		return errStorage
	}
	if stale {
		return errGlossaryChanged
	}
	return nil
}
