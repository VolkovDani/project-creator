package graph

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/VolkovDani/project-creator/internal/glossary"
	"github.com/VolkovDani/project-creator/internal/processgraph"
	graphql "github.com/graph-gophers/graphql-go"
)

var (
	errProcessGraphSources = errors.New("process graph sources unavailable")
	errProcessGraphRun     = errors.New("process graph run unavailable")
)

type ProcessGraphMode string

func (ProcessGraphMode) ImplementsGraphQLType(name string) bool { return name == "ProcessGraphMode" }

func (m *ProcessGraphMode) UnmarshalGraphQL(input any) error {
	name, ok := input.(string)
	if !ok || (name != processgraph.ProcessMode.Name() && name != processgraph.RequirementMode.Name()) {
		return fmt.Errorf("invalid process graph mode: %v", input)
	}
	*m = ProcessGraphMode(name)
	return nil
}

func (r *Resolver) BuildProcessGraph(ctx context.Context, args BuildProcessGraphArgs) (*ProcessGraphPayload, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	out := &ProcessGraphPayload{FieldErrors: []*FieldError{}}
	target, field := processGraphTarget(ctx, args.Input)
	if field != nil {
		out.Status = "invalid"
		out.FieldErrors = append(out.FieldErrors, field)
		return out, nil
	}

	snapshot, err := r.processGraphSnapshot(ctx, target, processgraph.MaxSources)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		out.Status = "not_found"
		return out, nil
	case err != nil:
		return nil, errProcessGraphSources
	case len(snapshot.request.Sources) == 0:
		out.Status = "no_sources"
		return out, nil
	case r.ProcessGraphs == nil:
		out.Status = "unavailable"
		return out, nil
	}
	key, err := snapshot.key(target)
	if err != nil {
		return nil, errProcessGraphSources
	}

	draft, err := processgraph.Build(ctx, r.ProcessGraphs, snapshot.request)
	if err != nil {
		out.Status = "unavailable"
		return out, nil
	}

	currentKey, err := r.currentProcessGraphKey(ctx, target, processgraph.MaxSources)
	switch {
	case err != nil:
		return nil, errProcessGraphSources
	case key != currentKey:
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

func (r *Resolver) StartProcessGraph(ctx context.Context, args StartProcessGraphArgs) (*ProcessGraphRunPayload, error) {
	return r.startProcessGraphRun(ctx, args.Input, r.ProcessGraphRuns.Start)
}

func (r *Resolver) StartBackgroundProcessGraph(ctx context.Context, args StartBackgroundProcessGraphArgs) (*ProcessGraphRunPayload, error) {
	return r.startProcessGraphRun(ctx, args.Input, func(ctx context.Context, target processgraph.Target, request processgraph.Request, key string, partial bool) (processgraph.RunResult, error) {
		if r.BackgroundGraphs == nil {
			return processgraph.RunResult{Status: "unavailable"}, nil
		}
		return r.BackgroundGraphs.Start(ctx, target, request, key, partial, r.currentProcessGraphRunKey)
	})
}

func (r *Resolver) startProcessGraphRun(ctx context.Context, input BuildProcessGraphInput,
	start func(context.Context, processgraph.Target, processgraph.Request, string, bool) (processgraph.RunResult, error),
) (*ProcessGraphRunPayload, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	out := &ProcessGraphRunPayload{FieldErrors: []*FieldError{}}
	target, field := processGraphTarget(ctx, input)
	if field != nil {
		out.Status = "invalid"
		out.FieldErrors = append(out.FieldErrors, field)
		return out, nil
	}
	snapshot, err := r.processGraphSnapshot(ctx, target, processgraph.MaxRunSources)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		out.Status = "not_found"
		return out, nil
	case err != nil:
		return nil, errProcessGraphSources
	case len(snapshot.request.Sources) == 0:
		out.Status = "no_sources"
		return out, nil
	}
	key, err := snapshot.key(target)
	if err != nil {
		return nil, errProcessGraphSources
	}
	result, err := start(ctx, target, snapshot.request, key, snapshot.partial)
	if err != nil {
		return nil, errProcessGraphRun
	}
	if result.Draft != nil {
		current, err := r.currentProcessGraphRunKey(ctx, target)
		if err != nil {
			return nil, errProcessGraphSources
		}
		if current != key {
			return processGraphRunPayload(processgraph.RunResult{Status: "stale"}), nil
		}
	}
	return processGraphRunPayload(result), nil
}

