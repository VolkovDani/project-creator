package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/VolkovDani/project-creator/internal/processgraph"
)

type gatedProcessGraphGenerator struct {
	apiSummaryGenerator
	entered chan processgraph.Request
	release chan struct{}
}

func (g *gatedProcessGraphGenerator) GenerateProcessGraph(ctx context.Context, req processgraph.Request) (processgraph.Suggestion, error) {
	select {
	case g.entered <- req:
	case <-ctx.Done():
		return processgraph.Suggestion{}, ctx.Err()
	}
	select {
	case <-g.release:
	case <-ctx.Done():
		return processgraph.Suggestion{}, ctx.Err()
	}
	return processgraph.Suggestion{
		UsedSources: len(req.Sources),
		Nodes:       []processgraph.Node{{ID: "actor", Kind: "actor", Label: "Dispatcher"}, {ID: "review", Kind: "action", Label: "Review delivery request"}},
		Edges: []processgraph.SuggestedEdge{{From: "actor", To: "review", Kind: "performs",
			Evidence: []processgraph.SuggestedEvidence{{Source: 1, Quote: req.Sources[0].Text}}}},
	}, nil
}

func TestBackgroundProcessGraphSurvivesRequestAndScopesLatestResult(t *testing.T) {
	const quote = "Dispatcher reviews a delivery request."
	generator := &gatedProcessGraphGenerator{entered: make(chan processgraph.Request, 2), release: make(chan struct{}, 2)}
	server, err := NewWithSummaryProviders(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"}, nil, nil, generator)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	if _, err := server.db.Exec(`INSERT INTO projects VALUES ('p','a','P','G','','now','now'), ('q','a','Q','G','','now','now'), ('z','b','Z','G','','now','now')`); err != nil {
		t.Fatal(err)
	}
	const sourceCount = processgraph.MaxSources + 1
	if _, err := server.db.Exec(`
		INSERT INTO requirements(id,workspace,project_id,text,version,created_at,updated_at)
		SELECT 'r'||lpad(i::text,2,'0'),'a','p',$1,1,'now','now' FROM generate_series(1,$2) i
	`, quote, sourceCount); err != nil {
		t.Fatal(err)
	}

	const fields = `status run { id topic revision completed total failed } draft { sourceCount partial }`
	latest := func(token, project string) map[string]any {
		t.Helper()
		out := summaryRequest(t, server, token, fmt.Sprintf(`{latestProcessGraphRun(projectId:%q){%s}}`, project, fields))
		if out["errors"] != nil {
			t.Fatal(out)
		}
		tags := out["extensions"].(map[string]any)["cacheTags"].([]any)
		authorized := token == "token-a" && (project == "p" || project == "q") || token == "token-b" && project == "z"
		for _, suffix := range []string{"", ":requirements", ":glossary"} {
			if slices.Contains(tags, any("project:"+project+suffix)) != authorized {
				t.Fatalf("wrong cache scope for %s, %s: %v", token, project, tags)
			}
		}
		result, _ := out["data"].(map[string]any)["latestProcessGraphRun"].(map[string]any)
		return result
	}
	if result := latest("token-a", "p"); result != nil {
		t.Fatalf("expected no run before starting: %v", result)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	body, err := json.Marshal(map[string]string{"query": `mutation {startBackgroundProcessGraph(input:{projectId:"p",keyword:" delivery request "}){` + fields + `}}`})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequestWithContext(ctx, "POST", "/graphql", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer token-a")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	cancel()
	var out map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &out); err != nil || out["errors"] != nil {
		t.Fatalf("start background graph: %s (%v)", response.Body.String(), err)
	}
	started := out["data"].(map[string]any)["startBackgroundProcessGraph"].(map[string]any)
	run, ok := started["run"].(map[string]any)
	if !ok || started["status"] != "ready" || run["id"] == "" || run["topic"] != "delivery request" ||
		run["completed"] != float64(0) || run["total"] != float64(2) || started["draft"] != nil {
		t.Fatalf("expected pending background run with a canonical topic: %v", started)
	}
	awaitBatch := func(wantSources int) {
		t.Helper()
		select {
		case req := <-generator.entered:
			if len(req.Sources) != wantSources {
				t.Fatalf("expected batch with %d sources: %+v", wantSources, req)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("background inference did not continue after the starting request ended")
		}
	}
	awaitBatch(processgraph.MaxSources)
	for _, scope := range []struct{ token, project string }{{"token-a", "q"}, {"token-b", "p"}, {"token-b", "z"}} {
		if result := latest(scope.token, scope.project); result != nil {
			t.Fatalf("run disclosed to %s for project %s: %v", scope.token, scope.project, result)
		}
	}
	generator.release <- struct{}{}
	awaitBatch(1)
	progress := latest("token-a", "p")
	if current, ok := progress["run"].(map[string]any); !ok || current["id"] != run["id"] || current["completed"] != float64(1) || current["total"] != float64(2) {
		t.Fatalf("returning request did not see progress: %v", progress)
	}
	generator.release <- struct{}{}
	deadline := time.Now().Add(5 * time.Second)
	for {
		result := latest("token-a", "p")
		if result["status"] == "suggested" {
			draft := result["draft"].(map[string]any)
			if draft["sourceCount"] != float64(sourceCount) || draft["partial"] != false || result["run"].(map[string]any)["id"] != run["id"] {
				t.Fatalf("wrong final background graph: %v", result)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("background graph did not finish: %v", result)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := server.db.Exec(`UPDATE requirements SET version=version+1 WHERE id='r01'`); err != nil {
		t.Fatal(err)
	}
	if result := latest("token-a", "p"); result["status"] != "stale" || result["draft"] != nil {
		t.Fatalf("expected retained result to invalidate after evidence changed: %v", result)
	}
}
