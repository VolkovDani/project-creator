package graph

import (
	"context"
	"errors"

	"github.com/VolkovDani/project-creator/internal/glossary"
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
	return r.summaryStates(ctx, rows)
}

func (r *Resolver) summaryStates(ctx context.Context, rows []summary.State) ([]*RequirementSummary, error) {
	out := make([]*RequirementSummary, 0, len(rows))
	var match func(string) []glossary.Match
	for _, v := range rows {
		state := summaryState(v)
		if v.Status == "ready" {
			if match == nil {
				terms, err := glossary.List(ctx, r.DB, workspace(ctx), v.ProjectID)
				if err != nil {
					return nil, errors.New("glossary unavailable")
				}
				match = glossary.MatchText(terms)
			}
			state.GlossaryMatches = glossaryMatches(match(v.Text))
		}
		out = append(out, state)
	}
	return out, nil
}
func (r *Resolver) singleSummaryState(ctx context.Context, v summary.State) (*RequirementSummary, error) {
	out, err := r.summaryStates(ctx, []summary.State{v})
	if err != nil {
		return nil, err
	}
	return out[0], nil
}

func (r *Resolver) FailedRequirementSummaryCount(ctx context.Context, args FailedRequirementSummaryCountArgs) (int32, error) {
	if err := consume(ctx); err != nil {
		return 0, err
	}
	if r.Summaries == nil {
		return 0, errors.New("summaries unavailable")
	}
	count, err := r.Summaries.FailedCount(ctx, workspace(ctx), string(args.ProjectId))
	if err != nil {
		return 0, errors.New("summaries unavailable")
	}
	return count, nil
}

func (r *Resolver) RetryFailedRequirementSummaries(ctx context.Context, args RetryFailedRequirementSummariesArgs) (int32, error) {
	if err := consume(ctx); err != nil {
		return 0, err
	}
	if r.Summaries == nil {
		return 0, errors.New("summaries unavailable")
	}
	count, err := r.Summaries.RetryFailed(ctx, workspace(ctx), string(args.ProjectId))
	if err != nil {
		return 0, errors.New("summary retry unavailable")
	}
	return count, nil
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
	return r.singleSummaryState(ctx, v)
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
	return r.singleSummaryState(ctx, v)
}
