package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type selectionAPIEmbedding struct {
	calls   atomic.Int32
	entered chan struct{}
	release chan struct{}
}

func (e *selectionAPIEmbedding) Profile() string { return "api-selection-v1" }
func (e *selectionAPIEmbedding) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	e.calls.Add(1)
	if e.entered != nil {
		select {
		case e.entered <- struct{}{}:
		default:
		}
		select {
		case <-e.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	out := make([][]float32, len(texts))
	for i, text := range texts {
		out[i] = make([]float32, 1024)
		if strings.Contains(text, "Satellite") {
			out[i][1] = 1
		} else {
			out[i][0] = 1
		}
	}
	return out, nil
}

func TestGraphQLQueueSelectionDuplicatesPagesAndHistory(t *testing.T) {
	embedder := &selectionAPIEmbedding{entered: make(chan struct{}, 1), release: make(chan struct{})}
	generator := &collisionGenerator{}
	s, err := NewWithProviders(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"}, embedder, generator)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now');INSERT INTO requirements VALUES('a','a','p','Payments must use cards.',1,'now','now'),('b','a','p',E'Payments  must\nuse cards.',2,'now','now'),('c','a','p','Satellite telemetry expires daily.',1,'now','now')`); err != nil {
		t.Fatal(err)
	}
	select {
	case <-embedder.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("automatic indexing did not start")
	}
	request := func(token, query string) map[string]any {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"query": query})
		req := httptest.NewRequest("POST", "/graphql", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		var payload map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		return payload
	}
	authorized := func(query string) map[string]any { return request("token-a", query) }
	data := func(query, field string) any {
		t.Helper()
		out := authorized(query)
		if out["errors"] != nil {
			t.Fatal(out)
		}
		return out["data"].(map[string]any)[field]
	}
	data(`mutation{startProjectAnalysis(projectId:"p"){id mode similarityThreshold}}`, "startProjectAnalysis")
	missing := waitAnalysis(t, authorized, "p")
	if missing["mode"] != "related" || missing["similarityThreshold"] != .65 || missing["status"] != "failed" || missing["reasonCode"] != "INDEX_INCOMPLETE" || missing["totalPairs"] != nil || generator.calls.Load() != 0 {
		t.Fatalf("default related preparation: %v", missing)
	}
	close(embedder.release)
	if got := waitForIndexQueueReady(t, s, "token-a", "p", 3); got["ready"] != float64(3) {
		t.Fatalf("automatic indexing did not finish: %v", got)
	}
	embedCallsBeforeFull := embedder.calls.Load()
	data(`mutation{startProjectAnalysis(projectId:"p",mode:"all"){id mode}}`, "startProjectAnalysis")
	full := waitAnalysis(t, authorized, "p")
	fullID := full["id"].(string)
	if full["status"] != "completed" || full["mode"] != "all" || full["possiblePairs"] != float64(3) || full["totalPairs"] != float64(3) || full["notSelectedPairs"] != float64(0) || full["duplicatePairs"] != float64(1) || full["collisionPairs"] != float64(2) || full["completedPairs"] != float64(3) || full["newPairs"] != float64(3) || generator.calls.Load() != 2 || embedder.calls.Load() != embedCallsBeforeFull {
		t.Fatalf("full mode must compare only non-exact pairs without adding embedding calls: %v calls=%d embeddings=%d before=%d", full, generator.calls.Load(), embedder.calls.Load(), embedCallsBeforeFull)
	}
	pairQuery := func(id, verdict string, offset int) string {
		t.Helper()
		offsetText := "0"
		if offset == 1 {
			offsetText = "1"
		}
		return `{analysisPairs(projectId:"p",analysisId:"` + id + `",limit:1,offset:` + offsetText + `,status:"completed",verdict:"` + verdict + `"){id left{id version text} right{id version text} verdict explanation resultOrigin selectionReasons similarityScore reusedFrom checkedAt generatorProfile}}`
	}
	duplicates := data(pairQuery(fullID, "duplicate", 0), "analysisPairs").([]any)
	if len(duplicates) != 1 {
		t.Fatal(duplicates)
	}
	exact := duplicates[0].(map[string]any)
	if exact["resultOrigin"] != "exact_text_v1" || exact["explanation"] != "analysis.exactDuplicateExplanation" || exact["checkedAt"] == "" || exact["left"].(map[string]any)["version"] != float64(1) || exact["right"].(map[string]any)["version"] != float64(2) {
		t.Fatalf("duplicate provenance/snapshots: %v", exact)
	}
	first := data(pairQuery(fullID, "collision", 0), "analysisPairs").([]any)
	second := data(pairQuery(fullID, "collision", 1), "analysisPairs").([]any)
	if len(first) != 1 || len(second) != 1 || first[0].(map[string]any)["id"] == second[0].(map[string]any)["id"] {
		t.Fatalf("group pagination: %v %v", first, second)
	}
	for _, id := range []string{"a", "b", "c"} {
		out := authorized(`mutation{indexRequirement(projectId:"p",requirementId:"` + id + `"){status}}`)
		if out["errors"] != nil {
			t.Fatal(out)
		}
	}
	data(`mutation{startProjectAnalysis(projectId:"p",mode:"related",threshold:0.8){id mode similarityThreshold}}`, "startProjectAnalysis")
	related := waitAnalysis(t, authorized, "p")
	relatedID := related["id"].(string)
	if related["status"] != "completed" || related["mode"] != "related" || related["similarityThreshold"] != .8 || related["possiblePairs"] != float64(3) || related["totalPairs"] != float64(1) || related["notSelectedPairs"] != float64(2) || related["duplicatePairs"] != float64(1) || related["reusedPairs"] != float64(1) || related["newPairs"] != float64(0) || related["pendingPairs"] != float64(0) || generator.calls.Load() != 2 {
		t.Fatalf("selection cache isolation and unselected coverage: %v calls=%d", related, generator.calls.Load())
	}
	reused := data(pairQuery(relatedID, "duplicate", 0), "analysisPairs").([]any)[0].(map[string]any)
	if reused["reusedFrom"] != exact["id"] || len(reused["selectionReasons"].([]any)) != 3 || reused["selectionReasons"].([]any)[0] != "exact_text" || reused["selectionReasons"].([]any)[1] != "semantic_similarity" || reused["selectionReasons"].([]any)[2] != "shared_terms" {
		t.Fatalf("selection reasons/reuse origin lost: %v", reused)
	}
	history := data(`{analysisHistory(projectId:"p",limit:1,offset:1){id mode similarityThreshold generatorProfile generatorProvenance possiblePairs totalPairs notSelectedPairs duplicatePairs}}`, "analysisHistory").([]any)
	if len(history) != 1 || history[0].(map[string]any)["id"] != fullID || history[0].(map[string]any)["mode"] != "all" || history[0].(map[string]any)["generatorProvenance"] == "" {
		t.Fatalf("history lost selection/generator provenance: %v", history)
	}
	original := data(`{projectAnalysis(projectId:"p",analysisId:"`+fullID+`"){id mode totalPairs collisionPairs duplicatePairs}}`, "projectAnalysis").(map[string]any)
	if original["id"] != fullID || original["mode"] != "all" || original["totalPairs"] != float64(3) || original["collisionPairs"] != float64(2) {
		t.Fatal(original)
	}
	for _, query := range []string{pairQuery(fullID, "duplicate", 0), `{analysisHistory(projectId:"p",limit:10,offset:0){id}}`, `{projectAnalysis(projectId:"p",analysisId:"` + fullID + `"){id}}`} {
		if denied := request("token-b", query); denied["errors"] == nil {
			t.Fatalf("cross-workspace history exposed: %v", denied)
		}
	}
	for _, query := range []string{`mutation{startProjectAnalysis(projectId:"p",mode:"unknown"){id}}`, `mutation{startProjectAnalysis(projectId:"p",threshold:1.1){id}}`, `{analysisPairs(projectId:"p",analysisId:"` + fullID + `",limit:101,offset:0,status:"",verdict:""){id}}`} {
		if invalid := authorized(query); invalid["errors"] == nil {
			t.Fatalf("invalid selection/page accepted: %v", invalid)
		}
	}
	if generator.calls.Load() != 2 {
		t.Fatal("read/denied/invalid operations dispatched model")
	}
}
