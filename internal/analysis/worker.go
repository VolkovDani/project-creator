package analysis

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// One session lock covers the complete job, including retrieval. No SQL
// transaction is held while a provider runs. Other instances can enqueue/read
// progress, but cannot issue competing generation requests.
const modelLock = 724183093

func (s *Service) work() {
	defer close(s.done)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}
		if s.workOnce() {
			continue
		}
		select {
		case <-s.ctx.Done():
			return
		case <-s.wake:
		case <-ticker.C:
		}
	}
}
func (s *Service) workOnce() bool {
	conn, err := s.db.Conn(s.ctx)
	if err != nil {
		return false
	}
	defer conn.Close()
	var locked bool
	if err = conn.QueryRowContext(s.ctx, `SELECT pg_try_advisory_lock($1)`, modelLock).Scan(&locked); err != nil || !locked {
		return false
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var unlocked bool
		if err := conn.QueryRowContext(ctx, `SELECT pg_advisory_unlock($1)`, modelLock).Scan(&unlocked); err != nil || !unlocked {
			// Never return an uncertain session lock to the connection pool.
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	s.reap()
	s.activateReplacement()
	workspace, r, err := s.claim(conn)
	if err != nil || r == nil {
		return false
	}
	s.process(conn, workspace, r)
	return true
}
func (s *Service) reap() {
	rows, err := s.db.QueryContext(s.ctx, `SELECT workspace,project_id FROM analysis_runs WHERE status='running' AND ((worker_id<>'' AND lease_until<=clock_timestamp()) OR (worker_id='' AND (generator_profile<>$1 OR (mode='related' AND embedding_profile<>$2))))`, s.generatorProfile(), s.embeddingProfile)
	if err != nil {
		return
	}
	type scope struct{ w, p string }
	var scopes []scope
	for rows.Next() {
		var v scope
		if rows.Scan(&v.w, &v.p) == nil {
			scopes = append(scopes, v)
		}
	}
	rows.Close()
	for _, v := range scopes {
		tx, err := s.db.BeginTx(s.ctx, nil)
		if err != nil {
			return
		}
		if s.authorize(s.ctx, tx, v.w, v.p, true) == nil && s.recoverProject(s.ctx, tx, v.w, v.p) == nil {
			_ = tx.Commit()
		} else {
			_ = tx.Rollback()
		}
	}
}
func (s *Service) claim(conn *sql.Conn) (string, *Run, error) {
	tx, err := conn.BeginTx(s.ctx, nil)
	if err != nil {
		return "", nil, err
	}
	defer tx.Rollback()
	// A lost lock session can be replaced before the previous adapter returns
	// from cancellation. Its valid lease fences every other queued job until
	// it has actually persisted completion/interruption (or its lease expires).
	var active bool
	if err = tx.QueryRowContext(s.ctx, `SELECT EXISTS(SELECT 1 FROM analysis_runs WHERE status='running' AND worker_id<>'' AND lease_until>clock_timestamp())`).Scan(&active); err != nil {
		return "", nil, err
	}
	if active {
		return "", nil, nil
	}
	var workspace, id string
	err = tx.QueryRowContext(s.ctx, `SELECT workspace,id FROM analysis_runs WHERE status='running' AND worker_id='' AND generator_profile=$1 AND (mode='all' OR embedding_profile=$2) ORDER BY seq LIMIT 1 FOR UPDATE SKIP LOCKED`, s.generatorProfile(), s.embeddingProfile).Scan(&workspace, &id)
	if err == sql.ErrNoRows {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	owner, err := identifier()
	if err != nil {
		return "", nil, err
	}
	if _, err = tx.ExecContext(s.ctx, `UPDATE analysis_runs SET worker_id=$2,lease_until=clock_timestamp()+interval '30 seconds' WHERE id=$1`, id, owner); err != nil {
		return "", nil, err
	}
	r, err := scanRun(tx.QueryRowContext(s.ctx, `SELECT `+runColumns+` FROM analysis_runs WHERE id=$1 AND workspace=$2`, id, workspace))
	if err != nil {
		return "", nil, err
	}
	if err = tx.Commit(); err != nil {
		return "", nil, err
	}
	return workspace, r, nil
}
func (s *Service) process(lockConnection *sql.Conn, workspace string, r *Run) {
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	s.workersMu.Lock()
	s.workers[r.ID] = cancel
	s.workersMu.Unlock()
	defer func() {
		s.workersMu.Lock()
		delete(s.workers, r.ID)
		s.workersMu.Unlock()
	}()
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(s.heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				beatCtx, beatCancel := context.WithTimeout(ctx, 3*time.Second)
				if err := lockConnection.PingContext(beatCtx); err != nil {
					beatCancel()
					cancel()
					return
				}
				result, err := s.db.ExecContext(beatCtx, `UPDATE analysis_runs SET lease_until=clock_timestamp()+interval '30 seconds' WHERE workspace=$1 AND project_id=$2 AND id=$3 AND status='running' AND worker_id=$4 AND lease_until>clock_timestamp()`, workspace, r.ProjectID, r.ID, r.workerID)
				beatCancel()
				if err != nil {
					cancel()
					return
				}
				n, err := result.RowsAffected()
				if err != nil || n != 1 {
					cancel()
					return
				}
			}
		}
	}()
	defer func() { cancel(); <-heartbeatDone }()

	finish := func(code string) {
		finishCtx, finishCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer finishCancel()
		tx, err := s.db.BeginTx(finishCtx, nil)
		if err != nil {
			return
		}
		defer tx.Rollback()
		if s.authorize(finishCtx, tx, workspace, r.ProjectID, true) != nil || fenceRun(finishCtx, tx, workspace, r, true) != nil {
			return
		}

		// Re-read committed preparation state: even a lost COMMIT acknowledgement
		// must never let failure handling overwrite a durable queue cursor.
		stored, readErr := scanRun(tx.QueryRowContext(finishCtx, `SELECT `+runColumns+` FROM analysis_runs WHERE id=$1`, r.ID))
		if readErr != nil {
			return
		}
		reason, missing := r.ReasonCode, r.MissingRequirementIDs
		*r = *stored
		if reason != "" {
			r.ReasonCode = reason
			r.MissingRequirementIDs = missing
		}
		if code != "" {
			if ctx.Err() != nil {
				code = "INTERRUPTED"
			}
			if interruptRun(finishCtx, tx, workspace, r, code, true) != nil {
				return
			}
		} else {
			if err = s.currentWith(finishCtx, tx, workspace, r); err != nil {
				if interruptRun(finishCtx, tx, workspace, r, errorCode(err), true) != nil {
					return
				}
			} else {
				var collision, insufficient, completed int
				if tx.QueryRowContext(finishCtx, `SELECT count(*) FILTER(WHERE status='completed' AND verdict='collision'),count(*) FILTER(WHERE status='completed' AND verdict='insufficient'),count(*) FILTER(WHERE status='completed') FROM analysis_pairs WHERE run_id=$1`, r.ID).Scan(&collision, &insufficient, &completed) != nil {
					return
				}
				r.Status = "completed"
				r.Stage = "finished"
				r.ActivePairID = ""
				r.Outcome = "no_collisions_found"
				if collision > 0 {
					r.Outcome = "possible_collision"
				} else if insufficient > 0 || completed == 0 {
					r.Outcome = "insufficient"
				}
				if insufficient > 0 {
					r.ReasonCode = "MODEL_INSUFFICIENT"
				} else if completed == 0 && r.ReasonCode == "" {
					r.ReasonCode = "NO_CANDIDATES"
				}
				if writeRun(finishCtx, tx, workspace, r, true) != nil {
					return
				}
			}
		}
		_ = tx.Commit()
	}
	if !r.fullQueue {
		finish("SOURCE_CHANGED")
		return
	}
	if r.TotalPairs == nil {
		// The budget applies to one bounded preparation transaction, not the entire
		// project; a large queue may span arbitrarily many committed batches.
		for {
			if stop, e := s.replacementBoundary(ctx, workspace, r); stop || e != nil {
				if e != nil {
					finish(errorCode(e))
				}
				return
			}
			prepCtx, prepCancel := context.WithTimeout(ctx, s.preparationTimeout)
			done, err := s.prepareBatch(prepCtx, workspace, r)
			prepCancel()
			if err != nil {
				finish(errorCode(err))
				return
			}
			if done {
				break
			}
		}
	}
	for {
		p, err := s.nextPair(ctx, workspace, r)
		if err != nil {
			finish(errorCode(err))
			return
		}
		if p == nil {
			finish("")
			return
		}
		pair := Pair{Left: p.Left, Right: p.Right}
		var finding Finding
		code := ""
		origin := "model"
		if exactDuplicate(pair) {
			origin = exactOrigin
			finding = Finding{LeftID: pair.Left.ID, RightID: pair.Right.ID, LeftVersion: pair.Left.Version, RightVersion: pair.Right.Version, Verdict: "duplicate", Explanation: "analysis.exactDuplicateExplanation"}
		} else if s.generator == nil || s.generatorProfile() == "" {
			code = "NOT_CONFIGURED"
		} else {
			pairCtx, pairCancel := context.WithTimeout(ctx, s.timeout)
			pairCtx = context.WithValue(pairCtx, npuCallKey{}, npuCall{RunID: r.ID, PairID: p.ID})
			finding, code = checkedCompare(pairCtx, s.generator, pair)
			pairCancel()
		}
		if code != "" {
			finish(code)
			return
		}
		if err = s.savePair(ctx, workspace, r, p, finding, origin); err != nil {
			finish(errorCode(err))
			return
		}
	}
}
func errorCode(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "TIMEOUT"
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, errOwnership) {
		return "INTERRUPTED"
	}
	switch err.Error() {
	case "SOURCE_CHANGED", "PROFILE_CHANGED", "INDEX_INCOMPLETE", "QUEUE_TOO_LARGE", "REPLACED":
		return err.Error()
	}
	return "STORAGE_FAILED"
}
func (s *Service) current(ctx context.Context, workspace string, r *Run) error {
	return s.currentWith(ctx, s.db, workspace, r)
}
func (s *Service) currentWith(ctx context.Context, q queryer, workspace string, r *Run) error {
	if err := s.authorize(ctx, q, workspace, r.ProjectID, false); err != nil {
		return err
	}
	if r.GeneratorProfile != s.generatorProfile() || (r.Mode == "related" && r.EmbeddingProfile != s.embeddingProfile) {
		return errors.New("PROFILE_CHANGED")
	}
	signature, err := sourceSignature(ctx, q, workspace, r.ProjectID)
	if err != nil {
		return err
	}
	if signature != r.signature {
		return errors.New("SOURCE_CHANGED")
	}
	return nil
}
func (s *Service) nextPair(ctx context.Context, workspace string, r *Run) (*PairResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, errStorage
	}
	defer tx.Rollback()
	if err = s.authorize(ctx, tx, workspace, r.ProjectID, true); err != nil {
		return nil, err
	}
	if err = fenceRun(ctx, tx, workspace, r, true); err != nil {
		return nil, err
	}
	if err = s.currentWith(ctx, tx, workspace, r); err != nil {
		return nil, err
	}
	var replacement bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM analysis_replacements WHERE run_id=$1 AND successor_id IS NULL)`, r.ID).Scan(&replacement); err != nil {
		return nil, errStorage
	}
	if replacement {
		return nil, errors.New("REPLACED")
	}
	var raw []byte
	err = tx.QueryRowContext(ctx, `SELECT `+pairJSON+` FROM analysis_pairs p WHERE run_id=$1 AND workspace=$2 AND project_id=$3 AND status='pending' ORDER BY position LIMIT 1 FOR UPDATE`, r.ID, workspace, r.ProjectID).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, errStorage
	}
	p := &PairResult{}
	if json.Unmarshal(raw, p) != nil {
		return nil, errStorage
	}
	if _, err = tx.ExecContext(ctx, `UPDATE analysis_pairs SET status='running',error_code='' WHERE id=$1 AND status='pending'`, p.ID); err != nil {
		return nil, errStorage
	}
	r.ActivePairID = p.ID
	if err = writeRun(ctx, tx, workspace, r, true); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, errStorage
	}
	return p, nil
}
func (s *Service) savePair(ctx context.Context, workspace string, r *Run, p *PairResult, f Finding, origin string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errStorage
	}
	defer tx.Rollback()
	if err = s.authorize(ctx, tx, workspace, r.ProjectID, true); err != nil {
		return err
	}
	if err = fenceRun(ctx, tx, workspace, r, true); err != nil {
		return err
	}
	// This check is inside the commit transaction, after inference, while source
	// edits are fenced by the same project lock. Late responses cannot enter cache.
	if err = s.currentWith(ctx, tx, workspace, r); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE analysis_pairs SET status='completed',verdict=$2,explanation=$3,result_origin=$4,checked_at=clock_timestamp(),error_code='' WHERE id=$1 AND run_id=$5 AND workspace=$6 AND project_id=$7 AND status='running'`, p.ID, f.Verdict, f.Explanation, origin, r.ID, workspace, r.ProjectID)
	if err != nil {
		return errStorage
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return errOwnership
	}
	r.ActivePairID = ""
	r.ProcessedPairs++
	if err = writeRun(ctx, tx, workspace, r, true); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return errStorage
	}
	return nil
}
