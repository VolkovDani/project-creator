package api

import (
	"reflect"
	"testing"
	"time"

	"github.com/VolkovDani/project-creator/internal/analysis"
	"github.com/VolkovDani/project-creator/internal/processgraph"
	"github.com/VolkovDani/project-creator/internal/processgraphcache"
)

func TestBackgroundProcessGraphReusesPersistedDraft(t *testing.T) {
	path := testDatabase(t)
	generator := &gatedProcessGraphGenerator{
		apiSummaryGenerator: apiSummaryGenerator{text: func(analysis.Source) string { return "Summary." }},
		entered:             make(chan processgraph.Request, 8),
		release:             make(chan struct{}),
	}
	close(generator.release)
	open := func() *Server {
		t.Helper()
		server, err := NewWithSummaryProviders(path, map[string]string{"token-a": "a", "token-b": "b"}, nil, nil, generator)
		if err != nil {
			t.Fatal(err)
		}
		return server
	}
	server := open()
	t.Cleanup(func() {
		if server != nil {
			server.Close()
		}
	})
	if _, err := server.db.Exec(`
		INSERT INTO projects VALUES ('p','a','P','G','','now','now');
		INSERT INTO requirements VALUES ('r','a','p','Dispatcher reviews a delivery request.',1,'now','now');
	`); err != nil {
		t.Fatal(err)
	}

	const fields = `status run{id} draft{nodes{label} edges{evidence{requirementId version quote}}}`
	call := func(token, field, document string) map[string]any {
		t.Helper()
		response := summaryRequest(t, server, token, document)
		if response["errors"] != nil {
			t.Fatal(response)
		}
		result, _ := response["data"].(map[string]any)[field].(map[string]any)
		return result
	}
	start := func() map[string]any {
		return call("token-a", "startBackgroundProcessGraph", `mutation{startBackgroundProcessGraph(input:{projectId:"p",keyword:"Dispatcher"}){`+fields+`}}`)
	}
	latest := func(token string) map[string]any {
		return call(token, "latestProcessGraphRun", `{latestProcessGraphRun(projectId:"p"){`+fields+`}}`)
	}
	waitDraft := func() map[string]any {
		t.Helper()
		for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
			if result := latest("token-a"); result["status"] == "suggested" {
				return result
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("background graph did not finish")
		return nil
	}

	start()
	original := waitDraft()
	if len(generator.entered) != 1 {
		t.Fatalf("expected one generation, got %d", len(generator.entered))
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	server = nil
	server = open()
	if restored := latest("token-a"); !reflect.DeepEqual(restored, original) {
		t.Fatalf("expected the full draft after restart, got %v", restored)
	}
	if result := latest("token-b"); result != nil {
		t.Fatalf("persisted graph disclosed to another workspace: %v", result)
	}

	repeated := start()
	if repeated["status"] != "suggested" || !reflect.DeepEqual(repeated["draft"], original["draft"]) ||
		reflect.DeepEqual(repeated["run"], original["run"]) || len(generator.entered) != 1 {
		t.Fatalf("expected immediate reuse with a new run ID and no generation: %v", repeated)
	}
	if current := latest("token-a"); !reflect.DeepEqual(current, repeated) {
		t.Fatalf("reused graph did not become the latest run: %v", current)
	}
	if _, err := server.db.Exec(`UPDATE requirements SET version=2 WHERE id='r'`); err != nil {
		t.Fatal(err)
	}
	if result := latest("token-a"); result["status"] != "stale" || result["draft"] != nil {
		t.Fatalf("expected saved result to become stale after editing its requirement: %v", result)
	}
	start()
	waitDraft()
	if len(generator.entered) != 2 {
		t.Fatalf("edited requirement should be processed again, got %d total calls", len(generator.entered))
	}
	if count, err := processgraphcache.Clear(t.Context(), server.db, "p"); err != nil || count != 2 {
		t.Fatalf("expected both distinct snapshots in the cache: %d, %v", count, err)
	}
	if result := latest("token-a"); result != nil {
		t.Fatalf("clearing the cache left a completed draft visible: %v", result)
	}
}
