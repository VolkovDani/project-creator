package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/VolkovDani/project-creator/internal/analysis"
	"github.com/VolkovDani/project-creator/internal/rag"
)

// Exercise both remote providers through the application's GraphQL and worker paths.
func TestCompatibleProvidersWithoutLocalRuntime(t *testing.T) {
	var calls atomic.Int64
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var req struct {
			Model    string
			Input    []string
			Messages []struct{ Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		switch r.URL.Path {
		case "/v1/embeddings":
			if req.Model != "embed" || r.Header.Get("Authorization") != "Bearer embedding-key" {
				t.Error("incorrect embedding configuration")
			}
			data := make([]map[string]any, len(req.Input))
			for i := range data {
				v := make([]float32, rag.Dimensions)
				v[0] = 1
				data[i] = map[string]any{"index": i, "embedding": v}
			}
			json.NewEncoder(w).Encode(map[string]any{"model": "embed", "data": data})
		case "/v1/chat/completions":
			if req.Model != "chat" || r.Header.Get("Authorization") != "Bearer generation-key" || len(req.Messages) < 2 {
				t.Error("incorrect chat configuration")
				return
			}
			input := []byte(req.Messages[1].Content)
			var pair analysis.Pair
			if err := json.Unmarshal(input, &pair); err != nil {
				t.Error(err)
				return
			}
			var content []byte
			if pair.Left.ID != "" {
				content, _ = json.Marshal(analysis.Finding{LeftID: pair.Left.ID, LeftVersion: pair.Left.Version, RightID: pair.Right.ID, RightVersion: pair.Right.Version, Verdict: "collision", Explanation: "Условия противоречат друг другу."})
			} else {
				var src analysis.Source
				if err := json.Unmarshal(input, &src); err != nil {
					t.Error(err)
					return
				}
				content, _ = json.Marshal(map[string]string{"summary": src.Text})
			}
			json.NewEncoder(w).Encode(map[string]any{"model": "chat", "choices": []any{map[string]any{"index": 0, "finish_reason": "stop", "message": map[string]string{"role": "assistant", "content": string(content)}}}})
		default:
			t.Errorf("unexpected request to %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer provider.Close()
	base := provider.URL + "/v1"
	gen, err := analysis.NewOpenAICompatible(base, "chat", "generation-key")
	if err != nil {
		t.Fatal(err)
	}
	sum, err := analysis.NewSummaryOpenAICompatible(base, "chat", "generation-key", "ru")
	if err != nil {
		t.Fatal(err)
	}
	embed, err := rag.NewOpenAICompatible(base, "embed", "embedding-key")
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewWithSummaryProviders(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"}, embed, gen, sum)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.db.Exec(`INSERT INTO projects VALUES('p','a','P','G','','now','now');INSERT INTO requirements VALUES('r1','a','p','Оплата только картой.',1,'now','now'),('r2','a','p','Оплата только наличными.',1,'now','now')`); err != nil {
		t.Fatal(err)
	}
	waitForIndexQueueReady(t, s, "token-a", "p", 2)
	for _, id := range []string{"r1", "r2"} {
		waitAPISummary(t, s, "token-a", "p", id, "ready")
	}
	authorized := func(q string) map[string]any { return summaryRequest(t, s, "token-a", q) }
	search := `{searchRequirements(projectId:"p",text:"Оплата",limit:5){requirementId}}`
	hits := authorized(search)
	if hits["errors"] != nil || len(hits["data"].(map[string]any)["searchRequirements"].([]any)) != 2 {
		t.Fatal(hits)
	}
	start := `mutation{startProjectAnalysis(projectId:"p",mode:"all"){id}}`
	if res := authorized(start); res["errors"] != nil {
		t.Fatal(res)
	}
	if run := waitAnalysis(t, authorized, "p"); run["collisionPairs"] != float64(1) {
		t.Fatal(run)
	}
	before := calls.Load()
	for _, q := range []string{search, start, `mutation{indexRequirement(projectId:"p",requirementId:"r1"){status}}`} {
		if res := summaryRequest(t, s, "token-b", q); res["errors"] == nil {
			t.Fatal("unauthorized operation", res)
		}
	}
	if calls.Load() != before {
		t.Fatal("unauthorized request invoked remote provider")
	}
}
