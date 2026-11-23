package classification

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"slices"
	"sync"
	"time"
)

// Profile identifies all inputs to a model decision, including its policy.
type Classifier interface {
	Profile() string
	Model() string
	Digest() string
	QuestionVersion() string
	Classify(context.Context, string) (Prediction, error)
}
type Prediction struct {
	Status, Semantic, Representation, Reason string
	Alternatives                             []Alternative
}

var (
	ErrUnavailable   = errors.New("classification unavailable")
	ErrNotConfigured = errors.New("classifier not configured")
	ErrConflict      = errors.New("classification source version changed")
)

type Service struct {
	db         *sql.DB
	classifier Classifier
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

func New(db *sql.DB, c Classifier) *Service { return &Service{db: db, classifier: c} }
func lock(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(724183092)`)
	return err
}
func (s *Service) Close() {
	if s.cancel != nil {
		s.cancel()
		s.wg.Wait()
	}
}
func (s *Service) Start(workspaces []string) error {
	s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		cancel()
		return e
	}
	defer tx.Rollback()
	fail := func(e error) error { cancel(); return e }
	if e = lock(ctx, tx); e != nil {
		return fail(e)
	}
	c := s.classifier
	if c != nil && (c.Profile() == "" || c.Model() == "" || c.Digest() == "" || c.QuestionVersion() == "") {
		return fail(ErrNotConfigured)
	}
	unique := map[string]bool{}
	for _, w := range workspaces {
		if w == "" {
			return fail(ErrUnavailable)
		}
		if unique[w] {
			continue
		}
		unique[w] = true
		profile := ""
		if c == nil {
			_, e = tx.ExecContext(ctx, `DELETE FROM classification_profiles WHERE workspace=$1`, w)
		} else {
			profile = c.Profile()
			_, e = tx.ExecContext(ctx, `INSERT INTO classification_profiles VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(workspace) DO UPDATE SET profile=excluded.profile,model=excluded.model,digest=excluded.digest,question_version=excluded.question_version,rules_version=excluded.rules_version`, w, profile, c.Model(), c.Digest(), c.QuestionVersion(), rules.Version)
		}
		if e != nil {
			return fail(e)
		}
		if _, e = tx.ExecContext(ctx, `UPDATE classification_jobs SET status='stale',attempt='' WHERE workspace=$1 AND profile<>$2 AND status IN ('queued','classifying','failed')`, w, profile); e != nil {
			return fail(e)
		}
		if _, e = tx.ExecContext(ctx, `DELETE FROM classification_backfills WHERE workspace=$1 AND profile<>$2`, w, profile); e != nil {
			return fail(e)
		}
		if c != nil {
			if _, e = tx.ExecContext(ctx, `INSERT INTO classification_backfills(workspace,project_id,profile,failed_only) VALUES($1,'',$2,false) ON CONFLICT DO NOTHING`, w, profile); e != nil {
				return fail(e)
			}
		}
	}
	if e = tx.Commit(); e != nil {
		return fail(e)
	}
	s.cancel = cancel
	if c != nil && len(unique) > 0 {
		s.wg.Add(1)
		go s.run(ctx, slices.Clone(workspaces))
	}
	return nil
}

const timeout = 90 * time.Second
const maxAttempts = 3

type job struct {
	id, w, p, text, attempt string
	version, attempts       int32
	reanalysis              bool
}

// Stage a small batch of older sources between claims. New edits are queued by their transaction.
const seedMissing = `
 INSERT INTO classification_jobs(workspace,project_id,requirement_id,version,profile)
 SELECT r.workspace,r.project_id,r.id,r.version,p.profile FROM active_requirements r JOIN classification_profiles p ON p.workspace=r.workspace
 WHERE r.workspace=$1 AND p.profile=$2 AND ($3::text='' OR r.project_id=$3)
 AND NOT EXISTS (SELECT FROM requirement_classifications c WHERE c.requirement_id=r.id AND c.text_version=r.version AND c.status='confirmed')
 AND NOT EXISTS (SELECT FROM classification_jobs j WHERE j.requirement_id=r.id AND j.version=r.version AND j.profile=p.profile AND j.status<>'stale')
 ORDER BY r.project_id,r.created_at,r.id LIMIT $4
 ON CONFLICT(requirement_id,version,profile) DO UPDATE SET status='queued',attempt='',attempts=0,error_code='',available_at=clock_timestamp(),reanalysis=false WHERE classification_jobs.status='stale'`

func (s *Service) run(ctx context.Context, ws []string) {
	defer s.wg.Done()
	for ctx.Err() == nil {
		item, e := s.claim(ctx, ws)
		if e == nil && item != nil {
			s.process(ctx, *item)
			continue
		}
		// Refill only when existing work is unavailable. Each transaction stages
		// at most a batch and gives CRUD and other queues the write lock back.
		if e == nil {
			_ = s.continueBackfill(ctx, ws)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(250 * time.Millisecond):
		}
	}
}
func (s *Service) claim(ctx context.Context, ws []string) (*job, error) {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	if e = lock(ctx, tx); e != nil {
		return nil, e
	}
	profile := s.classifier.Profile()
	// A crashed final attempt becomes visibly failed; earlier expired attempts can be reclaimed.
	if _, e = tx.ExecContext(ctx, `UPDATE classification_jobs SET status='failed',attempt='',error_code='INTERRUPTED'
 WHERE workspace=ANY($1) AND profile=$2 AND status='classifying' AND available_at<=clock_timestamp() AND attempts>=$3`, ws, profile, maxAttempts); e != nil {
		return nil, e
	}
	var j job
	e = tx.QueryRowContext(ctx, `WITH candidate AS (
 SELECT j.requirement_id,j.version,j.profile FROM classification_jobs j JOIN active_requirements r ON r.id=j.requirement_id AND r.version=j.version
 JOIN classification_profiles p ON p.workspace=j.workspace AND p.profile=j.profile
 WHERE j.workspace=ANY($1) AND j.profile=$2 AND j.status IN ('queued','classifying') AND j.available_at<=clock_timestamp()
 AND NOT EXISTS (SELECT FROM classification_call WHERE lease_until>clock_timestamp())
 ORDER BY j.available_at,j.requirement_id LIMIT 1 FOR UPDATE OF j SKIP LOCKED)
 UPDATE classification_jobs j SET status='classifying', attempts=attempts+1,attempt=gen_random_uuid()::text,
 available_at=clock_timestamp()+interval '2 minutes',error_code='' FROM candidate c,active_requirements r
 WHERE (j.requirement_id,j.version,j.profile)=(c.requirement_id,c.version,c.profile) AND r.id=j.requirement_id
 RETURNING j.requirement_id,j.workspace,j.project_id,j.version,j.attempt,j.attempts,j.reanalysis,r.text`, ws, profile).Scan(&j.id, &j.w, &j.p, &j.version, &j.attempt, &j.attempts, &j.reanalysis, &j.text)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, tx.Commit()
	}
	if e != nil {
		return nil, e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO classification_call(attempt,lease_until) SELECT attempt,available_at FROM classification_jobs WHERE requirement_id=$1 AND version=$2 AND profile=$3
	ON CONFLICT(singleton) DO UPDATE SET attempt=excluded.attempt,lease_until=excluded.lease_until`, j.id, j.version, profile)
	if e != nil {
		return nil, e
	}
	return &j, tx.Commit()
}

