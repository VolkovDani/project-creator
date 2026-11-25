package graph

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/VolkovDani/project-creator/internal/analysis"
	graphql "github.com/graph-gophers/graphql-go"
)

const similarPairsCursor = "similar-requirement-pairs"

func (r *Resolver) SimilarRequirementPairs(ctx context.Context, args SimilarRequirementPairsArgs) (*SimilarRequirementPairPage, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	first, err := connectionSize(args.First)
	if err != nil {
		return nil, err
	}
	if _, err := analysisOptions(nil, &args.Threshold, 0); err != nil {
		return nil, err
	}
	ws, projectID := workspace(ctx), string(args.ProjectId)
	scope := ws + "\x00" + projectID + "\x00" + strconv.FormatFloat(math.Abs(args.Threshold), 'g', -1, 64)
	var after analysis.SimilarPairPosition
	if args.After != nil {
		cursor, err := decodeConnectionCursor(*args.After, similarPairsCursor, scope)
		if err != nil {
			return nil, err
		}
		snapshot, left, ok := strings.Cut(cursor.Key, ":")
		if !ok || len(snapshot) != 32 || left == "" {
			return nil, errors.New("invalid pagination cursor")
		}
		after = analysis.SimilarPairPosition{Snapshot: snapshot, LeftID: left, RightID: cursor.ID}
	}
	page, err := r.Analysis.ListSimilarPairs(ctx, ws, projectID, args.Threshold, first, after)
	if err != nil {
		return nil, err
	}
	out := &SimilarRequirementPairPage{
		Status: page.Status, EmbeddingProfile: page.EmbeddingProfile, MissingRequirements: int32(page.MissingRequirements),
		Items: make([]*SimilarRequirementPair, 0, len(page.Items)), PageInfo: &PageInfo{},
	}
	for _, pair := range page.Items {
		out.Items = append(out.Items, &SimilarRequirementPair{
			Left:            &AnalysisSource{ID: graphql.ID(pair.Left.ID), Version: int32(pair.Left.Version), Text: pair.Left.Text},
			Right:           &AnalysisSource{ID: graphql.ID(pair.Right.ID), Version: int32(pair.Right.Version), Text: pair.Right.Text},
			SimilarityScore: pair.SimilarityScore,
			GlossaryContext: pairGlossaryHints(pair.ConfirmedGlossary),
		})
	}
	if next := page.Next; next != nil {
		cursor := encodeConnectionCursor(similarPairsCursor, scope, next.Snapshot+":"+next.LeftID, next.RightID)
		out.PageInfo.EndCursor = &cursor
		out.PageInfo.HasNextPage = true
	}
	addCacheDependency(ctx, "project:"+projectID+":requirements")
	addCacheDependency(ctx, "project:"+projectID+":glossary")
	return out, nil
}
