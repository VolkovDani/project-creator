package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"
)

type queueAPITestEmbedder struct{}

func (queueAPITestEmbedder) Profile() string { return "queue-api-test-v1" }
func (queueAPITestEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i := range result {
		result[i] = make([]float32, 1024)
		result[i][0] = 1
	}
	return result, nil
}

const indexProgressFields = `total queued running ready failed profile configured`

func indexQueueAPI(t *testing.T, s *Server, token, query string) map[string]any {
	t.Helper()
	body, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/graphql", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode GraphQL response: %v; body=%s", err, rec.Body.String())
	}
	return result
}

func indexQueueProgress(t *testing.T, s *Server, token, projectID string) map[string]any {
	t.Helper()
	result := indexQueueAPI(t, s, token, `{requirementIndexProgress(projectId:"`+projectID+`"){`+indexProgressFields+`}}`)
	if result["errors"] != nil {
		t.Fatalf("requirementIndexProgress: %v", result)
	}
	return result["data"].(map[string]any)["requirementIndexProgress"].(map[string]any)
}

func waitForIndexQueueReady(t *testing.T, s *Server, token, projectID string, total int) map[string]any {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		progress := indexQueueProgress(t, s, token, projectID)
		if progress["total"] == float64(total) && progress["ready"] == float64(total) && progress["queued"] == float64(0) && progress["running"] == float64(0) {
			return progress
		}
		if time.Now().After(deadline) {
			t.Fatalf("index queue did not become ready in time: %v", progress)
		}
		<-ticker.C
	}
}

func TestRequirementAutoIndexQueueCreateAndUpdate(t *testing.T) {
	s, err := NewWithEmbedder(testDatabase(t), map[string]string{"token-a": "a"}, queueAPITestEmbedder{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now')`); err != nil {
		t.Fatal(err)
	}

	created := indexQueueAPI(t, s, "token-a", `mutation{createRequirement(input:{projectId:"p",text:"First source version"}){requirement{id version}}}`)
	if created["errors"] != nil {
		t.Fatalf("create requirement: %v", created)
	}
	requirement := created["data"].(map[string]any)["createRequirement"].(map[string]any)["requirement"].(map[string]any)
	id := requirement["id"].(string)
	if requirement["version"] != float64(1) {
		t.Fatalf("created requirement: %v", requirement)
	}
	progress := waitForIndexQueueReady(t, s, "token-a", "p", 1)
	if progress["profile"] != "queue-api-test-v1" || progress["configured"] != true {
		t.Fatalf("configured embedder missing from progress: %v", progress)
	}

	updated := indexQueueAPI(t, s, "token-a", fmt.Sprintf(`mutation{updateRequirement(input:{id:"%s",projectId:"p",version:1,text:"Latest source version"}){requirement{id version}}}`, id))
	if updated["errors"] != nil {
		t.Fatalf("update requirement: %v", updated)
	}
	if updated["data"].(map[string]any)["updateRequirement"].(map[string]any)["requirement"].(map[string]any)["version"] != float64(2) {
		t.Fatalf("updated requirement: %v", updated)
	}
	waitForIndexQueueReady(t, s, "token-a", "p", 1)
	status := indexQueueAPI(t, s, "token-a", `{requirementIndexByIds(projectId:"p",requirementIds:["`+id+`"]){requirementId version status}}`)
	if status["errors"] != nil {
		t.Fatalf("read latest index status: %v", status)
	}
	rows := status["data"].(map[string]any)["requirementIndexByIds"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["version"] != float64(2) || rows[0].(map[string]any)["status"] != "ready" {
		t.Fatalf("latest requirement version was not indexed: %v", status)
	}
}

func TestRequirementIndexProgressCountsBeyondTwentyAndIsWorkspaceScoped(t *testing.T) {
	s, err := NewWithEmbedder(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"}, queueAPITestEmbedder{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now'),('q','b','Q','G','','now','now')`); err != nil {
		t.Fatal(err)
	}
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 25; i++ {
		if _, err := tx.Exec(`INSERT INTO requirements VALUES($1,'a','p',$2,1,'now','now')`, fmt.Sprintf("r%02d", i), fmt.Sprintf("Requirement %d", i)); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	progress := waitForIndexQueueReady(t, s, "token-a", "p", 25)
	if progress["total"] != float64(25) {
		t.Fatalf("progress was truncated at a page size: %v", progress)
	}
	denied := indexQueueAPI(t, s, "token-b", `{requirementIndexProgress(projectId:"p"){`+indexProgressFields+`}}`)
	if denied["errors"] == nil {
		t.Fatalf("foreign workspace progress exposed: %v", denied)
	}
	other := indexQueueProgress(t, s, "token-b", "q")
	if other["total"] != float64(0) || other["ready"] != float64(0) {
		t.Fatalf("progress leaked into another workspace: %v", other)
	}
}

func TestRequirementIndexQueueRollbackDoesNotCreateWork(t *testing.T) {
	s, err := NewWithEmbedder(testDatabase(t), map[string]string{"token-a": "a"}, queueAPITestEmbedder{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now')`); err != nil {
		t.Fatal(err)
	}
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO requirements VALUES('rolled-back','a','p','must not be indexed',1,'now','now')`); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	progress := indexQueueProgress(t, s, "token-a", "p")
	if progress["total"] != float64(0) || progress["queued"] != float64(0) || progress["running"] != float64(0) || progress["ready"] != float64(0) || progress["failed"] != float64(0) {
		t.Fatalf("rolled back requirement left indexing work: %v", progress)
	}
}

func TestRequirementIndexProgressWithoutEmbedder(t *testing.T) {
	s, err := New(testDatabase(t), map[string]string{"token-a": "a"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now')`); err != nil {
		t.Fatal(err)
	}
	created := indexQueueAPI(t, s, "token-a", `mutation{createRequirement(input:{projectId:"p",text:"Stored without embeddings"}){requirement{id}}}`)
	if created["errors"] != nil || created["data"].(map[string]any)["createRequirement"].(map[string]any)["requirement"] == nil {
		t.Fatalf("requirement creation must work without an embedder: %v", created)
	}
	progress := indexQueueProgress(t, s, "token-a", "p")
	if progress["configured"] != false || progress["profile"] != "" || progress["total"] != float64(1) || progress["queued"] != float64(0) {
		t.Fatalf("unexpected progress without embedder: %v", progress)
	}
}
