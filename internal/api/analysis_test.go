package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/VolkovDani/project-creator/internal/analysis"
)

type collisionGenerator struct{ calls atomic.Int32 }

const analysisFields = `id projectId status outcome reasonCode missingRequirementIds errorCode stale stage totalPairs processedPairs activePairId pairs{id position status left{id version text} right{id version text} verdict explanation errorCode} findings{leftId rightId leftVersion rightVersion verdict explanation} sources{id version text}`

func waitAnalysis(t *testing.T, request func(string) map[string]any, project string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		out := request(`{projectAnalysis(projectId:"` + project + `"){` + analysisFields + `}}`)
		if out["errors"] != nil {
			t.Fatal(out)
		}
		value, _ := out["data"].(map[string]any)["projectAnalysis"].(map[string]any)
		if value != nil && value["status"] != "running" {
			return value
		}
		if time.Now().After(deadline) {
			t.Fatalf("analysis never finished: %v", value)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (g *collisionGenerator) Profile() string { return "api-analysis-profile" }
func (g *collisionGenerator) Compare(ctx context.Context, pair analysis.Pair) (analysis.Finding, error) {
	g.calls.Add(1)
	return analysis.Finding{LeftID: pair.Left.ID, RightID: pair.Right.ID, LeftVersion: pair.Left.Version, RightVersion: pair.Right.Version, Verdict: "collision", Explanation: "Требования задают разные обязательные способы оплаты; требуется проверка условий."}, nil
}

func TestGraphQLAnalysisPreservesSourcesAndScope(t *testing.T) {
	embedder := &deterministicEmbedding{}
	generator := &collisionGenerator{}
	s, err := NewWithProviders(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"}, embedder, generator)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now');INSERT INTO requirements VALUES('r1','a','p','Оплата разрешена только картой.',1,'now','now'),('r2','a','p','Оплата разрешена только наличными.',1,'now','now')`); err != nil {
		t.Fatal(err)
	}
	run := func(token, q string) map[string]any {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"query": q})
		req := httptest.NewRequest("POST", "/graphql", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	authorized := func(q string) map[string]any { return run("token-a", q) }

	unindexed := run("token-a", `mutation{startProjectAnalysis(projectId:"p"){status outcome reasonCode missingRequirementIds}}`)
	if unindexed["errors"] != nil {
		t.Fatal(unindexed)
	}
	diagnostic := waitAnalysis(t, authorized, "p")
	if diagnostic["reasonCode"] != "INDEX_INCOMPLETE" || len(diagnostic["missingRequirementIds"].([]any)) != 2 {
		t.Fatal(diagnostic)
	}
	deniedDiagnostic := run("token-b", `{projectAnalysis(projectId:"p"){reasonCode missingRequirementIds}}`)
	if deniedDiagnostic["errors"] == nil {
		t.Fatal("cross-workspace diagnostics exposed")
	}
	for _, id := range []string{"r1", "r2"} {
		out := run("token-a", `mutation{indexRequirement(projectId:"p",requirementId:"`+id+`"){status}}`)
		if out["errors"] != nil {
			t.Fatal(out)
		}
	}
	before := embedder.calls.Load()
	denied := run("token-b", `mutation{startProjectAnalysis(projectId:"p"){id}}`)
	if denied["errors"] == nil || embedder.calls.Load() != before || generator.calls.Load() != 0 {
		t.Fatalf("unauthorized analysis: %v", denied)
	}
	out := run("token-a", `mutation{startProjectAnalysis(projectId:"p"){id status outcome stale findings{leftId rightId leftVersion rightVersion verdict explanation} sources{id version text}}}`)
	if out["errors"] != nil {
		t.Fatal(out)
	}
	value := waitAnalysis(t, authorized, "p")
	if value["status"] != "completed" || value["outcome"] != "possible_collision" || value["stale"] != false || len(value["findings"].([]any)) != 1 || len(value["sources"].([]any)) != 2 {
		t.Fatal(value)
	}
	if value["stage"] != "finished" || value["totalPairs"] != float64(1) || value["processedPairs"] != float64(1) || value["activePairId"] != "" || len(value["pairs"].([]any)) != 1 {
		t.Fatalf("missing persisted pair progress: %v", value)
	}
	pair := value["pairs"].([]any)[0].(map[string]any)
	if pair["status"] != "completed" || pair["verdict"] != "collision" || pair["left"].(map[string]any)["version"] != float64(1) {
		t.Fatal(pair)
	}
	deniedRetry := run("token-b", `mutation{retryProjectAnalysis(projectId:"p",analysisId:"`+value["id"].(string)+`"){id pairs{left{text}}}}`)
	if deniedRetry["errors"] == nil || generator.calls.Load() != 1 {
		t.Fatal("cross-workspace retry invoked provider or exposed sources", deniedRetry)
	}
	if _, err = s.db.Exec(`UPDATE requirements SET text='Оплата доступна любым способом.',version=2 WHERE id='r2'`); err != nil {
		t.Fatal(err)
	}
	out = run("token-a", `{projectAnalysis(projectId:"p"){stale sources{id version text}}}`)
	if out["errors"] != nil {
		t.Fatal(out)
	}
	latest := out["data"].(map[string]any)["projectAnalysis"].(map[string]any)
	if latest["stale"] != true {
		t.Fatal(latest)
	}
	for _, v := range latest["sources"].([]any) {
		source := v.(map[string]any)
		if source["id"] == "r2" && (source["version"] != float64(1) || source["text"] != "Оплата разрешена только наличными.") {
			t.Fatalf("source snapshot changed: %v", source)
		}
	}
	denied = run("token-b", `{projectAnalysis(projectId:"p"){id sources{text}}}`)
	if denied["errors"] == nil {
		t.Fatalf("cross-workspace source leak: %v", denied)
	}
}

type gatedAPIGenerator struct {
	calls   atomic.Int32
	entered chan analysis.Pair
	release chan bool
}

func (g *gatedAPIGenerator) Profile() string { return "api-gated" }
func (g *gatedAPIGenerator) Compare(ctx context.Context, pair analysis.Pair) (analysis.Finding, error) {
	g.calls.Add(1)
	select {
	case g.entered <- pair:
	case <-ctx.Done():
		return analysis.Finding{}, ctx.Err()
	}
	select {
	case success := <-g.release:
		if !success {
			return analysis.Finding{}, errors.New("synthetic failure")
		}
	case <-ctx.Done():
		return analysis.Finding{}, ctx.Err()
	}
	return analysis.Finding{LeftID: pair.Left.ID, RightID: pair.Right.ID, LeftVersion: pair.Left.Version, RightVersion: pair.Right.Version, Verdict: "collision", Explanation: "Synthetic collision for review."}, nil
}

func TestGraphQLProgressSurvivesRequestAndRetriesOnlyFailedPairs(t *testing.T) {
	embedder := &deterministicEmbedding{}
	generator := &gatedAPIGenerator{entered: make(chan analysis.Pair, 3), release: make(chan bool, 3)}
	s, err := NewWithProviders(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"}, embedder, generator)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now'); INSERT INTO requirements VALUES('one','a','p','First rule',1,'now','now'),('two','a','p','Second rule',1,'now','now'),('three','a','p','Third rule',1,'now','now')`); err != nil {
		t.Fatal(err)
	}
	request := func(ctx context.Context, token, query string) map[string]any {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"query": query})
		req := httptest.NewRequest("POST", "/graphql", bytes.NewReader(body)).WithContext(ctx)
		req.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		s.ServeHTTP(response, req)
		var value map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	authorized := func(query string) map[string]any { return request(context.Background(), "token-a", query) }
	for _, id := range []string{"one", "two", "three"} {
		out := authorized(`mutation{indexRequirement(projectId:"p",requirementId:"` + id + `"){status}}`)
		if out["errors"] != nil {
			t.Fatal(out)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	out := request(ctx, "token-a", `mutation{startProjectAnalysis(projectId:"p"){`+analysisFields+`}}`)
	cancel()
	if out["errors"] != nil {
		t.Fatal(out)
	}
	initial := out["data"].(map[string]any)["startProjectAnalysis"].(map[string]any)
	if initial["status"] != "running" || initial["totalPairs"] != nil || initial["stage"] != "checking_index" {
		t.Fatalf("start must return before preparation/model: %v", initial)
	}
	entered := func() {
		t.Helper()
		select {
		case <-generator.entered:
		case <-time.After(5 * time.Second):
			t.Fatal("background comparison never started")
		}
	}
	entered()
	var events int
	if err = s.db.QueryRow(`SELECT count(*) FROM cache_events WHERE workspace='a'`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	generator.release <- true
	entered()
	progress := authorized(`{projectAnalysis(projectId:"p"){` + analysisFields + `}}`)["data"].(map[string]any)["projectAnalysis"].(map[string]any)
	if progress["status"] != "running" || progress["totalPairs"] != float64(3) || progress["processedPairs"] != float64(1) || len(progress["findings"].([]any)) != 1 || progress["activePairId"] == "" {
		t.Fatalf("first result not exposed while second is running: %v", progress)
	}
	firstID := progress["pairs"].([]any)[0].(map[string]any)["id"]
	var committed int
	if err = s.db.QueryRow(`SELECT count(*) FROM cache_events WHERE workspace='a' AND keys @> ARRAY['project:p']`).Scan(&committed); err != nil || committed <= events {
		t.Fatalf("pair commit has no project invalidation: %d -> %d: %v", events, committed, err)
	}
	generator.release <- false
	failed := waitAnalysis(t, authorized, "p")
	if failed["status"] != "failed" || len(failed["findings"].([]any)) != 1 || failed["processedPairs"] != float64(2) {
		t.Fatalf("failure lost partial result or counted unprocessed pair: %v", failed)
	}
	id := initial["id"].(string)
	denied := request(context.Background(), "token-b", `mutation{retryProjectAnalysis(projectId:"p",analysisId:"`+id+`"){id}}`)
	if denied["errors"] == nil || generator.calls.Load() != 2 {
		t.Fatal("unauthorized retry reached model", denied)
	}
	retried := authorized(`mutation{retryProjectAnalysis(projectId:"p",analysisId:"` + id + `"){id status processedPairs}}`)
	if retried["errors"] != nil {
		t.Fatal(retried)
	}
	entered()
	generator.release <- true
	entered()
	generator.release <- true
	completed := waitAnalysis(t, authorized, "p")
	if completed["id"] != id || completed["status"] != "completed" || completed["processedPairs"] != float64(3) || len(completed["findings"].([]any)) != 3 || generator.calls.Load() != 4 {
		t.Fatalf("retry duplicated completed pair or changed run: %v; calls=%d", completed, generator.calls.Load())
	}
	if completed["pairs"].([]any)[0].(map[string]any)["id"] != firstID {
		t.Fatal("successful pair identity changed across retry")
	}
}
