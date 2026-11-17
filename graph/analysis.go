package graph

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/VolkovDani/project-creator/contracts"
	"github.com/VolkovDani/project-creator/internal/analysis"
	graphql "github.com/graph-gophers/graphql-go"
)

func projectAnalysis(value analysis.Run) *ProjectAnalysis {
	out := &ProjectAnalysis{ClassifierProfile: value.ClassifierProfile, LayaPendingPairs: int32(value.LayaPendingPairs), LayaRunningPairs: int32(value.LayaRunningPairs), LayaCompletedPairs: int32(value.LayaCompletedPairs), LayaFailedPairs: int32(value.LayaFailedPairs), LayaSkippedPairs: int32(value.LayaSkippedPairs), Revision: value.Revision, Mode: value.Mode, SimilarityThreshold: value.SimilarityThreshold, SelectionProfile: value.SelectionProfile, DuplicatePairs: int32(value.DuplicatePairs), ReusedPairs: int32(value.ReusedPairs), NewPairs: int32(value.NewPairs), CompletedPairs: int32(value.CompletedPairs), PendingPairs: int32(value.PendingPairs), RunningPairs: int32(value.RunningPairs), FailedPairs: int32(value.FailedPairs), CollisionPairs: int32(value.CollisionPairs), CompatiblePairs: int32(value.CompatiblePairs), InsufficientPairs: int32(value.InsufficientPairs), GeneratorProvenance: value.GeneratorProvenance, Stage: value.Stage, ProcessedPairs: int32(value.ProcessedPairs), ActivePairId: graphql.ID(value.ActivePairID), Pairs: []*AnalysisPair{}, ReasonCode: value.ReasonCode, MissingRequirementIds: []graphql.ID{}, ID: graphql.ID(value.ID), ProjectId: graphql.ID(value.ProjectID), Status: value.Status, Outcome: value.Outcome, ErrorCode: value.ErrorCode, CreatedAt: value.CreatedAt, CompletedAt: value.CompletedAt, Stale: value.Stale, GeneratorProfile: value.GeneratorProfile, EmbeddingProfile: value.EmbeddingProfile, Sources: []*AnalysisSource{}, Findings: []*AnalysisFinding{}}
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

func (r *Resolver) ProjectAnalysisSummary(ctx context.Context, args ProjectAnalysisSummaryArgs) (*ProjectAnalysisSummary, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	id := ""
	if args.AnalysisId != nil {
		id = string(*args.AnalysisId)
	}
	run, err := r.Analysis.Summary(ctx, workspace(ctx), string(args.ProjectId), id)
	if err != nil {
		return nil, errors.New("analysis unavailable")
	}
	if run == nil {
		return nil, nil
	}
	full := projectAnalysis(*run)
	return &ProjectAnalysisSummary{ClassifierProfile: full.ClassifierProfile, LayaPendingPairs: full.LayaPendingPairs, LayaRunningPairs: full.LayaRunningPairs, LayaCompletedPairs: full.LayaCompletedPairs, LayaFailedPairs: full.LayaFailedPairs, LayaSkippedPairs: full.LayaSkippedPairs,
		Revision: full.Revision, Mode: full.Mode, SimilarityThreshold: full.SimilarityThreshold, PossiblePairs: full.PossiblePairs,
		NotSelectedPairs: full.NotSelectedPairs, DuplicatePairs: full.DuplicatePairs, ReusedPairs: full.ReusedPairs, NewPairs: full.NewPairs,
		CompletedPairs: full.CompletedPairs, PendingPairs: full.PendingPairs, RunningPairs: full.RunningPairs, FailedPairs: full.FailedPairs,
		CollisionPairs: full.CollisionPairs, CompatiblePairs: full.CompatiblePairs, InsufficientPairs: full.InsufficientPairs,
		Stage: full.Stage, TotalPairs: full.TotalPairs, ProcessedPairs: full.ProcessedPairs, ActivePairId: full.ActivePairId,
		ReasonCode: full.ReasonCode, MissingRequirementIds: full.MissingRequirementIds, ID: full.ID, ProjectId: full.ProjectId,
		Status: full.Status, Outcome: full.Outcome, ErrorCode: full.ErrorCode, CreatedAt: full.CreatedAt, CompletedAt: full.CompletedAt, Stale: full.Stale,
	}, nil
}

func (r *Resolver) AnalysisRunMetadata(ctx context.Context, args AnalysisRunMetadataArgs) (*AnalysisRunMetadata, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	run, err := r.Analysis.Metadata(ctx, workspace(ctx), string(args.ProjectId), string(args.AnalysisId))
	if err != nil {
		return nil, errors.New("analysis metadata unavailable")
	}
	return &AnalysisRunMetadata{ID: graphql.ID(run.ID), ProjectId: graphql.ID(run.ProjectID), GeneratorProfile: run.GeneratorProfile, GeneratorProvenance: run.GeneratorProvenance, EmbeddingProfile: run.EmbeddingProfile, SelectionProfile: run.SelectionProfile}, nil
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

// ReplaceProjectAnalysis applies selection changes to the expected active run.
func (r *Resolver) ReplaceProjectAnalysis(ctx context.Context, args ReplaceProjectAnalysisArgs) (*ProjectAnalysis, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	options, err := analysisOptions(args.Mode, args.Threshold)
	if err != nil {
		return nil, err
	}
	if fields := contracts.AnalysisReplacementStrategy(args.Strategy); len(fields) > 0 {
		return nil, analysisSelectionError{fields}
	}
	run, err := r.Analysis.Replace(ctx, workspace(ctx), string(args.ProjectId), string(args.AnalysisId), options, args.Strategy)
	if err != nil {
		return nil, errors.New("analysis replacement unavailable")
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
	encoded, _ := json.Marshal(pair)
	detailsVersion := fmt.Sprintf("%x", sha256.Sum256(encoded))
	reasons := append([]string{}, pair.SelectionReasons...)
	var preclassification *string
	if pair.Preclassification != nil {
		raw, _ := json.Marshal(pair.Preclassification)
		value := string(raw)
		preclassification = &value
	}
	var checkedAt *string
	if pair.LayaCheckedAt != "" {
		checkedAt = &pair.LayaCheckedAt
	}
	return &AnalysisPair{DetailsVersion: detailsVersion, IsExactDuplicate: pair.Verdict == "duplicate" && pair.ResultOrigin == "exact_text_v1", LayaErrorCode: pair.LayaErrorCode, LayaStatus: pair.LayaStatus, Preclassification: preclassification, LayaCheckedAt: checkedAt, SelectionReasons: reasons, SimilarityScore: pair.SimilarityScore, ResultOrigin: pair.ResultOrigin, ID: graphql.ID(pair.ID), Position: int32(pair.Position), Status: pair.Status,
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

func (r *Resolver) AnalysisPair(ctx context.Context, args AnalysisPairArgs) (*AnalysisPair, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	pair, err := r.Analysis.Pair(ctx, workspace(ctx), string(args.ProjectId), string(args.AnalysisId), string(args.PairId))
	if err != nil {
		return nil, errors.New("analysis unavailable")
	}
	return analysisPair(pair), nil
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
