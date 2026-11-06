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
	ID               string   `json:"id"`
	Position         int      `json:"position"`
	Status           string   `json:"status"`
	Left             Source   `json:"left"`
	Right            Source   `json:"right"`
	Verdict          string   `json:"verdict"`
	Explanation      string   `json:"explanation"`
	ErrorCode        string   `json:"errorCode"`
	ReusedFrom       string   `json:"reusedFrom"`
	CheckedAt        string   `json:"checkedAt"`
	GeneratorProfile string   `json:"generatorProfile"`
	SelectionReasons []string `json:"selectionReasons"`
	SimilarityScore  *float64 `json:"similarityScore"`
	ResultOrigin     string   `json:"resultOrigin"`
}

type Options struct {
	Mode      string
	Threshold float64
}

const selectionVersion = "related-v1:cosine-max-chunk:two-shared-terms:exact-fields-v1"

type Run struct {
	ReasonCode                                                                                                                                         string
	MissingRequirementIDs                                                                                                                              []string
	ID, ProjectID, Status, Outcome, ErrorCode, CreatedAt, CompletedAt, GeneratorProfile, GeneratorProvenance, EmbeddingProfile                         string
	Revision                                                                                                                                           string
	Stage                                                                                                                                              string
	TotalPairs                                                                                                                                         *int
	PossiblePairs                                                                                                                                      *int
	NotSelectedPairs                                                                                                                                   *int
	ProcessedPairs                                                                                                                                     int
	ReusedPairs, NewPairs, CompletedPairs, PendingPairs, RunningPairs, FailedPairs, CollisionPairs, CompatiblePairs, InsufficientPairs, DuplicatePairs int
	ActivePairID                                                                                                                                       string
	Pairs                                                                                                                                              []PairResult
	Stale                                                                                                                                              bool
	Findings                                                                                                                                           []Finding
	Sources                                                                                                                                            []Source
	Mode                                                                                                                                               string
	SimilarityThreshold                                                                                                                                float64
	SelectionProfile                                                                                                                                   string
	signature                                                                                                                                          string
	incomplete                                                                                                                                         bool
	workerID                                                                                                                                           string
	cursorLeft, cursorRight                                                                                                                            string
	fullQueue                                                                                                                                          bool
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
	workersMu          sync.Mutex
	workers            map[string]context.CancelFunc
}

func New(db *sql.DB, retriever *rag.Service, generator Generator, embeddingProfile string) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{db: db, retriever: retriever, generator: generator, embeddingProfile: embeddingProfile,
		timeout: 120 * time.Second, preparationTimeout: 180 * time.Second, heartbeatInterval: 5 * time.Second, ctx: ctx, cancel: cancel, done: make(chan struct{}), wake: make(chan struct{}, 1), workers: make(map[string]context.CancelFunc)}
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

// Hash each row before aggregation so signature checks never build a JSON
// document containing every source text. Source edits are serialized against
// result commits by the project lock; versions and exact text both participate.
func sourceSignature(ctx context.Context, q queryer, workspace, project string) (string, error) {
	var signature string
	err := q.QueryRowContext(ctx, `SELECT md5(coalesce(string_agg(md5(jsonb_build_array(id,version,text)::text),'' ORDER BY id),'')) FROM requirements WHERE workspace=$1 AND project_id=$2`, workspace, project).Scan(&signature)
	if err != nil {
		return "", errStorage
	}
	return signature, nil
}

// Compatibility collections are bounded pages, never the complete queue. All
// counters below cover the run, independently of those pages.
const pairJSON = `jsonb_build_object('id',p.id,'position',p.position,'status',p.status,'left',p.left_source,'right',p.right_source,'verdict',p.verdict,'explanation',p.explanation,'errorCode',p.error_code,'reusedFrom',p.reused_from,'checkedAt',coalesce(to_char(p.checked_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),''),'generatorProfile',p.generator_profile,'selectionReasons',p.selection_reasons,'similarityScore',p.similarity_score,'resultOrigin',p.result_origin)`
const runColumns = `id,project_id,status,outcome,error_code,generator_profile,embedding_profile,source_signature,
 coalesce((SELECT jsonb_agg(source ORDER BY requirement_id) FROM (SELECT source,requirement_id FROM analysis_sources WHERE run_id=analysis_runs.id ORDER BY requirement_id LIMIT 100) s),'[]'::jsonb),
 coalesce((SELECT jsonb_agg(jsonb_build_object('leftId',p.left_source->>'id','rightId',p.right_source->>'id','leftVersion',p.left_source->'version','rightVersion',p.right_source->'version','verdict',p.verdict,'explanation',p.explanation) ORDER BY p.position) FROM (SELECT * FROM analysis_pairs WHERE run_id=analysis_runs.id AND status='completed' ORDER BY position LIMIT 100) p),'[]'::jsonb),
 created_at,completed_at,reason_code,missing_requirement_ids,stage,total_pairs,processed_pairs,active_pair_id,
 coalesce((SELECT jsonb_agg(` + pairJSON + ` ORDER BY p.position) FROM (SELECT * FROM analysis_pairs WHERE run_id=analysis_runs.id ORDER BY position LIMIT 100) p),'[]'::jsonb),
 incomplete,worker_id,generator_provenance,mode,similarity_threshold,selection_profile,possible_pairs,pair_cursor_left,pair_cursor_right,full_queue,
 (SELECT jsonb_build_array(count(*) FILTER(WHERE status='completed' AND reused_from<>''),count(*) FILTER(WHERE status='completed' AND reused_from=''),count(*) FILTER(WHERE status='completed'),count(*) FILTER(WHERE status='pending'),count(*) FILTER(WHERE status='running'),count(*) FILTER(WHERE status='failed'),count(*) FILTER(WHERE status='completed' AND verdict='collision'),count(*) FILTER(WHERE status='completed' AND verdict='compatible'),count(*) FILTER(WHERE status='completed' AND verdict='insufficient'),count(*) FILTER(WHERE status='completed' AND verdict='duplicate'),analysis_runs.processed_pairs) FROM analysis_pairs WHERE run_id=analysis_runs.id),revision::text`

