package api

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/VolkovDani/project-creator/internal/analysis"
	"github.com/VolkovDani/project-creator/internal/processgraph"
)

func TestProcessGraphRequiresProjectAccess(t *testing.T) {
	server, err := New(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	_, err = server.db.Exec(`
		INSERT INTO projects VALUES ('p','a','P','G','','now','now'), ('q','a','Q','G','','now','now');
		INSERT INTO requirements(id,workspace,project_id,text,version,created_at,updated_at)
		VALUES ('r','a','p','ГП проверяет заявку.',1,'now','now');
		INSERT INTO glossary_terms(workspace,project_id,id,name,definition,meaning_context,variants,version,source_id,source_version)
		VALUES ('a','p','t','Генеральный подрядчик','Исполнитель проекта.','','[{"kind":"abbreviation","text":"ГП"}]',1,'',0);
	`)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name, token, input, status string
	}{
		{"authorized keyword", "token-a", `projectId:"p",keyword:"ГП"`, "unavailable"},
		{"foreign project", "token-b", `projectId:"p",keyword:"ГП"`, "not_found"},
		{"foreign glossary term", "token-a", `projectId:"q",termId:"t"`, "not_found"},
		{"no sources", "token-a", `projectId:"q",keyword:"ГП"`, "no_sources"},
		{"conflicting anchors", "token-a", `projectId:"p",termId:"t",keyword:"ГП"`, "invalid"},
	} {
		for _, mutation := range []string{"buildProcessGraph", "startProcessGraph"} {
			t.Run(mutation+"/"+tc.name, func(t *testing.T) {
				out := summaryRequest(t, server, tc.token, `mutation { `+mutation+`(input:{`+tc.input+`}){ status draft { sourceCount } fieldErrors { path code messageKey params { key value } } } }`)
				if out["errors"] != nil {
					t.Fatal(out)
				}
				got := out["data"].(map[string]any)[mutation].(map[string]any)
				if got["status"] != tc.status || got["draft"] != nil {
					t.Fatalf("expected %s and no draft, got %v", tc.status, got)
				}
				if tc.status == "invalid" {
					fields := got["fieldErrors"].([]any)
					if len(fields) != 1 || fields[0].(map[string]any)["messageKey"] != "processGraph.invalidAnchor" {
						t.Fatalf("expected invalid anchor field error, got %v", got)
					}
				}
			})
		}
	}
}

type apiProcessGraphGenerator struct {
	apiSummaryGenerator
	generate func(processgraph.Request) (processgraph.Suggestion, error)
}

func (g *apiProcessGraphGenerator) GenerateProcessGraph(_ context.Context, req processgraph.Request) (processgraph.Suggestion, error) {
	return g.generate(req)
}

