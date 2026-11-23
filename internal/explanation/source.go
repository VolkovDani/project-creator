package explanation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// A review-only classifier may leave both labels unknown and still produce
// genuine ranked evidence. A queue state or input-limit result alone is insufficient.
const latestSuggestion = `
SELECT h.revision FROM requirement_classifications h
WHERE h.workspace=r.workspace AND h.project_id=r.project_id AND h.requirement_id=r.id
 AND h.text_version=r.version AND h.source='model' AND h.status IN ('suggested','needs_review')
 AND (h.semantic_type<>'unknown' OR h.representation_type<>'unknown' OR jsonb_array_length(h.alternatives)>0)
ORDER BY h.revision DESC LIMIT 1`

// SuggestionRevision also reports whether the active source belongs to this scope.
func (s *Service) SuggestionRevision(ctx context.Context, w, p, id string) (int32, bool, error) {
	var revision int32
	err := s.db.QueryRowContext(ctx, `SELECT coalesce((`+latestSuggestion+`),0) FROM active_requirements r
 WHERE r.workspace=$1 AND r.project_id=$2 AND r.id=$3`, w, p, id).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return revision, err == nil, err
}

// The projection uses the Go entry field names so the manual decision and the
// model proposal retain the same metadata as the classification API.
const sourceQuery = `
WITH current_record AS (
 SELECT * FROM current_requirement_classification r
 WHERE r.workspace=$1 AND r.project_id=$2 AND r.id=$3 AND r.version=$4 AND r.revision=$5
), revisions AS (
 SELECT r.*, (` + latestSuggestion + `) AS suggestion_revision FROM current_record r
), selected_entries AS (
 SELECT h.requirement_id AS "ID", h.text_version AS "Version", h.revision AS "Revision",
 h.status AS "Status", h.semantic_type AS "Semantic", h.representation_type AS "Representation",
 h.source AS "Source", h.rules_version AS "RulesVersion", h.model AS "Model",
 h.digest AS "Digest", h.profile AS "Profile", h.question_version AS "QuestionVersion",
 h.reason AS "Reason", h.created_at::text AS "CreatedAt", h.alternatives AS "Alternatives"
 FROM revisions r JOIN requirement_classifications h ON
  h.workspace=r.workspace AND h.project_id=r.project_id AND h.requirement_id=r.id
  AND h.text_version=r.version AND h.revision IN(r.revision,r.suggestion_revision)
)
SELECT r.text,to_jsonb(d),to_jsonb(s) FROM revisions r
LEFT JOIN selected_entries d ON d."Revision"=r.revision AND r.source<>'' AND r.status<>'stale'
LEFT JOIN selected_entries s ON s."Revision"=r.suggestion_revision`

func (s *Service) source(ctx context.Context, w string, req Request) (Source, string, error) {
	var source Source
	var decision, suggestion []byte
	err := s.db.QueryRowContext(ctx, sourceQuery, w, req.ProjectID, req.RequirementID, req.TextVersion, req.Revision).Scan(&source.Text, &decision, &suggestion)
	if errors.Is(err, sql.ErrNoRows) {
		return Source{}, "stale", nil
	}
	if err != nil {
		if ctx.Err() != nil {
			return Source{}, failureStatus(ctx.Err()), nil
		}
		return Source{}, "", err
	}
	if len(suggestion) == 0 {
		return Source{}, "unclassified", nil
	}
	if len(decision) > 0 {
		if err := json.Unmarshal(decision, &source.Decision); err != nil {
			return Source{}, "", err
		}
	}
	if err := json.Unmarshal(suggestion, &source.Suggestion); err != nil {
		return Source{}, "", err
	}
	if source.Suggestion.Revision != req.SuggestionRevision {
		return Source{}, "stale", nil
	}
	return source, "ready", nil
}
