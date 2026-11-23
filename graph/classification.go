package graph

import (
	"context"
	"errors"
	"slices"

	"github.com/VolkovDani/project-creator/contracts"

	"github.com/VolkovDani/project-creator/internal/classification"
	graphql "github.com/graph-gophers/graphql-go"
)

func classificationProjectTag(id graphql.ID) string {
	return "project:" + string(id) + ":classifications"
}

func classificationItemTag(projectID graphql.ID, id string) string {
	return "project:" + string(projectID) + ":classification:" + id
}

func classificationResult(projectID graphql.ID, e *classification.Entry) *RequirementClassification {
	if e == nil {
		return nil
	}
	v := &RequirementClassification{ProjectId: projectID, Digest: e.Digest, Profile: e.Profile, QuestionVersion: e.QuestionVersion, Reason: e.Reason, ErrorCode: e.ErrorCode, RequirementId: graphql.ID(e.ID), TextVersion: e.Version, Revision: e.Revision, Status: e.Status, SemanticType: e.Semantic, RepresentationType: e.Representation, Source: e.Source, RulesVersion: e.RulesVersion, Model: e.Model, CreatedAt: e.CreatedAt}
	for _, a := range e.Alternatives {
		v.Alternatives = append(v.Alternatives, &ClassificationAlternative{SemanticType: a.SemanticType, RepresentationType: a.RepresentationType, Score: a.Score})
	}
	return v
}
func (r *Resolver) RequirementClassification(ctx context.Context, a RequirementClassificationArgs) (*RequirementClassification, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	v, err := classification.Get(ctx, r.DB, workspace(ctx), string(a.ProjectId), string(a.RequirementId))
	if err != nil {
		return nil, errors.New("classification unavailable")
	}
	if v != nil {
		addCacheDependency(ctx, classificationItemTag(a.ProjectId, v.ID))
		addCacheDependency(ctx, "project:"+string(a.ProjectId)+":classification-profile")
	}
	return classificationResult(a.ProjectId, v), nil
}
func classificationRange(ctx context.Context, limit, offset int32) error {
	if err := consume(ctx); err != nil {
		return err
	}
	if limit < 1 || limit > 100 || offset < 0 {
		return errors.New("invalid pagination")
	}
	return nil
}
func classificationResults(projectID graphql.ID, rows []classification.Entry, err error) ([]*RequirementClassification, error) {
	if err != nil {
		return nil, errors.New("classification unavailable")
	}
	var result []*RequirementClassification
	for i := range rows {
		result = append(result, classificationResult(projectID, &rows[i]))
	}
	return result, nil
}
func (r *Resolver) RequirementClassificationHistory(ctx context.Context, a RequirementClassificationHistoryArgs) ([]*RequirementClassification, error) {
	if err := classificationRange(ctx, a.Limit, a.Offset); err != nil {
		return nil, err
	}
	rows, err := classification.History(ctx, r.DB, workspace(ctx), string(a.ProjectId), string(a.RequirementId), a.Limit, a.Offset)
	authorized := len(rows) > 0
	if err == nil && !authorized {
		err = r.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT FROM active_requirements WHERE workspace=$1 AND project_id=$2 AND id=$3)", workspace(ctx), a.ProjectId, a.RequirementId).Scan(&authorized)
	}
	if authorized {
		addCacheDependency(ctx, classificationItemTag(a.ProjectId, string(a.RequirementId)))
	}
	return classificationResults(a.ProjectId, rows, err)
}
func (r *Resolver) ClassifiedRequirements(ctx context.Context, a ClassifiedRequirementsArgs) ([]*RequirementClassification, error) {
	if err := classificationRange(ctx, a.Limit, a.Offset); err != nil {
		return nil, err
	}
	rows, err := classification.Filter(ctx, r.DB, workspace(ctx), string(a.ProjectId), a.SemanticType, a.RepresentationType, a.Limit, a.Offset)
	authorized := len(rows) > 0
	if err == nil && !authorized {
		err = r.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT FROM projects WHERE workspace=$1 AND id=$2)", workspace(ctx), a.ProjectId).Scan(&authorized)
	}
	if authorized {
		addCacheDependency(ctx, classificationProjectTag(a.ProjectId))
	}
	return classificationResults(a.ProjectId, rows, err)
}
func (r *Resolver) ConfirmRequirementClassification(ctx context.Context, a ConfirmRequirementClassificationArgs) (*ClassificationPayload, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	i := a.Input
	v, fields, err := classification.Confirm(ctx, r.DB, workspace(ctx), string(i.ProjectId), classification.Entry{ID: string(i.RequirementId), Version: i.TextVersion, Revision: i.Revision, Semantic: i.SemanticType, Representation: i.RepresentationType})
	if err != nil {
		return nil, errors.New("classification unavailable")
	}
	if v != nil {
		addCacheDependency(ctx, classificationProjectTag(i.ProjectId))
		addCacheDependency(ctx, classificationItemTag(i.ProjectId, v.ID))
	}
	return &ClassificationPayload{Classification: classificationResult(i.ProjectId, v), FieldErrors: fieldErrors(fields)}, nil
}

