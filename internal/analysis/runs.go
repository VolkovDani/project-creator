package analysis

import (
	"context"
	"database/sql"
	"errors"
	"github.com/VolkovDani/project-creator/contracts"
	"time"
)

func validPage(limit, offset int) bool { return contracts.ValidateAnalysisPage(limit, offset) }
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
func (s *Service) Get(ctx context.Context, workspace, project, id string) (*Run, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, errStorage
	}
	defer tx.Rollback()
	if err = s.authorize(ctx, tx, workspace, project, true); err != nil {
		return nil, err
	}
	r, err := scanRun(tx.QueryRowContext(ctx, `SELECT `+runColumns+` FROM analysis_runs WHERE workspace=$1 AND project_id=$2 AND id=$3`, workspace, project, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
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

// Summary loads a run without fetching source text, findings, or pair details.
func (s *Service) Summary(ctx context.Context, workspace, project, id string) (*Run, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, errStorage
	}
	defer tx.Rollback()
	if err = s.authorize(ctx, tx, workspace, project, false); err != nil {
		return nil, err
	}
	r, err := scanRun(tx.QueryRowContext(ctx, `SELECT `+compactRunColumns+` FROM analysis_runs WHERE workspace=$1 AND project_id=$2 AND ($3='' OR id=$3) ORDER BY seq DESC LIMIT 1`, workspace, project, id))
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

// Metadata loads stable run provenance on demand without source or pair details.
func (s *Service) Metadata(ctx context.Context, workspace, project, id string) (*Run, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, errStorage
	}
	defer tx.Rollback()
	if err = s.authorize(ctx, tx, workspace, project, false); err != nil {
		return nil, err
	}
	r := &Run{}
	err = tx.QueryRowContext(ctx, `SELECT id,project_id,generator_profile,generator_provenance,embedding_profile,selection_profile FROM analysis_runs WHERE workspace=$1 AND project_id=$2 AND id=$3`, workspace, project, id).Scan(&r.ID, &r.ProjectID, &r.GeneratorProfile, &r.GeneratorProvenance, &r.EmbeddingProfile, &r.SelectionProfile)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, errStorage
	}
	if err = tx.Commit(); err != nil {
		return nil, errStorage
	}
	return r, nil
}

// Cancel stops an active run and fences any provider response that arrives
// after cancellation. Completed pairs remain available for inspection/retry.
func (s *Service) Cancel(ctx context.Context, workspace, project, id string) (Run, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Run{}, errStorage
	}
	defer tx.Rollback()
	if err = s.authorize(ctx, tx, workspace, project, true); err != nil {
		return Run{}, err
	}
	r, err := scanRun(tx.QueryRowContext(ctx, `SELECT `+runColumns+` FROM analysis_runs WHERE workspace=$1 AND project_id=$2 AND id=$3 FOR UPDATE`, workspace, project, id))
	if err == sql.ErrNoRows {
		return Run{}, ErrNotFound
	}
	if err != nil {
		return Run{}, errStorage
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM analysis_replacements WHERE run_id=$1 AND successor_id IS NULL`, id); err != nil {
		return Run{}, errStorage
	}
	if r.Status == "running" {
		if _, err = tx.ExecContext(ctx, `UPDATE analysis_pairs SET status='failed',error_code='CANCELLED' WHERE run_id=$1 AND status IN ('pending','running')`, id); err != nil {
			return Run{}, errStorage
		}
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM analysis_pairs WHERE run_id=$1 AND status='completed'`, id).Scan(&r.ProcessedPairs); err != nil {
			return Run{}, errStorage
		}
		if r.ActivePairID != "" {
			r.ProcessedPairs++
		}
		r.Status = "failed"
		r.ErrorCode = "CANCELLED"
		r.Stage = "finished"
		r.Outcome = ""
		r.ActivePairID = ""
		r.workerID = ""
		if _, err = tx.ExecContext(ctx, `UPDATE analysis_runs SET status='failed',error_code='CANCELLED',outcome='',stage='finished',active_pair_id='',worker_id='',processed_pairs=$4,completed_at=clock_timestamp() WHERE workspace=$1 AND project_id=$2 AND id=$3`, workspace, project, id, r.ProcessedPairs); err != nil {
			return Run{}, errStorage
		}
	}
	r, err = scanRun(tx.QueryRowContext(ctx, `SELECT `+runColumns+` FROM analysis_runs WHERE workspace=$1 AND project_id=$2 AND id=$3`, workspace, project, id))
	if err != nil {
		return Run{}, errStorage
	}
	var cancelWorker context.CancelFunc
	if r.ErrorCode == "CANCELLED" {
		s.workersMu.Lock()
		cancelWorker = s.workers[id]
		s.workersMu.Unlock()
	}
	if err = tx.Commit(); err != nil {
		return Run{}, errStorage
	}
	if r.ErrorCode == "CANCELLED" {
		if cancelWorker != nil {
			cancelWorker()
		}
		s.notify()
	}
	if err = s.markStale(ctx, workspace, project, r); err != nil {
		return Run{}, err
	}
	return *r, nil
}

