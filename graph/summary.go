package graph

import (
	"context"
	"errors"

	"github.com/VolkovDani/project-creator/internal/summary"
	graphql "github.com/graph-gophers/graphql-go"
)

func summaryState(v summary.State) *RequirementSummary {
	return &RequirementSummary{ByteLength: int32(v.ByteLength), LayaEligible: v.LayaEligible, RequirementId: graphql.ID(v.RequirementID), ProjectId: graphql.ID(v.ProjectID), Version: int32(v.Version), Status: v.Status, Text: v.Text, Profile: v.Profile, Provenance: v.Provenance, CreatedAt: v.CreatedAt, ErrorCode: v.ErrorCode}
}
func (r *Resolver) RequirementSummariesByIds(ctx context.Context, args RequirementSummariesByIdsArgs) ([]*RequirementSummary, error) {
	if e := consume(ctx); e != nil {
		return nil, e
	}
	if r.Summaries == nil {
		return nil, errors.New("summaries unavailable")
	}
	ids := make([]string, len(args.RequirementIds))
	for i, v := range args.RequirementIds {
		ids[i] = string(v)
	}
	rows, e := r.Summaries.ByIDs(ctx, workspace(ctx), string(args.ProjectId), ids)
	if e != nil {
		return nil, errors.New("summaries unavailable")
	}
	out := make([]*RequirementSummary, 0, len(rows))
	for _, v := range rows {
		out = append(out, summaryState(v))
	}
	return out, nil
}
func (r *Resolver) RetryRequirementSummary(ctx context.Context, args RetryRequirementSummaryArgs) (*RequirementSummary, error) {
	if e := consume(ctx); e != nil {
		return nil, e
	}
	if r.Summaries == nil {
		return nil, errors.New("summaries unavailable")
	}
	v, e := r.Summaries.Retry(ctx, workspace(ctx), string(args.ProjectId), string(args.RequirementId))
	if e != nil {
		return nil, errors.New("summary retry unavailable")
	}
	return summaryState(v), nil
}
func (r *Resolver) CancelRequirementSummary(ctx context.Context, args CancelRequirementSummaryArgs) (*RequirementSummary, error) {
	if e := consume(ctx); e != nil {
		return nil, e
	}
	if r.Summaries == nil {
		return nil, errors.New("summaries unavailable")
	}
	v, e := r.Summaries.Cancel(ctx, workspace(ctx), string(args.ProjectId), string(args.RequirementId))
	if e != nil {
		return nil, errors.New("summary cancellation unavailable")
	}
	return summaryState(v), nil
}
