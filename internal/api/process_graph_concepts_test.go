package api

import (
	"context"
	"fmt"
	"testing"

	"github.com/VolkovDani/project-creator/internal/analysis"
	"github.com/VolkovDani/project-creator/internal/processgraph"
)

func TestProcessGraphKeepsDistinctConfirmedRolesInCitedEdges(t *testing.T) {
	const crossRole = "Представитель подрядной организации передаёт отчёт, после чего главный подрядчик проверяет отчёт."
	const contractorOnly = "Представитель подрядной организации подписывает накладную."
	var beforeReturn func() error
	generator := &apiProcessGraphGenerator{
		apiSummaryGenerator: apiSummaryGenerator{text: func(analysis.Source) string { return "Summary." }},
		generate: func(req processgraph.Request) (processgraph.Suggestion, error) {
			if req.Anchor.ConceptID != "gen" {
				t.Errorf("expected the selected anchor to resolve to gen: %+v", req.Anchor)
			}
			if beforeReturn != nil {
				if err := beforeReturn(); err != nil {
					t.Errorf("edit glossary during inference: %v", err)
					return processgraph.Suggestion{}, err
				}
			}
			return processgraph.Suggestion{
				UsedSources: len(req.Sources),
				Nodes: []processgraph.Node{
					{ID: "gen", Kind: "actor", Label: "Главный подрядчик"},
					{ID: "contractor", Kind: "actor", Label: "Представитель подрядной организации"},
					{ID: "review", Kind: "action", Label: "проверяет отчёт"},
					{ID: "send", Kind: "action", Label: "передаёт отчёт"},
					{ID: "sign", Kind: "action", Label: "подписывает накладную"},
				},
				Edges: []processgraph.SuggestedEdge{
					{From: "gen", To: "review", Kind: "performs", Evidence: []processgraph.SuggestedEvidence{{Source: 1, Quote: crossRole}}},
					{From: "contractor", To: "send", Kind: "performs", Evidence: []processgraph.SuggestedEvidence{{Source: 1, Quote: crossRole}}},
					{From: "send", To: "review", Kind: "follows", Evidence: []processgraph.SuggestedEvidence{{Source: 1, Quote: crossRole}}},
					{From: "gen", To: "sign", Kind: "performs", Evidence: []processgraph.SuggestedEvidence{{Source: 1, Quote: contractorOnly}}},
				},
			}, nil
		},
	}
	server, err := NewWithSummaryProviders(testDatabase(t), map[string]string{"token-a": "a"}, nil, nil, generator)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	if _, err := server.db.Exec(`
		INSERT INTO projects VALUES ('p','a','P','G','','now','now'), ('q','a','Q','G','','now','now');
		INSERT INTO glossary_terms(workspace,project_id,id,name,definition,meaning_context,variants,version,source_id,source_version) VALUES
			('a','p','gen','Генеральный подрядчик','Координатор работ.','','[{"kind":"synonym","text":"Главный подрядчик"}]',1,'',0),
			('a','p','contractor','Подрядчик','Исполнитель отдельных работ.','','[{"kind":"synonym","text":"Представитель подрядной организации"}]',1,'',0),
			('a','q','foreign','Главный подрядчик','Термин другого проекта.','','[]',1,'',0);
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO requirements(id,workspace,project_id,text,version,created_at,updated_at)
		VALUES ('r1','a','p',$1,1,'now','now'), ('r2','a','p','Генеральный подрядчик ведёт журнал.',1,'now','now')
	`, crossRole+" "+contractorOnly); err != nil {
		t.Fatal(err)
	}

	const draftFields = `draft { sourceCount nodes { id kind label } edges { from to evidence { requirementId version quote } } }`
	request := func(t *testing.T, mutationName, input string) map[string]any {
		t.Helper()
		fields := "status " + draftFields
		if mutationName != "buildProcessGraph" {
			fields += " run { id revision }"
		}
		out := summaryRequest(t, server, "token-a", fmt.Sprintf(`mutation { %s(input:{projectId:"p",%s}){ %s } }`, mutationName, input, fields))
		if out["errors"] != nil {
			t.Fatal(out)
		}
		return out["data"].(map[string]any)[mutationName].(map[string]any)
	}
	start := func(t *testing.T, anchor string) string {
		t.Helper()
		out := request(t, "startProcessGraph", anchor)
		if out["status"] != "ready" || out["run"] == nil {
			t.Fatalf("expected a ready graph run: %v", out)
		}
		run := out["run"].(map[string]any)
		return fmt.Sprintf(`runId:%q,revision:%d`, run["id"], int(run["revision"].(float64)))
	}

	for _, anchor := range []struct {
		name, input string
		sources     int
	}{
		{"confirmed term", `termId:"gen"`, 2},
		{"confirmed alias as keyword", `keyword:"Главный подрядчик"`, 1},
	} {
		for _, batched := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/batched=%t", anchor.name, batched), func(t *testing.T) {
				var out map[string]any
				if batched {
					out = request(t, "processGraphBatch", start(t, anchor.input))
				} else {
					out = request(t, "buildProcessGraph", anchor.input)
				}
				draft, found := out["draft"].(map[string]any)
				if out["status"] != "suggested" || !found || draft["sourceCount"] != float64(anchor.sources) {
					t.Fatalf("expected scoped suggestions and %d sources: %v", anchor.sources, out)
				}
				actors := make(map[string]string)
				for _, value := range draft["nodes"].([]any) {
					node := value.(map[string]any)
					if node["kind"] == "actor" {
						actors[node["label"].(string)] = node["id"].(string)
					}
				}
				genID := actors["Главный подрядчик"]
				contractorID := actors["Представитель подрядной организации"]
				if contractorID == "" {
					contractorID = actors["Подрядчик"]
				}
				if edges := draft["edges"].([]any); len(edges) != 3 || len(actors) != 2 || genID == "" || contractorID == "" || genID == contractorID {
					t.Fatalf("expected distinct roles in the cited process, with the unrelated signing edge removed: %v", draft)
				}
				for _, item := range draft["edges"].([]any) {
					edge := item.(map[string]any)
					evidence := edge["evidence"].([]any)[0].(map[string]any)
					if evidence["requirementId"] != "r1" || evidence["version"] != float64(1) || evidence["quote"] != crossRole {
						t.Fatalf("expected evidence naming both roles: %v", evidence)
					}
				}
			})
		}
	}

	beforeReturn = func() error {
		_, err := server.db.Exec(`UPDATE glossary_terms SET version=version+1 WHERE id='contractor'`)
		return err
	}
	t.Run("glossary version changes during legacy inference", func(t *testing.T) {
		out := request(t, "buildProcessGraph", `termId:"gen"`)
		if out["status"] != "stale" || out["draft"] != nil {
			t.Fatalf("expected version change to invalidate the evidence: %v", out)
		}
	})

	beforeReturn = func() error {
		_, err := server.db.Exec(`UPDATE glossary_terms SET variants='[]' WHERE id='contractor'`)
		return err
	}
	t.Run("glossary edit during keyword batch", func(t *testing.T) {
		out := request(t, "processGraphBatch", start(t, `keyword:"Главный подрядчик"`))
		if out["status"] != "stale" || out["draft"] != nil {
			t.Fatalf("expected glossary edit to invalidate the evidence: %v", out)
		}
	})
}

