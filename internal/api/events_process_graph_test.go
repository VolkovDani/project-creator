package api

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/VolkovDani/project-creator/internal/analysis"
	"github.com/VolkovDani/project-creator/internal/processgraph"
)

func TestProcessGraphProgressStream(t *testing.T) {
	const quote = "Dispatcher reviews a delivery request."
	generator := &gatedProcessGraphGenerator{
		apiSummaryGenerator: apiSummaryGenerator{text: func(analysis.Source) string { return "Summary." }},
		entered:             make(chan processgraph.Request, 2),
		release:             make(chan struct{}, 2),
	}
	api, err := NewWithSummaryProviders(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"}, nil, nil, generator)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { api.Close() })
	if _, err := api.db.Exec(`INSERT INTO projects VALUES ('p','a','P','G','','now','now'), ('q','b','Q','G','','now','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := api.db.Exec(`
		INSERT INTO requirements(id,workspace,project_id,text,version,created_at,updated_at)
		SELECT 'r'||lpad(i::text,2,'0'),'a','p',$1,1,'now','now' FROM generate_series(1,$2) i
	`, quote, processgraph.MaxSources+1); err != nil {
		t.Fatal(err)
	}

	unauthenticated := httptest.NewRecorder()
	api.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/events", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated event status = %d", unauthenticated.Code)
	}
	out := summaryRequest(t, api, "token-a", `mutation {
		startBackgroundProcessGraph(input:{projectId:"p",keyword:" delivery request "}) {
			status run { id revision }
		}
	}`)
	if out["errors"] != nil {
		t.Fatal(out)
	}
	started := out["data"].(map[string]any)["startBackgroundProcessGraph"].(map[string]any)
	if started["status"] != "ready" {
		t.Fatal(started)
	}
	run := started["run"].(map[string]any)
	runID, revision := run["id"].(string), int(run["revision"].(float64))

	appendEvent(t, api.db, "cursor-a", "a")
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	owner := connectProcessGraphEvents(t, server.URL, "token-a", "")
	owner("cache.reset", "")
	assertProcessGraphEvent(t, owner("process_graph.progress", ""), "ready", runID, revision, 0)

	other := connectProcessGraphEvents(t, server.URL, "token-b", "")
	other("cache.reset", "")
	generator.release <- struct{}{}
	assertProcessGraphEvent(t, owner("process_graph.progress", ""), "ready", runID, revision+1, 1)
	generator.release <- struct{}{}
	assertProcessGraphEvent(t, owner("process_graph.progress", ""), "suggested", runID, revision+2, 2)

	appendEvent(t, api.db, "cursor-other", "b")
	other("cache.invalidate", "cursor-other")
	appendEvent(t, api.db, "cursor-next", "a")
	owner("cache.invalidate", "cursor-next")
	reconnected := connectProcessGraphEvents(t, server.URL, "token-a", "cursor-a")
	reconnected("cache.invalidate", "cursor-next")
	assertProcessGraphEvent(t, reconnected("process_graph.progress", ""), "suggested", runID, revision+2, 2)
}

func connectProcessGraphEvents(t *testing.T, url, token, cursor string) func(string, string) string {
	t.Helper()
	client := &http.Client{Timeout: 4 * time.Second}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Last-Event-ID", cursor)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { response.Body.Close() })
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("event stream response = %d %v", response.StatusCode, response.Header)
	}
	scanner := bufio.NewScanner(response.Body)
	return func(expected, expectedID string) string {
		t.Helper()
		var lines []string
		for scanner.Scan() {
			line := scanner.Text()
			if line != "" {
				lines = append(lines, line)
				continue
			}
			block := strings.Join(lines, "\n")
			lines = nil
			for _, line := range strings.Split(block, "\n") {
				if name, ok := strings.CutPrefix(line, "event: "); ok {
					if name == "cache.invalidate" && (name != expected || !strings.Contains(block, "id: "+expectedID+"\n")) {
						break
					}
					if name != expected {
						t.Fatalf("%s stream received %s while waiting for %s: %s", token, name, expected, block)
					}
					return block
				}
			}
		}
		t.Fatalf("%s stream did not receive %s: %v", token, expected, scanner.Err())
		return ""
	}
}

func assertProcessGraphEvent(t *testing.T, block, status, id string, revision, completed int) {
	t.Helper()
	prefix := "event: process_graph.progress\ndata: "
	payload, ok := strings.CutPrefix(block, prefix)
	if !ok {
		t.Fatalf("expected a progress event without a cursor: %s", block)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(payload), &got); err != nil {
		t.Fatalf("decode progress event %q: %v", payload, err)
	}
	want := map[string]any{
		"schemaVersion": float64(1),
		"projectId":     "p",
		"status":        status,
		"run": map[string]any{
			"id": id, "topic": "delivery request", "revision": float64(revision),
			"completed": float64(completed), "total": float64(2), "failed": float64(0),
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("progress event = %v, want %v", got, want)
	}
}