func TestProcessGraphSelectsCurrentScopedEvidence(t *testing.T) {
	generator := &apiProcessGraphGenerator{
		apiSummaryGenerator: apiSummaryGenerator{text: func(analysis.Source) string { return "Summary." }},
	}
	server, err := NewWithSummaryProviders(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"}, nil, nil, generator)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	_, err = server.db.Exec(`
		INSERT INTO projects VALUES ('p','a','P','G','','now','now'), ('q','a','Q','G','','now','now'), ('z','b','Z','G','','now','now');
		INSERT INTO requirements(id,workspace,project_id,text,version,created_at,updated_at,archived_at) VALUES
		('r1','a','p','🧾 ГП проверяет заявку.',1,'now','now',NULL),
		('r2','a','p','ОП проверяет заявку.',1,'now','now',NULL),
		('r3','a','p','АГП проверяет заявку.',1,'now','now',NULL),
		('r4','a','p','ГП архивирует заявку.',1,'now','now',now()),
		('r5','a','q','ГП из другого проекта.',1,'now','now',NULL),
		('r6','b','z','ГП из другого пространства.',1,'now','now',NULL);
		INSERT INTO glossary_terms(workspace,project_id,id,name,definition,meaning_context,variants,version,source_id,source_version) VALUES
		('a','p','t','Генеральный подрядчик','Исполнитель проекта.','Строительство','[{"kind":"abbreviation","text":"ГП"},{"kind":"synonym","text":"ОП"}]',1,'',0),
		('a','p','u','Операционный подрядчик','Другой исполнитель.','','[{"kind":"abbreviation","text":"ОП"}]',1,'',0),
		('a','q','v','ГП','Чужое понятие.','','[]',1,'',0);
	`)
	if err != nil {
		t.Fatal(err)
	}

	var beforeReturn func() error
	const quote = "ГП проверяет заявку."
	generator.generate = func(req processgraph.Request) (processgraph.Suggestion, error) {
		anchor := processgraph.Anchor{Name: "Генеральный подрядчик", Definition: "Исполнитель проекта.", Context: "Строительство"}
		sources := []processgraph.Source{{ID: "r1", Version: 1, Text: "🧾 " + quote}}
		if req.Anchor != anchor || !slices.Equal(req.Sources, sources) {
			t.Errorf("unexpected model inputs: %+v", req)
			return processgraph.Suggestion{}, fmt.Errorf("unexpected model inputs")
		}
		if beforeReturn != nil {
			if err := beforeReturn(); err != nil {
				t.Errorf("mutating sources during inference: %v", err)
				return processgraph.Suggestion{}, err
			}
		}
		return processgraph.Suggestion{
			UsedSources: 1,
			Nodes:       []processgraph.Node{{ID: "actor", Kind: "actor", Label: "ГП"}, {ID: "check", Kind: "action", Label: "Проверяет заявку"}},
			Edges: []processgraph.SuggestedEdge{{From: "actor", To: "check", Kind: "performs",
				Evidence: []processgraph.SuggestedEvidence{{Source: 1, Quote: quote}}}},
		}, nil
	}

	query := func() map[string]any {
		t.Helper()
		out := summaryRequest(t, server, "token-a", `mutation {
			buildProcessGraph(input:{projectId:"p",termId:"t"}) {
				status draft {sourceCount partial nodes {id} edges {from to evidence {requirementId version start end quote}}}
			}
		}`)
		if out["errors"] != nil {
			t.Fatal(out)
		}
		return out["data"].(map[string]any)["buildProcessGraph"].(map[string]any)
	}

	out := query()
	draft, found := out["draft"].(map[string]any)
	if out["status"] != "suggested" || !found || draft["sourceCount"] != float64(1) || draft["partial"] != false {
		t.Fatalf("unexpected graph draft: %v", out)
	}
	evidence := draft["edges"].([]any)[0].(map[string]any)["evidence"].([]any)[0].(map[string]any)
	if evidence["requirementId"] != "r1" || evidence["version"] != float64(1) || evidence["start"] != float64(2) || evidence["quote"] != quote {
		t.Fatalf("unexpected evidence: %v", evidence)
	}

	beforeReturn = func() error {
		_, err := server.db.Exec(`UPDATE glossary_terms SET variants='[{"kind":"abbreviation","text":"ОП"},{"kind":"synonym","text":"ГП"}]',version=2 WHERE id='u'`)
		return err
	}
	if out = query(); out["status"] != "stale" || out["draft"] != nil {
		t.Fatalf("model output survived a newly ambiguous anchor: %v", out)
	}

	_, err = server.db.Exec(`UPDATE glossary_terms SET variants='[{"kind":"abbreviation","text":"ОП"}]',version=3 WHERE id='u'`)
	if err != nil {
		t.Fatal(err)
	}
	beforeReturn = func() error {
		_, err := server.db.Exec(`UPDATE requirements SET text='ГП проверяет договор.',version=2 WHERE id='r1'`)
		return err
	}
	if out = query(); out["status"] != "stale" || out["draft"] != nil {
		t.Fatalf("model output survived a requirement edit: %v", out)
	}
}

func TestProcessGraphReportsBoundedKeywordCoverage(t *testing.T) {
	const quote = "The dispatcher reviews a delivery request."
	generator := &apiProcessGraphGenerator{
		apiSummaryGenerator: apiSummaryGenerator{text: func(analysis.Source) string { return "Summary." }},
		generate: func(req processgraph.Request) (processgraph.Suggestion, error) {
			if req.Anchor != (processgraph.Anchor{Name: "delivery request"}) || len(req.Sources) != processgraph.MaxSources {
				t.Errorf("unexpected bounded inputs: %+v", req)
				return processgraph.Suggestion{}, fmt.Errorf("unexpected bounded inputs")
			}
			for index, source := range req.Sources {
				if source.ID != fmt.Sprintf("r%02d", index+1) || source.Text != quote {
					t.Errorf("unexpected source %d: %+v", index, source)
				}
			}
			return processgraph.Suggestion{
				UsedSources: len(req.Sources),
				Nodes:       []processgraph.Node{{ID: "actor", Kind: "actor", Label: "Dispatcher"}, {ID: "review", Kind: "action", Label: "Review delivery request"}},
				Edges: []processgraph.SuggestedEdge{{From: "actor", To: "review", Kind: "performs",
					Evidence: []processgraph.SuggestedEvidence{{Source: 1, Quote: quote}}}},
			}, nil
		},
	}
	server, err := NewWithSummaryProviders(testDatabase(t), map[string]string{"token-a": "a"}, nil, nil, generator)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	if _, err := server.db.Exec(`INSERT INTO projects VALUES ('p','a','P','G','','now','now')`); err != nil {
		t.Fatal(err)
	}
	_, err = server.db.Exec(`
		INSERT INTO requirements(id,workspace,project_id,text,version,created_at,updated_at)
		VALUES ('r00','a','p','The dispatcher reviews delivery requests.',1,'now','now')
	`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = server.db.Exec(`
		INSERT INTO requirements(id,workspace,project_id,text,version,created_at,updated_at)
		SELECT 'r'||lpad(i::text,2,'0'),'a','p',$1,1,'now','now'
		FROM generate_series(1,$2) AS i
	`, quote, processgraph.MaxSources+1)
	if err != nil {
		t.Fatal(err)
	}
	out := summaryRequest(t, server, "token-a", `mutation {
		buildProcessGraph(input:{projectId:"p",keyword:" delivery request "}) {
			status draft {sourceCount partial}
		}
	}`)
	if out["errors"] != nil {
		t.Fatal(out)
	}
	payload := out["data"].(map[string]any)["buildProcessGraph"].(map[string]any)
	draft, found := payload["draft"].(map[string]any)
	if payload["status"] != "suggested" || !found || draft["sourceCount"] != float64(processgraph.MaxSources) || draft["partial"] != true {
		t.Fatalf("expected a partial result from capped sources: %v", payload)
	}
}
