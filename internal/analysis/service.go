package analysis

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/VolkovDani/project-creator/internal/rag"
)

var ErrNotFound = errors.New("project not found")
var ErrRetryUnavailable = errors.New("analysis retry unavailable")
var errStorage = errors.New("analysis storage unavailable")
var errOwnership = errors.New("analysis worker no longer owns attempt")

type PairResult struct {
	ID          string `json:"id"`
	Position    int    `json:"position"`
	Status      string `json:"status"`
	Left        Source `json:"left"`
	Right       Source `json:"right"`
	Verdict     string `json:"verdict"`
	Explanation string `json:"explanation"`
	ErrorCode   string `json:"errorCode"`
}

type Run struct {
	ReasonCode                                                                                            string
	MissingRequirementIDs                                                                                 []string
	ID, ProjectID, Status, Outcome, ErrorCode, CreatedAt, CompletedAt, GeneratorProfile, EmbeddingProfile string
	Stage                                                                                                 string
	TotalPairs                                                                                            *int
	ProcessedPairs                                                                                        int
	ActivePairID                                                                                          string
	Pairs                                                                                                 []PairResult
	Stale                                                                                                 bool
	Findings                                                                                              []Finding
	Sources                                                                                               []Source
	signature                                                                                             string
	incomplete                                                                                            bool
	workerID                                                                                              string
}

type Service struct {
	db               *sql.DB
	retriever        *rag.Service
	generator        Generator
	embeddingProfile string
	// Budgets apply to preparation and each individual pair, never to the batch.
	timeout            time.Duration
	preparationTimeout time.Duration
	heartbeatInterval  time.Duration
	ctx                context.Context
	cancel             context.CancelFunc
	done               chan struct{}
	wake               chan struct{}
	closeOnce          sync.Once
}

func New(db *sql.DB, retriever *rag.Service, generator Generator, embeddingProfile string) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{db: db, retriever: retriever, generator: generator, embeddingProfile: embeddingProfile,
		timeout: 120 * time.Second, preparationTimeout: 180 * time.Second, heartbeatInterval: 5 * time.Second, ctx: ctx, cancel: cancel, done: make(chan struct{}), wake: make(chan struct{}, 1)}
	go s.work()
	return s
}
func (s *Service) Close() { s.closeOnce.Do(func() { s.cancel(); <-s.done }) }
func (s *Service) notify() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}
func (s *Service) generatorProfile() string {
	if s.generator == nil {
		return ""
	}
	return s.generator.Profile()
}
func identifier() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Service) authorize(ctx context.Context, q queryer, workspace, project string, lock bool) error {
	query := `SELECT id FROM projects WHERE workspace=$1 AND id=$2`
	if lock {
		query += ` FOR UPDATE`
	}
	var id string
	err := q.QueryRowContext(ctx, query, workspace, project).Scan(&id)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return errStorage
	}
	return nil
}

// One statement snapshot includes every source version in the signature while
// limiting retained text to the first 100 requirements.
func snapshot(ctx context.Context, q queryer, workspace, project string) ([]Source, string, bool, error) {
	var raw []byte
	var signature string
	var count int
	err := q.QueryRowContext(ctx, `WITH scoped AS (SELECT id,version,text FROM requirements WHERE workspace=$1 AND project_id=$2), limited AS (SELECT * FROM scoped ORDER BY id LIMIT 100) SELECT coalesce((SELECT jsonb_agg(to_jsonb(limited) ORDER BY id) FROM limited),'[]'::jsonb),(SELECT md5(coalesce(jsonb_agg(jsonb_build_array(id,version) ORDER BY id)::text,'[]')) FROM scoped),(SELECT count(*) FROM scoped)`, workspace, project).Scan(&raw, &signature, &count)
	if err != nil {
		return nil, "", false, errStorage
	}
	sources := []Source{}
	if json.Unmarshal(raw, &sources) != nil {
		return nil, "", false, errStorage
	}
	return sources, signature, count > 100, nil
}

const runColumns = `id,project_id,status,outcome,error_code,generator_profile,embedding_profile,source_signature,sources,findings,created_at,completed_at,reason_code,missing_requirement_ids,stage,total_pairs,processed_pairs,active_pair_id,pairs,incomplete,worker_id`

