package analysis

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// Pair loads one authorized pair detail without paging through its run.
func (s *Service) Pair(ctx context.Context, workspace, project, runID, pairID string) (PairResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PairResult{}, errStorage
	}
	defer tx.Rollback()
	if err = s.authorize(ctx, tx, workspace, project, false); err != nil {
		return PairResult{}, err
	}
	var raw []byte
	if err = tx.QueryRowContext(ctx, `SELECT `+pairJSON+` FROM analysis_pairs p WHERE workspace=$1 AND project_id=$2 AND run_id=$3 AND id=$4`, workspace, project, runID, pairID).Scan(&raw); err == sql.ErrNoRows {
		return PairResult{}, ErrNotFound
	} else if err != nil {
		return PairResult{}, errStorage
	}
	var pair PairResult
	if err = json.Unmarshal(raw, &pair); err != nil {
		return PairResult{}, errStorage
	}
	if err = tx.Commit(); err != nil {
		return PairResult{}, errStorage
	}
	return pair, nil
}

func jsonMarshal(value any) (string, error) { b, e := json.Marshal(value); return string(b), e }
func (s *Service) Pairs(ctx context.Context, workspace, project, runID string, limit, offset int, status, verdict string) ([]PairResult, error) {
	if !validPage(limit, offset) {
		return nil, errors.New("invalid analysis page")
	}
	if status != "" && status != "pending" && status != "running" && status != "completed" && status != "failed" {
		return nil, errors.New("invalid analysis status")
	}
	if verdict != "" && verdict != "collision" && verdict != "compatible" && verdict != "insufficient" && verdict != "duplicate" {
		return nil, errors.New("invalid analysis verdict")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, errStorage
	}
	defer tx.Rollback()
	if err = s.authorize(ctx, tx, workspace, project, true); err != nil {
		return nil, err
	}
	var id string
	if err = tx.QueryRowContext(ctx, `SELECT id FROM analysis_runs WHERE workspace=$1 AND project_id=$2 AND id=$3`, workspace, project, runID).Scan(&id); err == sql.ErrNoRows {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, errStorage
	}
	rows, err := tx.QueryContext(ctx, `SELECT `+pairJSON+` FROM analysis_pairs p WHERE workspace=$1 AND project_id=$2 AND run_id=$3 AND ($4='' OR status=$4 OR ($4='pending' AND status='running')) AND ($5='' OR verdict=$5) ORDER BY position LIMIT $6 OFFSET $7`, workspace, project, runID, status, verdict, limit, offset)
	if err != nil {
		return nil, errStorage
	}
	out := []PairResult{}
	for rows.Next() {
		var raw []byte
		var p PairResult
		if rows.Scan(&raw) != nil || json.Unmarshal(raw, &p) != nil {
			rows.Close()
			return nil, errStorage
		}
		out = append(out, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, errStorage
	}
	if err = tx.Commit(); err != nil {
		return nil, errStorage
	}
	return out, nil
}

// PairsConnection reads an authorized keyset page and the full authorized count.
// Position is immutable for a run; id is the unique deterministic tie-breaker.
func (s *Service) PairsConnection(ctx context.Context, workspace, project, runID string, first int, status, verdict string, after bool, position int64, id string) ([]PairResult, int, bool, error) {
	if first < 1 || first > 100 || (status != "" && status != "pending" && status != "running" && status != "completed" && status != "failed") || (verdict != "" && verdict != "collision" && verdict != "compatible" && verdict != "insufficient" && verdict != "duplicate") {
		return nil, 0, false, errors.New("invalid analysis page")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, 0, false, errStorage
	}
	defer tx.Rollback()
	if err = s.authorize(ctx, tx, workspace, project, false); err != nil {
		return nil, 0, false, err
	}
	var run string
	if err = tx.QueryRowContext(ctx, `SELECT id FROM analysis_runs WHERE workspace=$1 AND project_id=$2 AND id=$3`, workspace, project, runID).Scan(&run); err == sql.ErrNoRows {
		return nil, 0, false, ErrNotFound
	} else if err != nil {
		return nil, 0, false, errStorage
	}
	var total int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM analysis_pairs WHERE workspace=$1 AND project_id=$2 AND run_id=$3 AND ($4='' OR status=$4 OR ($4='pending' AND status='running')) AND ($5='' OR verdict=$5)`, workspace, project, runID, status, verdict).Scan(&total); err != nil {
		return nil, 0, false, errStorage
	}
	query := `SELECT ` + pairJSON + ` FROM analysis_pairs p WHERE workspace=$1 AND project_id=$2 AND run_id=$3 AND ($4='' OR status=$4 OR ($4='pending' AND status='running')) AND ($5='' OR verdict=$5)`
	var rows *sql.Rows
	if after {
		rows, err = tx.QueryContext(ctx, query+` AND (position,id)>($6,$7) ORDER BY position,id LIMIT $8`, workspace, project, runID, status, verdict, position, id, first+1)
	} else {
		rows, err = tx.QueryContext(ctx, query+` ORDER BY position,id LIMIT $6`, workspace, project, runID, status, verdict, first+1)
	}
	if err != nil {
		return nil, 0, false, errStorage
	}
	out := []PairResult{}
	for rows.Next() {
		var raw []byte
		var p PairResult
		if rows.Scan(&raw) != nil || json.Unmarshal(raw, &p) != nil {
			rows.Close()
			return nil, 0, false, errStorage
		}
		out = append(out, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, 0, false, errStorage
	}
	hasNext := len(out) > first
	if hasNext {
		out = out[:first]
	}
	if err = tx.Commit(); err != nil {
		return nil, 0, false, errStorage
	}
	return out, total, hasNext, nil
}
