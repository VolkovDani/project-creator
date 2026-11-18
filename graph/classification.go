package graph

import (
	"context"
	"errors"

	"github.com/VolkovDani/project-creator/internal/classification"
	graphql "github.com/graph-gophers/graphql-go"
)

func classificationResult(e *classification.Entry) *RequirementClassification {
	if e == nil {
		return nil
	}
	v := &RequirementClassification{RequirementId: graphql.ID(e.ID), TextVersion: e.Version, Revision: e.Revision, Status: e.Status, SemanticType: e.Semantic, RepresentationType: e.Representation, Source: e.Source, RulesVersion: e.RulesVersion, Model: e.Model, CreatedAt: e.CreatedAt}
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
	return classificationResult(v), nil
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
func classificationResults(rows []classification.Entry, err error) ([]*RequirementClassification, error) {
	if err != nil {
		return nil, errors.New("classification unavailable")
	}
	var result []*RequirementClassification
	for i := range rows {
		result = append(result, classificationResult(&rows[i]))
	}
	return result, nil
}
func (r *Resolver) RequirementClassificationHistory(ctx context.Context, a RequirementClassificationHistoryArgs) ([]*RequirementClassification, error) {
	if err := classificationRange(ctx, a.Limit, a.Offset); err != nil {
		return nil, err
	}
	return classificationResults(classification.History(ctx, r.DB, workspace(ctx), string(a.ProjectId), string(a.RequirementId), a.Limit, a.Offset))
}
func (r *Resolver) ClassifiedRequirements(ctx context.Context, a ClassifiedRequirementsArgs) ([]*RequirementClassification, error) {
	if err := classificationRange(ctx, a.Limit, a.Offset); err != nil {
		return nil, err
	}
	return classificationResults(classification.Filter(ctx, r.DB, workspace(ctx), string(a.ProjectId), a.SemanticType, a.RepresentationType, a.Limit, a.Offset))
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
	return &ClassificationPayload{Classification: classificationResult(v), FieldErrors: fieldErrors(fields)}, nil
}