type emptyGraphPlanProvider struct {
	apiProcessGraphGenerator
	plan func() error
}

func (g *emptyGraphPlanProvider) PlanProcessGraph(processgraph.Request) (processgraph.BatchPlan, error) {
	return processgraph.BatchPlan{Partial: true}, g.plan()
}

func (*emptyGraphPlanProvider) GenerateProcessGraphBatch(context.Context, processgraph.Request) (processgraph.Suggestion, error) {
	panic("an empty plan must not call the generator")
}

func (*emptyGraphPlanProvider) SplitProcessGraphBatch(processgraph.Request) (processgraph.Request, processgraph.Request, bool) {
	return processgraph.Request{}, processgraph.Request{}, false
}

func TestProcessGraphStartRejectsLiteralDraftWhenPlanningSourcesChanged(t *testing.T) {
	generator := &emptyGraphPlanProvider{apiProcessGraphGenerator: apiProcessGraphGenerator{
		apiSummaryGenerator: apiSummaryGenerator{text: func(analysis.Source) string { return "Summary." }},
	}}
	server, err := NewWithSummaryProviders(testDatabase(t), map[string]string{"token-a": "a"}, nil, nil, generator)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	if _, err := server.db.Exec(`
		INSERT INTO projects VALUES ('p','a','P','G','','now','now'), ('q','a','Q','G','','now','now');
		INSERT INTO requirements(id,workspace,project_id,text,version,created_at,updated_at)
		VALUES ('r1','a','p','Как оператор я хочу открыть заявку.',1,'now','now'),
		       ('r2','a','q','Как оператор я хочу открыть заявку.',1,'now','now')
	`); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct{ mutation, project string }{
		{"startProcessGraph", "p"}, {"startBackgroundProcessGraph", "q"},
	} {
		t.Run(test.mutation, func(t *testing.T) {
			generator.plan = func() error {
				_, err := server.db.Exec(`UPDATE requirements SET text='Как оператор я хочу проверить заявку.',version=2 WHERE project_id=$1`, test.project)
				return err
			}
			result := summaryRequest(t, server, "token-a", fmt.Sprintf(`mutation {
			  %s(input:{projectId:%q,keyword:"заявку"}) { status draft { sourceCount } }
			}`, test.mutation, test.project))
			if result["errors"] != nil {
				t.Fatal(result)
			}
			start := result["data"].(map[string]any)[test.mutation].(map[string]any)
			if start["status"] != "stale" || start["draft"] != nil {
				t.Fatalf("planning returned an edited source in its literal graph: %v", start)
			}
		})
	}
}
