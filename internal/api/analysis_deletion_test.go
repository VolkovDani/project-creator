package api

import (
	"encoding/json"
	"testing"
	"time"
)

func TestAnalysisDeletionGraphQL(t *testing.T) {
	s, err := New(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now');
INSERT INTO analysis_runs(id,workspace,project_id,status,generator_profile,embedding_profile,source_signature,sources,lease_until)
VALUES ('r','a','p','completed','g','e','sig','[]','now')`); err != nil {
		t.Fatal(err)
	}
	query := `{analysisDeletionRetentionDays analysisHistory(projectId:"p",limit:1,offset:0){id deletionScheduledFor} projectAnalysisSummary(projectId:"p"){deletionScheduledFor}}`
	request := func(token, operation string) map[string]any {
		t.Helper()
		return summaryRequest(t, s, token, operation)
	}
	before := request("token-a", query)
	if before["errors"] != nil {
		t.Fatal(before)
	}
	data := before["data"].(map[string]any)
	if data["analysisDeletionRetentionDays"] != float64(30) || data["projectAnalysisSummary"].(map[string]any)["deletionScheduledFor"] != nil {
		t.Fatal(data)
	}
	mark := `mutation{scheduleAnalysisDeletion(projectId:"p",analysisId:"r")}`
	remove := `mutation{deleteMarkedAnalysis(projectId:"p",analysisId:"r")}`
	if request("token-b", mark)["errors"] == nil || request("token-a", remove)["errors"] == nil {
		t.Fatal("deletion safeguards bypassed")
	}
	if result := request("token-a", mark); result["errors"] != nil {
		t.Fatal(result)
	}
	marked := request("token-a", query)["data"].(map[string]any)
	deadline := marked["analysisHistory"].([]any)[0].(map[string]any)["deletionScheduledFor"].(string)
	if _, err := time.Parse(time.RFC3339Nano, deadline); err != nil || marked["projectAnalysisSummary"].(map[string]any)["deletionScheduledFor"] != deadline {
		t.Fatal(marked)
	}
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := readAnalysisProgress(t.Context(), tx, "a", "p")
	tx.Rollback()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(snapshot)
	var fields map[string]any
	if err != nil || json.Unmarshal(encoded, &fields) != nil || fields["deletionScheduledFor"] != deadline {
		t.Fatalf("live deadline differs from history: %s", encoded)
	}
	if request("token-b", remove)["errors"] == nil {
		t.Fatal("cross-workspace deletion accepted")
	}
	if result := request("token-a", remove); result["errors"] != nil {
		t.Fatal(result)
	}
	after := request("token-a", query)["data"].(map[string]any)
	if len(after["analysisHistory"].([]any)) != 0 || after["projectAnalysisSummary"] != nil {
		t.Fatal(after)
	}
}
