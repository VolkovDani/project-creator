// Package processgraphcache stores completed process graph drafts in PostgreSQL.
package processgraphcache

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/VolkovDani/project-creator/internal/processgraph"
)

// Bump this version when stored results become incompatible with the current
// encoding or generation behavior. The operator can still clear older formats.
const formatVersion = 1

type Store struct {
	db      *sql.DB
	profile string
}

func New(db *sql.DB, profile string) *Store {
	return &Store{db: db, profile: profile}
}

// Reuse promotes an existing successful result in place. An update that races
// with a clear can use the row or miss, but cannot reinsert the cleared draft.
func (s *Store) Reuse(ctx context.Context, target processgraph.Target, snapshotKey, runID string) (processgraph.RunResult, error) {
	var encoded []byte
	err := s.db.QueryRowContext(ctx, `
		UPDATE process_graph_cache
		SET result=jsonb_set(result,'{Run,ID}',to_jsonb($8::text)), latest_order=DEFAULT
		WHERE workspace=$1 AND project_id=$2 AND profile=$3 AND format_version=$4
		  AND snapshot_key=$5 AND term_id=$6 AND keyword=$7 AND reusable
		RETURNING result
	`, target.Workspace, target.Project, s.profile, formatVersion, snapshotKey, target.TermID, target.Keyword, runID).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return processgraph.RunResult{}, nil
	}
	if err != nil {
		return processgraph.RunResult{}, err
	}
	var result processgraph.RunResult
	if err := json.Unmarshal(encoded, &result); err != nil {
		return processgraph.RunResult{}, err
	}
	return result, nil
}

func (s *Store) Latest(ctx context.Context, workspace, project string) (processgraph.CachedRun, error) {
	run := processgraph.CachedRun{Target: processgraph.Target{Workspace: workspace, Project: project}}
	var encoded []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT term_id,keyword,snapshot_key,result
		FROM process_graph_cache
		WHERE workspace=$1 AND project_id=$2 AND profile=$3 AND format_version=$4
		ORDER BY latest_order DESC LIMIT 1
	`, workspace, project, s.profile, formatVersion).Scan(&run.Target.TermID, &run.Target.Keyword, &run.SnapshotKey, &encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return processgraph.CachedRun{}, nil
	}
	if err != nil {
		return processgraph.CachedRun{}, err
	}
	if err := json.Unmarshal(encoded, &run.Result); err != nil {
		return processgraph.CachedRun{}, err
	}
	return run, nil
}

func (s *Store) Save(ctx context.Context, run processgraph.CachedRun) error {
	result := run.Result
	if result.Status != "suggested" || result.Run == nil || result.Draft == nil {
		return nil
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return err
	}
	// Keep a reusable draft even when a concurrent run for the same snapshot
	// completes later with failures and becomes the latest displayed draft.
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO process_graph_cache(workspace,project_id,profile,format_version,snapshot_key,reusable,term_id,keyword,result)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT(workspace,project_id,profile,format_version,snapshot_key,reusable)
		DO UPDATE SET result=EXCLUDED.result,latest_order=DEFAULT
	`, run.Target.Workspace, run.Target.Project, s.profile, formatVersion, run.SnapshotKey,
		result.Run.Failed == 0, run.Target.TermID, run.Target.Keyword, encoded)
	return err
}

// Count reports rows across all profiles and formats for a project. An empty
// project ID counts the whole installation.
func Count(ctx context.Context, db *sql.DB, projectID string) (int64, error) {
	var count int64
	err := db.QueryRowContext(ctx, `SELECT count(*) FROM process_graph_cache WHERE $1='' OR project_id=$1`, projectID).Scan(&count)
	return count, err
}

// Clear removes stored drafts and publishes project invalidations in the same
// transaction through the migration's delete trigger.
func Clear(ctx context.Context, db *sql.DB, projectID string) (int64, error) {
	result, err := db.ExecContext(ctx, `DELETE FROM process_graph_cache WHERE $1='' OR project_id=$1`, projectID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
