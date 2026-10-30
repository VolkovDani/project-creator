package analysis

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
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
	workspace, r, err := s.claim(conn)
	if err != nil || r == nil {
		return false
	}
	s.process(conn, workspace, r)
	return true
}
func (s *Service) reap() {
	rows, err := s.db.QueryContext(s.ctx, `SELECT workspace,project_id FROM analysis_runs WHERE status='running' AND ((worker_id<>'' AND lease_until<=clock_timestamp()) OR (worker_id='' AND (generator_profile<>$1 OR embedding_profile<>$2)))`, s.generatorProfile(), s.embeddingProfile)
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
	err = tx.QueryRowContext(s.ctx, `SELECT workspace,id FROM analysis_runs WHERE status='running' AND worker_id='' AND generator_profile=$1 AND embedding_profile=$2 ORDER BY seq LIMIT 1 FOR UPDATE SKIP LOCKED`, s.generatorProfile(), s.embeddingProfile).Scan(&workspace, &id)
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
		if code != "" {
			if ctx.Err() != nil {
				code = "INTERRUPTED"
			}
			interrupt(r, code)
		} else {
			r.Status = "completed"
			r.Stage = "finished"
			r.ActivePairID = ""
			r.tally()
			r.Outcome = outcome(r.Findings, r.incomplete)
			if r.incomplete && r.ReasonCode == "" {
				r.ReasonCode = "ANALYSIS_LIMIT"
			} else if r.TotalPairs != nil && *r.TotalPairs == 0 && r.ReasonCode == "" {
				r.ReasonCode = "NO_CANDIDATES"
			}
			for _, f := range r.Findings {
				if f.Verdict == "insufficient" && r.ReasonCode == "" {
					r.ReasonCode = "MODEL_INSUFFICIENT"
				}
			}
		}
		finishCtx, finishCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer finishCancel()
		_ = writeRun(finishCtx, s.db, workspace, r, true)
	}
	if r.TotalPairs == nil {
		prepCtx, prepCancel := context.WithTimeout(ctx, s.preparationTimeout)
		if err := s.current(prepCtx, workspace, r); err != nil {
			prepCancel()
			finish("SOURCE_CHANGED")
			return
		}
		pairs, code, incomplete, d := s.prepare(prepCtx, workspace, r.ProjectID, r.Sources, r.incomplete, func() error {
			r.Stage = "finding_pairs"
			return writeRun(prepCtx, s.db, workspace, r, true)
		})
		prepCancel()
		r.incomplete = incomplete
		r.ReasonCode = d.ReasonCode
		r.MissingRequirementIDs = d.MissingRequirementIDs
		if code != "" {
			finish(code)
			return
		}
		r.Pairs = []PairResult{}
		for i, pair := range pairs {
			r.Pairs = append(r.Pairs, PairResult{ID: fmt.Sprintf("%s:%d", r.ID, i), Position: i, Status: "pending", Left: pair.Left, Right: pair.Right})
		}
		n := len(r.Pairs)
		r.TotalPairs = &n
		r.Stage = "comparing"
		if err := writeRun(ctx, s.db, workspace, r, true); err != nil {
			return
		}
	}
	for i := range r.Pairs {
		p := &r.Pairs[i]
		if p.Status == "completed" {
			continue
		}
		if err := s.current(ctx, workspace, r); err != nil {
			finish("SOURCE_CHANGED")
			return
		}
		p.Status = "running"
		p.ErrorCode = ""
		r.ActivePairID = p.ID
		if err := writeRun(ctx, s.db, workspace, r, true); err != nil {
			return
		}
		pairCtx, pairCancel := context.WithTimeout(ctx, s.timeout)
		finding, code := checkedCompare(pairCtx, s.generator, Pair{Left: p.Left, Right: p.Right})
		pairCancel()
		if code != "" {
			finish(code)
			return
		}
		p.Status = "completed"
		p.Verdict = finding.Verdict
		p.Explanation = finding.Explanation
		r.ActivePairID = ""
		r.tally()
		if err := writeRun(ctx, s.db, workspace, r, true); err != nil {
			return
		}
	}
	finish("")
}
func (s *Service) current(ctx context.Context, workspace string, r *Run) error {
	if err := s.authorize(ctx, s.db, workspace, r.ProjectID, false); err != nil {
		return err
	}
	if r.GeneratorProfile != s.generatorProfile() || r.EmbeddingProfile != s.embeddingProfile {
		return errors.New("profile changed")
	}
	_, signature, _, err := snapshot(ctx, s.db, workspace, r.ProjectID)
	if err != nil {
		return err
	}
	if signature != r.signature {
		return errors.New("sources changed")
	}
	return nil
}
