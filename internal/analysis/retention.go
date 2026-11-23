package analysis

import (
	"context"
	"database/sql"
	"errors"
)

var ErrDeletionUnavailable = errors.New("analysis deletion unavailable")

func (s *Service) RetentionDays() int {
	if days := s.retentionDays.Load(); days > 0 {
		return int(days)
	}
	return DefaultRetentionDays
}

// SetRetentionDays applies to future marks. Previously stored deadlines stay fixed.
func (s *Service) SetRetentionDays(days int) {
	if days > 0 && days <= MaxRetentionDays {
		s.retentionDays.Store(int64(days))
	}
}

func (s *Service) ScheduleDeletion(ctx context.Context, workspace, project, id string) error {
	return s.changeDeletion(ctx, workspace, project, id, false)
}

func (s *Service) DeleteMarked(ctx context.Context, workspace, project, id string) error {
	return s.changeDeletion(ctx, workspace, project, id, true)
}

func (s *Service) changeDeletion(ctx context.Context, workspace, project, id string, deleteNow bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errStorage
	}
	defer tx.Rollback()
	if err = s.authorize(ctx, tx, workspace, project, true); err != nil {
		return err
	}
	var status string
	var marked bool
	err = tx.QueryRowContext(ctx, `SELECT status,deletion_scheduled_for IS NOT NULL FROM analysis_runs
		WHERE workspace=$1 AND project_id=$2 AND id=$3 FOR UPDATE`, workspace, project, id).Scan(&status, &marked)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return errStorage
	}
	if status == "running" || (deleteNow && !marked) {
		return ErrDeletionUnavailable
	}
	if deleteNow {
		_, err = tx.ExecContext(ctx, `DELETE FROM analysis_runs WHERE workspace=$1 AND project_id=$2 AND id=$3`, workspace, project, id)
	} else if !marked {
		_, err = tx.ExecContext(ctx, `UPDATE analysis_runs SET deletion_scheduled_for=clock_timestamp()+$4 * interval '24 hours'
			WHERE workspace=$1 AND project_id=$2 AND id=$3`, workspace, project, id, s.RetentionDays())
	}
	if err != nil || tx.Commit() != nil {
		return errStorage
	}
	return nil
}

// PurgeExpired limits the number of runs held by each transaction. Multiple
// service instances can sweep simultaneously without waiting on each other's rows.
func (s *Service) PurgeExpired(ctx context.Context) error {
	const batchSize = 32
	for {
		result, err := s.db.ExecContext(ctx, `DELETE FROM analysis_runs USING
			(SELECT r.id FROM projects p JOIN analysis_runs r ON r.workspace=p.workspace AND r.project_id=p.id
			 WHERE r.deletion_scheduled_for <= statement_timestamp()
			 ORDER BY r.deletion_scheduled_for,r.id LIMIT $1 FOR UPDATE OF p,r SKIP LOCKED) expired
			WHERE analysis_runs.id=expired.id`, batchSize)
		if err != nil {
			return err
		}
		deleted, err := result.RowsAffected()
		if err != nil || deleted < batchSize {
			return err
		}
	}
}
