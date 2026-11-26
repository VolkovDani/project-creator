package processgraph

import (
	"context"
	"testing"
)

func TestRequirementGraphGroundsBothEndpointsAndKeepsDistinctRequirements(t *testing.T) {
	req := Request{
		Mode: RequirementMode, Anchor: Anchor{Name: "invoice"},
		Sources: []Source{
			{ID: "specific", Version: 3, Text: "Lock an invoice after approval."},
			{ID: "general", Version: 7, Text: "Lock an invoice when processing is complete."},
			{ID: "other", Version: 2, Text: "Lock an invoice after approval."},
		},
	}
	citations := func(a, b int) []SuggestedEvidence {
		return []SuggestedEvidence{{Source: a, Quote: req.Sources[a-1].Text}, {Source: b, Quote: req.Sources[b-1].Text}}
	}
	suggestion := Suggestion{UsedSources: 3, Edges: []SuggestedEdge{
		{From: "1", To: "2", Kind: "refines", Evidence: citations(1, 2)},
		{From: "1", To: "3", Kind: "copies", Evidence: citations(1, 3)},
		{From: "2", To: "1", Kind: "refines", Evidence: citations(2, 3)},
		{From: "2", To: "3", Kind: "copies", Evidence: citations(2, 3)},
		{From: "1", To: "1", Kind: "traces", Evidence: citations(1, 1)},
		{From: "3", To: "1", Kind: "performs", Evidence: citations(3, 1)},
		{From: "1", To: "8", Kind: "traces", Evidence: citations(1, 3)},
	}}
	draft, err := groundDraft(suggestion, req, nil)
	if err != nil || len(draft.Edges) != 2 || len(draft.Nodes) != 3 {
		t.Fatalf("expected two grounded relationships across three requirements: %+v, %v", draft, err)
	}
	if draft.Nodes[0].RequirementID != "specific" || draft.Nodes[0].Version != 3 ||
		draft.Nodes[0].Label != req.Sources[0].Text || draft.Edges[0].Kind != "refines" ||
		draft.Edges[0].Evidence[1].Version != 7 {
		t.Fatalf("expected labels and evidence from source snapshots: %+v", draft)
	}
	var merged Draft
	mergeDraft(&merged, draft)
	mergeDraft(&merged, draft)
	if len(merged.Nodes) != 3 || len(merged.Edges) != 2 ||
		merged.Nodes[0].Label != merged.Nodes[2].Label || merged.Nodes[0].ID == merged.Nodes[2].ID {
		t.Fatalf("requirements with identical wording must keep distinct cards: %+v", merged)
	}
}

func TestRequirementRunWithOneSourceSkipsModelAndLiteralProcessEdges(t *testing.T) {
	req := Request{Mode: RequirementMode, Anchor: Anchor{Name: "invoice"}, Sources: []Source{
		{ID: "r1", Version: 1, Text: "After invoice approval, publish the invoice."},
	}}
	calls := 0
	store := NewRunStore(stubGenerator(func(_ context.Context, _ Request) (Suggestion, error) {
		calls++
		return Suggestion{}, nil
	}))
	result, err := store.Start(t.Context(), Target{Mode: RequirementMode}, req, "snapshot", false)
	if err != nil || result.Status != "insufficient" || result.Run == nil || result.Run.Mode != RequirementMode || calls != 0 {
		t.Fatalf("single-source map cannot infer relations: %+v, %v, calls=%d", result, err, calls)
	}
}
