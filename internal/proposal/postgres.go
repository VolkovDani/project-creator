package proposal

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

const selectProposal = `
	SELECT p.id, p.analysis, p.limitations, p.client, p.model, p.sharing, p.created_at,
	 (SELECT jsonb_agg(jsonb_build_object('requirementId', s."requirementId", 'version', s.version,
	   'status', CASE WHEN r.id IS NULL THEN 'unavailable'
	                  WHEN r.version <> s.version THEN 'stale' ELSE 'current' END) ORDER BY s."requirementId")
	  FROM jsonb_to_recordset(p.sources) AS s("requirementId" text, version int)
	  LEFT JOIN active_requirements r ON r.workspace=p.workspace AND r.project_id=p.project_id AND r.id=s."requirementId")
	FROM external_proposals p WHERE p.workspace=$1 AND p.project_id=$2
`

func (s *Service) save(ctx context.Context, workspace, projectID string, in Input, valid bool) (*Proposal, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = projectAccess(ctx, tx, workspace, projectID, true); err != nil || !valid {
		return nil, err
	}

	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	refs, err := json.Marshal(in.Sources)
	if err != nil {
		return nil, err
	}

	// Take the source locks before inserting a proposal. The insert trigger
	// takes the outbox lock, which also follows requirement changes.
	var sourceCount int
	if err = tx.QueryRowContext(ctx, `
		SELECT count(*) FROM (
		 SELECT r.id FROM requirements r
		 JOIN jsonb_to_recordset($3::jsonb) AS s("requirementId" text, version int)
		   ON r.id=s."requirementId" AND r.version=s.version
		 WHERE r.workspace=$1 AND r.project_id=$2 AND r.archived_at IS NULL
		 ORDER BY r.id FOR SHARE OF r
		) locked
	`, workspace, projectID, string(refs)).Scan(&sourceCount); err != nil {
		return nil, err
	}
	if sourceCount == len(in.Sources) {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO external_proposals(id,workspace,project_id,request_id,analysis,limitations,client,model,sharing,sources)
			VALUES($3,$1,$2,$4,$5,$6,$7,$8,$9,$10::jsonb)
			ON CONFLICT(workspace,project_id,request_id) DO NOTHING
		`, workspace, projectID, hex.EncodeToString(nonce[:]), in.RequestID, in.Analysis, in.Limitations, in.Client, in.Model, in.Sharing, string(refs)); err != nil {
			return nil, err
		}
	}
	var same bool
	err = tx.QueryRowContext(ctx, `
		SELECT analysis=$4 AND limitations=$5 AND client=$6 AND model=$7 AND sharing=$8 AND sources=$9::jsonb
		FROM external_proposals WHERE workspace=$1 AND project_id=$2 AND request_id=$3
	`, workspace, projectID, in.RequestID, in.Analysis, in.Limitations, in.Client, in.Model, in.Sharing, string(refs)).Scan(&same)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errSourceChanged
	}
	if err != nil {
		return nil, err
	}
	if !same {
		return nil, errRequestReused
	}
	saved, err := scanProposal(tx.QueryRowContext(ctx, selectProposal+` AND p.request_id=$3`, workspace, projectID, in.RequestID))
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &saved, nil
}

func (s *Service) list(ctx context.Context, workspace, projectID string, first int, before *time.Time, beforeID string) ([]Proposal, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = projectAccess(ctx, tx, workspace, projectID, false); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, selectProposal+`
		AND ($3::timestamptz IS NULL OR (p.created_at,p.id)<($3,$4))
		ORDER BY p.created_at DESC,p.id DESC LIMIT $5
	`, workspace, projectID, before, beforeID, first)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Proposal{}
	for rows.Next() {
		item, err := scanProposal(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func projectAccess(ctx context.Context, tx *sql.Tx, workspace, projectID string, lock bool) error {
	if workspace == "" || projectID == "" {
		return ErrUnavailable
	}
	query := `SELECT 1 FROM projects WHERE workspace=$1 AND id=$2`
	if lock {
		query += ` FOR KEY SHARE`
	}
	var exists int
	err := tx.QueryRowContext(ctx, query, workspace, projectID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrUnavailable
	}
	return err
}

func scanProposal(row interface{ Scan(...any) error }) (Proposal, error) {
	var p Proposal
	var sources []byte
	if err := row.Scan(&p.ID, &p.Analysis, &p.Limitations, &p.Client, &p.Model, &p.Sharing, &p.CreatedAt, &sources); err != nil {
		return Proposal{}, err
	}
	if err := json.Unmarshal(sources, &p.Sources); err != nil {
		return Proposal{}, err
	}
	return p, nil
}