var classificationRules = contracts.ClassificationRules()

func (r *Resolver) ClassificationProgress(ctx context.Context, a ClassificationProgressArgs) (*ClassificationProgress, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	v, err := r.Classifications.Progress(ctx, workspace(ctx), string(a.ProjectId))
	if errors.Is(err, classification.ErrUnavailable) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.New("classification unavailable")
	}
	addCacheDependency(ctx, classificationProjectTag(a.ProjectId))
	return &ClassificationProgress{
		ProjectId: a.ProjectId, Configured: v.Configured, Profile: v.Profile, Total: v.Total,
		Unclassified: v.Unclassified, Queued: v.Queued, Classifying: v.Classifying, Suggested: v.Suggested,
		NeedsReview: v.NeedsReview, Confirmed: v.Confirmed, Failed: v.Failed, Stale: v.Stale,
	}, nil
}

func classificationItems(projectID graphql.ID, rows []classification.ListItem, err error) ([]*ClassificationListItem, error) {
	if errors.Is(err, classification.ErrUnavailable) {
		return []*ClassificationListItem{}, nil
	}
	if err != nil {
		return nil, errors.New("classification unavailable")
	}
	out := make([]*ClassificationListItem, 0, len(rows))
	for _, v := range rows {
		out = append(out, &ClassificationListItem{ProjectId: projectID, RequirementId: graphql.ID(v.ID), Text: v.Text, TextVersion: v.Version, Revision: v.Revision,
			Status: v.Status, SemanticType: v.Semantic, RepresentationType: v.Representation, ErrorCode: v.ErrorCode})
	}
	return out, nil
}

func (r *Resolver) ClassificationList(ctx context.Context, a ClassificationListArgs) ([]*ClassificationListItem, error) {
	if err := classificationRange(ctx, a.Limit, a.Offset); err != nil {
		return nil, err
	}
	if (a.Status != nil && !slices.Contains(classificationRules.States, *a.Status)) ||
		(a.SemanticType != nil && !slices.Contains(classificationRules.Semantic, *a.SemanticType)) ||
		(a.RepresentationType != nil && !slices.Contains(classificationRules.Representation, *a.RepresentationType)) {
		return nil, errors.New("invalid classification filter")
	}
	rows, err := r.Classifications.List(ctx, workspace(ctx), string(a.ProjectId), a.SemanticType, a.RepresentationType, a.Status, a.Limit, a.Offset, graphql.HasSelectedField(ctx, "text"))
	if err == nil {
		addCacheDependency(ctx, classificationProjectTag(a.ProjectId))
	}
	return classificationItems(a.ProjectId, rows, err)
}