func (s *Service) History(ctx context.Context, workspace, project string, limit, offset int) ([]Run, error) {
	if !validPage(limit, offset) {
		return nil, errors.New("invalid analysis page")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, errStorage
	}
	defer tx.Rollback()
	if err = s.authorize(ctx, tx, workspace, project, true); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM analysis_runs WHERE workspace=$1 AND project_id=$2 ORDER BY seq DESC LIMIT $3 OFFSET $4`, workspace, project, limit, offset)
	if err != nil {
		return nil, errStorage
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, errStorage
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, errStorage
	}
	out := []Run{}
	for _, id := range ids {
		r, e := scanRun(tx.QueryRowContext(ctx, `SELECT `+runColumns+` FROM analysis_runs WHERE workspace=$1 AND project_id=$2 AND id=$3`, workspace, project, id))
		if e != nil {
			return nil, errStorage
		}
		r.Pairs = []PairResult{}
		r.Findings = []Finding{}
		r.Sources = []Source{}
		out = append(out, *r)
	}
	if err = tx.Commit(); err != nil {
		return nil, errStorage
	}
	for i := range out {
		if err = s.markStale(ctx, workspace, project, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Start preserves the programmatic full-scan entry point. The UI explicitly
// chooses related by default through StartWithOptions.
func (s *Service) Start(ctx context.Context, workspace, project string) (Run, error) {
	return s.StartWithOptions(ctx, workspace, project, Options{Mode: "all", Threshold: contracts.AnalysisConstraints().DefaultThreshold})
}
func (s *Service) StartWithOptions(ctx context.Context, workspace, project string, opts Options) (Run, error) {
	return s.startWithOptions(ctx, workspace, project, opts, "")
}
func (s *Service) startWithOptions(ctx context.Context, workspace, project string, opts Options, predecessor string) (Run, error) {
	empty := Run{ProjectID: project, Sources: []Source{}, Findings: []Finding{}, Pairs: []PairResult{}, MissingRequirementIDs: []string{}}
	if opts.Mode == "" {
		opts.Mode, opts.Threshold = contracts.AnalysisDefaults()
	}
	if !contracts.ValidateAnalysisSelection(opts.Mode, opts.Threshold) {
		return empty, errors.New("invalid analysis selection")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return empty, errStorage
	}
	defer tx.Rollback()
	if err = s.authorize(ctx, tx, workspace, project, true); err != nil {
		return empty, err
	}
	if predecessor != "" {
		var expected string
		if err = tx.QueryRowContext(ctx, `SELECT r.id FROM analysis_runs r JOIN analysis_replacements q ON q.run_id=r.id WHERE r.workspace=$1 AND r.project_id=$2 AND r.id=$3 AND r.status<>'running' AND q.successor_id IS NULL AND r.id=(SELECT id FROM analysis_runs WHERE workspace=$1 AND project_id=$2 ORDER BY seq DESC LIMIT 1) FOR UPDATE OF r,q`, workspace, project, predecessor).Scan(&expected); err != nil {
			return empty, ErrReplacementUnavailable
		}
		if err = tx.QueryRowContext(ctx, `SELECT mode,threshold FROM analysis_replacements WHERE run_id=$1`, predecessor).Scan(&opts.Mode, &opts.Threshold); err != nil {
			return empty, errStorage
		}
	}
	if err = s.recoverProject(ctx, tx, workspace, project); err != nil {
		return empty, err
	}
	active, err := scanRun(tx.QueryRowContext(ctx, `SELECT `+runColumns+` FROM analysis_runs WHERE workspace=$1 AND project_id=$2 AND status='running' FOR UPDATE`, workspace, project))
	if err == nil {
		sig, e := sourceSignature(ctx, tx, workspace, project)
		if e != nil {
			return empty, e
		}
		if active.GeneratorProfile == s.generatorProfile() && (opts.Mode == "all" || active.EmbeddingProfile == s.embeddingProfile) && active.Mode == opts.Mode && active.SimilarityThreshold == opts.Threshold && active.signature == sig {
			if err = tx.Commit(); err != nil {
				return empty, errStorage
			}
			return *active, nil
		}
		code := "PROFILE_CHANGED"
		if active.signature != sig {
			code = "SOURCE_CHANGED"
		}
		if err = interruptRun(ctx, tx, workspace, active, code, false); err != nil {
			return empty, err
		}
	} else if err != sql.ErrNoRows {
		return empty, errStorage
	}
	if predecessor == "" {
		if _, err = tx.ExecContext(ctx, `DELETE FROM analysis_replacements q USING analysis_runs r WHERE q.run_id=r.id AND r.workspace=$1 AND r.project_id=$2 AND q.successor_id IS NULL`, workspace, project); err != nil {
			return empty, errStorage
		}
	}
	signature, err := sourceSignature(ctx, tx, workspace, project)
	if err != nil {
		return empty, err
	}
	id, err := identifier()
	if err != nil {
		return empty, err
	}
	r := empty
	r.ID = id
	r.Revision = "1"
	r.Status = "running"
	r.Stage = "checking_index"
	r.signature = signature
	r.GeneratorProfile = s.generatorProfile()
	r.GeneratorProvenance = GeneratorProvenance(s.generator)
	r.EmbeddingProfile = s.embeddingProfile
	r.Mode = opts.Mode
	r.SimilarityThreshold = opts.Threshold
	r.SelectionProfile, _ = jsonMarshal(struct {
		Version          string  `json:"version"`
		Mode             string  `json:"mode"`
		Threshold        float64 `json:"threshold"`
		EmbeddingProfile string  `json:"embeddingProfile"`
	}{selectionVersion, r.Mode, r.SimilarityThreshold, r.EmbeddingProfile})
	r.fullQueue = true
	var created time.Time
	err = tx.QueryRowContext(ctx, `INSERT INTO analysis_runs(id,workspace,project_id,status,generator_profile,generator_provenance,embedding_profile,source_signature,sources,lease_until,stage,mode,similarity_threshold,selection_profile,full_queue) VALUES($1,$2,$3,'running',$4,$5,$6,$7,'[]','infinity','checking_index',$8,$9,$10,true) RETURNING created_at`, id, workspace, project, r.GeneratorProfile, r.GeneratorProvenance, r.EmbeddingProfile, signature, r.Mode, r.SimilarityThreshold, r.SelectionProfile).Scan(&created)
	if err != nil {
		return empty, errStorage
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO analysis_sources(run_id,requirement_id,source) SELECT $1,id,jsonb_build_object('id',id,'version',version,'text',text) FROM requirements WHERE workspace=$2 AND project_id=$3`, id, workspace, project)
	if err != nil {
		return empty, errStorage
	}
	// Return the committed bounded source page immediately, including before
	// the worker claims this run. CLI verification validates these snapshots.
	persisted, readErr := scanRun(tx.QueryRowContext(ctx, `SELECT `+runColumns+` FROM analysis_runs WHERE id=$1`, id))
	if readErr != nil {
		return empty, errStorage
	}
	r.Sources = persisted.Sources
	r.CreatedAt = created.UTC().Format(time.RFC3339Nano)
	if predecessor != "" {
		if _, err = tx.ExecContext(ctx, `UPDATE analysis_replacements SET successor_id=$2 WHERE run_id=$1`, predecessor, id); err != nil {
			return empty, errStorage
		}
	}

	if err = tx.Commit(); err != nil {
		return empty, errStorage
	}
	s.notify()
	return r, nil
}
func (s *Service) Retry(ctx context.Context, workspace, project, id string) (Run, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Run{}, errStorage
	}
	defer tx.Rollback()
	if err = s.authorize(ctx, tx, workspace, project, true); err != nil {
		return Run{}, err
	}
	if err = s.recoverProject(ctx, tx, workspace, project); err != nil {
		return Run{}, err
	}
	r, err := scanRun(tx.QueryRowContext(ctx, `SELECT `+runColumns+` FROM analysis_runs WHERE workspace=$1 AND project_id=$2 ORDER BY seq DESC LIMIT 1 FOR UPDATE`, workspace, project))
	if err != nil || r.ID != id {
		return Run{}, ErrRetryUnavailable
	}
	if r.Status == "running" {
		if err = tx.Commit(); err != nil {
			return Run{}, errStorage
		}
		return *r, nil
	}
	signature, err := sourceSignature(ctx, tx, workspace, project)
	if err != nil {
		return Run{}, err
	}
	if r.ErrorCode == "REPLACED" || r.Status != "failed" || signature != r.signature || r.GeneratorProfile != s.generatorProfile() || (r.Mode == "related" && r.EmbeddingProfile != s.embeddingProfile) {
		return Run{}, ErrRetryUnavailable
	}
	if r.FailedPairs == 0 && r.TotalPairs != nil {
		return Run{}, ErrRetryUnavailable
	}
	if _, err = tx.ExecContext(ctx, `UPDATE analysis_pairs SET status='pending',error_code='' WHERE run_id=$1 AND status='failed'`, id); err != nil {
		return Run{}, errStorage
	}
	r.Status = "running"
	r.Stage = "comparing"
	if r.TotalPairs == nil {
		r.Stage = "finding_pairs"
	}
	r.ErrorCode = ""
	r.ReasonCode = ""
	r.MissingRequirementIDs = []string{}
	r.Outcome = ""
	r.CompletedAt = ""
	r.workerID = ""
	r.ActivePairID = ""
	r.ProcessedPairs = r.CompletedPairs
	if err = writeRun(ctx, tx, workspace, r, false); err != nil {
		return Run{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE analysis_runs SET lease_until='infinity' WHERE id=$1`, id); err != nil {
		return Run{}, errStorage
	}
	current, err := scanRun(tx.QueryRowContext(ctx, `SELECT `+runColumns+` FROM analysis_runs WHERE id=$1`, id))
	if err != nil {
		return Run{}, errStorage
	}
	if err = tx.Commit(); err != nil {
		return Run{}, errStorage
	}
	s.notify()
	return *current, nil
}

type execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func writeRun(ctx context.Context, q execer, workspace string, r *Run, fenced bool) error {
	query := `UPDATE analysis_runs SET status=$4,outcome=$5,error_code=$6,stage=$7,total_pairs=$8,active_pair_id=$9,reason_code=$10,missing_requirement_ids=$11::jsonb,incomplete=$12,completed_at=CASE WHEN $4='running' THEN NULL ELSE clock_timestamp() END,worker_id=$13,pair_cursor_left=$14,pair_cursor_right=$15,possible_pairs=$16,processed_pairs=$17 WHERE workspace=$1 AND project_id=$2 AND id=$3`
	if fenced {
		query += ` AND status='running' AND worker_id=$13 AND lease_until>clock_timestamp()`
	}
	missing, _ := jsonMarshal(r.MissingRequirementIDs)
	result, err := q.ExecContext(ctx, query, workspace, r.ProjectID, r.ID, r.Status, r.Outcome, r.ErrorCode, r.Stage, r.TotalPairs, r.ActivePairID, r.ReasonCode, missing, r.incomplete, r.workerID, r.cursorLeft, r.cursorRight, r.PossiblePairs, r.ProcessedPairs)
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
func interruptRun(ctx context.Context, tx *sql.Tx, workspace string, r *Run, code string, fenced bool) error {
	if err := fenceRun(ctx, tx, workspace, r, fenced); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE analysis_pairs SET error_code=CASE WHEN status='running' THEN $2 ELSE 'NOT_PROCESSED' END,status='failed' WHERE run_id=$1 AND status IN ('running','pending')`, r.ID, code)
	if err != nil {
		return errStorage
	}
	// Failure/recovery is exceptional: recount once after marking interrupted
	// work, rather than scanning the complete queue on every successful pair.
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM analysis_pairs WHERE run_id=$1 AND (status='completed' OR (status='failed' AND error_code<>'NOT_PROCESSED'))`, r.ID).Scan(&r.ProcessedPairs); err != nil {
		return errStorage
	}
	r.Status = "failed"
	r.Stage = "finished"
	r.ErrorCode = code
	r.Outcome = ""
	r.ActivePairID = ""
	return writeRun(ctx, tx, workspace, r, fenced)
}
func fenceRun(ctx context.Context, tx *sql.Tx, workspace string, r *Run, fenced bool) error {
	query := `SELECT id FROM analysis_runs WHERE workspace=$1 AND project_id=$2 AND id=$3`
	args := []any{workspace, r.ProjectID, r.ID}
	if fenced {
		query += ` AND status='running' AND worker_id=$4 AND lease_until>clock_timestamp()`
		args = append(args, r.workerID)
	}
	query += ` FOR UPDATE`
	var id string
	if err := tx.QueryRowContext(ctx, query, args...).Scan(&id); err != nil {
		return errOwnership
	}
	return nil
}
