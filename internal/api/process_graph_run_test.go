package api

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/VolkovDani/project-creator/internal/analysis"
	"github.com/VolkovDani/project-creator/internal/processgraph"
)

func TestProcessGraphRunUsesScopedCurrentSourcesAcrossBatches(t *testing.T) {
	var calls int
	var beforeReturn func() error
	const quote = "Dispatcher reviews a delivery request."
	generator := &apiProcessGraphGenerator{
		apiSummaryGenerator: apiSummaryGenerator{text: func(analysis.Source) string { return "Summary." }},
		generate: func(req processgraph.Request) (processgraph.Suggestion, error) {
			calls++
			if len(req.Sources) > processgraph.MaxSources {
				t.Errorf("too many sources in one call: %d", len(req.Sources))
			}
			for _, source := range req.Sources {
				if source.Text != quote {
					t.Errorf("unexpected source: %+v", source)
				}
			}
			if beforeReturn != nil {
				if err := beforeReturn(); err != nil {
					t.Errorf("edit during inference: %v", err)
					return processgraph.Suggestion{}, err
				}
			}
			return processgraph.Suggestion{
				UsedSources: len(req.Sources),
				Nodes: []processgraph.Node{
					{ID: "actor", Kind: "actor", Label: "Dispatcher"},
					{ID: "review", Kind: "action", Label: "Review delivery request"},
				},
				Edges: []processgraph.SuggestedEdge{{From: "actor", To: "review", Kind: "performs",
					Evidence: []processgraph.SuggestedEvidence{{Source: 1, Quote: quote}}}},
			}, nil
		},
	}
	server, err := NewWithSummaryProviders(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"}, nil, nil, generator)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	if _, err := server.db.Exec(`
		INSERT INTO projects VALUES ('p','a','P','G','','now','now'), ('q','a','Q','G','','now','now'), ('z','b','Z','G','','now','now');
		INSERT INTO requirements(id,workspace,project_id,text,version,created_at,updated_at,archived_at) VALUES
			('archive','a','p','Archived delivery request.',1,'now','now',now()),
			('foreign-project','a','q','Foreign delivery request.',1,'now','now',NULL),
			('foreign-workspace','b','z','Foreign delivery request.',1,'now','now',NULL);
	`); err != nil {
		t.Fatal(err)
	}
	const sourceCount = processgraph.MaxSources + 1
	if _, err := server.db.Exec(`
		INSERT INTO requirements(id,workspace,project_id,text,version,created_at,updated_at)
		SELECT 'r'||lpad(i::text,2,'0'),'a','p',$1,1,'now','now' FROM generate_series(1,$2) i
	`, quote, sourceCount); err != nil {
		t.Fatal(err)
	}

	const fields = `status run { id revision completed total failed } draft { sourceCount partial edges { evidence { requirementId version } } }`
	request := func(token, name, input string) map[string]any {
		t.Helper()
		out := summaryRequest(t, server, token, fmt.Sprintf(`mutation { %s(input:{%s}){ %s } }`, name, input, fields))
		if out["errors"] != nil {
			t.Fatal(out)
		}
		return out["data"].(map[string]any)[name].(map[string]any)
	}
	start := func() map[string]any {
		t.Helper()
		out := request("token-a", "startProcessGraph", `projectId:"p",keyword:"delivery request"`)
		if out["status"] != "ready" || out["run"] == nil || out["draft"] != nil {
			t.Fatalf("expected a new run: %v", out)
		}
		run := out["run"].(map[string]any)
		if run["completed"] != float64(0) || run["total"] != float64(2) || run["failed"] != float64(0) {
			t.Fatalf("expected two unsettled batches: %v", run)
		}
		return run
	}
	advance := func(token, project string, run map[string]any) map[string]any {
		t.Helper()
		return request(token, "processGraphBatch", fmt.Sprintf(`projectId:%q,runId:%q,revision:%d`, project, run["id"], int(run["revision"].(float64))))
	}
	run := start()
	if calls != 0 {
		t.Fatalf("start invoked the model %d times", calls)
	}
	for _, scope := range []struct{ token, project string }{{"token-b", "p"}, {"token-a", "q"}} {
		if out := advance(scope.token, scope.project, run); out["status"] != "not_found" || out["run"] != nil || out["draft"] != nil {
			t.Fatalf("foreign run disclosed: %v", out)
		}
	}
	if calls != 0 {
		t.Fatalf("foreign requests invoked the model %d times", calls)
	}
	first := advance("token-a", "p", run)
	if first["status"] != "ready" || first["run"] == nil || first["draft"] != nil || calls != 1 {
		t.Fatalf("expected first batch progress, calls=%d: %v", calls, first)
	}
	progress := first["run"].(map[string]any)
	if progress["id"] != run["id"] || progress["revision"] == run["revision"] || progress["completed"] != float64(1) || progress["total"] != float64(2) || progress["failed"] != float64(0) {
		t.Fatalf("expected one of two batches completed: %v", progress)
	}
	if retry := advance("token-a", "p", run); !reflect.DeepEqual(retry, first) || calls != 1 {
		t.Fatalf("retry should replay current progress, calls=%d: %v", calls, retry)
	}
	last := advance("token-a", "p", progress)
	draft, found := last["draft"].(map[string]any)
	if last["status"] != "suggested" || !found || draft["sourceCount"] != float64(sourceCount) || draft["partial"] != false || calls != 2 {
		t.Fatalf("expected complete coverage beyond the legacy cap, calls=%d: %v", calls, last)
	}
	var secondBatchEvidence bool
	for _, item := range draft["edges"].([]any) {
		for _, evidence := range item.(map[string]any)["evidence"].([]any) {
			if evidence.(map[string]any)["requirementId"] == "r13" {
				secondBatchEvidence = true
			}
		}
	}
	if !secondBatchEvidence {
		t.Fatalf("expected evidence from the second batch: %v", last)
	}

	run = start()
	if _, err := server.db.Exec(`UPDATE requirements SET version=version+1 WHERE id='r13'`); err != nil {
		t.Fatal(err)
	}
	if out := advance("token-a", "p", run); out["status"] != "stale" || out["draft"] != nil || calls != 2 {
		t.Fatalf("a source beyond the first batch changed before inference, calls=%d: %v", calls, out)
	}
	run = start()
	beforeReturn = func() error {
		_, err := server.db.Exec(`UPDATE requirements SET text='Dispatcher approves a delivery request.' WHERE id='r13'`)
		return err
	}
	if out := advance("token-a", "p", run); out["status"] != "stale" || out["draft"] != nil || calls != 3 {
		t.Fatalf("a source beyond the first batch changed during inference, calls=%d: %v", calls, out)
	}
}
