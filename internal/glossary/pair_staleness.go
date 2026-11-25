package glossary

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
)

type analysisSourceID struct {
	run, requirement string
}

// Match altered spellings against immutable snapshots first. Only materialized
// pairs that contain a matching snapshot can have different glossary hints.
func reconcilePairContext(ctx context.Context, tx *sql.Tx, workspace, projectID string, changed, current []Term) (bool, error) {
	const batchSize = 128
	mentionsChanged := MatchText(changed)
	pairHints := PairHints(current)
	var cursor analysisSourceID
	changedRuns := map[string]struct{}{}
	for {
		rows, err := tx.QueryContext(ctx, `
			SELECT s.run_id,s.requirement_id,s.source->>'text'
			FROM analysis_runs r JOIN analysis_sources s ON s.run_id=r.id
			WHERE r.workspace=$1 AND r.project_id=$2 AND (s.run_id,s.requirement_id)>($3,$4)
			ORDER BY s.run_id,s.requirement_id LIMIT $5`, workspace, projectID, cursor.run, cursor.requirement, batchSize)
		if err != nil {
			return false, err
		}
		var candidates []analysisSourceID
		read := 0
		for rows.Next() {
			var source analysisSourceID
			var text string
			if err := rows.Scan(&source.run, &source.requirement, &text); err != nil {
				rows.Close()
				return false, err
			}
			cursor = source
			read++
			if len(mentionsChanged(text)) > 0 {
				candidates = append(candidates, source)
			}
		}
		if err := errors.Join(rows.Err(), rows.Close()); err != nil {
			return false, err
		}
		for _, candidate := range candidates {
			updated, err := reconcileSourcePairs(ctx, tx, workspace, projectID, candidate, pairHints)
			if err != nil {
				return false, err
			}
			if updated {
				changedRuns[candidate.run] = struct{}{}
			}
		}
		if read < batchSize {
			break
		}
	}
	if len(changedRuns) == 0 {
		return false, nil
	}
	ids := make([]string, 0, len(changedRuns))
	for id := range changedRuns {
		ids = append(ids, id)
	}
	_, err := tx.ExecContext(ctx, `UPDATE analysis_runs SET glossary_revision=glossary_revision+1
		WHERE workspace=$1 AND project_id=$2 AND id=ANY($3::text[])`, workspace, projectID, ids)
	return err == nil, err
}

func reconcileSourcePairs(ctx context.Context, tx *sql.Tx, workspace, projectID string, source analysisSourceID, pairHints func(string, string) []Hint) (bool, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id,left_source->>'text',right_source->>'text',glossary_context,glossary_stale
		FROM (
			SELECT id,left_source,right_source,glossary_context,glossary_stale,selection_reasons,result_origin
			FROM analysis_pairs WHERE workspace=$1 AND project_id=$2 AND run_id=$3 AND left_source->>'id'=$4
			UNION ALL
			SELECT id,left_source,right_source,glossary_context,glossary_stale,selection_reasons,result_origin
			FROM analysis_pairs WHERE workspace=$1 AND project_id=$2 AND run_id=$3 AND right_source->>'id'=$4
				AND left_source->>'id' IS DISTINCT FROM $4
		) candidates WHERE NOT (selection_reasons ? 'exact_text') AND result_origin<>'exact_text_v1'`,
		workspace, projectID, source.run, source.requirement)
	if err != nil {
		return false, err
	}
	var ids []string
	var shouldBeStale []bool
	for rows.Next() {
		var id, left, right string
		var raw []byte
		var wasStale bool
		if err := rows.Scan(&id, &left, &right, &raw, &wasStale); err != nil {
			rows.Close()
			return false, err
		}
		var stored []Hint
		if err := json.Unmarshal(raw, &stored); err != nil {
			rows.Close()
			return false, err
		}
		stale := !slices.Equal(stored, pairHints(left, right))
		if stale != wasStale {
			ids = append(ids, id)
			shouldBeStale = append(shouldBeStale, stale)
		}
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil || len(ids) == 0 {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE analysis_pairs p SET glossary_stale=c.stale
		FROM unnest($3::text[],$4::boolean[]) AS c(id,stale)
		WHERE p.workspace=$1 AND p.project_id=$2 AND p.id=c.id`, workspace, projectID, ids, shouldBeStale)
	return err == nil, err
}
