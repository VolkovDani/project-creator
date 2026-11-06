package analysis

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const preparationBatch = 128
const exactOrigin = "exact_text_v1"

func normalizedText(s string) string { return strings.Join(strings.Fields(s), " ") }
func exactDuplicate(p Pair) bool {
	return normalizedText(p.Left.Text) != "" && normalizedText(p.Left.Text) == normalizedText(p.Right.Text)
}
func significantTerms(text string) map[string]bool {
	stop := map[string]bool{"must": true, "shall": true, "should": true, "with": true, "from": true, "that": true, "this": true, "then": true, "only": true, "every": true, "requirement": true, "system": true, "система": true, "должна": true, "должен": true, "должны": true, "только": true, "каждый": true, "требование": true, "пользователь": true}
	out := map[string]bool{}
	for _, v := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		if utf8.RuneCountInString(v) >= 4 && !stop[v] {
			out[v] = true
		}
	}
	return out
}
func sharedTerms(p Pair) bool {
	left, right := significantTerms(p.Left.Text), significantTerms(p.Right.Text)
	n := 0
	for v := range left {
		if right[v] {
			n++
			if n >= 2 {
				return true
			}
		}
	}
	return false
}
func cacheKey(profile string, p Pair) string {
	// JSON boundaries preserve IDs, versions and texts as one canonical identity.
	b, _ := json.Marshal(struct {
		Profile string
		Pair    Pair
	}{profile, p})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func selection(p Pair, mode string, threshold float64, score *float64) []string {
	reasons := []string{}
	if mode == "all" {
		reasons = append(reasons, "all_pairs")
	}
	if exactDuplicate(p) {
		reasons = append(reasons, "exact_text")
	}
	if mode == "related" {
		if score != nil && *score >= threshold {
			reasons = append(reasons, "semantic_similarity")
		}
		if sharedTerms(p) {
			reasons = append(reasons, "shared_terms")
		}
	}
	return reasons
}

// checkIndexes reports a bounded diagnostic page but checks all current sources.
func (s *Service) checkIndexes(ctx context.Context, tx *sql.Tx, workspace string, r *Run) error {
	if r.Mode != "related" {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT a.requirement_id FROM analysis_sources a WHERE a.run_id=$1 AND NOT EXISTS(SELECT 1 FROM requirement_index i WHERE i.workspace=$2 AND i.project_id=$3 AND i.requirement_id=a.requirement_id AND i.version=(a.source->>'version')::int AND i.profile=$4 AND i.status='ready' AND EXISTS(SELECT 1 FROM requirement_chunks c WHERE c.workspace=i.workspace AND c.project_id=i.project_id AND c.requirement_id=i.requirement_id AND c.profile=i.profile AND c.version=i.version)) ORDER BY a.requirement_id LIMIT 100`, r.ID, workspace, r.ProjectID, r.EmbeddingProfile)
	if err != nil {
		return errStorage
	}
	missing := []string{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			rows.Close()
			return errStorage
		}
		missing = append(missing, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return errStorage
	}
	r.MissingRequirementIDs = missing
	if len(missing) > 0 {
		r.ReasonCode = "INDEX_INCOMPLETE"
		return errors.New("INDEX_INCOMPLETE")
	}
	return nil
}

// Each transaction advances a durable lexicographic source-pair cursor. No full
// project or quadratic queue is loaded into Go; even excluded pairs are paged.
func (s *Service) prepareBatch(ctx context.Context, workspace string, r *Run) (bool, error) {
	durable := r
	pending := *r
	r = &pending
	// Failed SQL batches must not advance the in-memory cursor later persisted
	// by finish(). Only diagnostics may escape a rolled-back batch.
	defer func() { durable.ReasonCode = r.ReasonCode; durable.MissingRequirementIDs = r.MissingRequirementIDs }()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, errStorage
	}
	defer tx.Rollback()
	if err = s.authorize(ctx, tx, workspace, r.ProjectID, true); err != nil {
		return false, err
	}
	if err = fenceRun(ctx, tx, workspace, r, true); err != nil {
		return false, err
	}
	if err = s.currentWith(ctx, tx, workspace, r); err != nil {
		return false, err
	}
	var count int64
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM analysis_sources WHERE run_id=$1`, r.ID).Scan(&count); err != nil {
		return false, errStorage
	}
	possible := count * (count - 1) / 2
	if possible > 2147483647 {
		return false, errors.New("QUEUE_TOO_LARGE")
	}
	n := int(possible)
	r.PossiblePairs = &n
	if count < 2 {
		r.ReasonCode = "TOO_FEW_REQUIREMENTS"
	} else if err = s.checkIndexes(ctx, tx, workspace, r); err != nil {
		return false, err
	}
	r.Stage = "finding_pairs"
	query := `SELECT a.source,b.source,NULL::double precision FROM analysis_sources a JOIN analysis_sources b ON b.run_id=a.run_id AND b.requirement_id>a.requirement_id WHERE a.run_id=$1 AND (a.requirement_id,b.requirement_id)>($2,$3) ORDER BY a.requirement_id,b.requirement_id LIMIT $4`
	args := []any{r.ID, r.cursorLeft, r.cursorRight, preparationBatch}
	if r.Mode == "related" {
		query = `SELECT a.source,b.source,(SELECT max(1-(lc.embedding OPERATOR(public.<=>) rc.embedding)) FROM requirement_chunks lc JOIN requirement_chunks rc ON rc.workspace=lc.workspace AND rc.project_id=lc.project_id AND rc.profile=lc.profile WHERE lc.workspace=$5 AND lc.project_id=$6 AND lc.profile=$7 AND lc.requirement_id=a.requirement_id AND rc.requirement_id=b.requirement_id AND lc.version=(a.source->>'version')::int AND rc.version=(b.source->>'version')::int) FROM analysis_sources a JOIN analysis_sources b ON b.run_id=a.run_id AND b.requirement_id>a.requirement_id WHERE a.run_id=$1 AND (a.requirement_id,b.requirement_id)>($2,$3) ORDER BY a.requirement_id,b.requirement_id LIMIT $4`
		args = append(args, workspace, r.ProjectID, r.EmbeddingProfile)
	}
	var position int
	if err = tx.QueryRowContext(ctx, `SELECT coalesce(max(position)+1,0) FROM analysis_pairs WHERE run_id=$1`, r.ID).Scan(&position); err != nil {
		return false, errStorage
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return false, errStorage
	}
	type entry struct {
		PairResult
		CacheKey string `json:"cacheKey"`
	}
	batch := []entry{}
	inspected := 0
	for rows.Next() {
		var l, rr []byte
		var score sql.NullFloat64
		var pair Pair
		if rows.Scan(&l, &rr, &score) != nil || json.Unmarshal(l, &pair.Left) != nil || json.Unmarshal(rr, &pair.Right) != nil {
			rows.Close()
			return false, errStorage
		}
		inspected++
		r.cursorLeft, r.cursorRight = pair.Left.ID, pair.Right.ID
		var similarity *float64
		if score.Valid {
			v := score.Float64
			similarity = &v
		}
		reasons := selection(pair, r.Mode, r.SimilarityThreshold, similarity)
		if len(reasons) == 0 {
			continue
		}
		p := PairResult{ID: fmt.Sprintf("%s:%d", r.ID, position), Position: position, Status: "pending", Left: pair.Left, Right: pair.Right, GeneratorProfile: r.GeneratorProfile, SelectionReasons: reasons, SimilarityScore: similarity, ResultOrigin: "model"}
		batch = append(batch, entry{p, cacheKey(r.GeneratorProfile, pair)})
		position++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, errStorage
	}
	firstPosition := position - len(batch)
	if len(batch) > 0 {
		raw, _ := jsonMarshal(batch)
		_, err = tx.ExecContext(ctx, `INSERT INTO analysis_pairs(id,run_id,workspace,project_id,position,left_source,right_source,generator_profile,cache_key,status,verdict,explanation,reused_from,checked_at,selection_reasons,similarity_score,result_origin,laya_status,laya_result,laya_checked_at)
 SELECT x.id,$2,$3,$4,x.position,x."left",x."right",$5,x."cacheKey",CASE WHEN c.id IS NULL THEN 'pending' ELSE 'completed' END,coalesce(c.verdict,''),coalesce(c.explanation,''),coalesce(c.id,''),c.checked_at,x."selectionReasons",x."similarityScore",coalesce(c.result_origin,'model'),CASE WHEN c.id IS NOT NULL THEN c.laya_status WHEN $6<>'' THEN 'pending' ELSE '' END,c.laya_result,c.laya_checked_at
 FROM jsonb_to_recordset($1::jsonb) AS x(id text,position int,"left" jsonb,"right" jsonb,"cacheKey" text,"selectionReasons" jsonb,"similarityScore" double precision)
 LEFT JOIN LATERAL(SELECT p.id,p.verdict,p.explanation,p.checked_at,p.result_origin,p.laya_status,p.laya_result,p.laya_checked_at FROM analysis_pairs p WHERE p.workspace=$3 AND p.project_id=$4 AND p.generator_profile=$5 AND p.cache_key=x."cacheKey" AND p.left_source=x."left" AND p.right_source=x."right" AND p.status='completed' AND p.reused_from='' ORDER BY p.checked_at,p.id LIMIT 1)c ON true`, raw, r.ID, workspace, r.ProjectID, r.GeneratorProfile, r.ClassifierProfile)
		if err != nil {
			return false, errStorage
		}
	}
	if len(batch) > 0 {
		var reused int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM analysis_pairs WHERE run_id=$1 AND position >= $2 AND status='completed'`, r.ID, firstPosition).Scan(&reused); err != nil {
			return false, errStorage
		}
		r.ProcessedPairs += reused
	}
	done := inspected < preparationBatch
	if done {
		r.TotalPairs = &position
		r.Stage = "comparing"
	}
	if err = writeRun(ctx, tx, workspace, r, true); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, errStorage
	}
	*durable = *r
	return done, nil
}

// Kept as a tiny pure compatibility helper for too-few-source diagnostics.
func (s *Service) prepare(ctx context.Context, workspace, project string, sources []Source, incomplete bool, findingPairs func() error) ([]Pair, string, bool, diagnostic) {
	d := diagnostic{MissingRequirementIDs: []string{}}
	if len(sources) < 2 {
		d.ReasonCode = "TOO_FEW_REQUIREMENTS"
		return []Pair{}, "", true, d
	}
	return []Pair{}, "", incomplete, d
}