func scanRun(row *sql.Row) (*Run, error) {
	r := &Run{Sources: []Source{}, Findings: []Finding{}, Pairs: []PairResult{}, MissingRequirementIDs: []string{}}
	var sources, findings, missing, pairs []byte
	var created time.Time
	var completed sql.NullTime
	var total sql.NullInt64
	err := row.Scan(&r.ID, &r.ProjectID, &r.Status, &r.Outcome, &r.ErrorCode, &r.GeneratorProfile, &r.EmbeddingProfile, &r.signature, &sources, &findings, &created, &completed, &r.ReasonCode, &missing, &r.Stage, &total, &r.ProcessedPairs, &r.ActivePairID, &pairs, &r.incomplete, &r.workerID)
	if err != nil {
		return nil, err
	}
	if json.Unmarshal(sources, &r.Sources) != nil || json.Unmarshal(findings, &r.Findings) != nil || json.Unmarshal(missing, &r.MissingRequirementIDs) != nil || json.Unmarshal(pairs, &r.Pairs) != nil {
		return nil, errStorage
	}
	if total.Valid {
		n := int(total.Int64)
		r.TotalPairs = &n
	}
	r.CreatedAt = created.UTC().Format(time.RFC3339Nano)
	if completed.Valid {
		r.CompletedAt = completed.Time.UTC().Format(time.RFC3339Nano)
	}
	return r, nil
}
func (s *Service) markStale(ctx context.Context, workspace, project string, r *Run) error {
	_, signature, _, err := snapshot(ctx, s.db, workspace, project)
	if err != nil {
		return err
	}
	r.Stale = signature != r.signature || r.EmbeddingProfile != s.embeddingProfile || r.GeneratorProfile != s.generatorProfile()
	return nil
}

