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

func TestGraphQLLayaPipelineSnapshotAndSourceIsolation(t *testing.T) {
	generator := &cancelAPIGenerator{entered: make(chan struct{})}
	s, err := NewWithProviders(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"}, nil, generator)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now');INSERT INTO requirements VALUES('r1','a','p','First obligation',1,'now','now'),('r2','a','p','Second obligation',1,'now','now')`); err != nil {
		t.Fatal(err)
	}
	request := func(token, query string) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(map[string]string{"query": query})
		r := httptest.NewRequest("POST", "/graphql", bytes.NewReader(raw))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	start := request("token-a", `mutation{startProjectAnalysis(projectId:"p",mode:"all"){id}}`)
	if start["errors"] != nil {
		t.Fatal(start)
	}
	id := start["data"].(map[string]any)["startProjectAnalysis"].(map[string]any)["id"].(string)
	select {
	case <-generator.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("LLM did not start")
	}
	// Simulate persisted stage metadata written by the previous server version.
	const classifierProfile = "historical-classifier"
	if _, err = s.db.Exec("UPDATE analysis_runs SET classifier_profile=$1 WHERE id=$2", classifierProfile, id); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`UPDATE analysis_pairs SET laya_status='completed', laya_result='{"decision":"uncertain","scores":{"related":0.2,"unrelated":0.1,"uncertain":0.7}}', laya_checked_at=now() WHERE run_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	query := `{projectAnalysisSummary(projectId:"p"){classifierProfile layaPendingPairs layaRunningPairs layaCompletedPairs layaFailedPairs runningPairs} analysisPairs(projectId:"p",analysisId:"` + id + `",limit:10,offset:0,status:"",verdict:""){id layaStatus preclassification layaCheckedAt}}`
	out := request("token-a", query)
	if out["errors"] != nil {
		t.Fatal(out)
	}
	data := out["data"].(map[string]any)
	summary := data["projectAnalysisSummary"].(map[string]any)
	if summary["classifierProfile"] != classifierProfile || summary["layaCompletedPairs"] != float64(1) || summary["runningPairs"] != float64(1) {
		t.Fatal(summary)
	}
	pair := data["analysisPairs"].([]any)[0].(map[string]any)
	var result analysis.Preclassification
	if json.Unmarshal([]byte(pair["preclassification"].(string)), &result) != nil || result.Decision != "uncertain" || pair["layaStatus"] != "completed" || pair["layaCheckedAt"] == nil {
		t.Fatal(pair)
	}
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := readAnalysisProgress(context.Background(), tx, "a", "p", s.analysis.Profile(), "")
	tx.Rollback()
	if err != nil || snapshot.Stale || snapshot.LayaCompletedPairs != 1 || snapshot.ClassifierProfile != classifierProfile {
		t.Fatalf("SSE lost stage/profile: %+v %v", snapshot, err)
	}
	if foreign := request("token-b", query); foreign["errors"] == nil {
		t.Fatal("foreign workspace read classifier results")
	}
}
