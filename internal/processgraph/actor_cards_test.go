package processgraph

import (
	"context"
	"slices"
	"strconv"
	"testing"
	"unicode/utf8"

	"github.com/VolkovDani/project-creator/internal/glossary"
)

func TestProcessGraphGroupsConfirmedActorCards(t *testing.T) {
	type relation struct{ from, to, kind, condition string }
	req := Request{
		Anchor: Anchor{Name: "workflow"},
		Glossary: []glossary.Term{
			{ID: "intake", Name: "Coordinator", Variants: []glossary.Variant{{Text: "Intake coordinator"}, {Text: "IC"}}},
			{ID: "support", Name: "Support coordinator"},
		},
	}
	approval := Node{ID: "approval", Kind: "action", Label: "Approval"}
	check := Node{ID: "check", Kind: "action", Label: "Checks records"}
	notice := Node{ID: "notice", Kind: "action", Label: "Sends notice"}
	coordinator := Node{ID: "coordinator", Kind: "actor", Label: "Coordinator"}
	intake := Node{ID: "intake", Kind: "actor", Label: "Intake coordinator"}
	short := Node{ID: "short", Kind: "actor", Label: "IC"}
	support := Node{ID: "support", Kind: "actor", Label: "Support coordinator"}
	temporary := Node{ID: "temporary", Kind: "actor", Label: "Temporary IC"}
	rows := []struct {
		from, to    Node
		kind, label string
		quote       string
	}{
		{intake, check, "performs", "when ready", "Intake coordinator checks records when ready."},
		{coordinator, check, "performs", "when ready", "Coordinator checks records when ready."},
		{short, check, "performs", "when delayed", "IC checks records when delayed."},
		{approval, short, "requires", "", "Approval requires IC."},
		{short, notice, "performs", "when ready", "IC sends notice when ready."},
		{support, check, "performs", "", "Support coordinator checks records."},
		{temporary, check, "performs", "", "Temporary IC checks records."},
	}
	for i, row := range rows {
		req.Sources = append(req.Sources, Source{ID: "req-" + strconv.Itoa(i+1), Version: i + 1, Offset: i, Text: row.quote})
	}
	generate := func(batch Request) Suggestion {
		suggestion := Suggestion{UsedSources: len(batch.Sources)}
		for i, source := range batch.Sources {
			row := rows[source.Version-1]
			for _, node := range []Node{row.from, row.to} {
				if !slices.Contains(suggestion.Nodes, node) {
					suggestion.Nodes = append(suggestion.Nodes, node)
				}
			}
			suggestion.Edges = append(suggestion.Edges, SuggestedEdge{
				From: row.from.ID, To: row.to.ID, Kind: row.kind, Label: row.label,
				Evidence: []SuggestedEvidence{{Source: i + 1, Quote: row.quote}},
			})
		}
		return suggestion
	}
	assertDraft := func(t *testing.T, got Draft) {
		t.Helper()
		if got.Partial || got.SourceCount != len(rows) || len(got.Nodes) != 6 || len(got.Edges) != 6 {
			t.Fatalf("expected one card for three spellings with six relations: %+v", got)
		}
		nodes := make(map[string]Node)
		for _, node := range got.Nodes {
			nodes[node.ID] = node
		}
		want := map[relation][]int{
			{"Coordinator", "Checks records", "performs", "when ready"}:   {0, 1},
			{"Coordinator", "Checks records", "performs", "when delayed"}: {2},
			{"Approval", "Coordinator", "requires", ""}:                   {3},
			{"Coordinator", "Sends notice", "performs", "when ready"}:     {4},
			{"Support coordinator", "Checks records", "performs", ""}:     {5},
			{"Temporary IC", "Checks records", "performs", ""}:            {6},
		}
		for _, edge := range got.Edges {
			key := relation{nodes[edge.From].Label, nodes[edge.To].Label, edge.Kind, edge.Label}
			citations, ok := want[key]
			if !ok || len(citations) != len(edge.Evidence) {
				t.Fatalf("unexpected relation or citation count: %+v", edge)
			}
			delete(want, key)
			for i, index := range citations {
				source := req.Sources[index]
				expected := Evidence{
					RequirementID: source.ID, Version: source.Version, Quote: source.Text,
					Start: source.Offset, End: source.Offset + utf8.RuneCountInString(source.Text),
				}
				if edge.Evidence[i] != expected {
					t.Fatalf("citation changed: %+v, want %+v", edge.Evidence[i], expected)
				}
			}
		}
	}
	t.Run("single request", func(t *testing.T) {
		draft, err := Build(t.Context(), stubGenerator(func(_ context.Context, batch Request) (Suggestion, error) {
			return generate(batch), nil
		}), req)
		if err != nil {
			t.Fatal(err)
		}
		assertDraft(t, draft)
	})
	t.Run("across batches", func(t *testing.T) {
		first, second := req, req
		first.Sources, second.Sources = req.Sources[:1], req.Sources[1:]
		g := &runGenerator{batches: []Request{first, second}, run: func(batch Request) (Suggestion, error) {
			return generate(batch), nil
		}}
		store := NewRunStore(g)
		result, err := store.Start(t.Context(), sampleTarget, req, "original", false)
		if err != nil || result.Run == nil {
			t.Fatalf("start: %+v, %v", result, err)
		}
		for range 2 {
			result, err = store.Advance(t.Context(), sampleTarget.Workspace, sampleTarget.Project, result.Run.ID, result.Run.Revision, stableGraphSnapshot)
			if err != nil || result.Run == nil {
				t.Fatalf("advance: %+v, %v", result, err)
			}
		}
		if result.Status != "suggested" || result.Draft == nil {
			t.Fatalf("expected a suggested draft: %+v", result)
		}
		assertDraft(t, *result.Draft)
	})
}
