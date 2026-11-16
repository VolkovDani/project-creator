package analysis

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// Profile is the complete comparison identity used by cache and SSE staleness.
func (s *Service) Profile() string { return s.generatorProfile() }
func (s *Service) comparisonProvenance() string {
	if s.classifier == nil {
		return GeneratorProvenance(s.generator)
	}
	classifier := s.classifier.Profile()
	if p, ok := s.classifier.(interface{ Provenance() string }); ok {
		classifier = p.Provenance()
	}
	raw, _ := jsonMarshal(map[string]any{"adapter": "laya-llm-pipeline-v2", "generator": GeneratorProvenance(s.generator), "classifier": classifier, "fallbackPolicy": "final-llm-on-laya-failure-with-full-originals-v1"})
	return raw
}

// Each loop holds at most one provider call and obtains its input from committed
// rows. The run lease and coordinator session fence other backend instances.
func (s *Service) processPipeline(ctx context.Context, workspace string, r *Run) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	layaDone := make(chan struct{})
	ready := make(chan struct{}, 1)
	results := make(chan error, 2)
	go func() {
		defer close(layaDone)
		for {
			p, err := s.claimPipelinePair(ctx, workspace, r, true)
			if err != nil || p == nil {
				if err != nil && err.Error() == "REPLACED" {
					err = nil
				}
				results <- err
				return
			}
			callCtx, stop := context.WithTimeout(ctx, s.timeout)
			var result Preclassification
			var callErr error
			if scoped, ok := s.classifier.(interface {
				ClassifyScoped(context.Context, string, string, Pair) (Preclassification, error)
			}); ok {
				result, callErr = scoped.ClassifyScoped(callCtx, workspace, r.ProjectID, Pair{Left: p.Left, Right: p.Right})
			} else {
				result, callErr = s.classifier.Classify(callCtx, Pair{Left: p.Left, Right: p.Right})
			}
			code := ""
			if callErr != nil {
				code = "CLASSIFICATION_FAILED"
				if errors.Is(callErr, ErrSummaryUnavailable) {
					code = "SUMMARY_UNAVAILABLE"
				}
				if errors.Is(callErr, ErrClassificationInputTooLong) {
					code = "INPUT_TOO_LONG"
				}
			}
			if callCtx.Err() != nil {
				code = errorCode(callCtx.Err())
			}
			stop()
			if ctx.Err() != nil {
				results <- ctx.Err()
				return
			}
			if code == "" && !validPreclassification(result) {
				code = "INVALID_OUTPUT"
			}
			if err = s.saveClassification(ctx, workspace, r, p, result, code); err != nil {
				results <- err
				return
			}
			select {
			case ready <- struct{}{}:
			default:
			}
		}
	}()
	go func() {
		for {
			p, err := s.claimPipelinePair(ctx, workspace, r, false)
			if err != nil {
				results <- err
				return
			}
			if p == nil {
				select {
				case <-layaDone:
					// Final recheck: a commit may race with the empty read above.
					p, err = s.claimPipelinePair(ctx, workspace, r, false)
					if err != nil || p == nil {
						results <- err
						return
					}
				case <-ctx.Done():
					results <- ctx.Err()
					return
				case <-ready:
					continue
				}
			}
			pair := Pair{Left: p.Left, Right: p.Right, Preclassification: p.Preclassification}
			callCtx, stop := context.WithTimeout(ctx, s.timeout)
			callCtx = context.WithValue(callCtx, npuCallKey{}, npuCall{RunID: r.ID, PairID: p.ID})
			var finding Finding
			code := ""
			origin := "model"
			if exactDuplicate(pair) {
				origin = exactOrigin
				finding = Finding{LeftID: pair.Left.ID, RightID: pair.Right.ID, LeftVersion: pair.Left.Version, RightVersion: pair.Right.Version, Verdict: "duplicate", Explanation: "analysis.exactDuplicateExplanation"}
			} else {
				finding, code = checkedCompare(callCtx, s.generator, pair)
			}
			stop()
			if err = s.savePipelineFinding(ctx, workspace, r, p, finding, code, origin); err != nil {
				results <- err
				return
			}
		}
	}()
	var first error
	for range 2 {
		if err := <-results; err != nil && first == nil {
			first = err
			cancel()
		}
	}
	return first
}

func validPreclassification(p Preclassification) bool {
	if p.Decision != "related" && p.Decision != "unrelated" && p.Decision != "uncertain" {
		return false
	}
	if len(p.Scores) != 3 {
		return false
	}
	sum := 0.0
	for _, key := range []string{"related", "unrelated", "uncertain"} {
		v, ok := p.Scores[key]
		if !ok || !(v >= 0 && v <= 1) {
			return false
		}
		sum += v
	}
	return sum > .999999 && sum < 1.000001
}

