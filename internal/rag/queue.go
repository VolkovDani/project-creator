package rag

import (
	"context"
	"crypto/rand"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"time"
)

type Progress struct {
	Total, Queued, Running, Ready, Failed int
	Profile                               string
	Configured                            bool
}

var ErrInactiveProfile = errors.New("embedding profile is not active for workspace")

type queueItem struct {
	workspace, project, requirement, profile, attempt, source string
	version, attempts                                         int
}

func (s *Service) StartQueue(workspaces []string) error {
	s.stopQueue()
	profile := s.profile()
	seen := map[string]bool{}
	for _, workspace := range workspaces {
		if workspace == "" || seen[workspace] {
			return ErrInvalid
		}
		seen[workspace] = true
	}
	for _, workspace := range workspaces {
		tx, err := s.db.Begin()
		if err != nil {
			return errStorage
		}
		if err = lockIndexWrites(context.Background(), tx); err != nil {
			tx.Rollback()
			return errStorage
		}
		if _, err = tx.Exec(`INSERT INTO embedding_profiles(workspace,profile) VALUES($1,$2) ON CONFLICT(workspace) DO UPDATE SET profile=excluded.profile,updated_at=clock_timestamp()`, workspace, profile); err != nil {
			tx.Rollback()
			return errStorage
		}
		if profile == "" {
			if _, err = tx.Exec(`UPDATE requirement_index SET status='failed',error_code='NOT_CONFIGURED',attempt='',lease_until=clock_timestamp() WHERE workspace=$1 AND status IN ('queued','running','pending')`, workspace); err != nil {
				tx.Rollback()
				return errStorage
			}
		} else {
			if _, err = tx.Exec(`UPDATE requirement_index SET status='failed',error_code='PROFILE_CHANGED',attempt='',lease_until=clock_timestamp() WHERE workspace=$1 AND profile<>$2 AND status IN ('queued','running','pending')`, workspace, profile); err != nil {
				tx.Rollback()
				return errStorage
			}
			if _, err = tx.Exec(`INSERT INTO requirement_index(workspace,project_id,requirement_id,profile,version,status,attempt,lease_until,error_code,attempts,available_at)
			 SELECT r.workspace,r.project_id,r.id,$2,r.version,'queued','',clock_timestamp(),'',0,clock_timestamp() FROM active_requirements r
			 LEFT JOIN requirement_index i ON i.workspace=r.workspace AND i.project_id=r.project_id AND i.requirement_id=r.id AND i.profile=$2 AND i.version=r.version
			 WHERE r.workspace=$1 AND (i.requirement_id IS NULL OR (i.status='failed' AND i.error_code IN ('PROFILE_CHANGED','NOT_CONFIGURED')))
			 ON CONFLICT(workspace,project_id,requirement_id,profile) DO UPDATE SET version=excluded.version,status='queued',attempt='',lease_until=clock_timestamp(),error_code='',attempts=0,available_at=clock_timestamp()`, workspace, profile); err != nil {
				tx.Rollback()
				return errStorage
			}
		}
		if err = tx.Commit(); err != nil {
			return errStorage
		}
	}
	if profile == "" || len(workspaces) == 0 {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.queueMu.Lock()
	s.queueCancel, s.queueDone = cancel, make(chan struct{})
	done := s.queueDone
	s.queueMu.Unlock()
	go s.queueLoop(ctx, workspaces, profile, done)
	return nil
}

func (s *Service) stopQueue() {
	s.queueMu.Lock()
	cancel, done := s.queueCancel, s.queueDone
	s.queueCancel, s.queueDone = nil, nil
	s.queueMu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}

func (s *Service) Close() error { s.stopQueue(); return nil }

func (s *Service) Summary(ctx context.Context, workspace, project string) (Progress, error) {
	var out Progress
	if err := s.authorize(ctx, workspace, project); err != nil {
		return out, err
	}
	profile := s.profile()
	var active string
	err := s.db.QueryRowContext(ctx, `SELECT profile FROM embedding_profiles WHERE workspace=$1`, workspace).Scan(&active)
	if err != nil && err != sql.ErrNoRows {
		return out, errStorage
	}
	out.Profile = profile
	out.Configured = profile != "" && err == nil && active == profile
	if !out.Configured {
		out.Profile = ""
	}
	err = s.db.QueryRowContext(ctx, `SELECT count(*),count(*) FILTER(WHERE i.requirement_id IS NULL OR i.status='queued' OR (i.status='pending' AND i.attempt='')),count(*) FILTER(WHERE i.status='running' OR (i.status='pending' AND i.attempt<>'')),count(*) FILTER(WHERE i.status='ready'),count(*) FILTER(WHERE i.status='failed')
	 FROM active_requirements r LEFT JOIN requirement_index i ON i.workspace=r.workspace AND i.project_id=r.project_id AND i.requirement_id=r.id AND i.version=r.version AND i.profile=$3
	 WHERE r.workspace=$1 AND r.project_id=$2`, workspace, project, out.Profile).Scan(&out.Total, &out.Queued, &out.Running, &out.Ready, &out.Failed)
	if err != nil {
		return Progress{}, errStorage
	}
	if out.Profile == "" {
		out.Queued = out.Total
		out.Running = 0
		out.Ready = 0
		out.Failed = 0
	}
	return out, nil
}

func (s *Service) queueLoop(ctx context.Context, workspaces []string, profile string, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		worked := false
		_ = s.withEmbeddingLock(ctx, func(callCtx context.Context) error {
			for _, workspace := range workspaces {
				item, err := s.claimQueue(callCtx, workspace, profile)
				if err != nil {
					return err
				}
				if item == nil {
					continue
				}
				worked = true
				s.processQueueItem(callCtx, *item)
			}
			return nil
		})
		if ctx.Err() != nil {
			return
		}
		if worked {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) claimQueue(ctx context.Context, workspace, profile string) (*queueItem, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := lockIndexWrites(ctx, tx); err != nil {
		return nil, err
	}
	// A lost session lock must not overlap an adapter that is still cancelling.
	// Its durable lease remains the fence until completion or expiry.
	var active bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM requirement_index WHERE status IN ('running','pending') AND attempt<>'' AND lease_until>clock_timestamp())`).Scan(&active); err != nil {
		return nil, err
	}
	if active {
		return nil, nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE requirement_index SET status='failed',error_code='INTERRUPTED',attempt='',lease_until=clock_timestamp()
 WHERE workspace=$1 AND profile=$2 AND status IN ('running','pending') AND attempts>=3 AND lease_until<=clock_timestamp()`, workspace, profile); err != nil {
		return nil, err
	}
	var item queueItem
	err = tx.QueryRowContext(ctx, `SELECT i.workspace,i.project_id,i.requirement_id,i.profile,r.version,r.text,i.attempts
	 FROM requirement_index i JOIN active_requirements r ON r.workspace=i.workspace AND r.project_id=i.project_id AND r.id=i.requirement_id
	 JOIN embedding_profiles c ON c.workspace=i.workspace AND c.profile=i.profile
	 WHERE i.workspace=$1 AND i.profile=$2 AND r.version=i.version AND
	 ((i.status='queued' AND i.available_at<=clock_timestamp()) OR (i.status='failed' AND i.attempts<3 AND i.error_code='EMBEDDING_FAILED' AND i.available_at<=clock_timestamp()) OR (i.status IN ('running','pending') AND i.attempts<3 AND i.lease_until<=clock_timestamp()))
	 ORDER BY i.available_at,r.created_at,i.requirement_id FOR UPDATE OF i SKIP LOCKED LIMIT 1`, workspace, profile).Scan(&item.workspace, &item.project, &item.requirement, &item.profile, &item.version, &item.source, &item.attempts)
	if err == sql.ErrNoRows {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var bytes [16]byte
	if _, err = rand.Read(bytes[:]); err != nil {
		return nil, err
	}
	item.attempt = hex.EncodeToString(bytes[:])
	item.attempts++
	if _, err = tx.ExecContext(ctx, `UPDATE requirement_index SET status='running',attempt=$5,attempts=$6,lease_until=clock_timestamp()+interval '2 minutes',error_code='' WHERE workspace=$1 AND project_id=$2 AND requirement_id=$3 AND profile=$4`, item.workspace, item.project, item.requirement, item.profile, item.attempt, item.attempts); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Service) processQueueItem(ctx context.Context, item queueItem) {
	if ok, err := s.currentAttempt(ctx, item); err != nil || !ok {
		return
	}
	state := State{RequirementID: item.requirement, Version: item.version, Profile: item.profile}
	var chunks []Chunk
	var vectors [][]float32
	code := ""
	if len(item.source) == 0 {
		code = "EMPTY_SOURCE"
	} else {
		chunks = Split(item.source)
		if len(chunks) == 0 {
			code = "EMPTY_SOURCE"
		} else {
			embedCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
			inferenceErr := s.withInference(embedCtx, false, func() error {
				vectors, code, _ = s.embed(embedCtx, chunks)
				return nil
			})
			if inferenceErr != nil {
				code = "EMBEDDING_FAILED"
			}
			if embedCtx.Err() != nil {
				code = "EMBEDDING_FAILED"
			}
			cancel()
		}
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	retryAfter := time.Duration(0)
	if code == "EMBEDDING_FAILED" && item.attempts < 3 {
		retryAfter = 5 * time.Second
		if item.attempts == 2 {
			retryAfter = 30 * time.Second
		}
	}
	_, _ = s.finishWithRetry(finishCtx, item.workspace, item.project, state, item.attempt, chunks, vectors, code, retryAfter)
}

func lockIndexWrites(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(724183092)`)
	return err
}

func (s *Service) currentAttempt(ctx context.Context, item queueItem) (bool, error) {
	var current bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM active_requirements r JOIN requirement_index i
 ON i.workspace=r.workspace AND i.project_id=r.project_id AND i.requirement_id=r.id AND i.version=r.version
 WHERE r.workspace=$1 AND r.project_id=$2 AND r.id=$3 AND r.version=$4 AND r.text=$5 AND i.profile=$6 AND i.attempt=$7
 AND i.status IN ('running','pending') AND i.lease_until>clock_timestamp()
 AND NOT EXISTS(SELECT 1 FROM embedding_profiles c WHERE c.workspace=r.workspace AND c.profile<>i.profile))`, item.workspace, item.project, item.requirement, item.version, item.source, item.profile, item.attempt).Scan(&current)
	return current, err
}

func (s *Service) waitEmbeddingIdle(ctx context.Context) error {
	for {
		var active bool
		if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM requirement_index WHERE status IN ('running','pending') AND attempt<>'' AND lease_until>clock_timestamp())`).Scan(&active); err != nil {
			return err
		}
		if !active {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (s *Service) withEmbeddingLock(ctx context.Context, fn func(context.Context) error) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	for {
		var locked bool
		if err = conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(724183094)`).Scan(&locked); err != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			return err
		}
		if locked {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		var unlocked bool
		if err := conn.QueryRowContext(releaseCtx, `SELECT pg_advisory_unlock(724183094)`).Scan(&unlocked); err != nil || !unlocked {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	callCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-callCtx.Done():
				return
			case <-ticker.C:
				beatCtx, beatCancel := context.WithTimeout(callCtx, time.Second)
				err := conn.PingContext(beatCtx)
				beatCancel()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()
	err = fn(callCtx)
	if callCtx.Err() != nil {
		err = callCtx.Err()
	}
	cancel()
	<-done
	return err
}
