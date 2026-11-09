// Package summary manages versioned derived previews in a bounded durable queue.
package summary

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/VolkovDani/project-creator/contracts"
	"github.com/VolkovDani/project-creator/internal/analysis"
)

type Generator interface {
	Profile() string
	Provenance() string
	Summarize(context.Context, analysis.Source) (string, error)
}
type State struct {
	LayaEligible                                            bool
	ByteLength                                              int
	RequirementID, ProjectID                                string
	Version                                                 int
	Status, Text, Profile, Provenance, CreatedAt, ErrorCode string
}
type Service struct {
	db         *sql.DB
	generator  Generator
	ctx        context.Context
	cancel     context.CancelFunc
	done       chan struct{}
	once       sync.Once
	workspaces []string
}

var ErrUnavailable = errors.New("summary unavailable")

func New(db *sql.DB, g Generator) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{db: db, generator: g, ctx: ctx, cancel: cancel, done: make(chan struct{})}
}
func (s *Service) Profile() string {
	if s.generator == nil {
		return ""
	}
	return s.generator.Profile()
}
func (s *Service) Close() { s.once.Do(func() { s.cancel(); <-s.done }) }
func lock(ctx context.Context, tx *sql.Tx) error {
	_, e := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(724183092)`)
	return e
}
func auth(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, w, p string) error {
	var id string
	if w == "" || q.QueryRowContext(ctx, `SELECT id FROM projects WHERE workspace=$1 AND id=$2`, w, p).Scan(&id) != nil {
		return ErrUnavailable
	}
	return nil
}
func (s *Service) Start(workspaces []string) error {
	s.workspaces = append([]string(nil), workspaces...)
	tx, e := s.db.BeginTx(s.ctx, nil)
	if e != nil {
		close(s.done)
		return e
	}
	defer tx.Rollback()
	if e = lock(s.ctx, tx); e != nil {
		close(s.done)
		return e
	}
	for _, w := range workspaces {
		if _, e = tx.ExecContext(s.ctx, `UPDATE requirement_summaries SET status='stale',attempt='' WHERE workspace=$1 AND profile<>$2 AND status IN ('queued','running')`, w, s.Profile()); e != nil {
			close(s.done)
			return e
		}
		if s.generator == nil {
			_, e = tx.ExecContext(s.ctx, `DELETE FROM summary_profiles WHERE workspace=$1`, w)
		} else {
			_, e = tx.ExecContext(s.ctx, `INSERT INTO summary_profiles VALUES($1,$2,$3) ON CONFLICT(workspace) DO UPDATE SET profile=excluded.profile,provenance=excluded.provenance`, w, s.Profile(), s.generator.Provenance())
			if e == nil {
				_, e = tx.ExecContext(s.ctx, `INSERT INTO requirement_summaries(workspace,project_id,requirement_id,version,profile,provenance,status) SELECT workspace,project_id,id,version,$2,$3,'queued' FROM requirements WHERE workspace=$1 ON CONFLICT DO NOTHING`, w, s.Profile(), s.generator.Provenance())
			}
		}
		if e != nil {
			close(s.done)
			return e
		}
	}
	if e = tx.Commit(); e != nil {
		close(s.done)
		return e
	}
	go s.work()
	return nil
}
func (s *Service) ByIDs(ctx context.Context, w, p string, ids []string) ([]State, error) {
	if len(ids) < 1 || len(ids) > 100 {
		return nil, ErrUnavailable
	}
	if e := auth(ctx, s.db, w, p); e != nil {
		return nil, e
	}
	rows, e := s.db.QueryContext(ctx, `SELECT r.id,r.version,coalesce(s.status,CASE WHEN $4='' THEN 'not_configured' ELSE 'missing' END),coalesce(s.text,''),coalesce(s.provenance,''),coalesce(s.created_at::text,''),coalesce(s.error_code,'') FROM unnest($3::text[]) WITH ORDINALITY requested(id,position) JOIN requirements r ON r.id=requested.id AND r.workspace=$1 AND r.project_id=$2 LEFT JOIN requirement_summaries s ON s.workspace=r.workspace AND s.project_id=r.project_id AND s.requirement_id=r.id AND s.version=r.version AND s.profile=$4 ORDER BY requested.position`, w, p, ids, s.Profile())
	if e != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	out := make([]State, 0, len(ids))
	for rows.Next() {
		v := State{ProjectID: p, Profile: s.Profile()}
		if rows.Scan(&v.RequirementID, &v.Version, &v.Status, &v.Text, &v.Provenance, &v.CreatedAt, &v.ErrorCode) != nil {
			return nil, ErrUnavailable
		}
		if v.Status != "ready" {
			v.Text = ""
		}
		v.ByteLength = len(v.Text)
		v.LayaEligible = v.Status == "ready" && contracts.SummaryLayaEligible(v.Text)
		out = append(out, v)
	}
	if rows.Err() != nil || len(out) != len(ids) {
		return nil, ErrUnavailable
	}
	return out, nil
}
func (s *Service) change(ctx context.Context, w, p, id string, cancel bool) (State, error) {
	if e := auth(ctx, s.db, w, p); e != nil {
		return State{}, e
	}
	if s.generator == nil {
		return State{}, ErrUnavailable
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return State{}, e
	}
	defer tx.Rollback()
	if e = lock(ctx, tx); e != nil {
		return State{}, e
	}
	if e = auth(ctx, tx, w, p); e != nil {
		return State{}, e
	}
	if cancel {
		_, e = tx.ExecContext(ctx, `UPDATE requirement_summaries s SET status='cancelled',attempt='',error_code='CANCELLED' FROM requirements r WHERE s.workspace=$1 AND s.project_id=$2 AND s.requirement_id=$3 AND s.profile=$4 AND s.status IN ('queued','running') AND r.workspace=s.workspace AND r.project_id=s.project_id AND r.id=s.requirement_id AND r.version=s.version`, w, p, id, s.Profile())
	} else {
		_, e = tx.ExecContext(ctx, `INSERT INTO requirement_summaries(workspace,project_id,requirement_id,version,profile,provenance,status) SELECT workspace,project_id,id,version,$4,$5,'queued' FROM requirements WHERE workspace=$1 AND project_id=$2 AND id=$3 ON CONFLICT(workspace,project_id,requirement_id,version,profile) DO UPDATE SET status='queued',error_code='',attempt='',text='' WHERE requirement_summaries.status IN ('failed','cancelled','stale')`, w, p, id, s.Profile(), s.generator.Provenance())
	}
	if e != nil {
		return State{}, e
	}
	if e = tx.Commit(); e != nil {
		return State{}, e
	}
	v, e := s.ByIDs(ctx, w, p, []string{id})
	if e != nil {
		return State{}, e
	}
	return v[0], nil
}
func (s *Service) Retry(ctx context.Context, w, p, id string) (State, error) {
	return s.change(ctx, w, p, id, false)
}
func (s *Service) Cancel(ctx context.Context, w, p, id string) (State, error) {
	return s.change(ctx, w, p, id, true)
}

type job struct {
	workspace, project, profile, attempt string
	source                               analysis.Source
}

func (s *Service) claim() (*job, error) {
	tx, e := s.db.BeginTx(s.ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	if e = lock(s.ctx, tx); e != nil {
		return nil, e
	}
	for _, w := range s.workspaces {
		// Interrupted inference is visible and requires explicit retry; never silently repeat it.
		if _, e = tx.ExecContext(s.ctx, `UPDATE requirement_summaries SET status='failed',error_code='INTERRUPTED',attempt='' WHERE workspace=$1 AND profile=$2 AND status='running' AND lease_until<=clock_timestamp()`, w, s.Profile()); e != nil {
			return nil, e
		}
		j := &job{workspace: w, profile: s.Profile()}
		var b [16]byte
		if _, e = rand.Read(b[:]); e != nil {
			return nil, e
		}
		j.attempt = hex.EncodeToString(b[:])
		e = tx.QueryRowContext(s.ctx, `SELECT s.project_id,r.id,r.version,r.text FROM requirement_summaries s JOIN requirements r ON r.workspace=s.workspace AND r.project_id=s.project_id AND r.id=s.requirement_id AND r.version=s.version JOIN summary_profiles p ON p.workspace=s.workspace AND p.profile=s.profile WHERE s.workspace=$1 AND s.profile=$2 AND s.status='queued' ORDER BY s.created_at,s.requirement_id LIMIT 1 FOR UPDATE OF s SKIP LOCKED`, w, s.Profile()).Scan(&j.project, &j.source.ID, &j.source.Version, &j.source.Text)
		if e == sql.ErrNoRows {
			continue
		}
		if e != nil {
			return nil, e
		}
		_, e = tx.ExecContext(s.ctx, `UPDATE requirement_summaries SET status='running',attempt=$5,lease_until=clock_timestamp()+interval '150 seconds' WHERE workspace=$1 AND project_id=$2 AND requirement_id=$3 AND version=$4 AND profile=$6`, w, j.project, j.source.ID, j.source.Version, j.attempt, j.profile)
		if e != nil {
			return nil, e
		}
		if e = tx.Commit(); e != nil {
			return nil, e
		}
		return j, nil
	}
	return nil, tx.Commit()
}
func (s *Service) owned(ctx context.Context, j *job) bool {
	var ok bool
	e := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM requirement_summaries s JOIN requirements r ON r.workspace=s.workspace AND r.project_id=s.project_id AND r.id=s.requirement_id AND r.version=s.version JOIN summary_profiles p ON p.workspace=s.workspace AND p.profile=s.profile WHERE s.workspace=$1 AND s.project_id=$2 AND s.requirement_id=$3 AND s.version=$4 AND s.profile=$5 AND s.attempt=$6 AND s.status='running' AND s.lease_until>clock_timestamp())`, j.workspace, j.project, j.source.ID, j.source.Version, j.profile, j.attempt).Scan(&ok)
	return e == nil && ok
}
func (s *Service) process(j *job) {
	ctx, cancel := context.WithTimeout(s.ctx, 120*time.Second)
	defer cancel()
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !s.owned(ctx, j) {
					cancel()
					return
				}
			}
		}
	}()
	text, e := s.generator.Summarize(ctx, j.source)
	code := ""
	if e != nil {
		code = "GENERATION_FAILED"
	}
	if errors.Is(e, analysis.ErrSummaryImpossible) {
		code = "CANNOT_COMPRESS"
	}
	if errors.Is(e, analysis.ErrSummaryInputTooLong) {
		code = "INPUT_TOO_LONG"
	}
	if errors.Is(e, analysis.ErrInvalidSummary) {
		code = "INVALID_OUTPUT"
	}
	if e == nil && !contracts.ValidSummary(text) {
		code = "INVALID_OUTPUT"
	}
	if e == nil && code == "" {
		if validator, ok := s.generator.(interface{ ValidateSummary(string) error }); ok && validator.ValidateSummary(text) != nil {
			code = "INVALID_OUTPUT"
		}
	}
	if ctx.Err() != nil {
		code = "INTERRUPTED"
	}
	cancel()
	<-watchDone
	tx, e := s.db.BeginTx(s.ctx, nil)
	if e != nil {
		return
	}
	defer tx.Rollback()
	if lock(s.ctx, tx) != nil {
		return
	}
	if !s.owned(s.ctx, j) {
		return
	}
	status := "ready"
	if code != "" {
		status = "failed"
		text = ""
	}
	_, e = tx.ExecContext(s.ctx, `UPDATE requirement_summaries SET status=$7,text=$8,error_code=$9,created_at=clock_timestamp(),attempt='' WHERE workspace=$1 AND project_id=$2 AND requirement_id=$3 AND version=$4 AND profile=$5 AND attempt=$6 AND status='running' AND lease_until>clock_timestamp()`, j.workspace, j.project, j.source.ID, j.source.Version, j.profile, j.attempt, status, text, code)
	if e == nil {
		_ = tx.Commit()
	}
}
func (s *Service) work() {
	defer close(s.done)
	if s.generator == nil {
		return
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if s.ctx.Err() != nil {
			return
		}
		j, e := s.claim()
		if e == nil && j != nil {
			s.process(j)
			continue
		}
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
