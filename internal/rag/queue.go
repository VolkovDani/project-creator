package rag

import (
	"context"
	"crypto/rand"
	"database/sql"
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
			 SELECT r.workspace,r.project_id,r.id,$2,r.version,'queued','',clock_timestamp(),'',0,clock_timestamp() FROM requirements r
			 LEFT JOIN requirement_index i ON i.workspace=r.workspace AND i.project_id=r.project_id AND i.requirement_id=r.id AND i.profile=$2 AND i.version=r.version
			 WHERE r.workspace=$1 AND i.requirement_id IS NULL
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
	err = s.db.QueryRowContext(ctx, `SELECT count(*),count(*) FILTER(WHERE coalesce(i.status,'queued') IN ('queued','pending')),count(*) FILTER(WHERE i.status IN ('running','pending') AND i.attempt<>''),count(*) FILTER(WHERE i.status='ready'),count(*) FILTER(WHERE i.status='failed')
	 FROM requirements r LEFT JOIN requirement_index i ON i.workspace=r.workspace AND i.project_id=r.project_id AND i.requirement_id=r.id AND i.version=r.version AND i.profile=$3
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
		for _, workspace := range workspaces {
			if ctx.Err() != nil {
				return
			}
			item, err := s.claimQueue(ctx, workspace, profile)
			if err != nil {
				continue
			}
			if item == nil {
				continue
			}
			worked = true
			s.processQueueItem(ctx, *item)
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
	var item queueItem
	err = tx.QueryRowContext(ctx, `SELECT i.workspace,i.project_id,i.requirement_id,i.profile,r.version,r.text,i.attempts
	 FROM requirement_index i JOIN requirements r ON r.workspace=i.workspace AND r.project_id=i.project_id AND r.id=i.requirement_id
	 JOIN embedding_profiles c ON c.workspace=i.workspace AND c.profile=i.profile
	 WHERE i.workspace=$1 AND i.profile=$2 AND r.version=i.version AND
	 ((i.status='queued' AND i.available_at<=clock_timestamp()) OR (i.status='failed' AND i.attempts<3 AND i.error_code='EMBEDDING_FAILED' AND i.available_at<=clock_timestamp()) OR (i.status='running' AND i.lease_until<=clock_timestamp()))
	 ORDER BY i.available_at,r.created_at,i.requirement_id FOR UPDATE OF i SKIP LOCKED LIMIT 1`, workspace, profile).Scan(&item.workspace, &item.project, &item.requirement, &item.profile, &item.version, &item.source, &item.attempts)
	if err == sql.ErrNoRows {
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
	state := State{RequirementID: item.requirement, Version: item.version, Profile: item.profile}
	var chunks []Chunk
	var vectors [][]float32
	code := ""
	if len(item.source) == 0 {
		code = "EMPTY_SOURCE"
	} else {
		chunks = Split(item.source)
		lockCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		err := s.withEmbeddingLock(lockCtx, func() error { var e error; vectors, code, e = s.embed(lockCtx, chunks); return e })
		cancel()
		if err != nil {
			if code == "" {
				code = "EMBEDDING_FAILED"
			}
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

func (s *Service) withEmbeddingLock(ctx context.Context, fn func() error) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, `SELECT pg_advisory_lock(724183094)`); err != nil {
		return err
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(releaseCtx, `SELECT pg_advisory_unlock(724183094)`)
	}()
	return fn()
}
