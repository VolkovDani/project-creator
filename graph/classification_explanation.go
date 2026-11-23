package graph

import (
	"context"
	"errors"

	"github.com/VolkovDani/project-creator/internal/explanation"
)

func (r *Resolver) ClassificationExplanationSuggestionRevision(ctx context.Context, a ClassificationExplanationSuggestionRevisionArgs) (int32, error) {
	if err := consume(ctx); err != nil {
		return 0, err
	}
	revision, authorized, err := r.Explanations.SuggestionRevision(ctx, workspace(ctx), string(a.ProjectId), string(a.RequirementId))
	if err != nil {
		return 0, errors.New("classification unavailable")
	}
	if authorized {
		addCacheDependency(ctx, classificationItemTag(a.ProjectId, string(a.RequirementId)))
	}
	return revision, nil
}

func (r *Resolver) ExplainRequirementClassification(ctx context.Context, a ExplainRequirementClassificationArgs) (*ClassificationExplanationPayload, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	in := a.Input
	result, status, fields, err := r.Explanations.Explain(ctx, workspace(ctx), explanation.Request{
		ProjectID: string(in.ProjectId), RequirementID: string(in.RequirementId), TextVersion: in.TextVersion, Revision: in.Revision, SuggestionRevision: in.SuggestionRevision,
		Target: explanation.Target{Semantic: in.SemanticType, Representation: in.RepresentationType, Locale: in.Locale},
	})
	if err != nil {
		return nil, errors.New("explanation unavailable")
	}
	out := &ClassificationExplanationPayload{Status: status, FieldErrors: fieldErrors(fields)}
	if result != nil {
		out.Result = &ClassificationExplanationResult{Meaning: explanationAssessment(result.Meaning), Form: explanationAssessment(result.Form), Questions: result.Questions, Wording: result.Wording, Limitations: result.Limitations}
	}
	return out, nil
}

func explanationAssessment(v explanation.Assessment) *ClassificationExplanationAssessment {
	out := &ClassificationExplanationAssessment{Verdict: v.Verdict, Reason: v.Reason}
	for _, c := range v.Criteria {
		out.Criteria = append(out.Criteria, &ClassificationExplanationCriterion{Name: c.Name, Status: c.Status, Evidence: c.Evidence, Note: c.Note})
	}
	return out
}
