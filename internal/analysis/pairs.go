package analysis

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

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
