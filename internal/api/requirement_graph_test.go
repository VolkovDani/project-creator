package api

import (
	"context"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/VolkovDani/project-creator/internal/analysis"
	"github.com/VolkovDani/project-creator/internal/processgraph"
)

type requirementGraphGenerator struct {
	apiSummaryGenerator
	calls atomic.Int32
}

func (g *requirementGraphGenerator) GenerateProcessGraph(_ context.Context, request processgraph.Request) (processgraph.Suggestion, error) {
	g.calls.Add(1)
	result := processgraph.Suggestion{UsedSources: len(request.Sources)}
	first := processgraph.SuggestedEvidence{Source: 1, Quote: request.Sources[0].Text}
	if request.Mode == processgraph.RequirementMode {
		second := processgraph.SuggestedEvidence{Source: 2, Quote: request.Sources[1].Text}
		result.Edges = []processgraph.SuggestedEdge{{
			From: "2", To: "1", Kind: "refines", Evidence: []processgraph.SuggestedEvidence{second, first},
		}}
		return result, nil
	}
	result.Nodes = []processgraph.Node{
		{ID: "actor", Kind: "actor", Label: "Dispatcher"}, {ID: "action", Kind: "action", Label: "Review delivery request"},
	}
	result.Edges = []processgraph.SuggestedEdge{{
		From: "actor", To: "action", Kind: "performs", Evidence: []processgraph.SuggestedEvidence{first},
	}}
	return result, nil
}

func TestBackgroundRequirementDiagramKeepsVersionedSourcesAndDistinctCache(t *testing.T) {
	path := testDatabase(t)
	generator := &requirementGraphGenerator{apiSummaryGenerator: apiSummaryGenerator{text: func(analysis.Source) string {
		return "Summary."
	}}}
	open := func() *Server {
		t.Helper()
		server, err := NewWithSummaryProviders(path, map[string]string{"token-a": "a", "token-b": "b"}, nil, nil, generator)
		if err != nil {
			t.Fatal(err)
		}
		return server
	}
	server := open()
	defer func() { server.Close() }()
	if _, err := server.db.Exec(`
		INSERT INTO projects VALUES ('p','a','P','G','','now','now');
		INSERT INTO requirements(id,workspace,project_id,text,version,created_at,updated_at) VALUES
		('r1','a','p','Dispatcher reviews a delivery request.',1,'now','now'),
		('r2','a','p','Dispatcher reviews an expedited delivery request within one hour.',2,'now','now');
	`); err != nil {
		t.Fatal(err)
	}

	const fields = `status run{mode} draft{nodes{kind label requirementId version} edges{kind evidence{requirementId version quote}}}`
	call := func(token, field, document string) map[string]any {
		t.Helper()
		response := summaryRequest(t, server, token, document)
		if response["errors"] != nil {
			t.Fatal(response)
		}
		result, _ := response["data"].(map[string]any)[field].(map[string]any)
		return result
	}
	start := func(modeField string) map[string]any {
		return call("token-a", "startBackgroundProcessGraph", `mutation{startBackgroundProcessGraph(input:{projectId:"p",keyword:"Dispatcher"`+modeField+`}){`+fields+`}}`)
	}
	latest := func(token string) map[string]any {
		return call(token, "latestProcessGraphRun", `{latestProcessGraphRun(projectId:"p"){`+fields+`}}`)
	}
	waitDraft := func(mode string) map[string]any {
		t.Helper()
		for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
			result := latest("token-a")
			if result["status"] == "suggested" && result["run"].(map[string]any)["mode"] == mode {
				return result
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("no draft for %s: %v", mode, latest("token-a"))
		return nil
	}

	start("")
	waitDraft("PROCESS")
	start(",mode:REQUIREMENTS")
	requirements := waitDraft("REQUIREMENTS")
	draft := requirements["draft"].(map[string]any)
	nodes, edges := draft["nodes"].([]any), draft["edges"].([]any)
	node := nodes[0].(map[string]any)
	edge := edges[0].(map[string]any)
	if node["kind"] != "requirement" || node["requirementId"] != "r2" || node["version"] != float64(2) ||
		edge["kind"] != "refines" || len(edge["evidence"].([]any)) != 2 || generator.calls.Load() != 2 {
		t.Fatalf("incorrect cached mode, sources, or relationship: %v, calls=%d", requirements, generator.calls.Load())
	}

	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	server = open()
	if restored := latest("token-a"); !reflect.DeepEqual(restored, requirements) {
		t.Fatalf("requirement diagram was not restored with its mode and sources: %v", restored)
	}
	if latest("token-b") != nil {
		t.Fatal("another workspace received a requirement diagram")
	}
	if result := start(",mode:PROCESS"); result["status"] != "suggested" ||
		result["run"].(map[string]any)["mode"] != "PROCESS" {
		t.Fatalf("PROCESS must reuse the default mode draft: %v", result)
	}
	if result := start(",mode:REQUIREMENTS"); result["status"] != "suggested" ||
		!reflect.DeepEqual(result["draft"], draft) || generator.calls.Load() != 2 {
		t.Fatalf("REQUIREMENTS must reuse the matching draft: %v, calls=%d", result, generator.calls.Load())
	}
}
