package graph

import (
	"context"
	"errors"

	"github.com/VolkovDani/project-creator/internal/analysis"
	graphql "github.com/graph-gophers/graphql-go"
)

func projectAnalysis(value analysis.Run) *ProjectAnalysis {
	out := &ProjectAnalysis{Stage: value.Stage, ProcessedPairs: int32(value.ProcessedPairs), ActivePairId: graphql.ID(value.ActivePairID), Pairs: []*AnalysisPair{}, ReasonCode: value.ReasonCode, MissingRequirementIds: []graphql.ID{}, ID: graphql.ID(value.ID), ProjectId: graphql.ID(value.ProjectID), Status: value.Status, Outcome: value.Outcome, ErrorCode: value.ErrorCode, CreatedAt: value.CreatedAt, CompletedAt: value.CompletedAt, Stale: value.Stale, GeneratorProfile: value.GeneratorProfile, EmbeddingProfile: value.EmbeddingProfile, Sources: []*AnalysisSource{}, Findings: []*AnalysisFinding{}}
	if value.TotalPairs != nil {
		total := int32(*value.TotalPairs)
		out.TotalPairs = &total
	}
	for _, pair := range value.Pairs {
		out.Pairs = append(out.Pairs, &AnalysisPair{
			ID: graphql.ID(pair.ID), Position: int32(pair.Position), Status: pair.Status,
			Left:    &AnalysisSource{ID: graphql.ID(pair.Left.ID), Version: int32(pair.Left.Version), Text: pair.Left.Text},
			Right:   &AnalysisSource{ID: graphql.ID(pair.Right.ID), Version: int32(pair.Right.Version), Text: pair.Right.Text},
			Verdict: pair.Verdict, Explanation: pair.Explanation, ErrorCode: pair.ErrorCode,
		})
	}
	for _, id := range value.MissingRequirementIDs {
		out.MissingRequirementIds = append(out.MissingRequirementIds, graphql.ID(id))
	}
	for _, source := range value.Sources {
		out.Sources = append(out.Sources, &AnalysisSource{ID: graphql.ID(source.ID), Version: int32(source.Version), Text: source.Text})
	}
	for _, finding := range value.Findings {
		out.Findings = append(out.Findings, &AnalysisFinding{LeftId: graphql.ID(finding.LeftID), RightId: graphql.ID(finding.RightID), LeftVersion: int32(finding.LeftVersion), RightVersion: int32(finding.RightVersion), Verdict: finding.Verdict, Explanation: finding.Explanation})
	}
	return out
}
func (r *Resolver) ProjectAnalysis(ctx context.Context, args ProjectAnalysisArgs) (*ProjectAnalysis, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	run, err := r.Analysis.Latest(ctx, workspace(ctx), string(args.ProjectId))
	if err != nil {
		return nil, errors.New("analysis unavailable")
	}
	if run == nil {
		return nil, nil
	}
	return projectAnalysis(*run), nil
}
func (r *Resolver) StartProjectAnalysis(ctx context.Context, args StartProjectAnalysisArgs) (*ProjectAnalysis, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	run, err := r.Analysis.Start(ctx, workspace(ctx), string(args.ProjectId))
	if err != nil {
		return nil, errors.New("analysis unavailable")
	}
	return projectAnalysis(run), nil
}

func (r *Resolver) RetryProjectAnalysis(ctx context.Context, args RetryProjectAnalysisArgs) (*ProjectAnalysis, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	run, err := r.Analysis.Retry(ctx, workspace(ctx), string(args.ProjectId), string(args.AnalysisId))
	if err != nil {
		return nil, errors.New("analysis retry unavailable")
	}
	return projectAnalysis(run), nil
}
