package graph

import (
	"context"
	"errors"
	"time"

	"github.com/VolkovDani/project-creator/internal/proposal"
	graphql "github.com/graph-gophers/graphql-go"
)

const externalProposalCursor = "external-proposals"

func (r *Resolver) ExternalProposals(ctx context.Context, args ExternalProposalsArgs) (*ExternalProposalPage, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	first, err := connectionSize(args.First)
	if err != nil {
		return nil, err
	}
	projectID := string(args.ProjectId)
	scope := workspace(ctx) + "\x00" + projectID
	var before *time.Time
	var beforeID string
	if args.After != nil {
		cursor, err := decodeConnectionCursor(*args.After, externalProposalCursor, scope)
		if err != nil {
			return nil, err
		}
		date, err := time.Parse(time.RFC3339Nano, cursor.Key)
		if err != nil {
			return nil, errors.New("invalid pagination cursor")
		}
		before, beforeID = &date, cursor.ID
	}
	items, err := r.Proposals.List(ctx, workspace(ctx), projectID, first+1, before, beforeID)
	if err != nil {
		return nil, externalProposalError(err)
	}
	page := &ExternalProposalPage{Items: []*ExternalProposal{}, PageInfo: &PageInfo{HasNextPage: len(items) > first}}
	if page.PageInfo.HasNextPage {
		items = items[:first]
	}
	for _, item := range items {
		view := externalProposal(item)
		page.Items = append(page.Items, view)
		cursor := encodeConnectionCursor(externalProposalCursor, scope, view.CreatedAt, item.ID)
		page.PageInfo.EndCursor = &cursor
	}
	addCacheDependency(ctx, externalProposalTag(projectID))
	addCacheDependency(ctx, "project:"+projectID+":requirements")
	return page, nil
}

func (r *Resolver) SaveExternalProposal(ctx context.Context, args SaveExternalProposalArgs) (*ExternalProposalPayload, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	in := args.Input
	sources := make([]proposal.Reference, 0, len(in.Sources))
	for _, source := range in.Sources {
		sources = append(sources, proposal.Reference{RequirementID: string(source.RequirementId), Version: source.Version})
	}
	saved, fields, err := r.Proposals.Save(ctx, workspace(ctx), string(in.ProjectId), proposal.Input{
		RequestID: in.RequestId, Analysis: in.Analysis, Limitations: in.Limitations,
		Client: in.Client, Model: in.Model, Sharing: in.Sharing, Sources: sources,
	})
	if err != nil {
		return nil, externalProposalError(err)
	}
	result := &ExternalProposalPayload{FieldErrors: fieldErrors(fields)}
	if saved != nil {
		result.Proposal = externalProposal(*saved)
		addCacheDependency(ctx, externalProposalTag(string(in.ProjectId)))
		addCacheDependency(ctx, "project:"+string(in.ProjectId)+":requirements")
	}
	return result, nil
}

func externalProposal(item proposal.Proposal) *ExternalProposal {
	result := &ExternalProposal{
		ID: graphql.ID(item.ID), Status: proposal.PendingReview,
		Analysis: item.Analysis, Limitations: item.Limitations, Client: item.Client,
		Model: item.Model, Sharing: item.Sharing, CreatedAt: item.CreatedAt.UTC().Format(time.RFC3339Nano),
		Sources: make([]*ExternalProposalSource, 0, len(item.Sources)),
	}
	for _, source := range item.Sources {
		result.Sources = append(result.Sources, &ExternalProposalSource{
			RequirementId: graphql.ID(source.RequirementID), Version: source.Version, Status: source.Status,
		})
	}
	return result
}

func externalProposalTag(projectID string) string {
	return "project:" + projectID + ":external-proposals"
}

func externalProposalError(err error) error {
	if errors.Is(err, proposal.ErrUnavailable) {
		return proposal.ErrUnavailable
	}
	return errors.New("storage failure")
}
