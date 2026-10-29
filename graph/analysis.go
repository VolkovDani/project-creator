package graph

import (
	"context"
	"errors"

	"github.com/VolkovDani/project-creator/internal/analysis"
	graphql "github.com/graph-gophers/graphql-go"
)

func projectAnalysis(value analysis.Run) *ProjectAnalysis {
	out := &ProjectAnalysis{ReasonCode: value.ReasonCode, MissingRequirementIds: []graphql.ID{}, ID: graphql.ID(value.ID), ProjectId: graphql.ID(value.ProjectID), Status: value.Status, Outcome: value.Outcome, ErrorCode: value.ErrorCode, CreatedAt: value.CreatedAt, CompletedAt: value.CompletedAt, Stale: value.Stale, GeneratorProfile: value.GeneratorProfile, EmbeddingProfile: value.EmbeddingProfile, Sources: []*AnalysisSource{}, Findings: []*AnalysisFinding{}}
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