func (r *Resolver) ClassificationListByIds(ctx context.Context, a ClassificationListByIdsArgs) ([]*ClassificationListItem, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	if len(a.RequirementIds) < 1 || len(a.RequirementIds) > 100 {
		return nil, errors.New("invalid requirement selection")
	}
	ids := make([]string, len(a.RequirementIds))
	for i, id := range a.RequirementIds {
		ids[i] = string(id)
	}
	rows, err := r.Classifications.ListByIDs(ctx, workspace(ctx), string(a.ProjectId), ids)
	if err == nil {
		if len(rows) > 0 {
			addCacheDependency(ctx, "project:"+string(a.ProjectId)+":classification-profile")
		}
		for _, row := range rows {
			addCacheDependency(ctx, classificationItemTag(a.ProjectId, row.ID))
		}
	}
	return classificationItems(a.ProjectId, rows, err)
}

func (r *Resolver) ClassificationGroups(ctx context.Context, a ClassificationGroupsArgs) ([]*ClassificationGroup, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	rows, err := r.Classifications.Groups(ctx, workspace(ctx), string(a.ProjectId))
	if errors.Is(err, classification.ErrUnavailable) {
		return []*ClassificationGroup{}, nil
	}
	if err != nil {
		return nil, errors.New("classification unavailable")
	}
	addCacheDependency(ctx, classificationProjectTag(a.ProjectId))
	out := make([]*ClassificationGroup, 0, len(rows))
	for _, v := range rows {
		out = append(out, &ClassificationGroup{ProjectId: a.ProjectId, SemanticType: v.Semantic, RepresentationType: v.Representation, Count: v.Count})
	}
	return out, nil
}

func classificationQueueResult(queued int32, err error, field string) (*ClassificationQueuePayload, error) {
	payload := &ClassificationQueuePayload{Queued: queued, FieldErrors: []*FieldError{}}
	if err == nil {
		return payload, nil
	}
	e := &FieldError{Path: []string{field}, Code: "CONFLICT", MessageKey: "validation.conflict", Params: []*ErrorParam{}}
	switch {
	case errors.Is(err, classification.ErrNotConfigured):
		e.Code = "NOT_CONFIGURED"
		e.MessageKey = "classification.notConfigured"
	case errors.Is(err, classification.ErrUnavailable), errors.Is(err, classification.ErrConflict):
	default:
		return nil, errors.New("classification unavailable")
	}
	payload.FieldErrors = append(payload.FieldErrors, e)
	return payload, nil
}

func (r *Resolver) BackfillClassifications(ctx context.Context, a BackfillClassificationsArgs) (*ClassificationQueuePayload, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	n, err := r.Classifications.Backfill(ctx, workspace(ctx), string(a.ProjectId))
	if err == nil {
		addCacheDependency(ctx, classificationProjectTag(a.ProjectId))
	}
	return classificationQueueResult(n, err, "projectId")
}
func (r *Resolver) RetryFailedClassifications(ctx context.Context, a RetryFailedClassificationsArgs) (*ClassificationQueuePayload, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	n, err := r.Classifications.RetryFailed(ctx, workspace(ctx), string(a.ProjectId))
	if err == nil {
		addCacheDependency(ctx, classificationProjectTag(a.ProjectId))
	}
	return classificationQueueResult(n, err, "projectId")
}
func (r *Resolver) RetryRequirementClassification(ctx context.Context, a RetryRequirementClassificationArgs) (*ClassificationQueuePayload, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	if a.TextVersion < 1 {
		return classificationQueueResult(0, classification.ErrConflict, "textVersion")
	}
	queued, err := r.Classifications.Retry(ctx, workspace(ctx), string(a.ProjectId), string(a.RequirementId), a.TextVersion, a.Reanalyze)
	var n int32
	if queued {
		n = 1
	}
	if err == nil {
		addCacheDependency(ctx, classificationProjectTag(a.ProjectId))
		addCacheDependency(ctx, classificationItemTag(a.ProjectId, string(a.RequirementId)))
	}
	return classificationQueueResult(n, err, "textVersion")
}
