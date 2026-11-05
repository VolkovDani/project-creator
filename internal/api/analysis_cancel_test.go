package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/VolkovDani/project-creator/internal/analysis"
)

type cancelAPIGenerator struct{ entered chan struct{} }

func (g *cancelAPIGenerator) Profile() string { return "cancel-api-test" }
func (g *cancelAPIGenerator) Compare(ctx context.Context, pair analysis.Pair) (analysis.Finding, error) {
	close(g.entered)
	<-ctx.Done()
	return analysis.Finding{}, ctx.Err()
}

func TestGraphQLCancelAnalysisIsScopedAndDurable(t *testing.T) {
	generator := &cancelAPIGenerator{entered: make(chan struct{})}
	s, err := NewWithProviders(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"}, nil, generator)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now');INSERT INTO requirements VALUES('r1','a','p','First requirement',1,'now','now'),('r2','a','p','Second requirement',1,'now','now')`); err != nil {
		t.Fatal(err)
	}
	request := func(token, query string) map[string]any {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"query": query})
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
	started := request("token-a", `mutation{startProjectAnalysis(projectId:"p",mode:"all"){id}}`)
	if started["errors"] != nil {
		t.Fatal(started)
	}
	id := started["data"].(map[string]any)["startProjectAnalysis"].(map[string]any)["id"].(string)
	select {
	case <-generator.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("model never started")
	}
	mutation := `mutation{cancelProjectAnalysis(projectId:"p",analysisId:"` + id + `"){id projectId status errorCode completedPairs pendingPairs runningPairs}}`
	if denied := request("token-b", mutation); denied["errors"] == nil {
		t.Fatal("foreign cancellation allowed")
	}
	for i := 0; i < 2; i++ {
		out := request("token-a", mutation)
		if out["errors"] != nil {
			t.Fatal(out)
		}
		run := out["data"].(map[string]any)["cancelProjectAnalysis"].(map[string]any)
		if run["status"] != "failed" || run["errorCode"] != "CANCELLED" || run["pendingPairs"] != float64(0) || run["runningPairs"] != float64(0) {
			t.Fatal(run)
		}
	}
	stored := request("token-a", `{projectAnalysis(projectId:"p"){errorCode}}`)
	if stored["errors"] != nil || stored["data"].(map[string]any)["projectAnalysis"].(map[string]any)["errorCode"] != "CANCELLED" {
		t.Fatal(stored)
	}
}