// All public mutations and lease recovery lock project before analysis rows.
// Recovery never dispatches a model: an interrupted attempt needs explicit retry.
func (s *Service) recoverProject(ctx context.Context, tx *sql.Tx, workspace, project string) error {
	r, err := scanRun(tx.QueryRowContext(ctx, `SELECT `+runColumns+` FROM analysis_runs WHERE workspace=$1 AND project_id=$2 AND status='running' AND ((worker_id<>'' AND lease_until<=clock_timestamp()) OR (worker_id='' AND (generator_profile<>$3 OR embedding_profile<>$4))) FOR UPDATE`, workspace, project, s.generatorProfile(), s.embeddingProfile))
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return errStorage
	}
	code := "INTERRUPTED"
	if r.workerID == "" {
		code = "PROFILE_CHANGED"
	}
	interrupt(r, code)
	return writeRun(ctx, tx, workspace, r, false)
}
func (s *Service) Latest(ctx context.Context, workspace, project string) (*Run, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, errStorage
	}
	defer tx.Rollback()
	if err = s.authorize(ctx, tx, workspace, project, true); err != nil {
		return nil, err
	}
	if err = s.recoverProject(ctx, tx, workspace, project); err != nil {
		return nil, err
	}
	r, err := scanRun(tx.QueryRowContext(ctx, `SELECT `+runColumns+` FROM analysis_runs WHERE workspace=$1 AND project_id=$2 ORDER BY seq DESC LIMIT 1`, workspace, project))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, errStorage
	}
	if err = tx.Commit(); err != nil {
		return nil, errStorage
	}
	if err = s.markStale(ctx, workspace, project, r); err != nil {
		return nil, err
	}
	return r, nil
}
func (s *Service) Start(ctx context.Context, workspace, project string) (Run, error) {
	empty := Run{ProjectID: project, Sources: []Source{}, Findings: []Finding{}, Pairs: []PairResult{}}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return empty, errStorage
	}
	defer tx.Rollback()
	if err = s.authorize(ctx, tx, workspace, project, true); err != nil {
		return empty, err
	}
	if err = s.recoverProject(ctx, tx, workspace, project); err != nil {
		return empty, err
	}
	active, err := scanRun(tx.QueryRowContext(ctx, `SELECT `+runColumns+` FROM analysis_runs WHERE workspace=$1 AND project_id=$2 AND status='running' FOR UPDATE`, workspace, project))
	if err == nil {
		if active.GeneratorProfile == s.generatorProfile() && active.EmbeddingProfile == s.embeddingProfile {
			if err = tx.Commit(); err != nil {
				return empty, errStorage
			}
			if err = s.markStale(ctx, workspace, project, active); err != nil {
				return empty, err
			}
			return *active, nil
		}
		interrupt(active, "PROFILE_CHANGED")
		if err = writeRun(ctx, tx, workspace, active, false); err != nil {
			return empty, err
		}
	} else if err != sql.ErrNoRows {
		return empty, errStorage
	}
	sources, signature, incomplete, err := snapshot(ctx, tx, workspace, project)
	if err != nil {
		return empty, err
	}
	id, err := identifier()
	if err != nil {
		return empty, errors.New("analysis identifier unavailable")
	}
	r := Run{ID: id, ProjectID: project, Status: "running", Stage: "checking_index", Sources: sources, Findings: []Finding{}, Pairs: []PairResult{}, MissingRequirementIDs: []string{}, signature: signature, incomplete: incomplete, GeneratorProfile: s.generatorProfile(), EmbeddingProfile: s.embeddingProfile}
	raw, _ := json.Marshal(sources)
	var created time.Time
	if err = tx.QueryRowContext(ctx, `INSERT INTO analysis_runs(id,workspace,project_id,status,generator_profile,embedding_profile,source_signature,sources,lease_until,stage,incomplete) VALUES($1,$2,$3,'running',$4,$5,$6,$7::jsonb,'infinity','checking_index',$8) RETURNING created_at`, id, workspace, project, r.GeneratorProfile, r.EmbeddingProfile, signature, string(raw), incomplete).Scan(&created); err != nil {
		return empty, errStorage
	}
	r.CreatedAt = created.UTC().Format(time.RFC3339Nano)
	if err = tx.Commit(); err != nil {
		return empty, errStorage
	}
	s.notify()
	return r, nil
}
func (s *Service) Retry(ctx context.Context, workspace, project, id string) (Run, error) {
	empty := Run{}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return empty, errStorage
	}
	defer tx.Rollback()
	if err = s.authorize(ctx, tx, workspace, project, true); err != nil {
		return empty, err
	}
	if err = s.recoverProject(ctx, tx, workspace, project); err != nil {
		return empty, err
	}
	r, err := scanRun(tx.QueryRowContext(ctx, `SELECT `+runColumns+` FROM analysis_runs WHERE workspace=$1 AND project_id=$2 ORDER BY seq DESC LIMIT 1 FOR UPDATE`, workspace, project))
	if err != nil || r.ID != id {
		return empty, ErrRetryUnavailable
	}
	if r.Status == "running" {
		if err = tx.Commit(); err != nil {
			return empty, errStorage
		}
		return *r, nil
	}
	_, signature, _, err := snapshot(ctx, tx, workspace, project)
	if err != nil {
		return empty, err
	}
	if r.Status != "failed" || signature != r.signature || r.GeneratorProfile != s.generatorProfile() || r.EmbeddingProfile != s.embeddingProfile {
		return empty, ErrRetryUnavailable
	}
	retry := false
	for i := range r.Pairs {
		if r.Pairs[i].Status == "failed" {
			r.Pairs[i].Status = "pending"
			r.Pairs[i].ErrorCode = ""
			retry = true
		}
	}
	if !retry {
		return empty, ErrRetryUnavailable
	}
	r.Status = "running"
	r.Stage = "comparing"
	r.ErrorCode = ""
	r.Outcome = ""
	r.CompletedAt = ""
	r.workerID = ""
	r.ActivePairID = ""
	r.tally()
	if err = writeRun(ctx, tx, workspace, r, false); err != nil {
		return empty, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE analysis_runs SET lease_until='infinity' WHERE workspace=$1 AND project_id=$2 AND id=$3`, workspace, project, id); err != nil {
		return empty, errStorage
	}
	if err = tx.Commit(); err != nil {
		return empty, errStorage
	}
	s.notify()
	return *r, nil
}

type execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func writeRun(ctx context.Context, q execer, workspace string, r *Run, fenced bool) error {
	pairs, _ := json.Marshal(r.Pairs)
	findings, _ := json.Marshal(r.Findings)
	missing, _ := json.Marshal(r.MissingRequirementIDs)
	query := `UPDATE analysis_runs SET status=$4,outcome=$5,error_code=$6,stage=$7,total_pairs=$8,processed_pairs=$9,active_pair_id=$10,pairs=$11::jsonb,findings=$12::jsonb,reason_code=$13,missing_requirement_ids=$14::jsonb,incomplete=$15,completed_at=CASE WHEN $4='running' THEN NULL ELSE clock_timestamp() END,worker_id=$16 WHERE workspace=$1 AND project_id=$2 AND id=$3`
	if fenced {
		query += ` AND status='running' AND worker_id=$16 AND lease_until>clock_timestamp()`
	}
	result, err := q.ExecContext(ctx, query, workspace, r.ProjectID, r.ID, r.Status, r.Outcome, r.ErrorCode, r.Stage, r.TotalPairs, r.ProcessedPairs, r.ActivePairID, string(pairs), string(findings), r.ReasonCode, string(missing), r.incomplete, r.workerID)
	if err != nil {
		return errStorage
	}
	n, err := result.RowsAffected()
	if err != nil {
		return errStorage
	}
	if n != 1 {
		return errOwnership
	}
	return nil
}
func (r *Run) tally() {
	r.ProcessedPairs = 0
	r.Findings = []Finding{}
	for _, pair := range r.Pairs {
		if pair.Status == "completed" {
			r.ProcessedPairs++
			r.Findings = append(r.Findings, Finding{LeftID: pair.Left.ID, RightID: pair.Right.ID, LeftVersion: pair.Left.Version, RightVersion: pair.Right.Version, Verdict: pair.Verdict, Explanation: pair.Explanation})
		} else if pair.Status == "failed" && pair.ErrorCode != "NOT_PROCESSED" {
			r.ProcessedPairs++
		}
	}
}
func interrupt(r *Run, code string) {
	for i := range r.Pairs {
		p := &r.Pairs[i]
		switch p.Status {
		case "running":
			p.Status = "failed"
			p.ErrorCode = code
		case "pending":
			p.Status = "failed"
			p.ErrorCode = "NOT_PROCESSED"
		}
	}
	r.Status = "failed"
	r.Stage = "finished"
	r.ErrorCode = code
	r.Outcome = ""
	r.ActivePairID = ""
	r.tally()
}

func validFinding(pair Pair, f Finding) bool {
	return f.LeftID == pair.Left.ID && f.RightID == pair.Right.ID && f.LeftVersion == pair.Left.Version && f.RightVersion == pair.Right.Version && (f.Verdict == "collision" || f.Verdict == "insufficient" || f.Verdict == "compatible") && strings.TrimSpace(f.Explanation) != "" && utf8.ValidString(f.Explanation) && utf8.RuneCountInString(f.Explanation) <= 2000
}
func outcome(findings []Finding, incomplete bool) string {
	for _, f := range findings {
		if f.Verdict == "collision" {
			return "possible_collision"
		}
		if f.Verdict == "insufficient" {
			incomplete = true
		}
	}
	if incomplete || len(findings) == 0 {
		return "insufficient"
	}
	return "no_collisions_found"
}

// Even a replacement provider that ignores cancellation cannot publish a late
// successful response after the analysis deadline.
func checkedCompare(ctx context.Context, generator Generator, pair Pair) (Finding, string) {
	finding, err := generator.Compare(ctx, pair)
	if ctx.Err() != nil {
		return Finding{}, "TIMEOUT"
	}
	if err != nil {
		return Finding{}, "GENERATION_FAILED"
	}
	if !validFinding(pair, finding) {
		return Finding{}, "INVALID_OUTPUT"
	}
	return finding, ""
}

// Diagnostic describes the saved attempt, not the current index state.
type diagnostic struct {
	ReasonCode            string
	MissingRequirementIDs []string
}

func missingIndexes(sources []Source, states []rag.State, profile string) []string {
	ready := map[string]int{}
	for _, state := range states {
		if state.Status == "ready" && state.Profile == profile {
			ready[state.RequirementID] = state.Version
		}
	}
	missing := []string{}
	for _, source := range sources {
		if ready[source.ID] != source.Version {
			missing = append(missing, source.ID)
		}
	}
	return missing
}