func (r *Resolver) LatestProcessGraphRun(ctx context.Context, args LatestProcessGraphRunArgs) (*ProcessGraphRunPayload, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	ws, projectID := workspace(ctx), string(args.ProjectId)
	var authorized bool
	if err := r.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM projects WHERE workspace=$1 AND id=$2)`, ws, projectID).Scan(&authorized); err != nil {
		return nil, errProcessGraphSources
	}
	if !authorized || r.BackgroundGraphs == nil {
		return nil, nil
	}
	addCacheDependency(ctx, "project:"+projectID)
	addCacheDependency(ctx, "project:"+projectID+":requirements")
	addCacheDependency(ctx, "project:"+projectID+":glossary")
	addCacheDependency(ctx, "project:"+projectID+":process-graph")
	result, err := r.BackgroundGraphs.Latest(ctx, ws, projectID, r.currentProcessGraphRunKey)
	if err != nil {
		return nil, errProcessGraphRun
	}
	if result.Status == "" {
		return nil, nil
	}
	return processGraphRunPayload(result), nil
}

func (r *Resolver) ProcessGraphBatch(ctx context.Context, args ProcessGraphBatchArgs) (*ProcessGraphRunPayload, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	if r.ProcessGraphRuns == nil {
		return processGraphRunPayload(processgraph.RunResult{Status: "not_found"}), nil
	}
	input := args.Input
	result, err := r.ProcessGraphRuns.Advance(ctx, workspace(ctx), string(input.ProjectId), string(input.RunId), int(input.Revision),
		r.currentProcessGraphRunKey)
	if err != nil {
		return nil, errProcessGraphRun
	}
	return processGraphRunPayload(result), nil
}

func (r *Resolver) currentProcessGraphRunKey(ctx context.Context, target processgraph.Target) (string, error) {
	return r.currentProcessGraphKey(ctx, target, processgraph.MaxRunSources)
}

func (r *Resolver) currentProcessGraphKey(ctx context.Context, target processgraph.Target, maxSources int) (string, error) {
	snapshot, err := r.processGraphSnapshot(ctx, target, maxSources)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", errProcessGraphSources
	}
	return snapshot.key(target)
}

func processGraphTarget(ctx context.Context, input BuildProcessGraphInput) (processgraph.Target, *FieldError) {
	target := processgraph.Target{Workspace: workspace(ctx), Project: string(input.ProjectId)}
	if input.Mode != nil && string(*input.Mode) == processgraph.RequirementMode.Name() {
		target.Mode = processgraph.RequirementMode
	}
	path := "keyword"
	valid := (input.TermId == nil) != (input.Keyword == nil)
	if input.TermId != nil {
		path = "termId"
		target.TermID = string(*input.TermId)
		valid = valid && strings.TrimSpace(target.TermID) != "" && !strings.ContainsRune(target.TermID, 0)
	}
	if input.Keyword != nil {
		path = "keyword"
		name, fields := glossary.DraftName(*input.Keyword)
		target.Keyword = name
		valid = valid && name != "" && len(fields) == 0
	}
	if !valid {
		return target, &FieldError{
			Path: []string{"input", path}, Code: "INVALID_VALUE", MessageKey: "processGraph.invalidAnchor", Params: []*ErrorParam{},
		}
	}
	return target, nil
}

type processGraphAnchorMatch struct {
	SourceID   string
	Start, End int
}

type processGraphSnapshot struct {
	request processgraph.Request
	matches []processGraphAnchorMatch
	partial bool
}

func (s processGraphSnapshot) key(target processgraph.Target) (string, error) {
	h := sha256.New()
	err := json.NewEncoder(h).Encode(struct {
		Target          processgraph.Target
		Anchor          processgraph.Anchor
		AnchorConceptID string
		Glossary        []glossary.Term
		Sources         []processgraph.Source
		Matches         []processGraphAnchorMatch
		Partial         bool
	}{target, s.request.Anchor, s.request.Anchor.ConceptID, s.request.Glossary, s.request.Sources, s.matches, s.partial})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (r *Resolver) processGraphSnapshot(ctx context.Context, target processgraph.Target, maxSources int) (processGraphSnapshot, error) {
	snapshot := processGraphSnapshot{request: processgraph.Request{Mode: target.Mode}}
	tx, err := r.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return snapshot, err
	}
	defer tx.Rollback()

	var projectExists int
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM projects WHERE workspace=$1 AND id=$2`, target.Workspace, target.Project).Scan(&projectExists)
	if err != nil {
		return snapshot, err
	}

	terms, err := glossary.List(ctx, tx, target.Workspace, target.Project)
	if err != nil {
		return snapshot, err
	}
	snapshot.request.Glossary = terms
	matchText := glossary.MatchText(terms)
	snapshot.request.Anchor.Name = target.Keyword
	if target.TermID != "" {
		index := slices.IndexFunc(terms, func(term glossary.Term) bool { return term.ID == target.TermID })
		if index < 0 {
			return snapshot, sql.ErrNoRows
		}
		term := terms[index]
		snapshot.request.Anchor = processgraph.Anchor{Name: term.Name, Definition: term.Definition, Context: term.Context, ConceptID: term.ID}
	} else {
		matches := matchText(target.Keyword)
		if len(matches) == 1 && matches[0].Text == target.Keyword && len(matches[0].Terms) == 1 {
			snapshot.request.Anchor.ConceptID = matches[0].Terms[0].ID
		}
		// Keywords select literal matches, even when they identify a confirmed term.
		matchText = glossary.MatchText([]glossary.Term{{Name: target.Keyword}})
	}

	rows, err := tx.QueryContext(ctx, `SELECT id,version,text FROM active_requirements WHERE workspace=$1 AND project_id=$2 ORDER BY id`, target.Workspace, target.Project)
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
			if len(match.Terms) == 1 && match.Terms[0].ID == target.TermID {
				snapshot.matches = append(snapshot.matches, processGraphAnchorMatch{source.ID, match.Start, match.End})
			}
		}
		if len(snapshot.matches) == size {
			continue
		}
		if len(snapshot.request.Sources) == maxSources {
			snapshot.matches = snapshot.matches[:size]
			snapshot.partial = true
			break
		}
		snapshot.request.Sources = append(snapshot.request.Sources, source)
	}
	return snapshot, rows.Err()
}