var reasonCode = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{0,63}$`)

func validPrediction(v Prediction) bool {
	if v.Status != "needs_review" && v.Status != "suggested" || !slices.Contains(rules.Semantic, v.Semantic) || !slices.Contains(rules.Representation, v.Representation) || v.Reason != "" && !reasonCode.MatchString(v.Reason) {
		return false
	}
	if len(v.Alternatives) > len(rules.Semantic)+len(rules.Representation) {
		return false
	}
	seen := map[string]bool{}
	for _, a := range v.Alternatives {
		hasSemantic := a.SemanticType != ""
		hasForm := a.RepresentationType != ""
		if hasSemantic == hasForm {
			return false
		}
		if hasSemantic && !slices.Contains(rules.Semantic, a.SemanticType) || hasForm && !slices.Contains(rules.Representation, a.RepresentationType) {
			return false
		}
		if a.Score == nil || math.IsNaN(*a.Score) || *a.Score < 0 || *a.Score > 1 {
			return false
		}
		key := a.SemanticType + "|" + a.RepresentationType
		if seen[key] {
			return false
		}
		seen[key] = true
	}
	return true
}

func (s *Service) process(ctx context.Context, j job) {
	call, cancel := context.WithTimeout(ctx, timeout)
	v, e := s.classifier.Classify(call, j.text)
	if call.Err() != nil {
		e = call.Err()
	}
	cancel()
	code := ""
	if ctx.Err() != nil {
		// The call has returned, so another worker can take over immediately.
		code = "SHUTDOWN"
	} else if e != nil {
		code = "CLASSIFIER_UNAVAILABLE"
	} else if !validPrediction(v) {
		code = "INVALID_CLASSIFIER_RESULT"
	}
	persist, stop := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer stop()
	_ = s.finish(persist, j, v, code)
}

func (s *Service) finish(ctx context.Context, j job, v Prediction, code string) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = lock(ctx, tx); e != nil {
		return e
	}
	var present bool
	e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT FROM classification_jobs j JOIN active_requirements r ON r.id=j.requirement_id AND r.workspace=j.workspace AND r.project_id=j.project_id AND r.version=j.version
 JOIN classification_profiles p ON p.workspace=j.workspace AND p.profile=j.profile
 WHERE j.requirement_id=$1 AND j.version=$2 AND j.profile=$3 AND j.attempt=$4 AND j.status='classifying' AND j.available_at>clock_timestamp())`, j.id, j.version, s.classifier.Profile(), j.attempt).Scan(&present)
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM classification_call WHERE attempt=$1`, j.attempt); e != nil {
		return e
	}
	if !present {
		return tx.Commit()
	}
	if code == "SHUTDOWN" {
		_, e = tx.ExecContext(ctx, `UPDATE classification_jobs SET status='queued',attempts=attempts-1,attempt='',error_code='',available_at=clock_timestamp() WHERE attempt=$1`, j.attempt)
		if e != nil {
			return e
		}
		return tx.Commit()
	}
	state := "done"
	delay := 0
	if code != "" {
		state = "failed"
		if j.attempts < maxAttempts && code != "INVALID_CLASSIFIER_RESULT" {
			state = "queued"
			delay = 5
			if j.attempts == 2 {
				delay = 30
			}
		}
	} else {
		alternatives, e := encodeAlternatives(v.Alternatives)
		if e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO requirement_classifications(workspace,project_id,requirement_id,revision,text_version,semantic_type,representation_type,status,source,rules_version,model,alternatives,profile,digest,question_version,reason,reanalysis)
  SELECT $1,$2,$3,(SELECT COALESCE(max(revision),0)+1 FROM requirement_classifications WHERE requirement_id=$3),$4,$5,$6,$7,'model',p.rules_version,p.model,$8,p.profile,p.digest,p.question_version,$9,$10 FROM classification_profiles p WHERE p.workspace=$1 AND p.profile=$11`, j.w, j.p, j.id, j.version, v.Semantic, v.Representation, v.Status, alternatives, v.Reason, j.reanalysis, s.classifier.Profile())
		if e != nil {
			return e
		}
	}
	_, e = tx.ExecContext(ctx, `UPDATE classification_jobs SET status=$2,error_code=$3,attempt='',available_at=clock_timestamp()+make_interval(secs=>$4) WHERE attempt=$1`, j.attempt, state, code, delay)
	if e != nil {
		return e
	}
	return tx.Commit()
}

func encodeAlternatives(a []Alternative) ([]byte, error) {
	if a == nil {
		a = []Alternative{}
	}
	return json.Marshal(a)
}
