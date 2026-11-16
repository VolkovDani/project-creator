package analysis

import (
	"context"
	"database/sql"
	"errors"

	"github.com/VolkovDani/project-creator/contracts"
)

var ErrReplacementUnavailable = errors.New("analysis replacement unavailable")

// Replace records an explicit replacement against an expected run. Repeating
// the same request is idempotent; a newer choice replaces an unconsumed request.
func (s *Service) Replace(ctx context.Context, workspace, project, id string, opts Options, strategy string) (Run, error) {
	if !contracts.ValidateAnalysisSelection(opts.Mode, opts.Threshold) || len(contracts.AnalysisReplacementStrategy(strategy)) > 0 {
		return Run{}, ErrReplacementUnavailable
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Run{}, errStorage
	}
	defer tx.Rollback()
	if err = s.authorize(ctx, tx, workspace, project, true); err != nil {
		return Run{}, err
	}
	r, err := scanRun(tx.QueryRowContext(ctx, `SELECT `+runColumns+` FROM analysis_runs WHERE workspace=$1 AND project_id=$2 AND id=$3 FOR UPDATE`, workspace, project, id))
	if err == sql.ErrNoRows {
		return Run{}, ErrNotFound
	}
	if err != nil {
		return Run{}, errStorage
	}
	var mode, previousStrategy string
	var threshold float64
	var successor sql.NullString
	requestErr := tx.QueryRowContext(ctx, `SELECT mode,threshold,strategy,successor_id FROM analysis_replacements WHERE run_id=$1`, id).Scan(&mode, &threshold, &previousStrategy, &successor)
	if requestErr != nil && requestErr != sql.ErrNoRows {
		return Run{}, errStorage
	}
	if successor.Valid {
		if mode != opts.Mode || threshold != opts.Threshold || previousStrategy != strategy {
			return Run{}, ErrReplacementUnavailable
		}
		next, e := scanRun(tx.QueryRowContext(ctx, `SELECT `+runColumns+` FROM analysis_runs WHERE id=$1`, successor.String))
		if e != nil {
			return Run{}, errStorage
		}
		return *next, nil
	}
	var latest string
	if err = tx.QueryRowContext(ctx, `SELECT id FROM analysis_runs WHERE workspace=$1 AND project_id=$2 ORDER BY seq DESC LIMIT 1`, workspace, project).Scan(&latest); err != nil {
		return Run{}, errStorage
	}
	if latest != id || (r.Status != "running" && requestErr == sql.ErrNoRows) {
		return Run{}, ErrReplacementUnavailable
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO analysis_replacements(run_id,mode,threshold,strategy) VALUES($1,$2,$3,$4) ON CONFLICT(run_id) DO UPDATE SET mode=$2,threshold=$3,strategy=$4`, id, opts.Mode, opts.Threshold, strategy); err != nil {
		return Run{}, errStorage
	}
	if strategy == "cancel" || r.ActivePairID == "" {
		if r.Status == "running" {
			if err = interruptRun(ctx, tx, workspace, r, "REPLACED", false); err != nil {
				return Run{}, err
			}
		}
	} else {
		if _, err = tx.ExecContext(ctx, `UPDATE analysis_runs SET stage='waiting_current_pair' WHERE id=$1`, id); err != nil {
			return Run{}, errStorage
		}
	}
	stored, err := scanRun(tx.QueryRowContext(ctx, `SELECT `+runColumns+` FROM analysis_runs WHERE id=$1`, id))
	if err != nil {
		return Run{}, errStorage
	}
	if err = tx.Commit(); err != nil {
		return Run{}, errStorage
	}
	if strategy == "cancel" {
		s.workersMu.Lock()
		cancel := s.workers[id]
		s.workersMu.Unlock()
		if cancel != nil {
			cancel()
		}
	}
	s.notify()
	return *stored, nil
}

// replacementBoundary runs only between bounded preparation batches or pairs.
// The current pair commits before this check; the next old pair never starts.
func (s *Service) replacementBoundary(ctx context.Context, workspace string, r *Run) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, errStorage
	}
	defer tx.Rollback()
	if err = s.authorize(ctx, tx, workspace, r.ProjectID, true); err != nil {
		return false, err
	}
	if err = fenceRun(ctx, tx, workspace, r, true); err != nil {
		return false, err
	}
	var requested bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM analysis_replacements WHERE run_id=$1 AND successor_id IS NULL)`, r.ID).Scan(&requested); err != nil {
		return false, errStorage
	}
	if !requested {
		return false, nil
	}
	if err = interruptRun(ctx, tx, workspace, r, "REPLACED", true); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, errStorage
	}
	return true, nil
}

func (s *Service) activateReplacement() {
	var workspace, project, id string
	var opts Options
	err := s.db.QueryRowContext(s.ctx, `SELECT r.workspace,r.project_id,r.id,q.mode,q.threshold FROM analysis_replacements q JOIN analysis_runs r ON r.id=q.run_id WHERE q.successor_id IS NULL AND r.status<>'running' AND r.id=(SELECT id FROM analysis_runs latest WHERE latest.workspace=r.workspace AND latest.project_id=r.project_id ORDER BY seq DESC LIMIT 1) ORDER BY r.seq LIMIT 1`).Scan(&workspace, &project, &id, &opts.Mode, &opts.Threshold)
	if err == nil {
		_, _ = s.startWithOptions(s.ctx, workspace, project, opts, id)
	}
}
