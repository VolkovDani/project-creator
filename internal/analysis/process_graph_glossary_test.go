package analysis

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/VolkovDani/project-creator/internal/glossary"
	"github.com/VolkovDani/project-creator/internal/processgraph"
)

func TestProcessGraphSendsOnlyMatchedGlossaryConcepts(t *testing.T) {
	req := processgraph.Request{
		Anchor: processgraph.Anchor{Name: "Заявка"},
		Sources: []processgraph.Source{{ID: "requirement-private-id", Version: 4,
			Text: "🧾 Представитель ген. подрядной организации проверяет заявку. Представитель подрядной организации подтверждает заявку. РП получает уведомление."}},
		Glossary: processGraphGlossaryFixture(),
	}
	want := []map[string]any{
		{"spelling": "Представитель ген. подрядной организации", "name": "Генподрядчик"},
		{"spelling": "Представитель подрядной организации", "name": "Подрядчик"},
		{"spelling": "РП", "ambiguous": true},
	}
	for _, tc := range []struct {
		name    string
		managed bool
	}{{"normal", false}, {"managed", true}} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			s := &chatSummary{
				managedNPU: tc.managed,
				verify:     context.Context.Err,
				complete: func(_ context.Context, messages []map[string]string, schema map[string]any) (string, error) {
					calls++
					content := messages[1]["content"]
					if got := graphTestGlossary(t, content); !reflect.DeepEqual(got, want) {
						t.Errorf("unexpected matched glossary: %#v", got)
					}
					for _, private := range []string{"requirement-private-id", "concept-private-id", "unrelated-secret", "definition-secret", "context-secret", "Руководитель проекта", "Рабочий процесс"} {
						if strings.Contains(content, private) {
							t.Errorf("provider received unrelated context %q", private)
						}
					}
					encodedSchema, _ := json.Marshal(schema)
					if !graphRequestFits(messages, len(encodedSchema), tc.managed) {
						t.Error("rendered messages with glossary exceed the current profile budget")
					}
					return `{"r":[]}`, nil
				},
			}
			plan, err := s.PlanProcessGraph(req)
			if err != nil || plan.Partial || len(plan.Batches) != 1 {
				t.Fatalf("expected one complete planned batch: %+v, %v", plan, err)
			}
			if _, err := s.GenerateProcessGraphBatch(t.Context(), plan.Batches[0]); err != nil || calls != 1 {
				t.Fatalf("expected one completion, got %d: %v", calls, err)
			}
		})
	}
}

func TestProcessGraphSplitDoesNotMatchSuffixOfLongerConcept(t *testing.T) {
	const full = "🧾 Представитель ген. подрядной организации проверяет заявку."
	s := &chatSummary{
		managedNPU: true,
		verify:     context.Context.Err,
		complete: func(_ context.Context, messages []map[string]string, _ map[string]any) (string, error) {
			content := messages[1]["content"]
			if got := graphTestGlossary(t, content); len(got) > 0 {
				t.Errorf("truncated alias must not identify another concept: %#v", got)
			}
			if strings.Contains(content, "Представитель ген.") || !strings.Contains(content, "подрядной организации проверяет заявку.") {
				t.Errorf("unexpected transmitted source range: %s", content)
			}
			return `{"r":[]}`, nil
		},
	}
	req := processgraph.Request{
		Anchor:   processgraph.Anchor{Name: "Заявка"},
		Sources:  []processgraph.Source{{ID: "r1", Version: 4, Text: full}},
		Glossary: processGraphGlossaryFixture(),
	}
	plan, err := s.PlanProcessGraph(req)
	if err != nil || len(plan.Batches) != 1 {
		t.Fatalf("expected one initial batch: %+v, %v", plan, err)
	}
	_, right, ok := s.SplitProcessGraphBatch(plan.Batches[0])
	if !ok || len(right.Glossary) != len(req.Glossary) {
		t.Fatalf("split lost glossary metadata: %+v", right)
	}
	if _, err := s.GenerateProcessGraphBatch(t.Context(), right); err != nil {
		t.Fatal(err)
	}
}

func TestProcessGraphGlossaryFitsBudgetAndFollowsRetainedPassages(t *testing.T) {
	req := processgraph.Request{Anchor: processgraph.Anchor{Name: "queue"}}
	var first strings.Builder
	for index := range 12 {
		alias := "queue" + strconv.Itoa(index)
		first.WriteString(alias + " ")
		req.Glossary = append(req.Glossary, glossary.Term{
			ID: alias, Name: strings.Repeat("Workflow ", 11) + alias,
			Variants: []glossary.Variant{{Text: alias}},
		})
	}
	first.WriteString("wait for review.")
	second := glossary.Term{ID: "other", Name: "Decision Maker", Variants: []glossary.Variant{{Text: "DM"}}}
	req.Glossary = append(req.Glossary, second)
	req.Sources = []processgraph.Source{
		{ID: "r1", Version: 1, Text: "DM checks the queue."},
		{ID: "r2", Version: 1, Text: first.String()},
	}
	calls := 0
	s := &chatSummary{
		managedNPU: true,
		verify:     context.Context.Err,
		complete: func(_ context.Context, messages []map[string]string, schema map[string]any) (string, error) {
			calls++
			encodedSchema, _ := json.Marshal(schema)
			if !graphRequestFits(messages, len(encodedSchema), true) {
				t.Error("model payload exceeds the managed budget")
			}
			concepts := graphTestGlossary(t, messages[1]["content"])
			if calls == 1 {
				size, _ := json.Marshal(concepts)
				if len(concepts) <= 1 || len(concepts) >= len(req.Glossary) || len(size) > maxProcessGraphGlossaryBytes {
					t.Errorf("expected a compact glossary, got %s", size)
				}
				return "", npuFailure("NPU_UNAVAILABLE")
			}
			if len(concepts) != 1 || concepts[0]["spelling"] != "DM" {
				t.Errorf("retry kept glossary for omitted passages: %#v", concepts)
			}
			return `{"r":[]}`, nil
		},
	}
	suggestion, err := s.GenerateProcessGraph(t.Context(), req)
	if err != nil || calls != 2 || !suggestion.Partial || suggestion.UsedSources != 1 {
		t.Fatalf("expected a partial legacy result after retry, got %d calls, %+v, %v", calls, suggestion, err)
	}
}

func graphTestGlossary(t *testing.T, content string) []map[string]any {
	t.Helper()
	var payload struct{ Glossary []map[string]any }
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		t.Fatalf("invalid model prompt data: %v", err)
	}
	return payload.Glossary
}

func processGraphGlossaryFixture() []glossary.Term {
	return []glossary.Term{
		{ID: "concept-private-id", Name: "Генподрядчик", Definition: "definition-secret", Context: "context-secret",
			Variants: []glossary.Variant{{Text: "представитель ген. подрядной организации"}}},
		{ID: "contractor", Name: "Подрядчик",
			Variants: []glossary.Variant{{Text: "представитель подрядной организации"}, {Text: "подрядной организации"}}},
		{ID: "manager", Name: "Руководитель проекта", Variants: []glossary.Variant{{Text: "РП"}}},
		{ID: "workflow", Name: "Рабочий процесс", Variants: []glossary.Variant{{Text: "РП"}}},
		{ID: "unrelated", Name: "unrelated-secret"},
	}
}
