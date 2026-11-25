package analysis

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/VolkovDani/project-creator/contracts"
)

const similarPairWindow = 128

const (
	similarPairsReady         = "ready"
	similarPairsNotConfigured = "embeddings_not_configured"
	similarPairsIncomplete    = "index_incomplete"
	similarPairsStale         = "cursor_stale"
)

type SimilarPair struct {
	Pair
	SimilarityScore float64
}

type SimilarPairPosition struct {
	Snapshot string
	LeftID   string
	RightID  string
}

type SimilarPairPage struct {
	Status              string
	EmbeddingProfile    string
	MissingRequirements int
	Items               []SimilarPair
	Next                *SimilarPairPosition
}

// chunkPairCosine uses fixed SQL expressions supplied by the two callers.
// Both the durable analysis queue and the read-only preview score source
// versions by the closest pair of chunks in the selected profile.
func chunkPairCosine(leftID, rightID, leftVersion, rightVersion string) string {
	return fmt.Sprintf(`
		SELECT max(1-(lc.embedding OPERATOR(public.<=>) rc.embedding))
		FROM requirement_chunks lc
		JOIN requirement_chunks rc ON rc.workspace=lc.workspace AND rc.project_id=lc.project_id AND rc.profile=lc.profile
		WHERE lc.workspace=$5 AND lc.project_id=$6 AND lc.profile=$7
		  AND lc.requirement_id=%s AND rc.requirement_id=%s
		  AND lc.version=%s AND rc.version=%s
	`, leftID, rightID, leftVersion, rightVersion)
}

var similarPairsQuery = `
	WITH candidates AS MATERIALIZED (
		SELECT a.id AS left_id, a.version AS left_version, a.text AS left_text,
		       b.id AS right_id, b.version AS right_version, b.text AS right_text
		FROM active_requirements a
		JOIN LATERAL (
			SELECT id, version, text FROM active_requirements
			WHERE workspace=$5 AND project_id=$6 AND
			  id > CASE WHEN a.id=$1 THEN greatest(a.id,$2) ELSE a.id END
			ORDER BY id LIMIT $4
		) b ON true
		WHERE a.workspace=$5 AND a.project_id=$6 AND a.id >= $1
		ORDER BY a.id,b.id LIMIT $4
	), scored AS MATERIALIZED (
		SELECT candidates.*, (` + chunkPairCosine("candidates.left_id", "candidates.right_id", "candidates.left_version", "candidates.right_version") + `) AS score
		FROM candidates
	)
	SELECT left_id, left_version, CASE WHEN score BETWEEN $3 AND 1 THEN left_text END,
	       right_id, right_version, CASE WHEN score BETWEEN $3 AND 1 THEN right_text END,
	       CASE WHEN score BETWEEN $3 AND 1 THEN score END
	FROM scored ORDER BY left_id,right_id
`

// ListSimilarPairs reads one snapshot of the active sources and their ready
// index entries. The cursor advances by inspected pairs, so even a sparse
// threshold never causes a single page to score the entire project.
func (s *Service) ListSimilarPairs(ctx context.Context, workspace, project string, threshold float64, first int, after SimilarPairPosition) (SimilarPairPage, error) {
	rules := contracts.AnalysisConstraints()
	if !rules.ValidThreshold(threshold) || first < rules.PageMin || first > rules.PageMax {
		return SimilarPairPage{}, errors.New("invalid threshold or pagination")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return SimilarPairPage{}, errStorage
	}
	defer tx.Rollback()
	if err := s.authorize(ctx, tx, workspace, project, false); err != nil {
		return SimilarPairPage{}, err
	}
	page := SimilarPairPage{Status: similarPairsReady, Items: []SimilarPair{}}
	err = tx.QueryRowContext(ctx, `SELECT profile FROM embedding_profiles WHERE workspace=$1`, workspace).Scan(&page.EmbeddingProfile)
	if err != nil && err != sql.ErrNoRows {
		return SimilarPairPage{}, errStorage
	}
	if page.EmbeddingProfile == "" {
		page.Status = similarPairsNotConfigured
		if after.Snapshot != "" {
			page.Status = similarPairsStale
		}
		return page, nil
	}

	// Ready index writes replace their attempt or completion time. Include
	// both alongside active versions so reindexing also expires a cursor.
	var snapshot string
	err = tx.QueryRowContext(ctx, `
		SELECT md5($3 || ':' || coalesce(string_agg(
		    md5(jsonb_build_array(r.id,r.version,i.attempt,extract(epoch FROM i.lease_until))::text),'' ORDER BY r.id),'')),
		    count(*) FILTER (WHERE i.requirement_id IS NULL)
		FROM active_requirements r
		LEFT JOIN requirement_index i ON i.workspace=r.workspace AND i.project_id=r.project_id AND i.requirement_id=r.id
		  AND i.profile=$3 AND i.version=r.version AND i.status='ready'
		  AND EXISTS (SELECT FROM requirement_chunks c WHERE c.workspace=i.workspace AND c.project_id=i.project_id
		      AND c.requirement_id=i.requirement_id AND c.profile=i.profile AND c.version=i.version)
		WHERE r.workspace=$1 AND r.project_id=$2
	`, workspace, project, page.EmbeddingProfile).Scan(&snapshot, &page.MissingRequirements)
	if err != nil {
		return SimilarPairPage{}, errStorage
	}
	if after.Snapshot != "" && after.Snapshot != snapshot {
		page.Status = similarPairsStale
		return page, nil
	}
	if page.MissingRequirements > 0 {
		page.Status = similarPairsIncomplete
		return page, nil
	}
	rows, err := tx.QueryContext(ctx, similarPairsQuery, after.LeftID, after.RightID, threshold, similarPairWindow+1,
		workspace, project, page.EmbeddingProfile)
	if err != nil {
		return SimilarPairPage{}, errStorage
	}
	defer rows.Close()

	last := SimilarPairPosition{Snapshot: snapshot}
	for inspected := 0; rows.Next(); inspected++ {
		if inspected == similarPairWindow || len(page.Items) == first {
			page.Next = &last
			break
		}
		var p SimilarPair
		var leftText, rightText sql.NullString
		var score sql.NullFloat64
		if err := rows.Scan(&p.Left.ID, &p.Left.Version, &leftText, &p.Right.ID, &p.Right.Version, &rightText, &score); err != nil {
			return SimilarPairPage{}, errStorage
		}
		last.LeftID, last.RightID = p.Left.ID, p.Right.ID
		if score.Valid {
			p.Left.Text, p.Right.Text, p.SimilarityScore = leftText.String, rightText.String, score.Float64
			page.Items = append(page.Items, p)
		}
	}
	if rows.Err() != nil {
		return SimilarPairPage{}, errStorage
	}
	return page, nil
}
