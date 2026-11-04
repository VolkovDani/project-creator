package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
)

type deterministicEmbedding struct{ calls atomic.Int32 }

func (p *deterministicEmbedding) Profile() string { return "api-test-profile" }
func (p *deterministicEmbedding) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	p.calls.Add(1)
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = make([]float32, 1024)
		out[i][0] = 1
	}
	return out, nil
}

func TestGraphQLRAGSourcesAndAccess(t *testing.T) {
	provider := &deterministicEmbedding{}
	s, err := NewWithEmbedder(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"}, provider)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now');INSERT INTO requirements VALUES('r','a','p','Оплата только картой',1,'now','now')`); err != nil {
		t.Fatal(err)
	}
	run := func(token, q string) map[string]any {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"query": q})
		req := httptest.NewRequest("POST", "/graphql", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)
		var result map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	denied := run("token-b", `mutation {indexRequirement(projectId:"p",requirementId:"r"){status}}`)
	if denied["errors"] == nil || provider.calls.Load() != 0 {
		t.Fatalf("unauthorized indexing: %v calls%d", denied, provider.calls.Load())
	}
	indexed := run("token-a", `mutation {indexRequirement(projectId:"p",requirementId:"r"){requirementId version status}}`)
	if indexed["errors"] != nil {
		t.Fatal(indexed)
	}
	state := indexed["data"].(map[string]any)["indexRequirement"].(map[string]any)
	if state["status"] != "ready" || state["version"] != float64(1) {
		t.Fatal(state)
	}
	statuses := run("token-a", `{requirementIndexByIds(projectId:"p",requirementIds:["r","hidden"]){requirementId projectId version status}}`)
	if statuses["errors"] != nil {
		t.Fatal(statuses)
	}
	statusRows := statuses["data"].(map[string]any)["requirementIndexByIds"].([]any)
	if len(statusRows) != 1 {
		t.Fatalf("out-of-scope status IDs leaked: %v", statuses)
	}
	statusRow := statusRows[0].(map[string]any)
	if statusRow["requirementId"] != "r" || statusRow["projectId"] != "p" || statusRow["status"] != "ready" {
		t.Fatalf("status row missing authorized project or current state: %v", statusRow)
	}
	if !reflect.DeepEqual(cacheTags(statuses["data"]), []string{"project:p", "project:p:requirements", "projects"}) {
		t.Fatalf("status response missing project cache tags: %v", statuses)
	}
	denied = run("token-b", `{requirementIndexByIds(projectId:"p",requirementIds:["r"]){requirementId projectId status}}`)
	if denied["errors"] == nil {
		t.Fatalf("unauthorized status batch: %v", denied)
	}
	found := run("token-a", `{searchRequirements(projectId:"p",text:"Оплата картой",limit:5){requirementId projectId version text sourceText start end score}}`)
	if found["errors"] != nil {
		t.Fatal(found)
	}
	hits := found["data"].(map[string]any)["searchRequirements"].([]any)
	if len(hits) != 1 {
		t.Fatal(found)
	}
	hit := hits[0].(map[string]any)
	if hit["requirementId"] != "r" || hit["version"] != float64(1) || hit["sourceText"] != "Оплата только картой" {
		t.Fatal(hit)
	}
	before := provider.calls.Load()
	denied = run("token-b", `{searchRequirements(projectId:"p",text:"Оплата",limit:5){requirementId}}`)
	if denied["errors"] == nil || provider.calls.Load() != before {
		t.Fatalf("unauthorized search calls: %v", denied)
	}
	if _, err = s.db.Exec(`UPDATE requirements SET text='Новая версия',version=2 WHERE id='r'`); err != nil {
		t.Fatal(err)
	}
	found = run("token-a", `{searchRequirements(projectId:"p",text:"Оплата",limit:5){requirementId}}`)
	if found["errors"] != nil {
		t.Fatal(found)
	}
	if len(found["data"].(map[string]any)["searchRequirements"].([]any)) != 0 {
		t.Fatalf("stale source returned: %v", found)
	}
}
