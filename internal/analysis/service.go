package analysis

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/VolkovDani/project-creator/internal/rag"
)

var ErrNotFound = errors.New("project not found")
var errStorage = errors.New("analysis storage unavailable")

type Run struct {
	ReasonCode                                                                                            string
	MissingRequirementIDs                                                                                 []string
	ID, ProjectID, Status, Outcome, ErrorCode, CreatedAt, CompletedAt, GeneratorProfile, EmbeddingProfile string
	Stale                                                                                                 bool
	Findings                                                                                              []Finding
	Sources                                                                                               []Source
	signature                                                                                             string
}
type Service struct {
	db               *sql.DB
	retriever        *rag.Service
	generator        Generator
	embeddingProfile string
	timeout          time.Duration
}

func New(db *sql.DB, retriever *rag.Service, generator Generator, embeddingProfile string) *Service {
	return &Service{db: db, retriever: retriever, generator: generator, embeddingProfile: embeddingProfile, timeout: 180 * time.Second}
}
func (s *Service) generatorProfile() string {
	if s.generator == nil {
		return ""
	}
	return s.generator.Profile()
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

// Both subqueries share one statement snapshot. Store all IDs/versions in the
// signature even when the bounded analysis can retain only the first100 texts.
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

const runColumns = `id,project_id,status,outcome,error_code,generator_profile,embedding_profile,source_signature,sources,findings,created_at,completed_at,reason_code,missing_requirement_ids`

func scanRun(row *sql.Row) (*Run, error) {
	r := &Run{Sources: []Source{}, Findings: []Finding{}}
	var sources, findings, missing []byte
	var created time.Time
	var completed sql.NullTime
	err := row.Scan(&r.ID, &r.ProjectID, &r.Status, &r.Outcome, &r.ErrorCode, &r.GeneratorProfile, &r.EmbeddingProfile, &r.signature, &sources, &findings, &created, &completed, &r.ReasonCode, &missing)
	if err != nil {
		return nil, err
	}
	if json.Unmarshal(sources, &r.Sources) != nil || json.Unmarshal(findings, &r.Findings) != nil || json.Unmarshal(missing, &r.MissingRequirementIDs) != nil {
		return nil, errStorage
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
func (s *Service) Latest(ctx context.Context, workspace, project string) (*Run, error) {
	if err := s.authorize(ctx, s.db, workspace, project, false); err != nil {
		return nil, err
	}
	r, err := scanRun(s.db.QueryRowContext(ctx, `SELECT `+runColumns+` FROM analysis_runs WHERE workspace=$1 AND project_id=$2 ORDER BY seq DESC LIMIT 1`, workspace, project))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, errStorage
	}
	if err = s.markStale(ctx, workspace, project, r); err != nil {
		return nil, err
	}
	return r, nil
}
func (s *Service) Start(ctx context.Context, workspace, project string) (Run, error) {
	empty := Run{ProjectID: project, Sources: []Source{}, Findings: []Finding{}}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return empty, errStorage
	}
	defer tx.Rollback()
	if err = s.authorize(ctx, tx, workspace, project, true); err != nil {
		return empty, err
	}
	// A failed worker or changed configuration can be retried explicitly. Updating
	// its status first prevents an old completion from replacing a newer run.
	if _, err = tx.ExecContext(ctx, `UPDATE analysis_runs SET status='failed',error_code='INTERRUPTED',completed_at=clock_timestamp() WHERE workspace=$1 AND project_id=$2 AND status='running' AND (lease_until<=clock_timestamp() OR generator_profile<>$3 OR embedding_profile<>$4)`, workspace, project, s.generatorProfile(), s.embeddingProfile); err != nil {
		return empty, errStorage
	}
	active, err := scanRun(tx.QueryRowContext(ctx, `SELECT `+runColumns+` FROM analysis_runs WHERE workspace=$1 AND project_id=$2 AND status='running'`, workspace, project))
	if err == nil {
		if e := tx.Commit(); e != nil {
			return empty, errStorage
		}
		if e := s.markStale(ctx, workspace, project, active); e != nil {
			return empty, e
		}
		return *active, nil
	}
	if err != sql.ErrNoRows {
		return empty, errStorage
	}
	sources, signature, incomplete, err := snapshot(ctx, tx, workspace, project)
	if err != nil {
		return empty, err
	}
	var bytes [16]byte
	if _, err = rand.Read(bytes[:]); err != nil {
		return empty, errors.New("analysis identifier unavailable")
	}
	r := Run{ID: hex.EncodeToString(bytes[:]), ProjectID: project, Status: "running", Sources: sources, Findings: []Finding{}, signature: signature, GeneratorProfile: s.generatorProfile(), EmbeddingProfile: s.embeddingProfile}
	raw, _ := json.Marshal(sources)
	if _, err = tx.ExecContext(ctx, `INSERT INTO analysis_runs(id,workspace,project_id,status,generator_profile,embedding_profile,source_signature,sources,lease_until) VALUES($1,$2,$3,'running',$4,$5,$6,$7::jsonb,clock_timestamp()+interval '3 minutes')`, r.ID, workspace, project, r.GeneratorProfile, r.EmbeddingProfile, signature, string(raw)); err != nil {
		return empty, errStorage
	}
	if err = tx.Commit(); err != nil {
		return empty, errStorage
	}
	analysisCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	findings, code, incomplete, diagnostic := s.analyze(analysisCtx, workspace, project, sources, incomplete)
	r.Findings = findings
	r.ReasonCode = diagnostic.ReasonCode
	r.MissingRequirementIDs = diagnostic.MissingRequirementIDs
	r.Status = "completed"
	r.Outcome = outcome(findings, incomplete)
	if code != "" {
		r.Status = "failed"
		r.Outcome = ""
		r.ErrorCode = code
	}
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer finishCancel()
	raw, _ = json.Marshal(findings)
	missingRaw, _ := json.Marshal(r.MissingRequirementIDs)
	if _, err = s.db.ExecContext(finishCtx, `UPDATE analysis_runs SET status=$4,outcome=$5,error_code=$6,findings=$7::jsonb,reason_code=$8,missing_requirement_ids=$9::jsonb,completed_at=clock_timestamp(),lease_until=clock_timestamp() WHERE workspace=$1 AND project_id=$2 AND id=$3 AND status='running'`, workspace, project, r.ID, r.Status, r.Outcome, r.ErrorCode, string(raw), r.ReasonCode, string(missingRaw)); err != nil {
		return r, errStorage
	}
	stored, err := scanRun(s.db.QueryRowContext(finishCtx, `SELECT `+runColumns+` FROM analysis_runs WHERE workspace=$1 AND project_id=$2 AND id=$3`, workspace, project, r.ID))
	if err != nil {
		return r, errStorage
	}
	if err = s.markStale(finishCtx, workspace, project, stored); err != nil {
		return r, err
	}
	return *stored, nil
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
func (s *Service) analyze(ctx context.Context, workspace, project string, sources []Source, incomplete bool) ([]Finding, string, bool, diagnostic) {
	findings := []Finding{}
	d := diagnostic{MissingRequirementIDs: []string{}}
	if incomplete {
		d.ReasonCode = "REQUIREMENT_LIMIT"
		return findings, "", true, d
	}
	if len(sources) < 2 {
		d.ReasonCode = "TOO_FEW_REQUIREMENTS"
		return findings, "", true, d
	}
	if s.generator == nil || s.generatorProfile() == "" || s.retriever == nil || s.embeddingProfile == "" {
		return findings, "NOT_CONFIGURED", true, d
	}
	states, err := s.retriever.Status(ctx, workspace, project, 100, 0)
	if err != nil {
		if ctx.Err() != nil {
			return findings, "TIMEOUT", true, d
		}
		return findings, "RETRIEVAL_FAILED", true, d
	}
	d.MissingRequirementIDs = missingIndexes(sources, states, s.embeddingProfile)
	if len(d.MissingRequirementIDs) > 0 {
		d.ReasonCode = "INDEX_INCOMPLETE"
		return findings, "", true, d
	}
	byID := map[string]Source{}
	for _, source := range sources {
		byID[source.ID] = source
	}
	pairs := []Pair{}
	seen := map[string]bool{}
	for _, source := range sources {
		if !validSource(source) {
			incomplete = true
			continue
		}
		hits, e := s.retriever.Search(ctx, workspace, project, source.Text, 6)
		if e != nil {
			if ctx.Err() != nil {
				return findings, "TIMEOUT", true, d
			}
			return findings, "RETRIEVAL_FAILED", true, d
		}
		for _, hit := range hits {
			other, exists := byID[hit.RequirementID]
			if !exists || other.ID == source.ID || other.Version != hit.Version {
				continue
			}
			if !validSource(other) {
				incomplete = true
				continue
			}
			left, right := source, other
			if left.ID > right.ID {
				left, right = right, left
			}
			key := left.ID + "\x00" + right.ID
			if seen[key] {
				continue
			}
			seen[key] = true
			if len(pairs) >= 20 {
				incomplete = true
				continue
			}
			pairs = append(pairs, Pair{Left: left, Right: right})
		}
	}
	for _, pair := range pairs {
		if ctx.Err() != nil {
			return findings, "TIMEOUT", true, d
		}
		var current int
		err = s.db.QueryRowContext(ctx, `SELECT count(*) FROM requirements WHERE workspace=$1 AND project_id=$2 AND ((id=$3 AND version=$4) OR (id=$5 AND version=$6))`, workspace, project, pair.Left.ID, pair.Left.Version, pair.Right.ID, pair.Right.Version).Scan(&current)
		if err != nil {
			if ctx.Err() != nil {
				return findings, "TIMEOUT", true, d
			}
			return findings, "RETRIEVAL_FAILED", true, d
		}
		if current != 2 {
			incomplete = true
			continue
		}
		finding, code := checkedCompare(ctx, s.generator, pair)
		if code != "" {
			return findings, code, true, d
		}
		findings = append(findings, finding)
	}
	if incomplete {
		d.ReasonCode = "ANALYSIS_LIMIT"
	} else if len(findings) == 0 {
		d.ReasonCode = "NO_CANDIDATES"
	} else {
		for _, f := range findings {
			if f.Verdict == "insufficient" {
				d.ReasonCode = "MODEL_INSUFFICIENT"
				break
			}
		}
	}
	return findings, "", incomplete, d
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
