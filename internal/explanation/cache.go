package explanation

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"time"

	"github.com/VolkovDani/project-creator/internal/project"
	"github.com/jackc/pgx/v5/pgtype"
)

var axes = [...]Axis{MeaningAxis, FormAxis}

func axisKeys(key cacheKey) map[Axis][]byte {
	digests := make(map[Axis][]byte, len(axes))
	for _, axis := range axes {
		request := key.request
		if axis == MeaningAxis {
			request.Target.Representation = ""
		} else {
			request.Target.Semantic = ""
		}
		// The axis results and their identity are persisted across process restarts.
		data, _ := json.Marshal(struct {
			Format            int
			Taxonomy, Profile string
			Request           Request
			Source            [sha256.Size]byte
		}{1, key.taxonomy, key.profile, request, key.source})
		digest := sha256.Sum256(data)
		digests[axis] = digest[:]
	}
	return digests
}

func (s *Service) cachedAxes(ctx context.Context, key cacheKey, digests map[Axis][]byte) (map[Axis]AxisResult, error) {
	ttl := pgtype.Interval{Microseconds: s.cacheTTL.Microseconds(), Valid: s.cacheTTL > 0}
	rows, err := s.db.QueryContext(ctx, `
		SELECT axis, result FROM classification_explanation_cache
		WHERE workspace=$1 AND project_id=$2 AND requirement_id=$3
		  AND ((axis='meaning' AND cache_key=$4) OR (axis='form' AND cache_key=$5))
		  AND ($6::interval IS NULL OR created_at > statement_timestamp()-$6::interval)
	`, key.workspace, key.request.ProjectID, key.request.RequirementID, digests[MeaningAxis], digests[FormAxis], ttl)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make(map[Axis]AxisResult, len(axes))
	for rows.Next() {
		var axis Axis
		var encoded []byte
		if err := rows.Scan(&axis, &encoded); err != nil {
			return nil, err
		}
		var result AxisResult
		if err := json.Unmarshal(encoded, &result); err != nil {
			return nil, err
		}
		results[axis] = result
	}
	return results, rows.Err()
}

func (s *Service) acquireAxes(ctx context.Context, key cacheKey, digests map[Axis][]byte) (map[Axis]AxisResult, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	results, err := s.cachedAxes(ctx, key, digests)
	if err != nil {
		if ctx.Err() != nil {
			return nil, failureStatus(ctx.Err()), nil
		}
		return nil, "", err
	}
	if len(results) == len(axes) {
		return results, "ready", nil
	}
	return results, s.admitLocked(key.workspace, time.Now()), nil
}

func (s *Service) storeAxes(ctx context.Context, key cacheKey, digests map[Axis][]byte, results map[Axis]AxisResult) error {
	if len(results) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, axis := range axes {
		result, ok := results[axis]
		if !ok {
			continue
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO classification_explanation_cache(workspace,project_id,requirement_id,axis,cache_key,result)
			VALUES($1,$2,$3,$4,$5,$6)
			ON CONFLICT(workspace,project_id,requirement_id,axis,cache_key)
			DO UPDATE SET result=EXCLUDED.result,created_at=clock_timestamp()
		`, key.workspace, key.request.ProjectID, key.request.RequirementID, axis, digests[axis], encoded)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Service) explainCachedAxes(ctx context.Context, key cacheKey, source Source) (*Result, string, []project.FieldError, error) {
	digests := axisKeys(key)
	results, status, err := s.acquireAxes(ctx, key, digests)
	if status == "ready" {
		result := CombineAxes(results[MeaningAxis], results[FormAxis], key.request.Target.Locale)
		return &result, "ready", nil, nil
	}
	if status != "" || err != nil {
		return nil, status, nil, err
	}
	defer s.release()
	var missing []Axis
	for _, axis := range axes {
		if _, ok := results[axis]; !ok {
			missing = append(missing, axis)
		}
	}
	call, cancel := context.WithTimeout(ctx, inferenceTimeout)
	defer cancel()
	generated, generationErr := s.generator.ExplainClassificationAxes(call, source, key.request.Target, missing)
	if call.Err() != nil {
		return nil, failureStatus(call.Err()), nil, nil
	}
	if len(generated) == 0 && generationErr != nil {
		return nil, failureStatus(generationErr), nil, nil
	}
	if status, err := s.checkCurrent(call, key); err != nil || status != "ready" {
		return nil, status, nil, err
	}
	if err := s.storeAxes(call, key, digests, generated); err != nil {
		if call.Err() != nil {
			return nil, failureStatus(call.Err()), nil, nil
		}
		return nil, "", nil, err
	}
	if generationErr != nil {
		return nil, failureStatus(generationErr), nil, nil
	}
	for _, axis := range missing {
		assessment, ok := generated[axis]
		if !ok {
			return nil, "invalid_output", nil, nil
		}
		results[axis] = assessment
	}
	result := CombineAxes(results[MeaningAxis], results[FormAxis], key.request.Target.Locale)
	return &result, "ready", nil, nil
}
