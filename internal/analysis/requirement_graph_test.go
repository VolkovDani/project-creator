package analysis

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/VolkovDani/project-creator/internal/processgraph"
)

func TestRequirementGraphUsesWholeSourcesAndProposesTypedCitedLinks(t *testing.T) {
	sources := []processgraph.Source{
		{ID: "first-private-id", Version: 3, Text: "Invoice processing includes approval.\nLock the invoice when processing is complete."},
		{ID: "second-private-id", Version: 4, Text: "Approval completes processing for a prepaid invoice.\nLock the prepaid invoice after approval."},
	}
	generator := &chatSummary{
		verify: context.Context.Err,
		complete: func(_ context.Context, messages []map[string]string, _ map[string]any) (string, error) {
			user := messages[1]["content"]
			prompt := messages[0]["content"]
			if !strings.Contains(prompt, "refines") || strings.Contains(prompt, "actor performs action") ||
				!strings.Contains(user, "Invoice processing includes approval.") || !strings.Contains(user, "Lock the prepaid invoice after approval.") ||
				strings.Contains(user, "private-id") {
				t.Fatalf("invalid requirement graph prompt: %v", messages)
			}
			return `{"r":[{"a":4,"b":2,"k":"refines"},{"a":3,"b":4,"k":"traces"},{"a":4,"b":10,"k":"derives"}]}`, nil
		},
	}
	draft, err := processgraph.Build(t.Context(), generator, processgraph.Request{
		Mode: processgraph.RequirementMode, Anchor: processgraph.Anchor{Name: "invoice"}, Sources: sources,
	})
	if err != nil || len(draft.Edges) != 1 || len(draft.Nodes) != 2 {
		t.Fatalf("expected only one source-grounded requirement relation: %+v, %v", draft, err)
	}
	relation := draft.Edges[0]
	if relation.Kind != "refines" || relation.Evidence[0].Quote != "Lock the prepaid invoice after approval." ||
		relation.Evidence[0].Version != 4 || relation.Evidence[1].Quote != "Lock the invoice when processing is complete." ||
		draft.Nodes[0].RequirementID != sources[1].ID {
		t.Fatalf("wrong direction, citation, or requirement identity: %+v", draft)
	}
}

func TestRequirementGraphBatchesOverlapAndKeepWholeSources(t *testing.T) {
	req := processgraph.Request{Mode: processgraph.RequirementMode, Sources: []processgraph.Source{
		{ID: "a", Text: "First full requirement."}, {ID: "b", Text: "Second full requirement."},
		{ID: "c", Text: "Third full requirement."}, {ID: "d", Text: "Fourth full requirement."},
	}}
	plan, err := planProcessGraph(req, func(batch processgraph.Request) (bool, error) {
		return len(batch.Sources) <= 3, nil
	})
	if err != nil || !plan.Partial || len(plan.Batches) != 2 ||
		!slices.Equal(plan.Batches[0].Sources, req.Sources[:3]) ||
		!slices.Equal(plan.Batches[1].Sources, req.Sources[2:]) {
		t.Fatalf("expected overlapping complete requirements and a partial comparison: %+v, %v", plan, err)
	}
}

func TestRequirementGraphPlanSkipsAnOversizedSourceBetweenFittingSources(t *testing.T) {
	req := processgraph.Request{Mode: processgraph.RequirementMode, Sources: []processgraph.Source{
		{ID: "a", Text: "First full requirement."},
		{ID: "big", Text: strings.Repeat("Oversized requirement. ", 100)},
		{ID: "c", Text: "Third full requirement."},
	}}
	plan, err := planProcessGraph(req, func(batch processgraph.Request) (bool, error) {
		for _, source := range batch.Sources {
			if len(source.Text) > 100 {
				return false, nil
			}
		}
		return true, nil
	})
	if err != nil || !plan.Partial || len(plan.Batches) != 1 ||
		!slices.Equal(plan.Batches[0].Sources, []processgraph.Source{req.Sources[0], req.Sources[2]}) {
		t.Fatalf("expected both fitting requirements to remain intact: %+v, %v", plan, err)
	}
}
