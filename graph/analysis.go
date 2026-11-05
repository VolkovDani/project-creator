package graph

import (
	"context"
	"errors"

	"github.com/VolkovDani/project-creator/contracts"
	"github.com/VolkovDani/project-creator/internal/analysis"
	graphql "github.com/graph-gophers/graphql-go"
)

func projectAnalysis(value analysis.Run) *ProjectAnalysis {
	out := &ProjectAnalysis{Mode: value.Mode, SimilarityThreshold: value.SimilarityThreshold, SelectionProfile: value.SelectionProfile, DuplicatePairs: int32(value.DuplicatePairs), ReusedPairs: int32(value.ReusedPairs), NewPairs: int32(value.NewPairs), CompletedPairs: int32(value.CompletedPairs), PendingPairs: int32(value.PendingPairs), RunningPairs: int32(value.RunningPairs), FailedPairs: int32(value.FailedPairs), CollisionPairs: int32(value.CollisionPairs), CompatiblePairs: int32(value.CompatiblePairs), InsufficientPairs: int32(value.InsufficientPairs), GeneratorProvenance: value.GeneratorProvenance, Stage: value.Stage, ProcessedPairs: int32(value.ProcessedPairs), ActivePairId: graphql.ID(value.ActivePairID), Pairs: []*AnalysisPair{}, ReasonCode: value.ReasonCode, MissingRequirementIds: []graphql.ID{}, ID: graphql.ID(value.ID), ProjectId: graphql.ID(value.ProjectID), Status: value.Status, Outcome: value.Outcome, ErrorCode: value.ErrorCode, CreatedAt: value.CreatedAt, CompletedAt: value.CompletedAt, Stale: value.Stale, GeneratorProfile: value.GeneratorProfile, EmbeddingProfile: value.EmbeddingProfile, Sources: []*AnalysisSource{}, Findings: []*AnalysisFinding{}}
	if value.PossiblePairs != nil {
		n := int32(*value.PossiblePairs)
		out.PossiblePairs = &n
	}
	if value.NotSelectedPairs != nil {
		n := int32(*value.NotSelectedPairs)
		out.NotSelectedPairs = &n
	}
	if value.TotalPairs != nil {
		total := int32(*value.TotalPairs)
		out.TotalPairs = &total
	}
	for _, pair := range value.Pairs {
		out.Pairs = append(out.Pairs, analysisPair(pair))
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
	var run *analysis.Run
	var err error
	if args.AnalysisId != nil {
		run, err = r.Analysis.Get(ctx, workspace(ctx), string(args.ProjectId), string(*args.AnalysisId))
	} else {
		run, err = r.Analysis.Latest(ctx, workspace(ctx), string(args.ProjectId))
	}
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
	options, err := analysisOptions(args.Mode, args.Threshold)
	if err != nil {
		return nil, err
	}
	run, err := r.Analysis.StartWithOptions(ctx, workspace(ctx), string(args.ProjectId), options)
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

func analysisPair(pair analysis.PairResult) *AnalysisPair {
	reasons := append([]string{}, pair.SelectionReasons...)
	return &AnalysisPair{SelectionReasons: reasons, SimilarityScore: pair.SimilarityScore, ResultOrigin: pair.ResultOrigin, ID: graphql.ID(pair.ID), Position: int32(pair.Position), Status: pair.Status,
		Left:    &AnalysisSource{ID: graphql.ID(pair.Left.ID), Version: int32(pair.Left.Version), Text: pair.Left.Text},
		Right:   &AnalysisSource{ID: graphql.ID(pair.Right.ID), Version: int32(pair.Right.Version), Text: pair.Right.Text},
		Verdict: pair.Verdict, Explanation: pair.Explanation, ErrorCode: pair.ErrorCode,
		ReusedFrom: graphql.ID(pair.ReusedFrom), CheckedAt: pair.CheckedAt, GeneratorProfile: pair.GeneratorProfile}
}
func (r *Resolver) AnalysisPairs(ctx context.Context, args AnalysisPairsArgs) ([]*AnalysisPair, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	if !contracts.ValidateAnalysisPage(int(args.Limit), int(args.Offset)) {
		return nil, errors.New("invalid analysis page")
	}
	pairs, err := r.Analysis.Pairs(ctx, workspace(ctx), string(args.ProjectId), string(args.AnalysisId), int(args.Limit), int(args.Offset), args.Status, args.Verdict)
	if err != nil {
		return nil, errors.New("analysis unavailable")
	}
	out := []*AnalysisPair{}
	for _, pair := range pairs {
		out = append(out, analysisPair(pair))
	}
	return out, nil
}
func (r *Resolver) AnalysisHistory(ctx context.Context, args AnalysisHistoryArgs) ([]*ProjectAnalysis, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	if !contracts.ValidateAnalysisPage(int(args.Limit), int(args.Offset)) {
		return nil, errors.New("invalid analysis page")
	}
	runs, err := r.Analysis.History(ctx, workspace(ctx), string(args.ProjectId), int(args.Limit), int(args.Offset))
	if err != nil {
		return nil, errors.New("analysis unavailable")
	}
	out := []*ProjectAnalysis{}
	for _, run := range runs {
		out = append(out, projectAnalysis(run))
	}
	return out, nil
}

type analysisSelectionError struct {
	fields []contracts.AnalysisFieldError
}

func (e analysisSelectionError) Error() string { return "invalid analysis selection" }
func (e analysisSelectionError) Extensions() map[string]interface{} {
	return map[string]interface{}{"fieldErrors": e.fields}
}

func analysisOptions(mode *string, threshold *float64) (analysis.Options, error) {
	normalized, fields := contracts.AnalysisSelection(mode, threshold)
	if len(fields) > 0 {
		return analysis.Options{}, analysisSelectionError{fields}
	}
	return analysis.Options{Mode: normalized.Mode, Threshold: normalized.Threshold}, nil
}

// CancelProjectAnalysis stops every unfinished pair in the specified run.
func (r *Resolver) CancelProjectAnalysis(ctx context.Context, args CancelProjectAnalysisArgs) (*ProjectAnalysis, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	run, err := r.Analysis.Cancel(ctx, workspace(ctx), string(args.ProjectId), string(args.AnalysisId))
	if err != nil {
		return nil, errors.New("analysis cancellation unavailable")
	}
	return projectAnalysis(run), nil
}
