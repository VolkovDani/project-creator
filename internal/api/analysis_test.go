package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/VolkovDani/project-creator/internal/analysis"
)

type collisionGenerator struct{ calls atomic.Int32 }

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
	value := out["data"].(map[string]any)["startProjectAnalysis"].(map[string]any)
	if value["status"] != "completed" || value["outcome"] != "possible_collision" || value["stale"] != false || len(value["findings"].([]any)) != 1 || len(value["sources"].([]any)) != 2 {
		t.Fatal(value)
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