func processGraphRunPayload(result processgraph.RunResult) *ProcessGraphRunPayload {
	out := &ProcessGraphRunPayload{Status: result.Status, FieldErrors: []*FieldError{}}
	if run := result.Run; run != nil {
		out.Run = &ProcessGraphRun{
			ID: graphql.ID(run.ID), Topic: run.Topic, Mode: ProcessGraphMode(run.Mode.Name()), Revision: int32(run.Revision), Completed: int32(run.Completed),
			Total: int32(run.Total), Failed: int32(run.Failed),
		}
	}
	if result.Draft != nil {
		out.Draft = processGraphDraft(*result.Draft)
	}
	return out
}

func processGraphDraft(draft processgraph.Draft) *ProcessGraphDraft {
	out := &ProcessGraphDraft{
		Nodes: make([]*ProcessGraphNode, 0, len(draft.Nodes)), Edges: make([]*ProcessGraphEdge, 0, len(draft.Edges)),
		SourceCount: int32(draft.SourceCount), Partial: draft.Partial,
	}
	for _, node := range draft.Nodes {
		view := &ProcessGraphNode{ID: graphql.ID(node.ID), Kind: node.Kind, Label: node.Label}
		if node.RequirementID != "" {
			id, version := graphql.ID(node.RequirementID), int32(node.Version)
			view.RequirementId, view.Version = &id, &version
		}
		out.Nodes = append(out.Nodes, view)
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
