package classification

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type Progress struct {
	Total, Unclassified, Queued, Classifying, Suggested, NeedsReview, Confirmed, Failed, Stale int32
	Configured                                                                                 bool
	Profile                                                                                    string
}
type Group struct {
	Semantic, Representation string
	Count                    int32
}
type ListItem struct {
	ID, Text, Status, Semantic, Representation, ErrorCode string
	Version, Revision                                     int32
}
type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func projectProfile(ctx context.Context, q queryer, w, p string) (string, error) {
	var profile string
	err := q.QueryRowContext(ctx, `SELECT coalesce(c.profile,'') FROM projects p LEFT JOIN classification_profiles c ON c.workspace=p.workspace WHERE p.workspace=$1 AND p.id=$2`, w, p).Scan(&profile)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrUnavailable
	}
	return profile, err
}
func (s *Service) Progress(ctx context.Context, w, p string) (Progress, error) {
	var out Progress
	profile, e := projectProfile(ctx, s.db, w, p)
	if e != nil {
		return out, e
	}
	out.Profile = profile
	out.Configured = profile != ""
	e = s.db.QueryRowContext(ctx, `SELECT count(*),count(*) FILTER(WHERE status='unclassified'),count(*) FILTER(WHERE status='queued'),count(*) FILTER(WHERE status='classifying'),count(*) FILTER(WHERE status='suggested'),count(*) FILTER(WHERE status='needs_review'),count(*) FILTER(WHERE status='confirmed'),count(*) FILTER(WHERE status='failed'),count(*) FILTER(WHERE status='stale') FROM current_requirement_classification WHERE workspace=$1 AND project_id=$2`, w, p).Scan(&out.Total, &out.Unclassified, &out.Queued, &out.Classifying, &out.Suggested, &out.NeedsReview, &out.Confirmed, &out.Failed, &out.Stale)
	return out, e
}
func (s *Service) Groups(ctx context.Context, w, p string) ([]Group, error) {
	if _, e := projectProfile(ctx, s.db, w, p); e != nil {
		return nil, e
	}
	rows, e := s.db.QueryContext(ctx, `SELECT semantic_type,representation_type,count(*) FROM current_requirement_classification WHERE workspace=$1 AND project_id=$2 GROUP BY semantic_type,representation_type ORDER BY semantic_type,representation_type`, w, p)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Group{}
	for rows.Next() {
		var g Group
		if e = rows.Scan(&g.Semantic, &g.Representation, &g.Count); e != nil {
			return nil, e
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

const listCols = `id,status,semantic_type,representation_type,error_code,version,revision`

func (s *Service) List(ctx context.Context, w, p string, semantic, representation, status *string, limit, offset int32, includeText bool) ([]ListItem, error) {
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, ErrUnavailable
	}
	if _, e := projectProfile(ctx, s.db, w, p); e != nil {
		return nil, e
	}
	content := "''"
	if includeText {
		content = "text"
	}
	return listItems(s.db.QueryContext(ctx, "SELECT "+listCols+","+content+latest+` AND ($3::text IS NULL OR semantic_type=$3) AND ($4::text IS NULL OR representation_type=$4) AND ($5::text IS NULL OR status=$5) ORDER BY requirement_created_at,id LIMIT $6 OFFSET $7`, w, p, semantic, representation, status, limit, offset))
}
func (s *Service) ListByIDs(ctx context.Context, w, p string, ids []string) ([]ListItem, error) {
	if len(ids) > 100 {
		return nil, ErrUnavailable
	}
	if _, e := projectProfile(ctx, s.db, w, p); e != nil {
		return nil, e
	}
	return listItems(s.db.QueryContext(ctx, "SELECT "+listCols+",text"+latest+` AND id=ANY($3) ORDER BY array_position($3::text[],id)`, w, p, ids))
}
func listItems(rows *sql.Rows, e error) ([]ListItem, error) {
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []ListItem{}
	for rows.Next() {
		var v ListItem
		if e = rows.Scan(&v.ID, &v.Status, &v.Semantic, &v.Representation, &v.ErrorCode, &v.Version, &v.Revision, &v.Text); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) editQueue(ctx context.Context, w, p string, run func(*sql.Tx, string) (int32, error)) (int32, error) {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return 0, e
	}
	defer tx.Rollback()
	if e = lock(ctx, tx); e != nil {
		return 0, e
	}
	profile, e := projectProfile(ctx, tx, w, p)
	if e != nil {
		return 0, e
	}
	if profile == "" || s.classifier == nil || s.classifier.Profile() != profile {
		return 0, ErrNotConfigured
	}
	n, e := run(tx, profile)
	if e != nil {
		return 0, e
	}
	return n, tx.Commit()
}

const batchSize = 64

func stageBatch(ctx context.Context, tx *sql.Tx, w, p, profile string, failedOnly bool, requested time.Time) (int32, error) {
	var res sql.Result
	var err error
	if failedOnly {
		res, err = tx.ExecContext(ctx, `WITH batch AS (SELECT j.requirement_id,j.version FROM classification_jobs j JOIN requirements r ON j.requirement_id=r.id AND j.version=r.version
		WHERE j.workspace=$1 AND j.project_id=$2 AND j.profile=$3 AND j.status='failed' AND j.available_at<=$4
		ORDER BY j.requirement_id LIMIT $5)
		UPDATE classification_jobs j SET status='queued',attempts=0,attempt='',error_code='',available_at=clock_timestamp() FROM batch
		WHERE (j.requirement_id,j.version)=(batch.requirement_id,batch.version) AND j.profile=$3`, w, p, profile, requested, batchSize)
	} else {
		res, err = tx.ExecContext(ctx, seedMissing, w, profile, p, batchSize)
	}
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if n < batchSize {
		_, err = tx.ExecContext(ctx, `DELETE FROM classification_backfills WHERE workspace=$1 AND project_id=$2 AND profile=$3 AND failed_only=$4`, w, p, profile, failedOnly)
	}
	return int32(n), err
}
func (s *Service) continueBackfill(ctx context.Context, ws []string) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = lock(ctx, tx); e != nil {
		return e
	}
	var w, p string
	var failed bool
	var requested time.Time
	e = tx.QueryRowContext(ctx, `SELECT b.workspace,b.project_id,b.failed_only,b.requested_at FROM classification_backfills b JOIN classification_profiles a ON a.workspace=b.workspace AND a.profile=b.profile
		WHERE b.workspace=ANY($1) AND b.profile=$2 ORDER BY b.requested_at LIMIT 1`, ws, s.classifier.Profile()).Scan(&w, &p, &failed, &requested)
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	if _, e = stageBatch(ctx, tx, w, p, s.classifier.Profile(), failed, requested); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Service) requestBatch(ctx context.Context, w, p string, failedOnly bool) (int32, error) {
	return s.editQueue(ctx, w, p, func(tx *sql.Tx, profile string) (int32, error) {
		var requested time.Time
		e := tx.QueryRowContext(ctx, `INSERT INTO classification_backfills(workspace,project_id,profile,failed_only) VALUES($1,$2,$3,$4)
		ON CONFLICT(workspace,project_id,profile,failed_only) DO UPDATE SET requested_at=excluded.requested_at RETURNING requested_at`, w, p, profile, failedOnly).Scan(&requested)
		if e != nil {
			return 0, e
		}
		return stageBatch(ctx, tx, w, p, profile, failedOnly, requested)
	})
}
func (s *Service) RetryFailed(ctx context.Context, w, p string) (int32, error) {
	return s.requestBatch(ctx, w, p, true)
}
func (s *Service) Backfill(ctx context.Context, w, p string) (int32, error) {
	return s.requestBatch(ctx, w, p, false)
}

func (s *Service) Retry(ctx context.Context, w, p, id string, version int32, reanalyze bool) (bool, error) {
	n, e := s.editQueue(ctx, w, p, func(tx *sql.Tx, profile string) (int32, error) {
		var current int32
		if e := tx.QueryRowContext(ctx, `SELECT version FROM requirements WHERE workspace=$1 AND project_id=$2 AND id=$3 FOR UPDATE`, w, p, id).Scan(&current); e != nil {
			if errors.Is(e, sql.ErrNoRows) {
				return 0, ErrUnavailable
			}
			return 0, e
		}
		if current != version {
			return 0, ErrConflict
		}
		res, e := tx.ExecContext(ctx, `INSERT INTO classification_jobs(workspace,project_id,requirement_id,version,profile,reanalysis)
  SELECT $1,$2,$3,$4,$5,$6 WHERE $6 OR NOT EXISTS(SELECT FROM requirement_classifications WHERE requirement_id=$3 AND text_version=$4 AND status='confirmed')
  ON CONFLICT(requirement_id,version,profile) DO UPDATE SET status='queued',attempt='',attempts=0,error_code='',available_at=clock_timestamp(),reanalysis=excluded.reanalysis
  WHERE classification_jobs.status IN ('failed','stale') OR (classification_jobs.status='done' AND excluded.reanalysis)`, w, p, id, version, profile, reanalyze)
		if e != nil {
			return 0, e
		}
		n, e := res.RowsAffected()
		return int32(n), e
	})
	return n > 0, e
}