func (s *Service) pipelineTransaction(ctx context.Context, workspace string, r *Run) (*sql.Tx, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, errStorage
	}
	if err = s.authorize(ctx, tx, workspace, r.ProjectID, true); err == nil {
		err = fenceRun(ctx, tx, workspace, r, true)
	}
	if err == nil {
		err = s.currentWith(ctx, tx, workspace, r)
	}
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}
func (s *Service) claimPipelinePair(ctx context.Context, workspace string, r *Run, laya bool) (*PairResult, error) {
	tx, err := s.pipelineTransaction(ctx, workspace, r)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var replacement bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM analysis_replacements WHERE run_id=$1 AND successor_id IS NULL)`, r.ID).Scan(&replacement); err != nil {
		return nil, errStorage
	}
	if replacement {
		return nil, errors.New("REPLACED")
	}
	eligibility := `laya_status IN ('completed','failed','skipped')`
	if laya {
		eligibility = `laya_status='pending'`
	}
	var raw []byte
	err = tx.QueryRowContext(ctx, `SELECT `+pairJSON+` FROM analysis_pairs p WHERE run_id=$1 AND workspace=$2 AND project_id=$3 AND status='pending' AND `+eligibility+` ORDER BY position LIMIT 1 FOR UPDATE`, r.ID, workspace, r.ProjectID).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, errStorage
	}
	var p PairResult
	if json.Unmarshal(raw, &p) != nil {
		return nil, errStorage
	}
	query := `UPDATE analysis_pairs SET status='running',error_code='' WHERE id=$1`
	if laya {
		query = `UPDATE analysis_pairs SET laya_status='running',error_code='' WHERE id=$1`
	} else {
		query = `UPDATE analysis_pairs SET status='running',error_code='' WHERE id=$1 AND laya_status IN ('completed','failed','skipped')`
	}
	if _, err = tx.ExecContext(ctx, query, p.ID); err != nil {
		return nil, errStorage
	}
	if err = touchPipeline(ctx, tx, r, false); err != nil {
		return nil, err
	}
	if tx.Commit() != nil {
		return nil, errStorage
	}
	return &p, nil
}
func touchPipeline(ctx context.Context, tx *sql.Tx, r *Run, processed bool) error {
	delta := 0
	if processed {
		delta = 1
	}
	// Never write a goroutine's stale Run snapshot over another stage's progress.
	_, err := tx.ExecContext(ctx, `UPDATE analysis_runs SET classifier_tick=classifier_tick+1,processed_pairs=processed_pairs+$2,active_pair_id=coalesce((SELECT id FROM analysis_pairs WHERE run_id=$1 AND (status='running' OR laya_status='running') ORDER BY (status='running') DESC,position LIMIT 1),'') WHERE id=$1`, r.ID, delta)
	if err != nil {
		return errStorage
	}
	return nil
}
func (s *Service) saveClassification(ctx context.Context, workspace string, r *Run, p *PairResult, result Preclassification, code string) error {
	tx, err := s.pipelineTransaction(ctx, workspace, r)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var update sql.Result
	if code != "" {
		status := "failed"
		if code == "INPUT_TOO_LONG" || code == "SUMMARY_UNAVAILABLE" {
			status = "skipped"
		}
		update, err = tx.ExecContext(ctx, `UPDATE analysis_pairs SET laya_status=$2,laya_error_code=$3,laya_checked_at=clock_timestamp() WHERE id=$1 AND run_id=$4 AND laya_status='running' AND status='pending'`, p.ID, status, "LAYA_"+code, r.ID)
	} else {
		raw, _ := jsonMarshal(result)
		update, err = tx.ExecContext(ctx, `UPDATE analysis_pairs SET laya_status='completed',laya_error_code='',laya_result=$2::jsonb,laya_checked_at=clock_timestamp() WHERE id=$1 AND run_id=$3 AND laya_status='running' AND status='pending'`, p.ID, raw, r.ID)
	}
	if err != nil {
		return errStorage
	}
	if n, err := update.RowsAffected(); err != nil || n != 1 {
		return errOwnership
	}
	if err = touchPipeline(ctx, tx, r, false); err != nil {
		return err
	}
	if tx.Commit() != nil {
		return errStorage
	}
	return nil
}
func (s *Service) savePipelineFinding(ctx context.Context, workspace string, r *Run, p *PairResult, f Finding, code, origin string) error {
	tx, err := s.pipelineTransaction(ctx, workspace, r)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	status := "completed"
	if code != "" {
		status = "failed"
	}
	var raw any
	if p.Preclassification != nil {
		encoded, _ := jsonMarshal(p.Preclassification)
		raw = string(encoded)
	}
	updated, err := tx.ExecContext(ctx, `UPDATE analysis_pairs SET status=$2,verdict=$3,explanation=$4,error_code=$5,checked_at=CASE WHEN $2='completed' THEN clock_timestamp() ELSE NULL END,result_origin=$8 WHERE id=$1 AND run_id=$6 AND status='running' AND laya_status IN ('completed','failed','skipped') AND laya_result IS NOT DISTINCT FROM $7::jsonb`, p.ID, status, f.Verdict, f.Explanation, code, r.ID, raw, origin)
	if err != nil {
		return errStorage
	}
	if n, err := updated.RowsAffected(); err != nil || n != 1 {
		return errOwnership
	}
	if err = touchPipeline(ctx, tx, r, true); err != nil {
		return err
	}
	if tx.Commit() != nil {
		return errStorage
	}
	return nil
}
