package graph

import (
	"context"
	"errors"

	"github.com/VolkovDani/project-creator/internal/rag"
	graphql "github.com/graph-gophers/graphql-go"
)

func indexState(value rag.State) *RequirementIndex {
	return &RequirementIndex{RequirementId: graphql.ID(value.RequirementID), Version: int32(value.Version), Status: value.Status, Profile: value.Profile, ErrorCode: value.ErrorCode}
}
func (r *Resolver) RequirementIndex(ctx context.Context, args RequirementIndexArgs) ([]*RequirementIndex, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	states, err := r.RAG.Status(ctx, workspace(ctx), string(args.ProjectId), int(args.Limit), int(args.Offset))
	if err != nil {
		return nil, errors.New("index status unavailable")
	}
	out := make([]*RequirementIndex, 0, len(states))
	for _, v := range states {
		out = append(out, indexState(v))
	}
	return out, nil
}
func (r *Resolver) IndexRequirement(ctx context.Context, args IndexRequirementArgs) (*RequirementIndex, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	state, err := r.RAG.Index(ctx, workspace(ctx), string(args.ProjectId), string(args.RequirementId))
	if err != nil {
		return nil, errors.New("indexing unavailable")
	}
	return indexState(state), nil
}
func (r *Resolver) SearchRequirements(ctx context.Context, args SearchRequirementsArgs) ([]*RequirementSearchHit, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	hits, err := r.RAG.Search(ctx, workspace(ctx), string(args.ProjectId), args.Text, int(args.Limit))
	if err != nil {
		return nil, errors.New("search unavailable")
	}
	out := make([]*RequirementSearchHit, 0, len(hits))
	for _, v := range hits {
		out = append(out, &RequirementSearchHit{RequirementId: graphql.ID(v.RequirementID), ProjectId: graphql.ID(v.ProjectID), Text: v.Text, SourceText: v.SourceText, Version: int32(v.Version), Start: int32(v.Start), End: int32(v.End), Score: v.Score})
	}
	return out, nil
}
