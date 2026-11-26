package graph

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"

	"github.com/VolkovDani/project-creator/internal/glossary"
	"github.com/VolkovDani/project-creator/internal/processgraph"
	graphql "github.com/graph-gophers/graphql-go"
)

func (r *Resolver) BuildProcessGraph(ctx context.Context, args BuildProcessGraphArgs) (*ProcessGraphPayload, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	out := &ProcessGraphPayload{FieldErrors: []*FieldError{}}
	var termID, keyword string
	input := args.Input
	path := "keyword"
	valid := (input.TermId == nil) != (input.Keyword == nil)
	if input.TermId != nil {
		path = "termId"
		termID = string(*input.TermId)
		valid = valid && strings.TrimSpace(termID) != "" && !strings.ContainsRune(termID, 0)
	}
	if input.Keyword != nil {
		path = "keyword"
		name, fields := glossary.DraftName(*input.Keyword)
		keyword = name
		valid = valid && name != "" && len(fields) == 0
	}
	if !valid {
		out.Status = "invalid"
		out.FieldErrors = append(out.FieldErrors, &FieldError{
			Path: []string{"input", path}, Code: "INVALID_VALUE", MessageKey: "processGraph.invalidAnchor", Params: []*ErrorParam{},
		})
		return out, nil
	}

	workspaceID, projectID := workspace(ctx), string(input.ProjectId)
	snapshot, err := r.processGraphSnapshot(ctx, workspaceID, projectID, termID, keyword)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		out.Status = "not_found"
		return out, nil
	case err != nil:
		return nil, errors.New("process graph sources unavailable")
	case len(snapshot.request.Sources) == 0:
		out.Status = "no_sources"
		return out, nil
	case r.ProcessGraphs == nil:
		out.Status = "unavailable"
		return out, nil
	}

	draft, err := processgraph.Build(ctx, r.ProcessGraphs, snapshot.request)
	if err != nil {
		out.Status = "unavailable"
		return out, nil
	}

	current, err := r.processGraphSnapshot(ctx, workspaceID, projectID, termID, keyword)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		out.Status = "stale"
	case err != nil:
		return nil, errors.New("process graph sources unavailable")
	case !snapshot.equal(current):
		out.Status = "stale"
	case len(draft.Edges) == 0:
		out.Status = "insufficient"
	default:
		out.Status = "suggested"
		draft.Partial = draft.Partial || snapshot.partial
		out.Draft = processGraphDraft(draft)
	}
	return out, nil
}

type processGraphAnchorMatch struct {
	sourceID   string
	start, end int
}

type processGraphSnapshot struct {
	request processgraph.Request
	version int
	matches []processGraphAnchorMatch
	partial bool
}

func (s processGraphSnapshot) equal(other processGraphSnapshot) bool {
	return s.request.Anchor == other.request.Anchor && s.version == other.version &&
		s.partial == other.partial && slices.Equal(s.request.Sources, other.request.Sources) &&
		slices.Equal(s.matches, other.matches)
}

func (r *Resolver) processGraphSnapshot(ctx context.Context, workspaceID, projectID, termID, keyword string) (processGraphSnapshot, error) {
	var snapshot processGraphSnapshot
	tx, err := r.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return snapshot, err
	}
	defer tx.Rollback()

	var projectExists int
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM projects WHERE workspace=$1 AND id=$2`, workspaceID, projectID).Scan(&projectExists)
	if err != nil {
		return snapshot, err
	}

	terms := []glossary.Term{{Name: keyword}}
	snapshot.request.Anchor.Name = keyword
	if termID != "" {
		terms, err = glossary.List(ctx, tx, workspaceID, projectID)
		if err != nil {
			return snapshot, err
		}
		index := slices.IndexFunc(terms, func(term glossary.Term) bool { return term.ID == termID })
		if index < 0 {
			return snapshot, sql.ErrNoRows
		}
		term := terms[index]
		snapshot.request.Anchor = processgraph.Anchor{Name: term.Name, Definition: term.Definition, Context: term.Context}
		snapshot.version = term.Version
	}

	matchText := glossary.MatchText(terms)
	rows, err := tx.QueryContext(ctx, `SELECT id,version,text FROM active_requirements WHERE workspace=$1 AND project_id=$2 ORDER BY id`, workspaceID, projectID)
	if err != nil {
		return snapshot, err
	}
	defer rows.Close()
	for rows.Next() {
		var source processgraph.Source
		if err := rows.Scan(&source.ID, &source.Version, &source.Text); err != nil {
			return snapshot, err
		}
		size := len(snapshot.matches)
		for _, match := range matchText(source.Text) {
			if len(match.Terms) == 1 && match.Terms[0].ID == termID {
				snapshot.matches = append(snapshot.matches, processGraphAnchorMatch{source.ID, match.Start, match.End})
			}
		}
		if len(snapshot.matches) == size {
			continue
		}
		if len(snapshot.request.Sources) == processgraph.MaxSources {
			snapshot.matches = snapshot.matches[:size]
			snapshot.partial = true
			break
		}
		snapshot.request.Sources = append(snapshot.request.Sources, source)
	}
	return snapshot, rows.Err()
}

func processGraphDraft(draft processgraph.Draft) *ProcessGraphDraft {
	out := &ProcessGraphDraft{
		Nodes: make([]*ProcessGraphNode, 0, len(draft.Nodes)), Edges: make([]*ProcessGraphEdge, 0, len(draft.Edges)),
		SourceCount: int32(draft.SourceCount), Partial: draft.Partial,
	}
	for _, node := range draft.Nodes {
		out.Nodes = append(out.Nodes, &ProcessGraphNode{ID: graphql.ID(node.ID), Kind: node.Kind, Label: node.Label})
	}
	for _, edge := range draft.Edges {
		view := &ProcessGraphEdge{From: graphql.ID(edge.From), To: graphql.ID(edge.To), Kind: edge.Kind, Label: edge.Label,
			Evidence: make([]*ProcessGraphEvidence, 0, len(edge.Evidence))}
		for _, evidence := range edge.Evidence {
			view.Evidence = append(view.Evidence, &ProcessGraphEvidence{
				RequirementId: graphql.ID(evidence.RequirementID), Version: int32(evidence.Version),
				Start: int32(evidence.Start), End: int32(evidence.End), Quote: evidence.Quote,
			})
		}
		out.Edges = append(out.Edges, view)
	}
	return out
}