const compactRunColumns = `id,project_id,status,outcome,error_code,generator_profile,embedding_profile,source_signature,
 '[]'::jsonb,'[]'::jsonb,created_at,completed_at,reason_code,missing_requirement_ids,stage,total_pairs,processed_pairs,active_pair_id,
 '[]'::jsonb,incomplete,worker_id,''::text,mode,similarity_threshold,''::text,possible_pairs,pair_cursor_left,pair_cursor_right,full_queue,
 (SELECT jsonb_build_array(count(*) FILTER(WHERE status='completed' AND reused_from<>''),count(*) FILTER(WHERE status='completed' AND reused_from=''),count(*) FILTER(WHERE status='completed'),count(*) FILTER(WHERE status='pending'),count(*) FILTER(WHERE status='running'),count(*) FILTER(WHERE status='failed'),count(*) FILTER(WHERE status='completed' AND verdict='collision'),count(*) FILTER(WHERE status='completed' AND verdict='compatible'),count(*) FILTER(WHERE status='completed' AND verdict='insufficient'),count(*) FILTER(WHERE status='completed' AND verdict='duplicate'),analysis_runs.processed_pairs) FROM analysis_pairs WHERE run_id=analysis_runs.id),revision::text`

func scanRun(row *sql.Row) (*Run, error) {
	r := &Run{Sources: []Source{}, Findings: []Finding{}, Pairs: []PairResult{}, MissingRequirementIDs: []string{}}
	var sources, findings, missing, pairs, counts []byte
	var created time.Time
	var completed sql.NullTime
	var total, possible sql.NullInt64
	err := row.Scan(&r.ID, &r.ProjectID, &r.Status, &r.Outcome, &r.ErrorCode, &r.GeneratorProfile, &r.EmbeddingProfile, &r.signature, &sources, &findings, &created, &completed, &r.ReasonCode, &missing, &r.Stage, &total, &r.ProcessedPairs, &r.ActivePairID, &pairs, &r.incomplete, &r.workerID, &r.GeneratorProvenance, &r.Mode, &r.SimilarityThreshold, &r.SelectionProfile, &possible, &r.cursorLeft, &r.cursorRight, &r.fullQueue, &counts, &r.Revision)
	if err != nil {
		return nil, err
	}
	var c []int
	if json.Unmarshal(sources, &r.Sources) != nil || json.Unmarshal(findings, &r.Findings) != nil || json.Unmarshal(missing, &r.MissingRequirementIDs) != nil || json.Unmarshal(pairs, &r.Pairs) != nil || json.Unmarshal(counts, &c) != nil || len(c) != 11 {
		return nil, errStorage
	}
	r.ReusedPairs, r.NewPairs, r.CompletedPairs, r.PendingPairs, r.RunningPairs, r.FailedPairs, r.CollisionPairs, r.CompatiblePairs, r.InsufficientPairs, r.DuplicatePairs, r.ProcessedPairs = c[0], c[1], c[2], c[3], c[4], c[5], c[6], c[7], c[8], c[9], c[10]
	if total.Valid {
		n := int(total.Int64)
		r.TotalPairs = &n
	}
	if possible.Valid {
		n := int(possible.Int64)
		r.PossiblePairs = &n
		if r.TotalPairs != nil {
			m := n - *r.TotalPairs
			r.NotSelectedPairs = &m
		}
	}
	r.CreatedAt = created.UTC().Format(time.RFC3339Nano)
	if completed.Valid {
		r.CompletedAt = completed.Time.UTC().Format(time.RFC3339Nano)
	}
	return r, nil
}
func (s *Service) markStale(ctx context.Context, workspace, project string, r *Run) error {
	signature, err := sourceSignature(ctx, s.db, workspace, project)
	if err != nil {
		return err
	}
	r.Stale = signature != r.signature || r.GeneratorProfile != s.generatorProfile() || (r.Mode == "related" && r.EmbeddingProfile != s.embeddingProfile)
	return nil
}

// All public mutations and lease recovery lock project before analysis rows.
// Recovery never dispatches a model: an interrupted attempt needs explicit retry.
func (s *Service) recoverProject(ctx context.Context, tx *sql.Tx, workspace, project string) error {
	r, err := scanRun(tx.QueryRowContext(ctx, `SELECT `+runColumns+` FROM analysis_runs WHERE workspace=$1 AND project_id=$2 AND status='running' AND ((worker_id<>'' AND lease_until<=clock_timestamp()) OR (worker_id='' AND (generator_profile<>$3 OR (mode='related' AND embedding_profile<>$4)))) FOR UPDATE`, workspace, project, s.generatorProfile(), s.embeddingProfile))
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
	return interruptRun(ctx, tx, workspace, r, code, false)
}
func validFinding(pair Pair, f Finding) bool {
	return f.LeftID == pair.Left.ID && f.RightID == pair.Right.ID && f.LeftVersion == pair.Left.Version && f.RightVersion == pair.Right.Version && (f.Verdict == "collision" || f.Verdict == "insufficient" || f.Verdict == "compatible" || f.Verdict == "duplicate") && strings.TrimSpace(f.Explanation) != "" && utf8.ValidString(f.Explanation) && utf8.RuneCountInString(f.Explanation) <= 2000
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
		var failure npuFailure
		if errors.As(err, &failure) && knownNPUFailure(string(failure)) {
			return Finding{}, string(failure)
		}
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
